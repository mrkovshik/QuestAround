package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"questaround/internal/runner"
	"questaround/internal/store"
)

const adminToken = "test-admin-token-123456"

const e2eYAML = `
version: 1
id: e2e
title: E2E
participants: {team_size: {min: 1, max: 4}}
leaderboard: {visibility: host}
levels:
  - id: l1
    title: Старт
    task: Найдите год
    points: 10
    answers: [{type: code, values: ["1864"]}]
    hints:
      - {on_request: true, penalty: {time: 5m}, text: Табличка}
    bonuses:
      - {id: selfie, title: Селфи, answer: {type: photo}, points: 5}
  - id: l2
    title: Фото
    task: Сфотографируйте мост
    answers: [{type: photo}]
  - id: l3
    title: Финиш
    task: Кодовое слово
    answers: [{type: code, values: ["финиш"]}]
`

type testServer struct {
	t   *testing.T
	srv *httptest.Server
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st := store.NewMemory()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := New(Config{
		Store:        st,
		Runners:      runner.NewManager(ctx, st, runner.RealClock{}, log),
		Media:        DiskMedia{Dir: t.TempDir()},
		AdminToken:   adminToken,
		Logger:       log,
		SSEKeepAlive: time.Hour,
		Static: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("spa"))
		}),
	})
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(cancel) // сначала закрыть SSE-подписки, потом сервер
	return &testServer{t: t, srv: srv}
}

type resp struct {
	status int
	body   []byte
	header http.Header
}

func (r resp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("json: %v\n%s", err, r.body)
	}
}

func (r resp) errCode(t *testing.T) string {
	var e errorBody
	r.json(t, &e)
	return e.Error
}

func (ts *testServer) call(method, path, token string, body any) resp {
	ts.t.Helper()
	var rd io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case string:
		rd, ctype = strings.NewReader(b), "application/yaml"
	case *multipartBody:
		rd, ctype = &b.buf, b.ctype
	default:
		data, _ := json.Marshal(b)
		rd, ctype = bytes.NewReader(data), "application/json"
	}
	req, _ := http.NewRequest(method, ts.srv.URL+path, rd)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		ts.t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{status: res.StatusCode, body: data, header: res.Header}
}

func (ts *testServer) must(status int, method, path, token string, body any) resp {
	ts.t.Helper()
	r := ts.call(method, path, token, body)
	if r.status != status {
		ts.t.Fatalf("%s %s: статус %d, ожидали %d\n%s", method, path, r.status, status, r.body)
	}
	return r
}

type multipartBody struct {
	buf   bytes.Buffer
	ctype string
}

func photoForm(t *testing.T, fields map[string]string, file []byte) *multipartBody {
	t.Helper()
	mb := &multipartBody{}
	w := multipart.NewWriter(&mb.buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if file != nil {
		fw, _ := w.CreateFormFile("photo", "p.png")
		_, _ = fw.Write(file)
	}
	_ = w.Close()
	mb.ctype = w.FormDataContentType()
	return mb
}

func pngBytes(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// sseEvents читает SSE-поток и отдаёт data событий state_changed.
func sseEvents(t *testing.T, ctx context.Context, url string) <-chan string {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	out := make(chan string, 16)
	go func() {
		defer res.Body.Close()
		defer close(out)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				out <- d
			}
		}
	}()
	return out
}

func recv(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("SSE: событие не пришло")
		return ""
	}
}

