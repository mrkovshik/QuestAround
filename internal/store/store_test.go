package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"questline/internal/core"
)

// Тесты Postgres запускаются, только если задан QUESTLINE_TEST_DATABASE_URL
// (пустая база, например postgres://postgres@localhost:5432/questline_test).
func stores(t *testing.T) map[string]Store {
	out := map[string]Store{"memory": NewMemory()}
	dsn := os.Getenv("QUESTLINE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Log("QUESTLINE_TEST_DATABASE_URL не задан — Postgres пропущен")
		return out
	}
	ctx := context.Background()
	pg, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	// Повторный запуск миграций не должен ломаться.
	if err := pg.migrate(ctx); err != nil {
		t.Fatalf("повторная миграция: %v", err)
	}
	t.Cleanup(pg.Close)
	out["postgres"] = pg
	return out
}

func forEachStore(t *testing.T, fn func(t *testing.T, s Store)) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) { fn(t, s) })
	}
}

var at = time.Date(2026, 11, 29, 12, 0, 0, 123456000, time.UTC)

func newRun(t *testing.T, s Store) Run {
	t.Helper()
	ctx := context.Background()
	sv, err := s.SaveScenarioVersion(ctx, DefaultOrg, "pilot", "version: 1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRun(ctx, NewRun{OrgID: DefaultOrg, ScenarioVersionID: sv.ID, Title: "Пилот"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func sampleEvents() []core.Event {
	idx := 1
	return []core.Event{
		{Seq: 1, Type: core.EvTeamRegistered, At: at, TeamID: "a", Payload: core.TeamRegistered{Name: "Ёжики"}},
		{Seq: 2, Type: core.EvRunStarted, At: at, Payload: core.RunStarted{}},
		{Seq: 3, Type: core.EvAnswerSubmitted, At: at.Add(time.Second), TeamID: "a", PlayerID: "p1", ClientEventID: "c-1",
			Payload: core.AnswerSubmitted{LevelID: "l1", Kind: core.KindCode, Value: "лев", Result: core.ResultCorrect, AnswerIdx: &idx}},
		{Seq: 4, Type: core.EvHintOpened, At: at.Add(time.Minute), TeamID: "a",
			Payload: core.HintOpened{LevelID: "l1", HintIdx: 0, Trigger: "request", PenaltyTime: 5 * time.Minute}},
	}
}

func TestEventsRoundTrip(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		ctx := context.Background()
		r := newRun(t, s)
		events := sampleEvents()
		if err := s.Append(ctx, r.ID, events[:2]); err != nil {
			t.Fatal(err)
		}
		if err := s.Append(ctx, r.ID, events[2:]); err != nil {
			t.Fatal(err)
		}
		got, err := s.Load(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, events) {
			t.Errorf("журнал после загрузки отличается:\n got %+v\nwant %+v", got, events)
		}

		empty, err := s.Load(ctx, "no-such-run")
		if err != nil || len(empty) != 0 {
			t.Errorf("пустой журнал: %v, %v", empty, err)
		}
	})
}

func TestAppendConflict(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		ctx := context.Background()
		r := newRun(t, s)
		events := sampleEvents()
		if err := s.Append(ctx, r.ID, events[:2]); err != nil {
			t.Fatal(err)
		}
		cases := map[string][]core.Event{
			"повтор seq":  events[1:3],
			"пропуск seq": events[3:],
		}
		for name, batch := range cases {
			if err := s.Append(ctx, r.ID, batch); !errors.Is(err, ErrConflict) {
				t.Errorf("%s: ожидали ErrConflict, получили %v", name, err)
			}
		}
		got, _ := s.Load(ctx, r.ID)
		if len(got) != 2 {
			t.Errorf("после конфликтов в журнале %d событий, ожидали 2", len(got))
		}
	})
}

// Параллельные дописывания одного и того же Seq: ровно одно проходит.
func TestAppendRace(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		ctx := context.Background()
		r := newRun(t, s)
		ev := sampleEvents()[:1]
		var (
			wg        sync.WaitGroup
			mu        sync.Mutex
			ok, confl int
		)
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.Append(ctx, r.ID, ev)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, ErrConflict):
					confl++
				default:
					t.Errorf("неожиданная ошибка: %v", err)
				}
			}()
		}
		wg.Wait()
		if ok != 1 || confl != 7 {
			t.Errorf("успешных %d, конфликтов %d; ожидали 1 и 7", ok, confl)
		}
	})
}

func TestScenarioVersionsAndRuns(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		ctx := context.Background()
		slug := "s-" + newID()
		v1, err := s.SaveScenarioVersion(ctx, DefaultOrg, slug, "a: 1")
		if err != nil {
			t.Fatal(err)
		}
		v2, err := s.SaveScenarioVersion(ctx, DefaultOrg, slug, "a: 2")
		if err != nil {
			t.Fatal(err)
		}
		if v1.Version != 1 || v2.Version != 2 {
			t.Errorf("версии %d, %d", v1.Version, v2.Version)
		}
		list, err := s.ListScenarioVersions(ctx, DefaultOrg)
		if err != nil {
			t.Fatal(err)
		}
		var mine []ScenarioVersion
		for _, sv := range list {
			if sv.Slug == slug {
				mine = append(mine, sv)
			}
			if sv.Source != "" {
				t.Error("список версий не должен содержать исходник")
			}
		}
		if len(mine) != 2 || mine[0].Version != 2 {
			t.Errorf("список версий: %+v", mine)
		}

		got, err := s.GetScenarioVersion(ctx, v1.ID)
		if err != nil || got.Source != "a: 1" || got.Slug != slug {
			t.Errorf("GetScenarioVersion: %+v, %v", got, err)
		}
		if _, err := s.GetScenarioVersion(ctx, "nope"); !errors.Is(err, ErrNotFound) {
			t.Errorf("ожидали ErrNotFound, получили %v", err)
		}

		r, err := s.CreateRun(ctx, NewRun{OrgID: DefaultOrg, ScenarioVersionID: v2.ID, Title: "Игра"})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.PublicCode) != 6 || r.ID == "" || r.Seed == 0 {
			t.Errorf("run: %+v", r)
		}
		byID, err := s.GetRun(ctx, r.ID)
		if err != nil || !reflect.DeepEqual(byID, r) {
			t.Errorf("GetRun: %+v, %v\nwant %+v", byID, err, r)
		}
		byCode, err := s.GetRunByCode(ctx, r.PublicCode)
		if err != nil || byCode.ID != r.ID {
			t.Errorf("GetRunByCode: %+v, %v", byCode, err)
		}
		runs, err := s.ListRuns(ctx, DefaultOrg)
		if err != nil || len(runs) == 0 {
			t.Errorf("ListRuns: %v, %v", runs, err)
		}
		if _, err := s.CreateRun(ctx, NewRun{OrgID: DefaultOrg, ScenarioVersionID: "nope"}); !errors.Is(err, ErrNotFound) {
			t.Errorf("run без сценария: ожидали ErrNotFound, получили %v", err)
		}
	})
}
