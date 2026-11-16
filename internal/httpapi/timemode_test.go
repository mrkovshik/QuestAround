package httpapi

import (
	"strings"
	"testing"
)

const timeYAML = `
version: 1
id: tm
title: По времени
scoring: {mode: time}
leaderboard: {visibility: always}
levels:
  - id: l1
    title: Старт
    task: Код
    answers: [{type: code, values: ["1"]}]
    hints: [{on_request: true, penalty: {time: 5m}, text: Подсказка}]
    bonuses: [{id: b, title: Бонус, answer: {type: code, values: ["бонус"]}, time: 7m}]
`

func TestTimeModeScoring(t *testing.T) {
	ts := newTestServer(t)
	bad := strings.Replace(timeYAML, "time: 7m}", "points: 5}", 1)
	r := ts.must(422, "POST", "/admin/v1/scenarios", adminToken, bad)
	if !strings.Contains(string(r.body), "в зачёте по времени бонус — только временем") {
		t.Fatalf("ошибка: %s", r.body)
	}
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, timeYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/start", adminToken, nil)

	ts.must(204, "POST", "/api/v1/levels/l1/hints/0", a.Token, nil)
	ts.must(200, "POST", "/api/v1/answers", a.Token, map[string]any{"client_event_id": "c1", "level_id": "l1", "kind": "code", "value": "бонус"})

	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if st.ScoringMode != "time" || st.PenaltySec != 300 || st.BonusSec != 420 || st.Levels[0].Bonuses[0].TimeSec != 420 {
		t.Fatalf("состояние: mode=%s penalty=%d bonus=%d bonusTime=%d", st.ScoringMode, st.PenaltySec, st.BonusSec, st.Levels[0].Bonuses[0].TimeSec)
	}
	if len(st.Score) != 2 || st.Score[1].TimeSec != -420 {
		t.Fatalf("расшифровка: %+v", st.Score)
	}
	var lb struct {
		Mode      string        `json:"mode"`
		Standings []standingDTO `json:"standings"`
	}
	ts.must(200, "GET", "/api/v1/leaderboard", a.Token, nil).json(t, &lb)
	if lb.Mode != "time" || lb.Standings[0].PenaltySec != 300 || lb.Standings[0].BonusSec != 420 {
		t.Fatalf("таблица: %+v", lb)
	}
}