func TestFullGameOverHTTP(t *testing.T) {
	ts := newTestServer(t)

	// --- ведущий: сценарий и игра ---
	r := ts.must(422, "POST", "/admin/v1/scenarios", adminToken, "version: 1\nid: x\ntitle: x\nlevels: []\n")
	var bad errorBody
	r.json(t, &bad)
	if bad.Error != "invalid_scenario" || len(bad.Details) == 0 || bad.Details[0].Line == 0 {
		t.Fatalf("ошибки сценария: %+v", bad)
	}
	ts.must(401, "POST", "/admin/v1/scenarios", "wrong", e2eYAML)

	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML).json(t, &sv)
	if sv.Version != 1 || sv.Slug != "e2e" {
		t.Fatalf("версия: %+v", sv)
	}
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID, "title": "Пилот"}).json(t, &run)
	if run.Status != "pending" || len(run.PublicCode) != 6 {
		t.Fatalf("игра: %+v", run)
	}
	admin := "/admin/v1/runs/" + run.ID

	// --- регистрация ---
	var info gameInfoDTO
	ts.must(200, "GET", "/api/v1/games/"+strings.ToLower(run.PublicCode), "", nil).json(t, &info)
	if !info.Registration.Open || info.Title != "Пилот" {
		t.Fatalf("инфо: %+v", info)
	}
	game := "/api/v1/games/" + run.PublicCode

	var capCred, plCred credentialsDTO
	ts.must(201, "POST", game+"/teams", "", map[string]string{"team_name": "Ёжики", "captain_name": "Коля", "contact": "@kolya"}).json(t, &capCred)
	ts.must(201, "POST", game+"/join", "", map[string]string{"invite_code": strings.ToLower(capCred.InviteCode), "name": "Маша"}).json(t, &plCred)
	if plCred.TeamID != capCred.TeamID {
		t.Fatal("игрок попал не в ту команду")
	}
	if c := ts.must(404, "POST", game+"/join", "", map[string]string{"invite_code": "NOPE", "name": "x"}).errCode(t); c != "unknown_invite" {
		t.Fatalf("код ошибки %s", c)
	}
	ts.must(400, "POST", game+"/join", "", map[string]any{"invite_code": "x", "extra": 1})
	ts.must(404, "GET", "/api/v1/games/ZZZZZZ", "", nil)

	cpt, pl := capCred.Token, plCred.Token
	ts.must(401, "GET", "/api/v1/state", "", nil)
	ts.must(401, "GET", "/api/v1/state", capCred.Token+"x", nil)
	ts.must(401, "GET", "/api/v1/state", "a.b.c", nil)

	var team teamDTO
	ts.must(200, "GET", "/api/v1/team", pl, nil).json(t, &team)
	if len(team.Players) != 2 || team.CaptainID != capCred.PlayerID {
		t.Fatalf("команда: %+v", team)
	}
	if c := ts.must(403, "PATCH", "/api/v1/team", pl, map[string]string{"name": "Взлом"}).errCode(t); c != "not_allowed" {
		t.Fatalf("код %s", c)
	}
	ts.must(200, "PATCH", "/api/v1/team", cpt, map[string]string{"name": "Ежи"})

	// Вторая команда — от ведущего.
	var hostTeam map[string]string
	ts.must(201, "POST", admin+"/teams", adminToken, map[string]string{"name": "Корпоратив"}).json(t, &hostTeam)

	// --- старт ---
	sseCtx, stopSSE := context.WithCancel(context.Background())
	defer stopSSE()
	events := sseEvents(t, sseCtx, ts.srv.URL+"/api/v1/stream?token="+pl)
	recv(t, events) // текущий seq при подключении

	var dash dashboardDTO
	ts.must(200, "POST", admin+"/start", adminToken, nil).json(t, &dash)
	if dash.RunStatus != "running" || len(dash.Teams) != 2 {
		t.Fatalf("дашборд: %+v", dash)
	}
	recv(t, events)

	var st playerStateDTO
	r = ts.must(200, "GET", "/api/v1/state", pl, nil)
	r.json(t, &st)
	if strings.Contains(string(r.body), "1864") || strings.Contains(string(r.body), "финиш\"") {
		t.Fatal("в состоянии игрока утёк ответ")
	}
	if strings.Contains(string(r.body), "null") {
		t.Fatalf("null в состоянии игрока (клиент ждёт массивы):\n%s", r.body)
	}
	if len(st.Levels) != 1 || st.Levels[0].ID != "l1" || st.IsCaptain || st.Team.Name != "Ежи" {
		t.Fatalf("состояние: %+v", st)
	}

	// --- ответы ---
	var ans answerResultDTO
	ts.must(200, "POST", "/api/v1/answers", pl, map[string]string{"kind": "code", "value": "1000"}).json(t, &ans)
	if ans.Result != "wrong" {
		t.Fatalf("ожидали wrong: %+v", ans)
	}
	ts.must(400, "POST", "/api/v1/answers", pl, map[string]string{"kind": "photo"})

	// Подсказка: только капитан.
	if c := ts.must(403, "POST", "/api/v1/levels/l1/hints/0", pl, nil).errCode(t); c != "not_allowed" {
		t.Fatalf("код %s", c)
	}
	ts.must(204, "POST", "/api/v1/levels/l1/hints/0", cpt, nil)
	ts.must(409, "POST", "/api/v1/levels/l1/hints/0", cpt, nil)

	// Фото-бонус.
	if c := ts.must(415, "POST", "/api/v1/photos", pl, photoForm(t, map[string]string{"level_id": "l1", "bonus_id": "selfie"}, []byte("not an image"))).errCode(t); c != "not_image" {
		t.Fatalf("код %s", c)
	}
	ts.must(200, "POST", "/api/v1/photos", pl, photoForm(t, map[string]string{"level_id": "l1", "bonus_id": "selfie"}, pngBytes(t))).json(t, &ans)
	if ans.Result != "pending" || ans.BonusID != "selfie" {
		t.Fatalf("фото: %+v", ans)
	}

	ts.must(200, "POST", "/api/v1/answers", pl, map[string]string{"kind": "code", "value": " 1864 ", "client_event_id": "c1"}).json(t, &ans)
	if ans.Result != "correct" {
		t.Fatalf("ожидали correct: %+v", ans)
	}
	recv(t, events)

	// Модерация.
	var queue []moderationDTO
	ts.must(200, "GET", admin+"/moderation", adminToken, nil).json(t, &queue)
	if len(queue) != 1 || queue[0].Target != "Бонус: Селфи" || queue[0].PlayerName != "Маша" {
		t.Fatalf("очередь: %+v", queue)
	}
	img := ts.must(200, "GET", queue[0].MediaURL, adminToken, nil)
	if img.header.Get("Content-Type") != "image/png" {
		t.Fatalf("медиа: %s", img.header.Get("Content-Type"))
	}
	ts.must(401, "GET", queue[0].MediaURL, pl, nil)
	ts.must(404, "GET", "/admin/v1/media/..%2F..%2Fetc%2Fpasswd", adminToken, nil)
	ts.must(200, "POST", admin+"/moderation/"+itoa64(queue[0].Seq), adminToken, map[string]any{"approved": true}).json(t, &queue)
	if len(queue) != 0 {
		t.Fatalf("очередь после решения: %+v", queue)
	}

	// Уровень 2: фото без цели — берётся первый фото-ответ уровня.
	ts.must(200, "POST", "/api/v1/photos", cpt, photoForm(t, map[string]string{"level_id": "l2"}, pngBytes(t))).json(t, &ans)
	ts.must(200, "POST", admin+"/moderation/"+itoa64(ans.Seq), adminToken, map[string]any{"approved": true})

	// Лидерборд скрыт (visibility: host), ведущий включает.
	if c := ts.must(403, "GET", "/api/v1/leaderboard", pl, nil).errCode(t); c != "leaderboard_hidden" {
		t.Fatalf("код %s", c)
	}
	ts.must(200, "PUT", admin+"/leaderboard", adminToken, map[string]any{"visible": true})
	var lb struct {
		Standings []standingDTO `json:"standings"`
	}
	ts.must(200, "GET", "/api/v1/leaderboard", pl, nil).json(t, &lb)
	if len(lb.Standings) != 2 || lb.Standings[0].TeamID != capCred.TeamID || lb.Standings[0].Points != 15 {
		t.Fatalf("таблица: %+v", lb.Standings)
	}

	// Ведущий: штраф без причины нельзя; код за команду — можно.
	tAdmin := admin + "/teams/" + hostTeam["team_id"]
	if c := ts.must(400, "POST", tAdmin+"/override", adminToken, map[string]any{"action": "adjust_time", "time": "2m"}).errCode(t); c != "reason_required" {
		t.Fatalf("код %s", c)
	}
	ts.must(200, "POST", tAdmin+"/override", adminToken, map[string]any{"action": "adjust_time", "time": "-1m30s", "reason": "без связи"})
	ts.must(200, "POST", tAdmin+"/answers", adminToken, map[string]string{"kind": "code", "value": "1864"}).json(t, &ans)
	if ans.Result != "correct" {
		t.Fatalf("ведущий за команду: %+v", ans)
	}

	ts.must(200, "POST", admin+"/broadcast", adminToken, map[string]string{"text": "Финиш у фонтана"})
	r = ts.must(200, "GET", "/api/v1/state", pl, nil)
	r.json(t, &st)
	if strings.Contains(string(r.body), "null") {
		t.Fatalf("null в состоянии игрока:\n%s", r.body)
	}
	if len(st.Messages) != 1 || st.Levels[len(st.Levels)-1].ID != "l3" {
		t.Fatalf("состояние: %+v", st)
	}

	ts.must(200, "POST", "/api/v1/answers", cpt, map[string]string{"kind": "code", "value": "ФИНИШ"})
	ts.must(200, "POST", admin+"/finish", adminToken, nil).json(t, &dash)
	if dash.RunStatus != "finished" {
		t.Fatalf("статус: %s", dash.RunStatus)
	}

	var journal []json.RawMessage
	ts.must(200, "GET", admin+"/events", adminToken, nil).json(t, &journal)
	if len(journal) < 20 {
		t.Fatalf("журнал: %d событий", len(journal))
	}
	var runs []runDTO
	ts.must(200, "GET", "/admin/v1/runs", adminToken, nil).json(t, &runs)
	if len(runs) != 1 || runs[0].Status != "finished" || runs[0].Teams != 2 {
		t.Fatalf("список игр: %+v", runs)
	}
}

