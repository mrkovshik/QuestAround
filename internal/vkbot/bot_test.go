package vkbot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/store"
)

const testYAML = `
version: 1
id: vk
title: VK-тест
participants: {team_size: {min: 1, max: 3}}
leaderboard: {visibility: host}
levels:
  - id: l1
    title: Старт
    task: Найдите год
    media: [start.jpg]
    answers: [{type: code, values: ["1864"]}]
    hints:
      - {on_request: true, penalty: {time: 5m}, text: Табличка слева}
    skip: {penalty: {time: 30m}}
  - id: l2
    title: Мост
    task: Встаньте на мост
    answers: [{type: geo, point: [44.6338, 39.1352], radius_m: 25, max_accuracy_m: 50}]
    location: {text: Подвесной мост, penalty: {time: 10m}}
  - id: l3
    title: Финиш
    task: Кодовое слово
    answers: [{type: code, values: ["финиш"]}]
`

// fakeAPI запоминает отправленное.
type fakeAPI struct {
	mu      sync.Mutex
	sent    []Outgoing
	uploads int
	updates chan Update // для Run: очередь Long Poll
}

func (f *fakeAPI) LongPollServer(context.Context) (LongPoll, error) { return LongPoll{TS: "1"}, nil }

func (f *fakeAPI) Poll(ctx context.Context, lp LongPoll) (PollResult, error) {
	select {
	case u := <-f.updates:
		return PollResult{TS: lp.TS, Updates: []Update{u}}, nil
	case <-time.After(20 * time.Millisecond):
		return PollResult{TS: lp.TS}, nil
	case <-ctx.Done():
		return PollResult{}, ctx.Err()
	}
}

