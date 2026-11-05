package core

import (
	"testing"
	"time"
)

const linearYAML = `
version: 1
id: lin
title: lin
scoring: {mode: mixed}
defaults:
  hint_after: [10m]
  rate_limit: {wrong_answers: 3, per: 1m}
levels:
  - id: l1
    task: t
    points: 10
    answers: [{type: code, values: ["Ёж"]}]
    hints:
      - text: auto
      - {on_request: true, penalty: {time: 5m}, text: paid}
      - {on_request: true, penalty: {points: 3}, text: paid2}
    bonuses:
      - {id: b-code, answer: {type: code, values: ["бонус"]}, points: 2}
      - {id: b-photo, answer: {type: photo}, points: 5}
  - id: l2
    task: t
    points: 10
    require: all
    answers:
      - {type: code, values: ["лев"]}
      - {type: regex, pattern: "^(19|20)\\d{2}$"}
  - id: l3
    task: t
    points: 5
    answers:
      - {type: geo, point: [44.6338, 39.1352], radius_m: 25, max_accuracy_m: 50}
  - id: l4
    task: t
    points: 5
    require: any
    answers:
      - {type: photo}
      - {type: code, values: ["пароль"]}
`

func TestLinearHappyPath(t *testing.T) {
	h := newHarness(t, linearYAML, "a", "b")
	h.do(StartRun{})
	h.eq("статус", h.g.Status, RunStatusRunning)
	h.eq("открыт", h.open("a"), []string{"l1"})

	h.advance(time.Minute)
	h.eq("нормализация", h.code("a", "  ЕЖ "), ResultCorrect)
	h.eq("после l1", h.open("a"), []string{"l2"})

	h.eq("сектор 1", h.code("a", "Лев"), ResultCorrect)
	h.eq("ещё открыт", h.open("a"), []string{"l2"})
	h.eq("повтор сектора", h.code("a", "лев"), ResultDuplicate)
	h.eq("сектор 2", h.code("a", "1987"), ResultCorrect)
	h.eq("после l2", h.open("a"), []string{"l3"})

	geo := func(lat, lng, acc float64) Result {
		return h.do(SubmitAnswer{TeamID: "a", Kind: KindGeo, Lat: lat, Lng: lng, Accuracy: acc}).Answer.Result
	}
	h.eq("далеко", geo(44.64, 39.14, 10), ResultWrong)
	h.eq("плохой GPS", geo(44.6338, 39.1352, 120), ResultInaccurate)
	h.eq("на месте", geo(44.63385, 39.13525, 10), ResultCorrect)

	h.eq("require any", h.code("a", "пароль"), ResultCorrect)
	a := h.team("a")
	h.eq("финиш", a.Status, TeamStatusFinished)
	h.eq("причина", a.FinishReason, "completed")
	h.eq("баллы", a.Points, 30)
	h.eq("уровни", a.LevelsDone, 4)
	h.eq("игра идёт, пока b играет", h.g.Status, RunStatusRunning)

	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "x"}, ErrTeamNotPlaying)

	h.do(FinishRun{})
	h.eq("b финиш", h.team("b").FinishReason, "admin")
	h.eq("игра", h.g.Status, RunStatusFinished)
}

