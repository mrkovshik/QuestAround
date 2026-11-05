package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"questline/internal/scenario"
)

var t0 = time.Date(2026, 11, 29, 12, 0, 0, 0, time.UTC)

// harness — игра + часы + журнал. После каждого теста проверяет, что
// журнал (в т.ч. после JSON-сериализации) восстанавливает то же состояние.
type harness struct {
	t   *testing.T
	g   *Game
	now time.Time
	log []Event
}

func newHarness(t *testing.T, yaml string, teams ...string) *harness {
	t.Helper()
	sc, err := scenario.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("сценарий: %v", err)
	}
	h := &harness{t: t, g: NewGame("run-1", sc, 7), now: t0}
	for _, id := range teams {
		h.do(RegisterTeam{TeamID: id, Name: "Команда " + id, ByHost: true})
	}
	t.Cleanup(h.checkReplay)
	return h
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

func (h *harness) do(cmd Command) Outcome {
	h.t.Helper()
	out, err := h.g.Handle(h.now, cmd)
	if err != nil {
		h.t.Fatalf("%T: %v", cmd, err)
	}
	h.log = append(h.log, out.Events...)
	return out
}

func (h *harness) fail(cmd Command, want error) {
	h.t.Helper()
	seq := h.g.Seq()
	_, err := h.g.Handle(h.now, cmd)
	if !errors.Is(err, want) {
		h.t.Fatalf("%T: ожидали %v, получили %v", cmd, want, err)
	}
	if h.g.Seq() != seq {
		h.t.Fatalf("%T: ошибка, но появились события", cmd)
	}
}

func (h *harness) tick() []Event { return h.do(Tick{}).Events }

func (h *harness) code(team, value string) Result {
	h.t.Helper()
	return h.do(SubmitAnswer{TeamID: team, PlayerID: "p", Kind: KindCode, Value: value}).Answer.Result
}

func (h *harness) team(id string) *Team { return h.g.Teams[id] }

// open — открытые незавершённые уровни команды в порядке сценария.
func (h *harness) open(team string) []string {
	var out []string
	for _, l := range h.g.Scenario.Levels {
		if h.team(team).Levels[l.ID].IsOpen() {
			out = append(out, l.ID)
		}
	}
	return out
}

func (h *harness) eq(name string, got, want any) {
	h.t.Helper()
	if !reflect.DeepEqual(got, want) {
		h.t.Errorf("%s: получили %v, ожидали %v", name, got, want)
	}
}

func (h *harness) checkReplay() {
	if h.t.Failed() {
		return
	}
	for i, e := range h.log {
		if e.Seq != int64(i+1) {
			h.t.Fatalf("seq: событие %d имеет Seq %d", i, e.Seq)
		}
	}
	direct := Replay(h.g.RunID, h.g.Scenario, h.g.Seed, h.log)
	if !reflect.DeepEqual(direct, h.g) {
		h.t.Fatal("replay журнала не совпал с текущим состоянием")
	}

	data, err := json.Marshal(h.log)
	if err != nil {
		h.t.Fatal(err)
	}
	var decoded []Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		h.t.Fatal(err)
	}
	fromJSON := Replay(h.g.RunID, h.g.Scenario, h.g.Seed, decoded)
	// time.Time после JSON теряет monotonic/location — сравниваем через повторную сериализацию.
	a, _ := json.Marshal(snapshot(fromJSON, h.now))
	b, _ := json.Marshal(snapshot(h.g, h.now))
	if string(a) != string(b) {
		h.t.Fatalf("replay после JSON отличается:\n%s\n%s", a, b)
	}
}

// snapshot — наблюдаемое состояние для сравнения.
func snapshot(g *Game, now time.Time) any {
	return map[string]any{
		"status": g.Status, "teams": g.Teams, "players": g.Players, "reg": g.Registration, "lb": g.Leaderboard,
		"standings": g.Standings(now), "pending": g.PendingModeration(), "seq": g.Seq(),
	}
}