func (f *fakeAPI) Send(_ context.Context, m Outgoing) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeAPI) UploadPhoto(_ context.Context, _ string, r io.Reader) (string, error) {
	_, _ = io.ReadAll(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads++
	return fmt.Sprintf("photo-1_%d", f.uploads), nil
}

func (f *fakeAPI) UserName(_ context.Context, id int64) (string, string, error) {
	return "Игрок", fmt.Sprintf("Номер%d", id), nil
}

// take возвращает и очищает отправленное пользователю.
func (f *fakeAPI) take(peer int64) []Outgoing {
	f.mu.Lock()
	defer f.mu.Unlock()
	var mine, rest []Outgoing
	for _, m := range f.sent {
		if m.PeerID == peer {
			mine = append(mine, m)
		} else {
			rest = append(rest, m)
		}
	}
	f.sent = rest
	return mine
}

type fakeMedia struct{}

func (fakeMedia) AssetKey(name string) (string, bool) {
	if name == "start.jpg" {
		return "assets/abc.jpg", true
	}
	return "", false
}

func (fakeMedia) Open(_ context.Context, key string) (io.ReadSeekCloser, string, error) {
	return nopCloser{bytes.NewReader([]byte("img:" + key))}, "image/jpeg", nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

type harness struct {
	t     *testing.T
	ctx   context.Context
	api   *fakeAPI
	st    *store.Memory
	bot   *Bot
	run   store.Run
	rn    *runner.Runner
	msgID int64
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st := store.NewMemory()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sv, err := st.SaveScenarioVersion(ctx, store.DefaultOrg, "vk", testYAML)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateRun(ctx, store.NewRun{OrgID: store.DefaultOrg, ScenarioVersionID: sv.ID, Title: "Пилот"})
	if err != nil {
		t.Fatal(err)
	}
	mgr := runner.NewManager(ctx, st, runner.RealClock{}, log)
	rn, err := mgr.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rn.Do(ctx, core.RegisterTeam{TeamID: "t1", Name: "Ёжики", InviteCode: "ABCDEFGH", ByHost: true}); err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{}
	bot := New(Config{API: api, Store: st, VK: st, Runners: mgr, Media: fakeMedia{}, Logger: log, Location: time.UTC})
	return &harness{t: t, ctx: ctx, api: api, st: st, bot: bot, run: run, rn: rn}
}

func (h *harness) msg(user int64, text string, mods ...func(*Message)) []Outgoing {
	h.t.Helper()
	h.msgID++
	m := Message{ID: h.msgID, FromID: user, PeerID: user, Text: text}
	for _, f := range mods {
		f(&m)
	}
	h.bot.HandleMessage(h.ctx, m)
	return h.api.take(user)
}

func withPayload(p payload) func(*Message) {
	return func(m *Message) {
		m.Payload = string(mustJSON(p))
	}
}

func withGeo(lat, lng float64) func(*Message) {
	return func(m *Message) {
		m.Geo = &Geo{}
		m.Geo.Coordinates.Latitude, m.Geo.Coordinates.Longitude = lat, lng
	}
}

func mustJSON(p payload) []byte {
	b := textButton("x", "", p)
	return []byte(b.Action.Payload)
}

func (h *harness) sync() {
	h.t.Helper()
	if err := h.bot.SyncRun(h.ctx, h.run.ID, h.rn); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) do(cmd core.Command) {
	h.t.Helper()
	if _, err := h.rn.Do(h.ctx, cmd); err != nil {
		h.t.Fatalf("%T: %v", cmd, err)
	}
}

func texts(ms []Outgoing) string {
	var parts []string
	for _, m := range ms {
		parts = append(parts, m.Text)
	}
	return strings.Join(parts, "\n---\n")
}

func expect(t *testing.T, ms []Outgoing, subs ...string) {
	t.Helper()
	all := texts(ms)
	for _, s := range subs {
		if !strings.Contains(all, s) {
			t.Errorf("нет %q в сообщениях:\n%s", s, all)
		}
	}
}

func hasButton(kb *Keyboard, label string) bool {
	if kb == nil {
		return false
	}
	for _, row := range kb.Buttons {
		for _, b := range row {
			if strings.Contains(b.Action.Label, label) {
				return true
			}
		}
	}
	return false
}

func findButton(t *testing.T, ms []Outgoing, label string) payload {
	t.Helper()
	for _, m := range ms {
		if m.Keyboard == nil {
			continue
		}
		for _, row := range m.Keyboard.Buttons {
			for _, b := range row {
				if strings.Contains(b.Action.Label, label) {
					return parsePayload(b.Action.Payload)
				}
			}
		}
	}
	t.Fatalf("нет кнопки %q в:\n%s", label, texts(ms))
	return payload{}
}

const (
	alice = int64(101)
	bob   = int64(202)
)

func TestGameFlow(t *testing.T) {
	h := newHarness(t)

	// Незнакомец получает инструкцию.
	expect(t, h.msg(alice, "Привет"), "Пришлите приглашение")

	// Вход по голому инвайту и по ссылке.
	got := h.msg(alice, "abcdefgh")
	expect(t, got, "Вы в игре «Пилот»", "Команда «Ёжики»", "Вы капитан", "Игра ещё не началась")
	if !hasButton(got[0].Keyboard, labelSliv) {
		t.Error("у капитана нет кнопки сливов")
	}
	got = h.msg(bob, "вот ссылка https://quest.example/g/"+strings.ToLower(h.run.PublicCode)+"/join/ABCDEFGH")
	expect(t, got, "Вы в игре «Пилот»", "игроков: 2")
	if hasButton(got[0].Keyboard, labelSliv) {
		t.Error("у не-капитана есть кнопка сливов")
	}
	if b, err := h.st.GetVK(h.ctx, bob); err != nil || b.RunID != h.run.ID {
		t.Fatalf("привязка: %+v %v", b, err)
	}
	// Повторное приглашение в ту же команду не создаёт игрока.
	expect(t, h.msg(bob, "https://quest.example/g/"+h.run.PublicCode+"/join/ABCDEFGH"), "уже в этой команде")

	h.sync() // первый проход только ставит курсор
	if n := len(h.api.take(alice)) + len(h.api.take(bob)); n != 0 {
		t.Fatalf("первый проход разослал %d сообщений", n)
	}

	// Старт: задание приходит обоим, с картинкой и клавиатурой.
	h.do(core.StartRun{})
	h.sync()
	for _, u := range []int64{alice, bob} {
		ms := h.api.take(u)
		expect(t, ms, "Старт!", "Уровень 1 из 3 · «Старт»", "Найдите год", "по запросу, штраф 5 мин")
		var withPhoto bool
		for _, m := range ms {
			withPhoto = withPhoto || len(m.Attachments) == 1
		}
		if !withPhoto {
			t.Errorf("пользователь %d: задание без картинки", u)
		}
	}
	if h.api.uploads != 1 {
		t.Errorf("картинка загружена %d раз, ожидали 1 (кэш)", h.api.uploads)
	}

	expect(t, h.msg(bob, "1863"), "Неверно")

	// Подсказка по запросу: меню → кнопка → уведомление всем.
	menu := h.msg(alice, "", withPayload(payload{Cmd: cmdHint}))
	take := findButton(t, menu, "Подсказка 1")
	if take.Cmd != cmdHintTake || take.Level != "l1" {
		t.Fatalf("кнопка подсказки: %+v", take)
	}
	h.msg(alice, "Подсказка 1", withPayload(take))
	h.sync()
	expect(t, h.api.take(bob), "Подсказка 1 — «Старт» (штраф 5 мин)", "Табличка слева")
	h.api.take(alice)

	// Верный код — следующий уровень с гео.
	expect(t, h.msg(bob, "1864"), "Верно")
	h.sync()
	ms := h.api.take(alice)
	expect(t, ms, "Уровень «Старт» пройден", "Уровень 2 из 3 · «Мост»", "Отправить местоположение")
	h.api.take(bob)

	// Повтор того же сообщения из Long Poll не создаёт событий.
	before, _ := h.st.Load(h.ctx, h.run.ID)
	h.bot.HandleMessage(h.ctx, Message{ID: h.msgID, FromID: bob, PeerID: bob, Text: "1864"})
	h.api.take(bob)
	after, _ := h.st.Load(h.ctx, h.run.ID)
	if len(after) != len(before) {
		t.Fatalf("повтор сообщения добавил %d событий", len(after)-len(before))
	}

	// Сливы — только капитан; локация приходит с точкой.
	expect(t, h.msg(bob, "", withPayload(payload{Cmd: cmdSliv})), "только капитан")
	menu = h.msg(alice, "", withPayload(payload{Cmd: cmdSliv}))
	loc := findButton(t, menu, "Слить локацию · штраф 10 мин")
	h.msg(alice, "", withPayload(loc))
	h.sync()
	ms = h.api.take(bob)
	expect(t, ms, "Локация уровня «Мост» (штраф 10 мин)", "Подвесной мост", "yandex.ru/maps")
	if ms[0].Lat == nil {
		t.Error("слив локации без точки на карте")
	}
	h.api.take(alice)

	// Гео: далеко и близко.
	expect(t, h.msg(alice, "", withGeo(44.70, 39.20)), "Не то место")
	expect(t, h.msg(alice, "", withGeo(44.63381, 39.13521)), "Точка засчитана")
	h.sync()
	expect(t, h.api.take(bob), "Уровень «Мост» пройден", "«Финиш»")
	h.api.take(alice)

	// Сообщения ведущего: всем и в чат команды.
	h.do(core.SendBroadcast{Text: "Через 10 минут дождь"})
	h.do(core.PostChat{TeamID: "t1", FromHost: true, Text: "Вы молодцы"})
	h.sync()
	expect(t, h.api.take(bob), "📢 Через 10 минут дождь", "💬 Ведущий: Вы молодцы")
	h.api.take(alice)

	// Капитан пишет ведущему.
	expect(t, h.msg(bob, "", withPayload(payload{Cmd: cmdHost})), "только капитан")
	expect(t, h.msg(alice, "", withPayload(payload{Cmd: cmdHost})), "Напишите сообщение ведущему")
	expect(t, h.msg(alice, "Мы у фонтана"), "Отправлено ведущему")
	// Режим снят: следующий текст — снова ответ.
	expect(t, h.msg(alice, "не то"), "Неверно")

	// Тревога от любого игрока: место, потом описание — два сигнала.
	expect(t, h.msg(bob, "", withPayload(payload{Cmd: cmdSOS})), "Опишите, что случилось")
	expect(t, h.msg(bob, "", withGeo(44.6, 39.1)), "Сигнал отправлен")
	expect(t, h.msg(bob, "Подвернул ногу"), "Сигнал отправлен")
	expect(t, h.msg(bob, "", withPayload(payload{Cmd: cmdCancel})), "продолжаем игру")

	var chat, sos int
	var sosText string
	_ = h.rn.Read(h.ctx, func(g *core.Game) {
		chat = len(g.Teams["t1"].Chat)
		sos = len(g.SOS)
		if sos > 0 {
			sosText = g.SOS[len(g.SOS)-1].Text
		}
	})
	if chat != 2 || sos != 2 || sosText != "Подвернул ногу" {
		t.Fatalf("чат %d (ожидали 2), сигналов %d (ожидали 2), текст %q", chat, sos, sosText)
	}

	// Финиш.
	expect(t, h.msg(bob, "ФИНИШ"), "Верно")
	h.sync()
	expect(t, h.api.take(alice), "Финиш! Ваша команда прошла игру")

	// Выход.
	expect(t, h.msg(bob, "/выход"), "вышли из игры")
	if _, err := h.st.GetVK(h.ctx, bob); err == nil {
		t.Fatal("после выхода привязка осталась")
	}
	expect(t, h.msg(bob, "1864"), "Пришлите приглашение")
}

func TestUnknownInviteAndRemovedPlayer(t *testing.T) {
	h := newHarness(t)
	expect(t, h.msg(alice, "ZZZZZZZZ"), "Приглашение недействительно")

	h.msg(alice, "ABCDEFGH")
	h.msg(bob, "ABCDEFGH")
	h.sync()
	b, _ := h.st.GetVK(h.ctx, bob)
	h.do(core.RemovePlayer{Actor: core.Actor{Host: true}, TeamID: "t1", PlayerID: b.PlayerID})
	h.sync()
	expect(t, h.api.take(bob), "Вас убрали из команды")
	if _, err := h.st.GetVK(h.ctx, bob); err == nil {
		t.Fatal("удалённый игрок остался привязан")
	}
}

func TestParseInvite(t *testing.T) {
	cases := []struct {
		text      string
		bare      bool
		ok        bool
		run, code string
	}{
		{"https://q.ru/g/abc234/join/ABCDEFGH", false, true, "ABC234", "ABCDEFGH"},
		{"ABCDEFGH", true, true, "", "ABCDEFGH"},
		{"ABCDEFGH", false, false, "", ""}, // играющему — это ответ на задание
		{"ABC234 ABCDEFGH", true, true, "ABC234", "ABCDEFGH"},
		{"привет", true, false, "", ""},
		{"1864", true, false, "", ""},
	}
	for _, c := range cases {
		inv, ok := parseInvite(c.text, c.bare)
		if ok != c.ok || inv.runCode != c.run || inv.code != c.code {
			t.Errorf("%q (bare=%v): %+v %v", c.text, c.bare, inv, ok)
		}
	}
}

func TestFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		40 * time.Second:                "40 сек",
		12 * time.Minute:                "12 мин",
		time.Hour:                       "1 ч",
		time.Hour + 5*time.Minute:       "1 ч 05 мин",
		2*time.Hour + 30*time.Minute:    "2 ч 30 мин",
		12*time.Minute + 29*time.Second: "12 мин",
	} {
		if got := dur(d); got != want {
			t.Errorf("dur(%v) = %q, ожидали %q", d, got, want)
		}
	}
	for n, want := range map[int]string{1: "1 балл", 2: "2 балла", 5: "5 баллов", 11: "11 баллов", 21: "21 балл", 22: "22 балла", 112: "112 баллов"} {
		if got := points(n); got != want {
			t.Errorf("points(%d) = %q, ожидали %q", n, got, want)
		}
	}
}

