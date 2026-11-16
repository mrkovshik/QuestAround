package httpapi

import (
	"strings"
	"testing"
)

func TestValidateScenarioDoesNotSave(t *testing.T) {
	ts := newTestServer(t)

	var v validateDTO
	ts.must(200, "POST", "/admin/v1/scenarios/validate", adminToken, e2eYAML).json(t, &v)
	if !v.OK || v.ID != "e2e" || v.Levels != 3 || v.NextVersion != 1 {
		t.Fatalf("validate: %+v", v)
	}

	var list []scenarioVersionDTO
	ts.must(200, "GET", "/admin/v1/scenarios", adminToken, nil).json(t, &list)
	if len(list) != 0 {
		t.Fatalf("проверка не должна сохранять: %d версий", len(list))
	}

	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML)
	ts.must(200, "POST", "/admin/v1/scenarios/validate", adminToken, e2eYAML).json(t, &v)
	if v.NextVersion != 2 {
		t.Fatalf("после сохранения следующая версия 2, получили %d", v.NextVersion)
	}
}

func TestValidateScenarioErrorsHavePaths(t *testing.T) {
	ts := newTestServer(t)
	bad := strings.Replace(e2eYAML, `values: ["1864"]`, `values: []`, 1)
	r := ts.must(422, "POST", "/admin/v1/scenarios/validate", adminToken, bad)
	var e errorBody
	r.json(t, &e)
	if e.Error != "invalid_scenario" || len(e.Details) == 0 {
		t.Fatalf("ошибка: %+v", e)
	}
	if e.Details[0].Path != "levels[0].answers[0].values" {
		t.Fatalf("путь: %q", e.Details[0].Path)
	}

	// синтаксис JSON тоже понимаем — YAML его надмножество
	r = ts.must(422, "POST", "/admin/v1/scenarios/validate", adminToken, `{"version": 1, "id": "x", "title": "x", "levels": []}`)
	r.json(t, &e)
	if e.Details[0].Path != "levels" {
		t.Fatalf("путь: %q", e.Details[0].Path)
	}
}

const routesYAML = `
version: 1
id: rt
title: Маршруты
flow: {mode: linear, routes: rotate}
levels:
  - {id: s, pin: first, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: a, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: b, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: c, task: t, answers: [{type: code, values: ["1"]}]}
  - {id: f, pin: last, task: t, answers: [{type: code, values: ["1"]}]}
`

func TestPreviewRoutes(t *testing.T) {
	ts := newTestServer(t)
	var rp routesPreviewDTO
	ts.must(200, "POST", "/admin/v1/scenarios/routes?teams=3", adminToken, routesYAML).json(t, &rp)
	want := []string{"s,a,b,c,f", "s,b,c,a,f", "s,c,a,b,f"}
	if rp.Mode != "rotate" || len(rp.Routes) != 3 {
		t.Fatalf("превью: %+v", rp)
	}
	for i, r := range rp.Routes {
		if got := strings.Join(r, ","); got != want[i] {
			t.Errorf("команда %d: %s, ожидали %s", i+1, got, want[i])
		}
	}

	ts.must(400, "POST", "/admin/v1/scenarios/routes?teams=0", adminToken, routesYAML)
	ts.must(422, "POST", "/admin/v1/scenarios/routes?teams=2", adminToken, "version: 1\n")

	storm := strings.Replace(routesYAML, "flow: {mode: linear, routes: rotate}", "flow: {mode: storm}", 1)
	storm = strings.ReplaceAll(storm, "pin: first, ", "")
	storm = strings.ReplaceAll(storm, "pin: last, ", "")
	ts.must(200, "POST", "/admin/v1/scenarios/routes?teams=2", adminToken, storm).json(t, &rp)
	if rp.Mode != "storm" || rp.Routes != nil {
		t.Fatalf("storm: %+v", rp)
	}
}
