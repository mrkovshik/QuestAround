package core

import (
	"errors"
	"time"

	"questaround/internal/scenario"
)

// Сливы: капитан за штраф получает место уровня («слив локации»)
// или засчитывает уровень без ответа («слив уровня»).

// События.
const (
	EvLocationRevealed EventType = "location_revealed"
	EvLevelSkipped     EventType = "level_skipped"
)

// LocationRevealed — команде открыто место уровня.
type LocationRevealed struct {
	LevelID       string        `json:"level_id"`
	PenaltyTime   time.Duration `json:"penalty_time,omitempty"`
	PenaltyPoints int           `json:"penalty_points,omitempty"`
}

// LevelSkipped — уровень слит; следом идёт LevelCompleted{Skipped: true}.
type LevelSkipped struct {
	LevelID       string        `json:"level_id"`
	PenaltyTime   time.Duration `json:"penalty_time,omitempty"`
	PenaltyPoints int           `json:"penalty_points,omitempty"`
}

func (LocationRevealed) Type() EventType { return EvLocationRevealed }
func (LevelSkipped) Type() EventType     { return EvLevelSkipped }

func init() {
	payloadFactory[EvLocationRevealed] = func() Payload { return &LocationRevealed{} }
	payloadFactory[EvLevelSkipped] = func() Payload { return &LevelSkipped{} }
}

// Команды.

// RevealLocation — слить локацию уровня. Только капитан.
type RevealLocation struct {
	TeamID    string
	PlayerID  string
	IsCaptain bool
	LevelID   string
}

// SkipLevel — слить уровень: засчитать без ответа. Только капитан.
type SkipLevel struct {
	TeamID    string
	PlayerID  string
	IsCaptain bool
	LevelID   string
}

var (
	ErrNoLocation       = errors.New("у этого уровня нет слива локации")
	ErrLocationRevealed = errors.New("локация уже слита")
	ErrSkipNotAvailable = errors.New("этот уровень слить нельзя")
)

func (g *Game) applySliv(e Event) bool {
	switch p := e.Payload.(type) {
	case LocationRevealed:
		t := g.Teams[e.TeamID]
		t.Levels[p.LevelID].LocationRevealed = true
		g.applyPenalty(t, e, "location", p.LevelID, p.PenaltyTime, p.PenaltyPoints)
	case LevelSkipped:
		t := g.Teams[e.TeamID]
		t.Levels[p.LevelID].Skipped = true
		g.applyPenalty(t, e, "skip", p.LevelID, p.PenaltyTime, p.PenaltyPoints)
	default:
		return false
	}
	return true
}

func (g *Game) applyPenalty(t *Team, e Event, kind, levelID string, d time.Duration, pts int) {
	t.PenaltyTime += d
	t.Points -= pts
	// строка в расшифровке — даже без штрафа: видно, что слив был
	t.Ledger = append(t.Ledger, ScoreEntry{At: e.At, Kind: kind, LevelID: levelID, Points: -pts, Time: d})
}

func (g *Game) handleSliv(t *tx, cmd Command) (bool, error) {
	switch c := cmd.(type) {
	case RevealLocation:
		team, sl, err := g.slivTarget(c.TeamID, c.PlayerID, c.IsCaptain, c.LevelID)
		if err != nil {
			return true, err
		}
		if sl.Location == nil {
			return true, ErrNoLocation
		}
		if t.now.Before(team.Levels[c.LevelID].OpenedAt.Add(sl.Location.AvailableAfter.D())) {
			return true, ErrNotAvailableYet
		}
		if team.Levels[c.LevelID].LocationRevealed {
			return true, ErrLocationRevealed
		}
		p := LocationRevealed{LevelID: c.LevelID}
		p.PenaltyTime, p.PenaltyPoints = penaltyOf(sl.Location.Penalty)
		t.emit(team.ID, c.PlayerID, "", p)
		return true, nil
	case SkipLevel:
		team, sl, err := g.slivTarget(c.TeamID, c.PlayerID, c.IsCaptain, c.LevelID)
		if err != nil {
			return true, err
		}
		if sl.Skip == nil {
			return true, ErrSkipNotAvailable
		}
		if t.now.Before(team.Levels[c.LevelID].OpenedAt.Add(sl.Skip.AvailableAfter.D())) {
			return true, ErrNotAvailableYet
		}
		p := LevelSkipped{LevelID: c.LevelID}
		p.PenaltyTime, p.PenaltyPoints = penaltyOf(sl.Skip.Penalty)
		t.emit(team.ID, c.PlayerID, "", p)
		t.emit(team.ID, "", "", LevelCompleted{LevelID: c.LevelID, Skipped: true}) // баллы за уровень не начисляются
		g.advance(t, team, c.LevelID)
		return true, nil
	}
	return false, nil
}

// slivTarget — общие проверки: команда играет, уровень открыт, просит капитан.
func (g *Game) slivTarget(teamID, playerID string, isCaptain bool, levelID string) (*Team, *scenario.Level, error) {
	team, err := g.playingTeam(teamID)
	if err != nil {
		return nil, nil, err
	}
	if !isCaptain && (playerID == "" || team.CaptainID != playerID) {
		return nil, nil, ErrNotAllowed
	}
	sl, ok := g.level(levelID)
	if !ok {
		return nil, nil, ErrUnknownLevel
	}
	if !team.Levels[levelID].IsOpen() {
		return nil, nil, ErrLevelNotOpen
	}
	return team, sl, nil
}

func penaltyOf(p *scenario.Penalty) (time.Duration, int) {
	if p == nil {
		return 0, 0
	}
	return p.Time.D(), p.Points
}

func (RevealLocation) isCommand() {}
func (SkipLevel) isCommand()      {}
