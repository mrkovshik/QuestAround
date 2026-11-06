package scenario

import (
	"errors"
	"strings"
	"testing"
)

const compatBase = `
version: 1
id: c
title: c
defaults: {hint_after: [10m]}
levels:
  - id: a
    title: Старт
    task: Найдите код
    points: 10
    answers: [{type: code, values: ["1"]}]
    hints:
      - text: подсказка
      - {on_request: true, penalty: {time: 5m}, text: платная}
    bonuses:
      - {id: b1, answer: {type: photo}, points: 5}
  - id: g
    task: Встаньте на точку
    answers:
      - {type: geo, point: [44.6, 39.1], radius_m: 50}
`

func mustParse(t *testing.T, src string) *Scenario {
	t.Helper()
	s, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("сценарий: %v", err)
	}
	return s
}

func TestCompatibleContentChanges(t *testing.T) {
	old := mustParse(t, compatBase)
	changed := strings.NewReplacer(
		"Найдите код", "Найдите код на табличке",
		`values: ["1"]`, `values: ["1", "один"]`,
		"points: 10", "points: 20",
		"[44.6, 39.1], radius_m: 50", "[44.63, 39.12], radius_m: 150, max_accuracy_m: 80",
		"text: подсказка", "text: новая подсказка",
		"penalty: {time: 5m}", "penalty: {points: 3}",
		"title: Старт", "title: Сбор",
	).Replace(compatBase)
	if err := CheckCompatible(old, mustParse(t, changed)); err != nil {
		t.Fatalf("правка содержимого должна быть совместима: %v", err)
	}
}

func TestIncompatibleStructureChanges(t *testing.T) {
	old := mustParse(t, compatBase)
	cases := map[string]struct{ from, to, want string }{
		"новый уровень":           {"  - id: g\n", "  - id: x\n    task: t\n    answers: [{type: photo}]\n  - id: g\n", "число уровней"},
		"тип ответа":              {`[{type: code, values: ["1"]}]`, `[{type: photo}]`, "тип code → photo"},
		"лишний ответ":            {`[{type: code, values: ["1"]}]`, `[{type: code, values: ["1"]}, {type: photo}]`, "число ответов"},
		"убрали подсказку":        {"      - text: подсказка\n", "", "число подсказок"},
		"подсказка стала платной": {"      - text: подсказка\n", "      - {on_request: true, text: подсказка}\n", "по таймеру ↔ по запросу"},
		"переименовали бонус":     {"id: b1", "id: b2", `бонус "b1"`},
		"переименовали уровень":   {"  - id: g\n", "  - id: gg\n", `пропал уровень "g"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := CheckCompatible(old, mustParse(t, strings.Replace(compatBase, c.from, c.to, 1)))
			var inc *Incompatible
			if !errors.As(err, &inc) {
				t.Fatalf("ожидали Incompatible, получили %v", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("нет %q в %q", c.want, err)
			}
		})
	}
}