// Run: сообщения из Long Poll и фоновая рассылка уведомлений.
func TestRunLoop(t *testing.T) {
	h := newHarness(t)
	h.api.updates = make(chan Update, 4)
	ctx, cancel := context.WithCancel(h.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = h.bot.Run(ctx)
	}()
	defer func() { cancel(); <-done }()

	var u Update
	u.Type = "message_new"
	u.Object.Message = Message{ID: 1, FromID: alice, PeerID: alice, Text: "ABCDEFGH"}
	h.api.updates <- u

	waitFor := func(what string) []Outgoing {
		t.Helper()
		var all []Outgoing
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			all = append(all, h.api.take(alice)...)
			if strings.Contains(texts(all), what) {
				return all
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("не дождались %q, пришло:\n%s", what, texts(all))
		return nil
	}
	waitFor("Вы в игре")

	// Курсор ставится фоновым наблюдателем; ждём его, чтобы старт не потерялся.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, ok, _ := h.st.VKCursor(h.ctx, h.run.ID); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("наблюдатель не поставил курсор")
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.do(core.StartRun{})
	waitFor("Найдите год")
}

// Команда целиком играла в приложении, потом включили «белые списки»:
// в VK переходят те же игроки по коду переноса, даже если команда полная.
func TestTransferFromApp(t *testing.T) {
	h := newHarness(t)
	hashOf := func(secret string) string {
		sum := sha256.Sum256([]byte(secret))
		return hex.EncodeToString(sum[:])
	}
	for i, name := range []string{"Капитан", "Вася", "Петя"} { // team_size.max = 3
		h.do(core.JoinTeam{InviteCode: "ABCDEFGH", Player: core.NewPlayer{ID: fmt.Sprintf("p%d", i), Name: name, TokenHash: hashOf(fmt.Sprintf("secret-%d", i))}})
	}
	h.do(core.StartRun{})

	// Новым игроком не войти — мест нет.
	expect(t, h.msg(bob, "ABCDEFGH"), "нет мест")

	// Код капитана — в любом написании.
	code := core.TransferCode(hashOf("secret-0"))
	loose := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	got := h.msg(alice, "мой код "+loose)
	expect(t, got, "Капитан, вы продолжаете игру «Пилот» в VK", "Вы капитан", "Уровень 1 из 3")
	if b, err := h.st.GetVK(h.ctx, alice); err != nil || b.PlayerID != "p0" {
		t.Fatalf("привязка: %+v %v", b, err)
	}
	var players int
	_ = h.rn.Read(h.ctx, func(g *core.Game) { players = len(g.ActivePlayers("t1")) })
	if players != 3 {
		t.Fatalf("игроков стало %d, ожидали 3", players)
	}

	// Ответ из VK засчитывается команде.
	expect(t, h.msg(alice, "1864"), "Верно")

	expect(t, h.msg(bob, "QA-ZZZZ-ZZZZ"), "Код не подошёл")
}