func TestRoutingStaticVsAPI(t *testing.T) {
	ts := newTestServer(t)
	if r := ts.must(200, "GET", "/g/ABC/join/XYZ", "", nil); string(r.body) != "spa" {
		t.Fatalf("SPA: %s", r.body)
	}
	if c := ts.must(404, "GET", "/api/v1/nope", "", nil).errCode(t); c != "not_found" {
		t.Fatalf("код %s", c)
	}
	ts.must(404, "POST", "/admin/v1/nope", adminToken, nil)
}

func TestRegistrationLimits(t *testing.T) {
	ts := newTestServer(t)
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	game := "/api/v1/games/" + run.PublicCode

	ts.must(200, "PUT", "/admin/v1/runs/"+run.ID+"/registration", adminToken, map[string]any{"open": true, "max_teams": 1})
	var cred credentialsDTO
	ts.must(201, "POST", game+"/teams", "", map[string]string{"team_name": "A", "captain_name": "a"}).json(t, &cred)
	if c := ts.must(409, "POST", game+"/teams", "", map[string]string{"team_name": "B", "captain_name": "b"}).errCode(t); c != "registration_full" {
		t.Fatalf("код %s", c)
	}
	members := make([]credentialsDTO, 3)
	for i := range members {
		ts.must(201, "POST", game+"/join", "", map[string]string{"invite_code": cred.InviteCode, "name": "p" + itoa64(int64(i))}).json(t, &members[i])
	}
	if c := ts.must(409, "POST", game+"/join", "", map[string]string{"invite_code": cred.InviteCode, "name": "лишний"}).errCode(t); c != "team_full" {
		t.Fatalf("код %s", c)
	}

	// Перевыпуск инвайта: старый перестаёт работать.
	var team teamDTO
	ts.must(200, "POST", "/api/v1/team/invite/rotate", cred.Token, nil).json(t, &team)
	if team.InviteCode == cred.InviteCode {
		t.Fatal("инвайт не сменился")
	}

	// Капитан удаляет игрока — его токен перестаёт действовать.
	ts.must(200, "GET", "/api/v1/team", members[0].Token, nil)
	ts.must(200, "DELETE", "/api/v1/team/players/"+members[0].PlayerID, cred.Token, nil).json(t, &team)
	ts.must(401, "GET", "/api/v1/team", members[0].Token, nil)
	if len(team.Players) != 3 {
		t.Fatalf("состав: %d", len(team.Players))
	}

	ts.must(200, "POST", "/api/v1/team/withdraw", cred.Token, nil)
	var info gameInfoDTO
	ts.must(200, "GET", game, "", nil).json(t, &info)
	if info.Registration.Teams != 0 {
		t.Fatalf("команд после снятия: %d", info.Registration.Teams)
	}
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

func TestChangeScenarioBeforeStart(t *testing.T) {
	ts := newTestServer(t)
	var v1, v2 scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML).json(t, &v1)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": v1.ID}).json(t, &run)
	admin := "/admin/v1/runs/" + run.ID

	// Команда успела зарегистрироваться по анонсу.
	var cred credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Ранние", "captain_name": "К"}).json(t, &cred)

	// Ведущий поправил сценарий и заменил его в игре.
	fixed := strings.Replace(e2eYAML, "Найдите год", "Найдите год основания", 1)
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, fixed).json(t, &v2)
	if v2.Version != 2 {
		t.Fatalf("версия %d", v2.Version)
	}
	ts.must(404, "PUT", admin+"/scenario", adminToken, map[string]string{"scenario_version_id": "nope"})
	ts.must(200, "PUT", admin+"/scenario", adminToken, map[string]string{"scenario_version_id": v2.ID}).json(t, &run)
	if run.ScenarioVersionID != v2.ID || run.Teams != 1 {
		t.Fatalf("после замены: %+v", run)
	}

	// Токен капитана работает, команда на месте, задание — из новой версии.
	ts.must(200, "POST", admin+"/start", adminToken, nil)
	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", cred.Token, nil).json(t, &st)
	if len(st.Levels) == 0 || st.Levels[0].Task != "Найдите год основания" {
		t.Fatalf("задание: %+v", st.Levels)
	}

	// После старта структуру менять нельзя: лишний уровень — отказ с перечнем отличий.
	var v3 scenarioVersionDTO
	extra := strings.Replace(fixed, "  - id: l3\n", "  - id: lx\n    task: новый\n    answers: [{type: photo}]\n  - id: l3\n", 1)
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, extra).json(t, &v3)
	r := ts.must(409, "PUT", admin+"/scenario", adminToken, map[string]string{"scenario_version_id": v3.ID})
	var body errorBody
	r.json(t, &body)
	if body.Error != "scenario_incompatible" || len(body.Details) == 0 {
		t.Fatalf("ответ: %+v", body)
	}
	ts.must(200, "GET", "/api/v1/state", cred.Token, nil) // игра после отказа продолжает работать

	// …а содержимое — можно: правим текст прямо во время игры.
	ts.must(200, "PUT", admin+"/scenario", adminToken, map[string]string{"scenario_version_id": v1.ID})
	ts.must(200, "GET", "/api/v1/state", cred.Token, nil).json(t, &st)
	if st.Levels[0].Task != "Найдите год" {
		t.Fatalf("текст после правки во время игры: %q", st.Levels[0].Task)
	}
}

