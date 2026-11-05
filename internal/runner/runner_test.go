package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"questline/internal/core"
	"questline/internal/scenario"
	"questline/internal/store"
)

// --- фейковые часы ---

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at      time.Time
	ch      chan time.Time
	stopped bool
	fired   bool
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	if d <= 0 {
		t.fired = true
		t.ch <- c.now
	} else {
		c.timers = append(c.timers, t)
	}
	return &fakeTimerHandle{c: c, t: t}
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.stopped && !t.fired && !t.at.After(c.now) {
			t.fired = true
			t.ch <- c.now
		}
	}
}

type fakeTimerHandle struct {
	c *fakeClock
	t *fakeTimer
}

func (h *fakeTimerHandle) C() <-chan time.Time { return h.t.ch }
func (h *fakeTimerHandle) Stop() bool {
	h.c.mu.Lock()
	defer h.c.mu.Unlock()
	was := !h.t.stopped && !h.t.fired
	h.t.stopped = true
	return was
}

// --- окружение ---

var t0 = time.Date(2026, 11, 29, 12, 0, 0, 0, time.UTC)

const yamlStaggered = `
version: 1
id: x
title: x
start: {mode: staggered, interval: 5m}
timing: {time_limit: 1h}
defaults: {hint_after: [10m]}
levels:
  - id: a
    task: t
    points: 1
    answers: [{type: code, values: ["1"]}]
    hints: [{text: подсказка}]
  - {id: b, task: t, points: 1, answers: [{type: code, values: ["2"]}]}
`

type env struct {
	t     *testing.T
	ctx   context.Context
	stop  context.CancelFunc
	clock *fakeClock
	store store.EventStore
	sc    *scenario.Scenario
	r     *Runner
}

func newEnv(t *testing.T, st store.EventStore) *env {
	t.Helper()
	sc, err := scenario.Parse([]byte(yamlStaggered))
	if err != nil {
		t.Fatal(err)
	}
	if st == nil {
		st = store.NewMemory()
	}
	e := &env{t: t, clock: newFakeClock(t0), store: st, sc: sc}
	e.start()
	return e
}

