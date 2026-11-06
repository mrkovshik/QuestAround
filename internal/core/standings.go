package core

import (
	"cmp"
	"slices"
	"time"

	"questaround/internal/scenario"
)

// Standing — строка лидерборда.
type Standing struct {
	Rank     int           `json:"rank"`
	TeamID   string        `json:"team_id"`
	Name     string        `json:"name"`
	Status   TeamStatus    `json:"status"`
	Points   int           `json:"points"`
	Levels   int           `json:"levels"`
	Elapsed  time.Duration `json:"elapsed"` // чистое время + штрафы
	Finished bool          `json:"finished"`
}

// Elapsed — время команды на момент now: от её старта до финиша (или now),
// не больше лимита, плюс штрафное время.
func (g *Game) Elapsed(team *Team, now time.Time) time.Duration {
	if team.StartedAt.IsZero() {
		return 0
	}
	end := now
	if !team.FinishedAt.IsZero() {
		end = team.FinishedAt
	}
	d := end.Sub(team.StartedAt)
	if limit := g.Scenario.Timing.TimeLimit.D(); limit > 0 && d > limit {
		d = limit
	}
	if d < 0 {
		d = 0
	}
	return d + team.PenaltyTime
}

// Standings — живая таблица на момент now.
//
//	time:   больше уровней ↓, меньше время ↑
//	points: больше баллов ↓, больше уровней ↓
//	mixed:  больше баллов ↓, меньше время ↑
//
// Нестартовавшие команды — внизу. Равные по ключу делят место (1, 1, 3).
func (g *Game) Standings(now time.Time) []Standing {
	out := make([]Standing, 0, len(g.TeamOrder))
	for _, t := range g.ActiveTeams() {
		out = append(out, Standing{
			TeamID:   t.ID,
			Name:     t.Name,
			Status:   t.Status,
			Points:   t.Points,
			Levels:   t.LevelsDone,
			Elapsed:  g.Elapsed(t, now),
			Finished: t.Status == TeamStatusFinished,
		})
	}

	mode := g.Scenario.Scoring.Mode
	key := func(s Standing) int {
		started := 0
		if s.Status == TeamStatusRegistered || s.Status == TeamStatusScheduled {
			started = 1
		}
		return started
	}
	compare := func(a, b Standing) int {
		if c := cmp.Compare(key(a), key(b)); c != 0 {
			return c
		}
		switch mode {
		case scenario.ScoringTime:
			if c := cmp.Compare(b.Levels, a.Levels); c != 0 {
				return c
			}
			return cmp.Compare(a.Elapsed, b.Elapsed)
		case scenario.ScoringMixed:
			if c := cmp.Compare(b.Points, a.Points); c != 0 {
				return c
			}
			return cmp.Compare(a.Elapsed, b.Elapsed)
		default: // points
			if c := cmp.Compare(b.Points, a.Points); c != 0 {
				return c
			}
			return cmp.Compare(b.Levels, a.Levels)
		}
	}
	slices.SortStableFunc(out, compare)
	for i := range out {
		if i > 0 && compare(out[i-1], out[i]) == 0 {
			out[i].Rank = out[i-1].Rank
		} else {
			out[i].Rank = i + 1
		}
	}
	return out
}

// LeaderboardVisible — видит ли команда лидерборд сейчас.
// Утверждённые итоги видны всем; до утверждения — по правилам сценария
// и переключателю ведущего (в том числе после завершения игры).
func (g *Game) LeaderboardVisible(teamID string) bool {
	if g.Results != nil {
		return true
	}
	if o := g.Leaderboard.Override; o != nil {
		return *o
	}
	switch g.Scenario.Leaderboard.Visibility {
	case scenario.VisibilityAlways:
		return true
	case scenario.VisibilityAfterFinish:
		t := g.Teams[teamID]
		return t != nil && t.Status == TeamStatusFinished
	default: // hidden, host
		return false
	}
}

// LeaderboardFor — таблица для команды: (nil, false), если скрыта;
// утверждённые итоги, если они есть; снимок на момент заморозки,
// если заморожена и игра не закончена.
func (g *Game) LeaderboardFor(teamID string, now time.Time) ([]Standing, bool) {
	if !g.LeaderboardVisible(teamID) {
		return nil, false
	}
	if g.Results != nil {
		return g.Results.Standings, true
	}
	if g.Leaderboard.Frozen && g.Status != RunStatusFinished {
		return g.Leaderboard.Snapshot, true
	}
	return g.Standings(now), true
}

// ScoringRule — как распределяются места, одной фразой для игроков.
func (g *Game) ScoringRule() string {
	switch g.Scenario.Scoring.Mode {
	case scenario.ScoringTime:
		return "Места — по числу пройденных уровней; при равенстве выше тот, у кого меньше время " +
			"(со старта команды плюс штрафные минуты за подсказки). Баллы на места не влияют."
	case scenario.ScoringMixed:
		return "Места — по баллам: за пройденные уровни и бонусы, минус штрафы. " +
			"При равенстве баллов выше тот, у кого меньше время (со старта команды плюс штрафные минуты)."
	default:
		return "Места — по баллам: за пройденные уровни и бонусы, минус штрафы. " +
			"При равенстве баллов выше тот, кто прошёл больше уровней."
	}
}
