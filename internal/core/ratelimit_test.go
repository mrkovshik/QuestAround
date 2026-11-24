package core

import (
	"testing"
	"time"
)

const levelLimitYAML = `
version: 1
id: rl
title: rl
scoring: {mode: time}
defaults:
  rate_limit: {wrong_answers: 100, per: 1m}
levels:
  - id: lock
    task: t
    answers: [{type: code, values: ["1"]}]
    rate_limit: {wrong_answers: 3, per: 1m, block_for: 5m}
  - id: safe
    task: t
    answers: [{type: code, values: ["2"]}]
    rate_limit: {wrong_answers: 2, per: 30s, action: penalty, penalty: {time: 10m}}
  - id: free
    task: t
    answers: [{type: code, values: ["3"]}]
`

func TestLevelRateLimitBlock(t *testing.T) {
	h := newHarness(t, levelLimitYAML, "a")
	h.do(StartRun{})

	wrong := func() *RateLimitHit {
		out := h.do(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "0"})
		h.eq("неверно", out.Answer.Result, ResultWrong)
		h.eq("ошибка засчитана уровню", out.Answer.LevelID, "lock")
		return out.Answer.RateLimit
	}
	h.eq("1-я", wrong() == nil, true)
	h.advance(70 * time.Second) // первая ошибка вышла из окна
	h.eq("2-я", wrong() == nil, true)
	h.eq("3-я", wrong() == nil, true)
	hit := wrong()
	if hit == nil || hit.Action != "block" || !hit.Until.Equal(h.now.Add(5*time.Minute)) {
		t.Fatalf("блокировка: %+v", hit)
	}
	h.fail(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "1"}, ErrRateLimited)
	// ведущий вводит за команду — блокировка не мешает
	h.eq("ведущий", h.do(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "0", ByHost: true}).Answer.RateLimit == nil, true)
	h.advance(5 * time.Minute)
	h.eq("после блокировки", h.code("a", "1"), ResultCorrect)
	h.eq("штрафа нет", h.team("a").PenaltyTime, time.Duration(0))
}

func TestLevelRateLimitPenalty(t *testing.T) {
	h := newHarness(t, levelLimitYAML, "a")
	h.do(StartRun{})
	h.do(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "1", ByHost: true}) // lock пройден

	h.code("a", "0")
	hit := h.do(SubmitAnswer{TeamID: "a", Kind: KindCode, Value: "0"}).Answer.RateLimit
	if hit == nil || hit.Action != "penalty" || hit.PenaltyTime != 10*time.Minute {
		t.Fatalf("штраф: %+v", hit)
	}
	h.eq("ввод открыт", h.code("a", "0"), ResultWrong)
	h.eq("снова считаем с нуля", h.team("a").Levels["safe"].Wrong != nil && len(h.team("a").Levels["safe"].Wrong) == 1, true)
	h.code("a", "0")
	h.eq("второй штраф", h.team("a").PenaltyTime, 20*time.Minute)
	last := h.team("a").Ledger[len(h.team("a").Ledger)-1]
	h.eq("расшифровка", last.Kind, "rate_limit")

	// уровень без своей защиты — действует общая (здесь с большим порогом)
	h.code("a", "2")
	for range 5 {
		h.code("a", "0")
	}
	h.eq("free без своей защиты", h.code("a", "3"), ResultCorrect)
}