func TestRunFinishesWhenAllTeamsDone(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
levels: [{id: a, task: t, answers: [{type: code, values: ["1"]}]}]
`, "a", "b")
	h.do(StartRun{})
	h.code("a", "1")
	h.eq("ещё идёт", h.g.Status, RunStatusRunning)
	h.code("b", "1")
	h.eq("закончена", h.g.Status, RunStatusFinished)
}

func TestWrongRateLimitAndDedupe(t *testing.T) {
	h := newHarness(t, linearYAML, "a")
	h.do(StartRun{})

	for range 3 {
		h.eq("неверно", h.code("a", "нет"), ResultWrong)
	}
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "ёж"}, ErrRateLimited)
	h.advance(61 * time.Second)
	h.eq("после паузы", h.code("a", "ёж"), ResultCorrect)

	// Повтор из офлайн-очереди: тот же результат, без новых событий.
	cmd := SubmitAnswer{TeamID: "a", ClientEventID: "c-1", Kind: KindCode, Value: "лев"}
	first := h.do(cmd)
	h.eq("первый", first.Answer.Result, ResultCorrect)
	again := h.do(cmd)
	h.eq("повтор: результат", again.Answer.Result, ResultCorrect)
	h.eq("повтор: seq", again.Answer.Seq, first.Answer.Seq)
	h.eq("повтор: событий", len(again.Events), 0)

	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "  "}, ErrEmptyAnswer)
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "x", LevelID: "l4"}, ErrLevelNotOpen)
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "x", LevelID: "zzz"}, ErrUnknownLevel)
}

func TestBonuses(t *testing.T) {
	h := newHarness(t, linearYAML, "a")
	h.do(StartRun{})

	h.eq("бонус-код", h.code("a", "Бонус"), ResultCorrect)
	h.eq("бонус засчитан", h.team("a").Bonuses["b-code"], true)
	h.eq("баллы", h.team("a").Points, 2)
	h.eq("повтор бонуса", h.code("a", "бонус"), ResultDuplicate)
	h.eq("уровень не пройден бонусом", h.open("a"), []string{"l1"})

	h.code("a", "ёж") // проходим уровень — фото-бонус всё ещё можно сдать
	photo := SubmitAnswer{TeamID: "a", Kind: KindPhoto, LevelID: "l1", BonusID: "b-photo", MediaKey: "m/1.jpg"}
	out := h.do(photo)
	h.eq("на модерации", out.Answer.Result, ResultPending)
	h.fail(photo, ErrAlreadyPending)

	q := h.g.PendingModeration()
	h.eq("очередь", len(q), 1)
	h.do(Moderate{Ref: q[0].Seq, Approved: true, By: "host"})
	h.eq("очередь пуста", len(h.g.PendingModeration()), 0)
	h.eq("баллы", h.team("a").Points, 2+10+5)

	out = h.do(photo)
	h.eq("фото-бонус повтор", out.Answer.Result, ResultDuplicate)
	h.fail(Moderate{Ref: q[0].Seq, Approved: true}, ErrNotPending)
}

func TestModeratedAnswer(t *testing.T) {
	h := newHarness(t, linearYAML, "a")
	h.do(StartRun{})
	for _, l := range []string{"l1", "l2", "l3"} {
		h.do(Override{TeamID: "a", Action: OverrideCompleteLevel, LevelID: l, Reason: "тест"})
	}
	h.eq("на l4", h.open("a"), []string{"l4"})

	h.fail(SubmitAnswer{TeamID: "a", Kind: KindPhoto, MediaKey: "m"}, ErrNoTarget) // без уровня
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindPhoto, LevelID: "l4"}, ErrEmptyAnswer)
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindText, LevelID: "l4", Value: "x"}, ErrNoTarget) // нет text-ответа

	cmd := SubmitAnswer{TeamID: "a", ClientEventID: "ph-1", Kind: KindPhoto, LevelID: "l4", MediaKey: "m/1.jpg"}
	out := h.do(cmd)
	h.eq("pending", out.Answer.Result, ResultPending)
	h.eq("цель", *out.Answer.AnswerIdx, 0)

	h.do(Moderate{Ref: out.Answer.Seq, Approved: false, Comment: "не то место"})
	h.eq("уровень открыт", h.open("a"), []string{"l4"})
	h.eq("результат обновлён", h.do(cmd).Answer.Result, ResultWrong)

	out = h.do(SubmitAnswer{TeamID: "a", Kind: KindPhoto, LevelID: "l4", MediaKey: "m/2.jpg"})
	h.do(Moderate{Ref: out.Answer.Seq, Approved: true})
	h.eq("финиш", h.team("a").Status, TeamStatusFinished)
}

func TestHints(t *testing.T) {
	h := newHarness(t, linearYAML, "a")
	h.do(StartRun{})

	at, ok := h.g.NextDeadline()
	h.eq("дедлайн", at, t0.Add(10*time.Minute))
	h.eq("дедлайн есть", ok, true)

	h.advance(9 * time.Minute)
	h.eq("рано", len(h.tick()), 0)
	h.advance(time.Minute)
	ev := h.tick()
	h.eq("автоподсказка", len(ev), 1)
	h.eq("триггер", ev[0].Payload.(HintOpened).Trigger, "timer")
	h.eq("открыта", h.team("a").Levels["l1"].Hints, []bool{true, false, false})
	h.eq("повторный tick", len(h.tick()), 0)

	req := RequestHint{TeamID: "a", LevelID: "l1", HintIdx: 1}
	h.fail(req, ErrNotAllowed) // не капитан
	req.IsCaptain = true
	h.do(req)
	h.fail(req, ErrHintAlreadyOpen)
	h.eq("штраф временем", h.team("a").PenaltyTime, 5*time.Minute)
	h.do(RequestHint{TeamID: "a", IsCaptain: true, LevelID: "l1", HintIdx: 2})
	h.eq("штраф баллами", h.team("a").Points, -3)
	h.fail(RequestHint{TeamID: "a", IsCaptain: true, LevelID: "l1", HintIdx: 0}, ErrNotAllowed) // автоподсказку не запросить
	h.fail(RequestHint{TeamID: "a", IsCaptain: true, LevelID: "l1", HintIdx: 9}, ErrUnknownHint)

	h.code("a", "ёж")
	_, ok = h.g.NextDeadline()
	h.eq("на l2 нет автоподсказок", ok, false)
}

func TestTimeLimitAndStandings(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
timing: {time_limit: 1h}
scoring: {mode: time}
levels:
  - {id: a, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: b, task: t, answers: [{type: code, values: ["2"]}]}
`, "fast", "slow", "idle")
	h.do(StartRun{})
	h.advance(10 * time.Minute)
	h.code("fast", "1")
	h.advance(10 * time.Minute)
	h.code("slow", "1")
	h.do(Override{TeamID: "fast", Action: OverrideAdjustTime, Time: 15 * time.Minute, Reason: "штраф"})

	at, _ := h.g.NextDeadline()
	h.eq("дедлайн = лимит", at, t0.Add(time.Hour))

	h.advance(2 * time.Hour) // актор опоздал — время всё равно обрезается лимитом
	h.tick()
	for _, id := range []string{"fast", "slow", "idle"} {
		h.eq(id+" причина", h.team(id).FinishReason, "time_limit")
	}
	h.eq("игра", h.g.Status, RunStatusFinished)

	st := h.g.Standings(h.now)
	// fast и slow по 1 уровню; slow быстрее (1ч против 1ч15м штрафа).
	h.eq("порядок", []string{st[0].TeamID, st[1].TeamID, st[2].TeamID}, []string{"slow", "fast", "idle"})
	h.eq("время fast", st[1].Elapsed, 75*time.Minute)
	h.eq("время slow", st[0].Elapsed, time.Hour)
}

