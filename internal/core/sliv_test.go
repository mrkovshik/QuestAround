package core

import (
	"testing"
	"time"
)

const slivYAML = `
version: 1
id: s
title: s
scoring: {mode: mixed}
levels:
  - id: a
    title: Парк
    task: t
    points: 10
    answers: [{type: geo, point: [44.1, 39.2], radius_m: 30}]
    location: {text: "Вход в парк со стороны реки", penalty: {time: 10m}}
    skip: {penalty: {points: 3}}
  - id: b
    task: t
    points: 10
    answers: [{type: code, values: ["2"]}]
`

func TestSliv(t *testing.T) {
	h := newHarness(t, slivYAML, "x")
	h.do(StartRun{})

	// не капитан — нельзя
	h.fail(RevealLocation{TeamID: "x", PlayerID: "someone", LevelID: "a"}, ErrNotAllowed)
	h.fail(SkipLevel{TeamID: "x", LevelID: "a"}, ErrNotAllowed)
	// у уровня b сливов нет, и он ещё закрыт
	h.fail(RevealLocation{TeamID: "x", IsCaptain: true, LevelID: "b"}, ErrLevelNotOpen)

	h.do(RevealLocation{TeamID: "x", IsCaptain: true, LevelID: "a"})
	h.fail(RevealLocation{TeamID: "x", IsCaptain: true, LevelID: "a"}, ErrLocationRevealed)
	tm := h.team("x")
	h.eq("локация", tm.Levels["a"].LocationRevealed, true)
	h.eq("штраф временем", tm.PenaltyTime, 10*time.Minute)

	h.do(SkipLevel{TeamID: "x", IsCaptain: true, LevelID: "a"})
	h.eq("уровень пройден", [3]any{tm.LevelsDone, tm.Levels["a"].Skipped, !tm.Levels["a"].CompletedAt.IsZero()}, [3]any{1, true, true})
	h.eq("баллы: за слитый уровень 0, штраф −3", tm.Points, -3)
	h.eq("открыт следующий", tm.Levels["b"].IsOpen(), true)
	h.fail(SkipLevel{TeamID: "x", IsCaptain: true, LevelID: "b"}, ErrSkipNotAvailable)
	h.fail(RevealLocation{TeamID: "x", IsCaptain: true, LevelID: "b"}, ErrNoLocation)

	var kinds []string
	for _, e := range tm.Ledger {
		kinds = append(kinds, e.Kind)
	}
	h.eq("расшифровка", kinds, []string{"location", "skip"})
}
