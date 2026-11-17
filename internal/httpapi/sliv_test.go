package httpapi

import "testing"

const slivYAML = `
version: 1
id: sliv
title: Сливы
scoring: {mode: time}
levels:
  - id: l1
    title: Парк
    task: Найдите точку
    answers: [{type: geo, point: [44.4875, 39.2321], radius_m: 30}]
    location: {text: "Вход со стороны реки", penalty: {time: 10m}}
    skip: {penalty: {time: 30m}}
  - id: l2
    title: Финиш
    task: Код
    answers: [{type: code, values: ["1"]}]
`

func TestSlivAPI(t *testing.T) {
	ts := newTestServer(t)
	// в зачёте по времени штраф баллами — ошибка
	ts.must(422, "POST", "/admin/v1/scenarios", adminToken, slivYAML[:len(slivYAML)-1]+"\n    skip: {penalty: {points: 3}}\n")
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, slivYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	var j struct {
		Token string `json:"token"`
	}
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/join", "", map[string]string{"invite_code": a.InviteCode, "name": "Петя"}).json(t, &j)
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/start", adminToken, nil)

	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	l := st.Levels[0]
	if l.Location == nil || l.Location.Revealed || l.Location.Text != "" || l.Location.Lat != nil || l.Location.PenaltySec != 600 ||
		l.Skip == nil || l.Skip.PenaltySec != 1800 {
		t.Fatalf("до слива: location=%+v skip=%+v", l.Location, l.Skip)
	}

	ts.must(403, "POST", "/api/v1/levels/l1/location", j.Token, nil) // не капитан
	ts.must(204, "POST", "/api/v1/levels/l1/location", a.Token, nil)
	ts.must(409, "POST", "/api/v1/levels/l1/location", a.Token, nil)
	ts.must(200, "GET", "/api/v1/state", j.Token, nil).json(t, &st)
	loc := st.Levels[0].Location
	if !loc.Revealed || loc.Text != "Вход со стороны реки" || loc.Lat == nil || *loc.Lat != 44.4875 || st.PenaltySec != 600 {
		t.Fatalf("после слива локации: %+v, штраф %d", loc, st.PenaltySec)
	}

	ts.must(403, "POST", "/api/v1/levels/l1/skip", j.Token, nil)
	ts.must(204, "POST", "/api/v1/levels/l1/skip", a.Token, nil)
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if st.LevelsDone != 1 || st.PenaltySec != 2400 || len(st.Score) != 2 || st.Score[1].Title != "Слив уровня — «Парк»" {
		t.Fatalf("после слива уровня: done=%d penalty=%d score=%+v", st.LevelsDone, st.PenaltySec, st.Score)
	}
	ts.must(404, "POST", "/api/v1/levels/l2/skip", a.Token, nil)

	var dash dashboardDTO
	ts.must(200, "GET", "/admin/v1/runs/"+run.ID+"/dashboard", adminToken, nil).json(t, &dash)
	if lv := dash.Teams[0].Levels[0]; !lv.Skipped || !lv.LocationOut {
		t.Fatalf("дашборд: %+v", lv)
	}
}