func (e *env) start() {
	e.t.Helper()
	e.ctx, e.stop = context.WithCancel(context.Background())
	e.t.Cleanup(e.stop)
	r, err := Start(e.ctx, Config{
		RunID: "run-1", Scenario: e.sc, Seed: 1, Store: e.store, Clock: e.clock,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.r = r
}

func (e *env) restart() {
	e.t.Helper()
	e.stop()
	<-e.r.Done()
	e.start()
}

func (e *env) do(cmd core.Command) core.Outcome {
	e.t.Helper()
	out, err := e.r.Do(context.Background(), cmd)
	if err != nil {
		e.t.Fatalf("%T: %v", cmd, err)
	}
	return out
}

func (e *env) read(fn func(g *core.Game)) {
	e.t.Helper()
	if err := e.r.Read(context.Background(), fn); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) teamStatus(id string) (s core.TeamStatus) {
	e.read(func(g *core.Game) { s = g.Teams[id].Status })
	return s
}

// eventually ждёт, пока актор обработает сработавший таймер.
func (e *env) eventually(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (e *env) setup(teams ...string) {
	for _, id := range teams {
		e.do(core.RegisterTeam{TeamID: id, Name: id, ByHost: true})
	}
	e.do(core.StartRun{})
}

// --- тесты ---

func TestCommandsArePersisted(t *testing.T) {
	e := newEnv(t, nil)
	e.setup("a", "b")
	out := e.do(core.SubmitAnswer{TeamID: "a", Kind: core.KindCode, Value: "1"})
	if out.Answer.Result != core.ResultCorrect {
		t.Fatalf("результат %s", out.Answer.Result)
	}

	saved, _ := e.store.Load(context.Background(), "run-1")
	var seq int64
	e.read(func(g *core.Game) { seq = g.Seq() })
	if int64(len(saved)) != seq || saved[len(saved)-1].Seq != seq {
		t.Fatalf("в журнале %d событий, в памяти seq %d", len(saved), seq)
	}

	if _, err := e.r.Do(context.Background(), core.StartRun{}); !errors.Is(err, core.ErrRunStarted) {
		t.Fatalf("ожидали ErrRunStarted, получили %v", err)
	}
}

func TestTimersFireAutomatically(t *testing.T) {
	e := newEnv(t, nil)
	e.setup("a", "b")
	if s := e.teamStatus("b"); s != core.TeamStatusScheduled {
		t.Fatalf("b: %s", s)
	}

	e.clock.Advance(5 * time.Minute)
	e.eventually("старт b по расписанию", func() bool { return e.teamStatus("b") == core.TeamStatusPlaying })

	e.clock.Advance(5 * time.Minute) // 10 мин от старта a — автоподсказка
	e.eventually("автоподсказка a", func() bool {
		var open bool
		e.read(func(g *core.Game) { open = g.Teams["a"].Levels["a"].Hints[0] })
		return open
	})

	e.clock.Advance(time.Hour)
	e.eventually("лимит времени", func() bool { return e.teamStatus("b") == core.TeamStatusFinished })
	var st core.RunStatus
	e.read(func(g *core.Game) { st = g.Status })
	if st != core.RunStatusFinished {
		t.Fatalf("игра: %s", st)
	}
}

func TestRestartRestoresState(t *testing.T) {
	e := newEnv(t, nil)
	e.setup("a", "b")
	e.clock.Advance(time.Minute)
	e.do(core.SubmitAnswer{TeamID: "a", ClientEventID: "c-1", Kind: core.KindCode, Value: "1"})

	snapshot := func() string {
		var s string
		e.read(func(g *core.Game) { s = fmt.Sprintf("%d %+v", g.Seq(), g.Standings(e.clock.Now())) })
		return s
	}
	before := snapshot()
	e.restart()
	if after := snapshot(); after != before {
		t.Fatalf("после рестарта:\n%s\nдо:\n%s", after, before)
	}

	// Идемпотентность переживает рестарт.
	out := e.do(core.SubmitAnswer{TeamID: "a", ClientEventID: "c-1", Kind: core.KindCode, Value: "1"})
	if len(out.Events) != 0 || out.Answer.Result != core.ResultCorrect {
		t.Fatalf("повтор после рестарта: %+v", out)
	}

	// Таймеры тоже восстанавливаются: b стартует по расписанию.
	e.clock.Advance(4 * time.Minute)
	e.eventually("старт b после рестарта", func() bool { return e.teamStatus("b") == core.TeamStatusPlaying })
}

// flakyStore отказывает в записи по требованию.
type flakyStore struct {
	store.EventStore
	mu   sync.Mutex
	fail bool
}

func (f *flakyStore) Append(ctx context.Context, runID string, ev []core.Event) error {
	f.mu.Lock()
	fail := f.fail
	f.mu.Unlock()
	if fail {
		return errors.New("диск сгорел")
	}
	return f.EventStore.Append(ctx, runID, ev)
}

func TestAppendFailureRollsBack(t *testing.T) {
	fs := &flakyStore{EventStore: store.NewMemory()}
	e := newEnv(t, fs)
	e.setup("a", "b")

	fs.mu.Lock()
	fs.fail = true
	fs.mu.Unlock()
	if _, err := e.r.Do(context.Background(), core.SubmitAnswer{TeamID: "a", Kind: core.KindCode, Value: "1"}); err == nil {
		t.Fatal("ожидали ошибку записи")
	}
	var done int
	e.read(func(g *core.Game) { done = g.Teams["a"].LevelsDone })
	if done != 0 {
		t.Fatal("состояние не откатилось после сбоя записи")
	}

	fs.mu.Lock()
	fs.fail = false
	fs.mu.Unlock()
	out := e.do(core.SubmitAnswer{TeamID: "a", Kind: core.KindCode, Value: "1"})
	if out.Answer.Result != core.ResultCorrect {
		t.Fatalf("после восстановления: %s", out.Answer.Result)
	}
}

func TestSubscriptions(t *testing.T) {
	e := newEnv(t, nil)
	e.setup("a", "b")

	chA, cancelA := e.r.Subscribe("a")
	chB, cancelB := e.r.Subscribe("b")
	chHost, cancelHost := e.r.Subscribe("")
	defer cancelB()
	defer cancelHost()

	out := e.do(core.SubmitAnswer{TeamID: "a", Kind: core.KindCode, Value: "1"})
	last := out.Events[len(out.Events)-1].Seq

	if got := <-chA; got != last {
		t.Errorf("a: seq %d, ожидали %d", got, last)
	}
	if got := <-chHost; got != last {
		t.Errorf("ведущий: seq %d, ожидали %d", got, last)
	}
	select {
	case v := <-chB:
		t.Errorf("b получила чужое изменение: %d", v)
	default:
	}

	// Общее событие — всем; несколько изменений схлопываются в последнее.
	e.do(core.SendBroadcast{Text: "раз"})
	out = e.do(core.SendBroadcast{Text: "два"})
	if got := <-chB; got != out.Events[0].Seq {
		t.Errorf("b: seq %d, ожидали последнее %d", got, out.Events[0].Seq)
	}

	cancelA()
	n := 0
	for range chA { // в буфере может остаться одно непрочитанное значение
		if n++; n > 1 {
			t.Error("канал a не закрыт после cancel")
			break
		}
	}
	cancelA() // повторный cancel безопасен

	e.stop()
	<-e.r.Done()
	<-chB // мог остаться непрочитанный seq
	if _, ok := <-chB; ok {
		t.Error("канал b не закрыт после остановки")
	}
	if _, err := e.r.Do(context.Background(), core.Tick{}); !errors.Is(err, ErrStopped) {
		t.Errorf("после остановки: %v", err)
	}
}

func TestConcurrentCommands(t *testing.T) {
	e := newEnv(t, nil)
	e.setup("a", "b")
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.r.Do(context.Background(), core.SendBroadcast{Text: fmt.Sprint(i)})
			_ = e.r.Read(context.Background(), func(g *core.Game) { _ = g.Standings(t0) })
		}()
	}
	wg.Wait()
	saved, _ := e.store.Load(context.Background(), "run-1")
	for i, ev := range saved {
		if ev.Seq != int64(i+1) {
			t.Fatalf("дыра в журнале: позиция %d, seq %d", i, ev.Seq)
		}
	}
	var msgs int
	e.read(func(g *core.Game) { msgs = len(g.Messages) })
	if msgs != 50 {
		t.Fatalf("сообщений %d", msgs)
	}
}

func TestManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := store.NewMemory()
	sv, _ := st.SaveScenarioVersion(ctx, store.DefaultOrg, "x", yamlStaggered)
	run, _ := st.CreateRun(ctx, store.NewRun{OrgID: store.DefaultOrg, ScenarioVersionID: sv.ID})

	m := NewManager(ctx, st, newFakeClock(t0), slog.New(slog.NewTextHandler(io.Discard, nil)))
	r1, err := m.Get(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := m.Get(ctx, run.ID)
	if r1 != r2 {
		t.Fatal("второй Get должен вернуть тот же актор")
	}
	if _, err := m.Get(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, получили %v", err)
	}
}