func TestStandingsTies(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
levels: [{id: a, task: t, points: 5, answers: [{type: code, values: ["1"]}]}]
`, "a", "b", "c")
	h.do(StartRun{})
	h.code("a", "1")
	h.code("b", "1")
	st := h.g.Standings(h.now)
	h.eq("места", []int{st[0].Rank, st[1].Rank, st[2].Rank}, []int{1, 1, 3})
}

const staggeredYAML = `
version: 1
id: st
title: st
start: {mode: staggered, interval: 5m}
flow: {mode: linear, routes: rotate}
levels:
  - {id: s, pin: first, task: t, answers: [{type: code, values: ["s"]}]}
  - {id: x, task: t, answers: [{type: code, values: ["x"]}]}
  - {id: y, task: t, answers: [{type: code, values: ["y"]}]}
  - {id: z, task: t, answers: [{type: code, values: ["z"]}]}
  - {id: f, pin: last, task: t, answers: [{type: code, values: ["f"]}]}
`

func TestStaggeredStartAndRoutes(t *testing.T) {
	h := newHarness(t, staggeredYAML, "a", "b", "c")
	h.do(StartRun{})
	h.eq("a стартовала", h.team("a").Status, TeamStatusPlaying)
	h.eq("b ждёт", h.team("b").Status, TeamStatusScheduled)
	h.eq("c слот", h.team("c").ScheduledAt, t0.Add(10*time.Minute))
	h.fail(SubmitAnswer{TeamID: "b", Kind: KindCode, Value: "s"}, ErrTeamNotPlaying)

	seconds := map[string]bool{}
	for _, id := range []string{"a", "b", "c"} {
		r := h.team(id).Route
		h.eq(id+" старт", r[0], "s")
		h.eq(id+" финиш", r[len(r)-1], "f")
		seconds[r[1]] = true
	}
	h.eq("разные первые точки", len(seconds), 3)

	at, _ := h.g.NextDeadline()
	h.eq("следующий старт", at, t0.Add(5*time.Minute))
	h.advance(5 * time.Minute)
	h.tick()
	h.eq("b стартовала", h.team("b").Status, TeamStatusPlaying)
	h.eq("c ждёт", h.team("c").Status, TeamStatusScheduled)

	// Ручной старт досрочно.
	h.do(StartTeam{TeamID: "c"})
	h.fail(StartTeam{TeamID: "c"}, ErrTeamAlreadyStarted)

	// Команда идёт по своему маршруту.
	r := h.team("b").Route
	for _, id := range r {
		h.eq("уровень "+id, h.open("b"), []string{id})
		h.code("b", id)
	}
	h.eq("b финиш", h.team("b").Status, TeamStatusFinished)
}

func TestStorm(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
flow: {mode: storm}
levels:
  - {id: a, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: b, task: t, answers: [{type: code, values: ["2"]}]}
  - {id: c, task: t, answers: [{type: code, values: ["3"]}]}
`, "t")
	h.do(StartRun{})
	h.eq("все открыты", h.open("t"), []string{"a", "b", "c"})
	h.eq("код без уровня", h.code("t", "2"), ResultCorrect)
	h.eq("b закрыт", h.open("t"), []string{"a", "c"})
	h.eq("ответ не к тому уровню", h.do(SubmitAnswer{TeamID: "t", Kind: KindCode, Value: "1", LevelID: "c"}).Answer.Result, ResultWrong)
	h.code("t", "1")
	h.code("t", "3")
	h.eq("финиш", h.team("t").Status, TeamStatusFinished)
}

