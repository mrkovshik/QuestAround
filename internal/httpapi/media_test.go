package httpapi

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"strings"
	"testing"
)

func assetForm(files map[string][]byte) *multipartBody {
	mb := &multipartBody{}
	w := multipart.NewWriter(&mb.buf)
	for name, data := range files {
		fw, _ := w.CreateFormFile("file", name)
		_, _ = fw.Write(data)
	}
	_ = w.Close()
	mb.ctype = w.FormDataContentType()
	return mb
}

const mediaYAML = `
version: 1
id: media
title: Картинки
levels:
  - id: l1
    title: Старт
    task: Где это?
    media: [park.png, "https://example.com/x.jpg"]
    answers: [{type: code, values: ["1"]}]
    hints:
      - {on_request: true, text: Смотри, media: [hint.png]}
    bonuses:
      - {id: selfie, title: Селфи, media: [park.png], answer: {type: photo}, points: 5}
  - id: l2
    title: Финиш
    task: Слово
    answers: [{type: code, values: ["финиш"]}]
`

func TestScenarioMediaChatPhotosAndGallery(t *testing.T) {
	ts := newTestServer(t)
	img := pngBytes(t)

	// сценарий ссылается на незагруженные картинки — ошибка со строкой
	r := ts.must(422, "POST", "/admin/v1/scenarios", adminToken, mediaYAML)
	if !strings.Contains(string(r.body), `картинка \"park.png\" не загружена`) || !strings.Contains(string(r.body), "hint.png") {
		t.Fatalf("ошибка: %s", r.body)
	}
	if c := ts.must(400, "POST", "/admin/v1/assets", adminToken, assetForm(map[string][]byte{"a.bmp": img})).errCode(t); c != "bad_name" {
		t.Fatal(c)
	}
	if c := ts.must(415, "POST", "/admin/v1/assets", adminToken, assetForm(map[string][]byte{"a.png": []byte("not an image")})).errCode(t); c != "not_image" {
		t.Fatal(c)
	}
	var up []assetDTO
	ts.must(201, "POST", "/admin/v1/assets", adminToken, assetForm(map[string][]byte{"park.png": img, "hint.png": img})).json(t, &up)
	var list []assetDTO
	ts.must(200, "GET", "/admin/v1/assets", adminToken, nil).json(t, &list)
	if len(list) != 2 || list[0].Name != "hint.png" || !strings.HasPrefix(list[0].URL, "/m/assets/") || list[0].URL != list[1].URL {
		t.Fatalf("библиотека: %+v", list)
	}
	// файл отдаётся без авторизации и кэшируется навсегда
	pub := ts.must(200, "GET", list[0].URL, "", nil)
	if !bytes.Equal(pub.body, img) || !strings.Contains(pub.header.Get("Cache-Control"), "immutable") {
		t.Fatalf("раздача: %d байт, %q", len(pub.body), pub.header.Get("Cache-Control"))
	}
	ts.must(404, "GET", "/m/assets/nope.png", "", nil)

	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, mediaYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	admin := "/admin/v1/runs/" + run.ID
	var a, b credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Бета", "captain_name": "Боря"}).json(t, &b)
	ts.must(200, "POST", admin+"/start", adminToken, nil)

	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	l := st.Levels[0]
	if len(l.Media) != 2 || l.Media[0] != list[0].URL || l.Media[1] != "https://example.com/x.jpg" {
		t.Fatalf("media уровня: %v", l.Media)
	}
	if len(l.Hints[0].Media) != 0 || l.Hints[0].Text != "" {
		t.Fatalf("закрытая подсказка выдала картинку: %+v", l.Hints[0])
	}
	if len(l.Bonuses[0].Media) != 1 {
		t.Fatalf("media бонуса: %v", l.Bonuses[0].Media)
	}
	ts.must(204, "POST", "/api/v1/levels/l1/hints/0", a.Token, nil)
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if h := st.Levels[0].Hints[0]; len(h.Media) != 1 || h.Media[0] != list[0].URL {
		t.Fatalf("открытая подсказка: %+v", h)
	}

	// фото в чате: капитан грузит фото и пишет без текста; ключ чужой игры не принимается
	var u uploadDTO
	ts.must(201, "POST", "/api/v1/uploads", a.Token, photoForm(t, nil, img)).json(t, &u)
	if !strings.HasPrefix(u.Key, run.ID+"/") || u.URL != "/m/"+u.Key {
		t.Fatalf("загрузка: %+v", u)
	}
	ts.must(204, "POST", "/api/v1/chat", a.Token, map[string]string{"media_key": u.Key})
	if c := ts.must(400, "POST", "/api/v1/chat", a.Token, map[string]string{"text": "x", "media_key": "other/abc.png"}).errCode(t); c != "bad_media" {
		t.Fatal(c)
	}
	ts.must(400, "POST", "/api/v1/chat", a.Token, map[string]string{}) // ни текста, ни фото
	var hu uploadDTO
	ts.must(201, "POST", admin+"/uploads", adminToken, photoForm(t, nil, img)).json(t, &hu)
	ts.must(200, "POST", admin+"/teams/"+a.TeamID+"/chat", adminToken, map[string]string{"text": "Вот где мы", "media_key": hu.Key})
	ts.must(200, "POST", admin+"/broadcast", adminToken, map[string]string{"media_key": hu.Key})
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if len(st.Chat) != 2 || st.Chat[0].MediaURL != u.URL || st.Chat[0].Text != "" || st.Chat[1].MediaURL != hu.URL {
		t.Fatalf("чат: %+v", st.Chat)
	}
	if len(st.Messages) != 1 || st.Messages[0].MediaURL != hu.URL {
		t.Fatalf("объявления: %+v", st.Messages)
	}

	// галерея: два фото бонуса, одно отклонено, одно принято
	for _, tok := range []string{a.Token, b.Token} {
		ts.must(200, "POST", "/api/v1/photos", tok, photoForm(t, map[string]string{"level_id": "l1", "bonus_id": "selfie"}, img))
	}
	var ph []photoDTO
	ts.must(200, "GET", admin+"/photos", adminToken, nil).json(t, &ph)
	if len(ph) != 2 || ph[0].Status != "pending" || ph[0].TeamName != "Альфа" || ph[0].PlayerName != "Аня" ||
		ph[0].Title != "Старт · бонус «Селфи»" || !strings.HasPrefix(ph[0].URL, "/m/"+run.ID+"/") {
		t.Fatalf("галерея: %+v", ph)
	}
	ts.must(200, "POST", admin+"/moderation/"+itoa64(ph[0].Seq), adminToken, map[string]any{"approved": true})
	ts.must(200, "POST", admin+"/moderation/"+itoa64(ph[1].Seq), adminToken, map[string]any{"approved": false, "comment": "не все в кадре"})
	ts.must(200, "GET", admin+"/photos", adminToken, nil).json(t, &ph)
	if ph[0].Status != "accepted" || ph[1].Status != "rejected" || ph[1].Comment != "не все в кадре" {
		t.Fatalf("статусы: %+v", ph)
	}
	ts.must(200, "GET", "/api/v1/state", b.Token, nil).json(t, &st)
	if len(st.Photos) != 1 || st.Photos[0].Status != "rejected" || st.Photos[0].TeamName != "" {
		t.Fatalf("фото у игрока: %+v", st.Photos)
	}

	// архив: по ссылке с токеном в query; отклонённые — только с all=1
	ts.must(401, "GET", admin+"/photos.zip", "", nil)
	for q, want := range map[string]int{"": 1, "&all=1": 2} {
		z := ts.must(200, "GET", admin+"/photos.zip?token="+adminToken+q, "", nil)
		zr, err := zip.NewReader(bytes.NewReader(z.body), int64(len(z.body)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != want {
			t.Fatalf("архив%s: %d файлов, ожидали %d", q, len(zr.File), want)
		}
		if n := zr.File[0].Name; !strings.HasPrefix(n, "Альфа/") || !strings.HasSuffix(n, "Старт · бонус «Селфи».png") {
			t.Fatalf("имя в архиве: %q", n)
		}
	}

	// удалённая из библиотеки картинка пропадает из задания, но сценарий не ломается
	ts.must(204, "DELETE", "/admin/v1/assets/park.png", adminToken, nil)
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if m := st.Levels[0].Media; len(m) != 1 || m[0] != "https://example.com/x.jpg" {
		t.Fatalf("после удаления: %v", m)
	}
}

func TestChatAndSOS(t *testing.T) {
	ts := newTestServer(t)
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	admin := "/admin/v1/runs/" + run.ID
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	var j struct {
		Token string `json:"token"`
	}
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/join", "", map[string]string{"invite_code": a.InviteCode, "name": "Петя"}).json(t, &j)

	ts.must(204, "POST", "/api/v1/chat", a.Token, map[string]string{"text": "Привет"})
	ts.must(403, "POST", "/api/v1/chat", j.Token, map[string]string{"text": "Я не капитан"})
	ts.must(200, "PUT", admin+"/contact", adminToken, map[string]string{"text": "+7 900 000-00-00"})
	var s struct {
		Seq int64 `json:"seq"`
	}
	ts.must(201, "POST", "/api/v1/sos", j.Token, map[string]any{"text": "Упал", "lat": 44.1, "lng": 39.1}).json(t, &s)
	var dash dashboardDTO
	ts.must(200, "GET", admin+"/dashboard", adminToken, nil).json(t, &dash)
	if len(dash.SOS) != 1 || dash.SOS[0].PlayerName != "Петя" || dash.SOS[0].ResolvedAt != nil {
		t.Fatalf("тревога: %+v", dash.SOS)
	}
	ts.must(200, "POST", admin+"/sos/"+itoa64(s.Seq)+"/resolve", adminToken, map[string]string{"note": "едем"})
	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", j.Token, nil).json(t, &st)
	if st.HostContact != "+7 900 000-00-00" || len(st.Chat) != 1 || len(st.SOS) != 1 || st.SOS[0].Note != "едем" {
		t.Fatalf("состояние игрока: contact=%q chat=%+v sos=%+v", st.HostContact, st.Chat, st.SOS)
	}
}
