package core

import "time"

func (g *Game) tick(t *tx) {
	if g.Status != RunStatusRunning {
		return
	}
	g.tickStarts(t)

	limit := g.Scenario.Timing.TimeLimit.D()
	for _, id := range g.TeamOrder {
		team := g.Teams[id]
		if team.Status != TeamStatusPlaying {
			continue
		}
		if limit > 0 && !t.now.Before(team.StartedAt.Add(limit)) {
			g.finishTeam(t, team, "time_limit", true)
			continue
		}
		g.tickHints(t, team)
	}

	if g.Status == RunStatusRunning && !g.Leaderboard.Frozen {
		if at, ok := g.freezeAt(); ok && !t.now.Before(at) {
			t.emit("", "", "", LeaderboardFrozen{})
		}
	}
}

func (g *Game) tickStarts(t *tx) {
	for _, id := range g.TeamOrder {
		team := g.Teams[id]
		if team.Status == TeamStatusScheduled && !team.ScheduledAt.After(t.now) {
			g.startTeam(t, team)
		}
	}
}

func (g *Game) tickHints(t *tx, team *Team) {
	for _, l := range g.Scenario.Levels {
		ls := team.Levels[l.ID]
		if !ls.IsOpen() {
			continue
		}
		for i, h := range l.Hints {
			if h.IsAuto() && !ls.Hints[i] && !t.now.Before(ls.OpenedAt.Add(h.After.D())) {
				t.emit(team.ID, "", "", HintOpened{LevelID: l.ID, HintIdx: i, Trigger: "timer"})
			}
		}
	}
}

// plannedEnd — когда истечёт лимит у последней стартовавшей команды.
// Известно, только когда стартовали все команды и задан time_limit.
func (g *Game) plannedEnd() (time.Time, bool) {
	limit := g.Scenario.Timing.TimeLimit.D()
	if limit == 0 {
		return time.Time{}, false
	}
	var last time.Time
	for _, id := range g.TeamOrder {
		team := g.Teams[id]
		switch team.Status {
		case TeamStatusRegistered, TeamStatusScheduled:
			return time.Time{}, false
		}
		if team.StartedAt.After(last) {
			last = team.StartedAt
		}
	}
	if last.IsZero() {
		return time.Time{}, false
	}
	return last.Add(limit), true
}

// freezeAt — момент заморозки лидерборда: за freeze_before_end
// до конца лимита последней стартовавшей команды.
func (g *Game) freezeAt() (time.Time, bool) {
	freeze := g.Scenario.Leaderboard.FreezeBeforeEnd.D()
	if freeze == 0 {
		return time.Time{}, false
	}
	end, ok := g.plannedEnd()
	if !ok {
		return time.Time{}, false
	}
	return end.Add(-freeze), true
}

// NextDeadline — ближайший момент, когда Tick что-то сделает.
// Актор Run ставит таймер на него вместо опроса раз в секунду.
func (g *Game) NextDeadline() (time.Time, bool) {
	var best time.Time
	found := false
	consider := func(at time.Time) {
		if !found || at.Before(best) {
			best, found = at, true
		}
	}
	if g.Status != RunStatusRunning {
		return best, false
	}

	limit := g.Scenario.Timing.TimeLimit.D()
	for _, id := range g.TeamOrder {
		team := g.Teams[id]
		switch team.Status {
		case TeamStatusScheduled:
			consider(team.ScheduledAt)
		case TeamStatusPlaying:
			if limit > 0 {
				consider(team.StartedAt.Add(limit))
			}
			for _, l := range g.Scenario.Levels {
				ls := team.Levels[l.ID]
				if !ls.IsOpen() {
					continue
				}
				for i, h := range l.Hints {
					if h.IsAuto() && !ls.Hints[i] {
						consider(ls.OpenedAt.Add(h.After.D()))
					}
				}
			}
		}
	}
	if !g.Leaderboard.Frozen {
		if at, ok := g.freezeAt(); ok {
			consider(at)
		}
	}
	return best, found
}
