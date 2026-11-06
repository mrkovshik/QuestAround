package scenario

import (
	"errors"
	"strings"
	"testing"
	"time"

	"questaround/internal/routes"
)

func TestParsePilot(t *testing.T) {
	s, err := ParseFile("testdata/pilot.yaml")
	if err != nil {
		t.Fatalf("pilot.yaml должен быть валидным:\n%v", err)
	}
	if s.ID != "gk-pilot-01" || len(s.Levels) != 6 {
		t.Fatalf("id=%q уровней=%d", s.ID, len(s.Levels))
	}
	if s.Timing.TimeLimit.D() != 3*time.Hour {
		t.Errorf("time_limit = %s", s.Timing.TimeLimit)
	}
	if s.Start.Mode != StartStaggered || s.Start.Interval.D() != 5*time.Minute {
		t.Errorf("start = %+v", s.Start)
	}
	if s.Flow.Routes != routes.ModeRotate {
		t.Errorf("routes = %q", s.Flow.Routes)
	}

	// Автоподсказка без after получила время из defaults.hint_after.
	start, _ := s.Level("start")
	if got := start.Hints[0].After.D(); got != 15*time.Minute {
		t.Errorf("start.hints[0].after = %s, ожидали 15m", got)
	}
	// Явный after не перезаписывается.
	r2, _ := s.Level("riddle-2")
	if got := r2.Hints[0].After.D(); got != 20*time.Minute {
		t.Errorf("riddle-2.hints[0].after = %s, ожидали 20m", got)
	}
	if r2.Require.Needed(len(r2.Answers)) != 2 {
		t.Errorf("riddle-2: require all → нужно 2 ответа")
	}

	// Маршруты строятся из сценария: цепочка riddle-1 → riddle-2 не разрывается.
	rs, err := routes.Build(s.RouteLevels(), routes.Options{Mode: s.Flow.Routes, Teams: 4})
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rs {
		if r[0] != "start" || r[len(r)-1] != "final" {
			t.Errorf("команда %d: %v", i, r)
		}
	}
}