func TestGraph(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
flow: {mode: graph}
levels:
  - {id: a, task: t, answers: [{type: code, values: ["a"]}]}
  - {id: b, task: t, answers: [{type: code, values: ["b"]}]}
  - {id: c, task: t, answers: [{type: code, values: ["c"]}], unlock: {all_of: [a, b]}}
  - {id: d, task: t, answers: [{type: code, values: ["d"]}], unlock: {count: 1, of: [c, a]}}
`, "t")
	h.do(StartRun{})
	h.eq("старт", h.open("t"), []string{"a", "b"})
	h.code("t", "a")
	h.eq("d открылся по count", h.open("t"), []string{"b", "d"})
	h.code("t", "b")
	h.eq("c открылся по all_of", h.open("t"), []string{"c", "d"})
	h.code("t", "c")
	h.code("t", "d")
	h.eq("финиш", h.team("t").Status, TeamStatusFinished)
}

func TestLeaderboardVisibilityAndFreeze(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
timing: {time_limit: 1h}
leaderboard: {visibility: host, freeze_before_end: 30m}
levels:
  - {id: a, task: t, points: 1, answers: [{type: code, values: ["1"]}]}
  - {id: b, task: t, points: 1, answers: [{type: code, values: ["2"]}]}
`, "x", "y")
	h.do(StartRun{})
	_, ok := h.g.LeaderboardFor("x", h.now)
	h.eq("host: скрыт", ok, false)

	yes := true
	h.do(SetLeaderboard{Visible: &yes})
	_, ok = h.g.LeaderboardFor("x", h.now)
	h.eq("ведущий включил", ok, true)

	at, _ := h.g.NextDeadline()
	h.eq("дедлайн заморозки", at, t0.Add(30*time.Minute))

	h.advance(10 * time.Minute)
	h.code("x", "1")
	h.advance(20 * time.Minute)
	h.tick()
	h.eq("заморожен", h.g.Leaderboard.Frozen, true)

	h.code("y", "1")
	h.code("y", "2") // y обходит x, но таблица заморожена
	st, _ := h.g.LeaderboardFor("x", h.now)
	h.eq("снимок: лидер", st[0].TeamID, "x")
	h.eq("ведущий видит живую", h.g.Standings(h.now)[0].TeamID, "y")

	h.do(SetLeaderboard{Visible: nil})
	_, ok = h.g.LeaderboardFor("x", h.now)
	h.eq("снова по сценарию", ok, false)

	h.do(FinishRun{})
	st, ok = h.g.LeaderboardFor("x", h.now)
	h.eq("после игры видно", ok, true)
	h.eq("после игры живая", st[0].TeamID, "y")
}

