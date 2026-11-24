package httpapi

import (
	"strings"
	"testing"
)

const hostNotesYAML = `
version: 1
id: notes
title: Заметки
description: Видят игроки
host_notes: |
  СЕКРЕТ-ОБЩИЙ: стартуем от фонтана
levels:
  - id: l1
    title: Парк
    host_notes: СЕКРЕТ-УРОВНЯ конверт под скамейкой
    task: t
    answers: [{type: code, values: ["1"]}]
  - id: l2
    title: Мост
    task: t
    answers: [{type: code, values: ["2"]}]
`

// Заметки организаторам есть в админке и не уходят игрокам.
func TestHostNotesHiddenFromPlayers(t *testing.T) {
	ts := newTestServer(t)
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, hostNotesYAML).json(t, &sv)

	var got scenarioVersionDTO
	ts.must(200, "GET", "/admin/v1/scenarios/"+sv.ID, adminToken, nil).json(t, &got)
	if !strings.Contains(got.HostNotes, "СЕКРЕТ-ОБЩИЙ") || len(got.LevelNotes) != 1 ||
		got.LevelNotes[0].LevelID != "l1" || got.LevelNotes[0].Title != "Парк" {
		t.Fatalf("админка: %+v", got)
	}

	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	info := ts.must(200, "GET", "/api/v1/games/"+run.PublicCode, "", nil)
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	ts.must(200, "POST", "/admin/v1/runs/"+run.ID+"/start", adminToken, nil)
	st := ts.must(200, "GET", "/api/v1/state", a.Token, nil)
	for name, body := range map[string]string{"инфо об игре": string(info.body), "состояние": string(st.body)} {
		if strings.Contains(body, "СЕКРЕТ") {
			t.Errorf("%s: заметки организаторам ушли игроку: %s", name, body)
		}
	}
}
