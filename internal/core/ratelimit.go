package core

import (
	"time"

	"questaround/internal/scenario"
)

// Защита от перебора кодов на уровне (rate_limit у уровня в сценарии).
//
// Неверный код засчитывается уровню, если он отправлен на этот уровень или это
// единственный открытый уровень команды (линейная игра). В штурме с общим полем
// ввода ошибки не привязаны к уровню — там действует только defaults.rate_limit.

const EvRateLimitTriggered EventType = "rate_limit_triggered"

// hostPlayerID — автор ответов, введённых ведущим за команду.
const hostPlayerID = "host"

// RateLimitTriggered — сработала защита уровня: блокировка ввода или штраф.
type RateLimitTriggered struct {
	LevelID       string                   `json:"level_id"`
	Action        scenario.RateLimitAction `json:"action"`
	Until         time.Time                `json:"until,omitzero"` // для block
	PenaltyTime   time.Duration            `json:"penalty_time,omitempty"`
	PenaltyPoints int                      `json:"penalty_points,omitempty"`
}

func (RateLimitTriggered) Type() EventType { return EvRateLimitTriggered }

func init() {
	payloadFactory[EvRateLimitTriggered] = func() Payload { return &RateLimitTriggered{} }
}

// RateLimitHit — что произошло с ответом из-за защиты (для сообщения игроку).
type RateLimitHit struct {
	Action        scenario.RateLimitAction
	Until         time.Time
	PenaltyTime   time.Duration
	PenaltyPoints int
}

func (g *Game) applyRateLimit(e Event) bool {
	p, ok := e.Payload.(RateLimitTriggered)
	if !ok {
		return false
	}
	t := g.Teams[e.TeamID]
	ls := t.Levels[p.LevelID]
	if ls != nil {
		ls.Wrong = nil
		if p.Action == scenario.RateLimitBlock {
			ls.BlockedUntil = p.Until
		}
	}
	if p.Action == scenario.RateLimitPenalty {
		g.applyPenalty(t, e, "rate_limit", p.LevelID, p.PenaltyTime, p.PenaltyPoints)
	}
	return true
}

// countLevelWrong — неверный код на уровне со своей защитой: в окно per.
func (g *Game) countLevelWrong(team *Team, levelID string, at time.Time) {
	rl := g.levelLimit(levelID)
	ls := team.Levels[levelID]
	if rl == nil || !ls.IsOpen() {
		return
	}
	from := at.Add(-rl.Per.D())
	keep := ls.Wrong[:0]
	for _, w := range ls.Wrong {
		if w.After(from) {
			keep = append(keep, w)
		}
	}
	ls.Wrong = append(keep, at)
}

// codeLevel — уровень, которому засчитывается код: указанный или единственный открытый.
func (g *Game) codeLevel(team *Team, levels []string) string {
	open := ""
	for _, id := range levels {
		if team.Levels[id].IsOpen() {
			if open != "" {
				return ""
			}
			open = id
		}
	}
	return open
}

// levelLimit — защита уровня, если у него своя.
func (g *Game) levelLimit(levelID string) *scenario.RateLimit {
	if levelID == "" {
		return nil
	}
	sl, ok := g.level(levelID)
	if !ok {
		return nil
	}
	return sl.RateLimit
}

// codeBlocked — принимать ли код: своя блокировка уровня или общая защита.
func (g *Game) codeBlocked(team *Team, levelID string, now time.Time) bool {
	if rl := g.levelLimit(levelID); rl != nil {
		return now.Before(team.Levels[levelID].BlockedUntil)
	}
	return g.rateLimited(team, now)
}

// checkLevelLimit — после неверного кода: не набралось ли ошибок на срабатывание.
func (g *Game) checkLevelLimit(t *tx, team *Team, levelID string) *RateLimitHit {
	rl := g.levelLimit(levelID)
	ls := team.Levels[levelID]
	if rl == nil || !ls.IsOpen() {
		return nil
	}
	from := t.now.Add(-rl.Per.D())
	n := 0
	for _, at := range ls.Wrong {
		if at.After(from) {
			n++
		}
	}
	if n < rl.WrongAnswers {
		return nil
	}
	p := RateLimitTriggered{LevelID: levelID, Action: rl.Action}
	if rl.Action == scenario.RateLimitBlock {
		p.Until = t.now.Add(rl.BlockDuration())
	} else {
		p.PenaltyTime, p.PenaltyPoints = penaltyOf(rl.Penalty)
	}
	t.emit(team.ID, "", "", p)
	return &RateLimitHit{Action: p.Action, Until: p.Until, PenaltyTime: p.PenaltyTime, PenaltyPoints: p.PenaltyPoints}
}