func TestLeaderboardAfterFinish(t *testing.T) {
	h := newHarness(t, `
version: 1
id: x
title: x
leaderboard: {visibility: after_finish}
levels: [{id: a, task: t, answers: [{type: code, values: ["1"]}]}]
`, "x", "y")
	h.do(StartRun{})
	h.code("x", "1")
	_, okX := h.g.LeaderboardFor("x", h.now)
	_, okY := h.g.LeaderboardFor("y", h.now)
	h.eq("x финишировала", okX, true)
	h.eq("y ещё играет", okY, false)
}

func TestOverridesAndErrors(t *testing.T) {
	h := newHarness(t, linearYAML, "a")
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "ёж"}, ErrTeamNotPlaying)
	h.fail(Tick{}, nil)
	h.do(StartRun{})
	h.fail(StartRun{}, ErrRunStarted)
	h.fail(RegisterTeam{TeamID: "z", Name: "z", ByHost: true}, ErrRunStarted)
	h.fail(SubmitAnswer{TeamID: "nope", Kind: KindCode, Value: "1"}, ErrUnknownTeam)

	h.fail(Override{TeamID: "a", Action: OverrideAdjustPoints, Points: 5}, ErrReasonRequired)
	h.do(Override{TeamID: "a", Action: OverrideAdjustPoints, Points: 5, Reason: "за костюмы"})
	h.do(Override{TeamID: "a", Action: OverrideAdjustTime, Time: -2 * time.Minute, Reason: "без связи"})
	h.eq("баллы", h.team("a").Points, 5)
	h.eq("время", h.team("a").PenaltyTime, -2*time.Minute)

	h.do(Override{TeamID: "a", Action: OverrideOpenHint, LevelID: "l1", HintIdx: 1, Reason: "застряли"})
	h.eq("без штрафа", h.team("a").PenaltyTime, -2*time.Minute)

	h.do(SendBroadcast{Text: "Все на финиш!"})
	h.eq("сообщения", len(h.g.Messages), 1)

	h.do(Override{TeamID: "a", Action: OverrideFinishTeam, Reason: "сошли"})
	h.eq("финиш", h.team("a").FinishReason, "admin")
	h.eq("игра закончилась", h.g.Status, RunStatusFinished)
	h.fail(FinishRun{}, ErrRunNotRunning)
}

func TestNoTeams(t *testing.T) {
	h := newHarness(t, linearYAML)
	h.fail(StartRun{}, ErrNoTeams)
}