func TestDefaults(t *testing.T) {
	s, err := Parse([]byte(`
version: 1
id: tiny
title: t
levels:
  - id: a
    task: x
    answers: [{type: code, values: ["1"]}]
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Participants.Mode != ParticipantsTeam || s.Participants.TeamSize != (TeamSize{1, 5}) {
		t.Errorf("participants = %+v", s.Participants)
	}
	if s.Participants.HintRequests != HintRequestsCaptain {
		t.Errorf("hint_requests = %q", s.Participants.HintRequests)
	}
	if s.Flow.Mode != FlowLinear || s.Flow.Routes != routes.ModeSame {
		t.Errorf("flow = %+v", s.Flow)
	}
	if s.Start.Mode != StartSimultaneous || s.Scoring.Mode != ScoringPoints {
		t.Errorf("start=%q scoring=%q", s.Start.Mode, s.Scoring.Mode)
	}
	if s.Leaderboard.Visibility != VisibilityAlways {
		t.Errorf("visibility = %q", s.Leaderboard.Visibility)
	}
	if strings.Join(s.Defaults.AnswerNormalize, ",") != "trim,lower,yo_to_e" {
		t.Errorf("normalize = %v", s.Defaults.AnswerNormalize)
	}
}

func TestSoloDefaults(t *testing.T) {
	s, err := Parse([]byte(`
version: 1
id: solo
title: t
participants: {mode: solo}
levels:
  - {id: a, task: x, answers: [{type: photo}]}
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Participants.TeamSize != (TeamSize{1, 1}) {
		t.Errorf("team_size = %+v", s.Participants.TeamSize)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		ops  []string
		want string
	}{
		{"  Ёлка ", []string{NormTrim, NormLower, NormYoToE}, "елка"},
		{"ЁЖ", []string{NormYoToE}, "ЕЖ"},
		{"a   b \t c", []string{NormCollapseSpaces}, "a b c"},
		{" X ", nil, " X "},
	}
	for _, c := range cases {
		if got := Normalize(c.in, c.ops); got != c.want {
			t.Errorf("Normalize(%q, %v) = %q, want %q", c.in, c.ops, got, c.want)
		}
	}
}

// head — валидная шапка сценария; уровни дописываются в тестах.
const head = `version: 1
id: t
title: t
`

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string // подстрока ошибки
		line int    // ожидаемая строка (0 — не проверять)
	}{
		{
			name: "нет уровней",
			yaml: head,
			want: "нет ни одного уровня",
		},
		{
			name: "версия схемы",
			yaml: "version: 2\nid: t\ntitle: t\nlevels: [{id: a, task: x, answers: [{type: photo}]}]\n",
			want: "version: 1",
			line: 1,
		},
		{
			name: "дубль id уровня",
			yaml: head + `levels:
  - {id: a, task: x, answers: [{type: photo}]}
  - {id: a, task: y, answers: [{type: photo}]}
`,
			want: `id "a" уже используется`,
			line: 6,
		},
		{
			name: "плохой id",
			yaml: head + `levels:
  - {id: "Мост", task: x, answers: [{type: photo}]}
`,
			want: "только строчные латинские",
		},
		{
			name: "нет ответов",
			yaml: head + `levels:
  - id: a
    task: x
`,
			want: "нет ни одного ответа",
			line: 5,
		},
		{
			name: "битый regex",
			yaml: head + `levels:
  - id: a
    task: x
    answers:
      - type: regex
        pattern: "(["
`,
			want: "некорректное регулярное выражение",
			line: 9,
		},
		{
			name: "geo без радиуса и с кривой широтой",
			yaml: head + `levels:
  - id: a
    task: x
    answers:
      - type: geo
        point: [95, 39.1]
`,
			want: "широта 95",
		},
		{
			name: "choice: правильного ответа нет в options",
			yaml: head + `levels:
  - id: a
    task: x
    answers:
      - {type: choice, options: ["a", "b"], correct: "c"}
`,
			want: "не найден среди options",
		},
		{
			name: "лишнее поле в ответе",
			yaml: head + `levels:
  - id: a
    task: x
    answers:
      - {type: code, values: ["1"], radius_m: 10}
`,
			want: "radius_m не используется",
		},
		{
			name: "require больше числа ответов",
			yaml: head + `levels:
  - id: a
    task: x
    require: 3
    answers: [{type: code, values: ["1"]}]
`,
			want: "require: 3",
			line: 7,
		},
		{
			name: "автоподсказка без расписания",
			yaml: head + `levels:
  - id: a
    task: x
    answers: [{type: photo}]
    hints:
      - text: "подсказка"
`,
			want: "defaults.hint_after",
			line: 9,
		},
		{
			name: "штраф у автоподсказки",
			yaml: head + `levels:
  - id: a
    task: x
    answers: [{type: photo}]
    hints:
      - {after: 5m, text: h, penalty: {time: 1m}}
`,
			want: "только у подсказок on_request",
		},
		{
			name: "штраф и временем, и баллами",
			yaml: head + `levels:
  - id: a
    task: x
    answers: [{type: photo}]
    hints:
      - {on_request: true, text: h, penalty: {time: 1m, points: 3}}
`,
			want: "ровно одно: time или points",
		},
		{
			name: "два pin: first",
			yaml: head + `levels:
  - {id: a, pin: first, task: x, answers: [{type: photo}]}
  - {id: b, pin: first, task: x, answers: [{type: photo}]}
`,
			want: "pin: first может быть только у одного",
		},
		{
			name: "pin внутри цепочки",
			yaml: head + `levels:
  - {id: a, pin: first, chain: c, task: x, answers: [{type: photo}]}
`,
			want: "не может входить в цепочку",
		},
		{
			name: "разорванная цепочка",
			yaml: head + `levels:
  - {id: a, chain: c, task: x, answers: [{type: photo}]}
  - {id: b, task: x, answers: [{type: photo}]}
  - {id: c, chain: c, task: x, answers: [{type: photo}]}
`,
			want: `цепочки "c" должны быть объявлены подряд`,
			line: 7,
		},
		{
			name: "staggered без интервала",
			yaml: head + `start: {mode: staggered}
levels: [{id: a, task: x, answers: [{type: photo}]}]
`,
			want: "нужен interval",
		},
		{
			name: "заморозка без лимита",
			yaml: head + `leaderboard: {freeze_before_end: 10m}
levels: [{id: a, task: x, answers: [{type: photo}]}]
`,
			want: "требует timing.time_limit",
		},
		{
			name: "заморозка дольше лимита",
			yaml: head + `timing: {time_limit: 1h}
leaderboard: {freeze_before_end: 2h}
levels: [{id: a, task: x, answers: [{type: photo}]}]
`,
			want: "меньше time_limit",
		},
		{
			name: "sync пока не поддерживается",
			yaml: head + `timing: {mode: sync}
levels: [{id: a, task: x, answers: [{type: photo}]}]
`,
			want: "пока не поддерживается",
		},
		{
			name: "routes не для linear",
			yaml: head + `flow: {mode: storm, routes: rotate}
levels: [{id: a, task: x, answers: [{type: photo}]}]
`,
			want: "только для flow: linear",
		},
		{
			name: "explicit: маршрут без уровня",
			yaml: head + `flow:
  mode: linear
  routes: explicit
  route_list:
    - [a]
levels:
  - {id: a, task: x, answers: [{type: photo}]}
  - {id: b, task: x, answers: [{type: photo}]}
`,
			want: `нет уровня "b"`,
			line: 8,
		},
		{
			name: "unlock вне graph",
			yaml: head + `levels:
  - {id: a, task: x, answers: [{type: photo}]}
  - {id: b, task: x, answers: [{type: photo}], unlock: {all_of: [a]}}
`,
			want: "только во flow: graph",
		},
		{
			name: "graph: цикл",
			yaml: head + `flow: {mode: graph}
levels:
  - {id: s, task: x, answers: [{type: photo}]}
  - {id: a, task: x, answers: [{type: photo}], unlock: {all_of: [b]}}
  - {id: b, task: x, answers: [{type: photo}], unlock: {any_of: [a]}}
`,
			want: "циклическая зависимость",
		},
		{
			name: "graph: нет стартового уровня",
			yaml: head + `flow: {mode: graph}
levels:
  - {id: a, task: x, answers: [{type: photo}], unlock: {all_of: [b]}}
  - {id: b, task: x, answers: [{type: photo}], unlock: {all_of: [a]}}
`,
			want: "без unlock",
		},
		{
			name: "graph: несуществующая ссылка",
			yaml: head + `flow: {mode: graph}
levels:
  - {id: a, task: x, answers: [{type: photo}]}
  - {id: b, task: x, answers: [{type: photo}], unlock: {count: 2, of: [a, zzz]}}
`,
			want: `несуществующий уровень "zzz"`,
		},
		{
			name: "graph: две формы unlock сразу",
			yaml: head + `flow: {mode: graph}
levels:
  - {id: a, task: x, answers: [{type: photo}]}
  - {id: b, task: x, answers: [{type: photo}], unlock: {all_of: [a], any_of: [a]}}
`,
			want: "ровно одно: all_of, any_of или count+of",
		},
		{
			name: "дубль id бонуса",
			yaml: head + `levels:
  - id: a
    task: x
    answers: [{type: photo}]
    bonuses:
      - {id: p, answer: {type: photo}}
  - id: b
    task: x
    answers: [{type: photo}]
    bonuses:
      - {id: p, answer: {type: photo}}
`,
			want: `id бонуса "p" уже используется`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			if err == nil {
				t.Fatalf("ожидали ошибку %q", c.want)
			}
			var errs Errors
			if !errors.As(err, &errs) {
				t.Fatalf("ожидали Errors, получили %T: %v", err, err)
			}
			for _, e := range errs {
				if strings.Contains(e.Msg, c.want) {
					if c.line != 0 && e.Line != c.line {
						t.Errorf("ошибка %q на строке %d, ожидали %d", e.Msg, e.Line, c.line)
					}
					return
				}
			}
			t.Errorf("нет ошибки с %q, получили:\n%v", c.want, err)
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"опечатка в ключе", head + "levles: []\n", "levles"},
		{"плохая длительность", head + "timing: {time_limit: 3 часа}\n", "некорректная длительность"},
		{"плохой require", head + "levels: [{id: a, task: x, require: most, answers: [{type: photo}]}]\n", "require"},
		{"синтаксис", "version: [1\n", "синтаксис YAML"},
		{"пустой", "", "пустой сценарий"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("ожидали ошибку с %q, получили %v", c.want, err)
			}
		})
	}
}

func TestAllErrorsReportedAtOnce(t *testing.T) {
	_, err := Parse([]byte(head + `levels:
  - {id: a, task: "", answers: []}
  - {id: a, task: x, answers: [{type: telepathy}]}
`))
	var errs Errors
	if !errors.As(err, &errs) || len(errs) < 3 {
		t.Fatalf("ожидали ≥3 ошибок за один проход, получили: %v", err)
	}
}