func TestFixGeoPointDuringGame(t *testing.T) {
	ts := newTestServer(t)
	geoYAML := `
version: 1
id: geo
title: geo
levels:
  - id: g
    task: Отметьтесь
    answers: [{type: geo, point: [44.6338, 39.1352], radius_m: 100}]
  - id: f
    task: Финиш
    answers: [{type: code, values: ["ok"]}]
`
	var v1, v2 scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, geoYAML).json(t, &v1)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": v1.ID}).json(t, &run)
	var cred credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "T", "captain_name": "K"}).json(t, &cred)
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/start", adminToken, nil)

	here := map[string]any{"kind": "geo", "lat": 44.6200, "lng": 39.1100, "accuracy": 15}
	var ans answerResultDTO
	ts.must(200, "POST", "/api/v1/answers", cred.Token, here).json(t, &ans)
	if ans.Result != "wrong" {
		t.Fatalf("до правки: %+v", ans)
	}

	// Ведущий поправил координаты и заменил сценарий прямо в игре.
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, strings.Replace(geoYAML, "[44.6338, 39.1352]", "[44.6201, 39.1101]", 1)).json(t, &v2)
	ts.must(200, "PUT", "/admin/v1/runs/"+run.ID+"/scenario", adminToken, map[string]string{"scenario_version_id": v2.ID})

	ts.must(200, "POST", "/api/v1/answers", cred.Token, here).json(t, &ans)
	if ans.Result != "correct" {
		t.Fatalf("после правки: %+v", ans)
	}
}
