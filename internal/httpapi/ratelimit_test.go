package httpapi

import (
	"testing"
	"time"
)

const rateLimitYAML = `
version: 1
id: rl
title: Перебор
scoring: {mode: time}
levels:
  - id: l1
    title: Замок
    task: t
    answers: [{type: code, values: ["1"]}]
    rate_limit: {wrong_answers: 2, per: 1m, block_for: 3m}
  - id: l2
    title: Сейф
    task: t
    answers: [{type: code, values: ["2"]}]
    rate_limit: {wrong_answers: 2, per: 1m, action: penalty, penalty: {time: 5m}}
`

func TestLevelRateLimitAPI(t *testing.T) {
	ts := newTestServer(t)
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, rateLimitYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/start", adminToken, nil)

	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	rl := st.Levels[0].RateLimit
	if rl == nil || rl.WrongAnswers != 2 || rl.PerSec != 60 || rl.Action != "block" || rl.BlockSec != 180 || rl.BlockedUntil != nil {
		t.Fatalf("правило: %+v", rl)
	}

	wrong := func() answerResultDTO {
		var ans answerResultDTO
		ts.must(200, "POST", "/api/v1/answers", a.Token, map[string]string{"kind": "code", "value": "0"}).json(t, &ans)
		return ans
	}
	if ans := wrong(); ans.Result != "wrong" || ans.LevelID != "l1" || ans.RateLimit != nil {
		t.Fatalf("1-я ошибка: %+v", ans)
	}
	ans := wrong()
	if ans.RateLimit == nil || ans.RateLimit.Action != "block" || ans.RateLimit.Until == nil ||
		ans.RateLimit.Until.Sub(time.Now()) < 2*time.Minute {
		t.Fatalf("блокировка: %+v", ans.RateLimit)
	}
	ts.must(429, "POST", "/api/v1/answers", a.Token, map[string]string{"kind": "code", "value": "1"})
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if st.Levels[0].RateLimit.BlockedUntil == nil {
		t.Fatal("в состоянии нет blocked_until")
	}

	// ведущий засчитывает уровень за команду — блок на l1 больше не мешает l2
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/teams/"+a.TeamID+"/answers", adminToken, map[string]string{"kind": "code", "value": "1"})
	wrong()
	ans = wrong()
	if ans.RateLimit == nil || ans.RateLimit.Action != "penalty" || ans.RateLimit.PenaltySec != 300 {
		t.Fatalf("штраф: %+v", ans.RateLimit)
	}
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	last := st.Score[len(st.Score)-1]
	if st.PenaltySec != 300 || last.Title != "Перебор кодов — «Сейф»" || last.TimeSec != 300 {
		t.Fatalf("штраф в расшифровке: penalty=%d %+v", st.PenaltySec, last)
	}
}
