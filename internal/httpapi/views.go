package httpapi

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"questaround/internal/core"
	"questaround/internal/scenario"
)

// DTO: то, что видит клиент. Коды ответов, регэкспы и координаты гео-точек
// игрокам не отдаются никогда.

type standingDTO struct {
	Rank       int    `json:"rank"`
	TeamID     string `json:"team_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Points     int    `json:"points"`
	Levels     int    `json:"levels"`
	ElapsedSec int64  `json:"elapsed_sec"`
	Finished   bool   `json:"finished"`
}

func standingsDTO(st []core.Standing) []standingDTO {
	out := make([]standingDTO, len(st))
	for i, s := range st {
		out[i] = standingDTO{
			Rank: s.Rank, TeamID: s.TeamID, Name: s.Name, Status: string(s.Status),
			Points: s.Points, Levels: s.Levels, ElapsedSec: int64(s.Elapsed / time.Second), Finished: s.Finished,
		}
	}
	return out
}

type messageDTO struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

func messagesDTO(ms []core.Message) []messageDTO {
	out := make([]messageDTO, len(ms))
	for i, m := range ms {
		out[i] = messageDTO{At: m.At, Text: m.Text}
	}
	return out
}

type answerSlotDTO struct {
	Index   int      `json:"index"`
	Type    string   `json:"type"` // code | geo | photo | text
	Options []string `json:"options,omitempty"`
	Solved  bool     `json:"solved"`
	Pending bool     `json:"pending"`
}

type hintDTO struct {
	Index         int        `json:"index"`
	Opened        bool       `json:"opened"`
	Text          string     `json:"text,omitempty"`
	OnRequest     bool       `json:"on_request"`
	PenaltySec    int64      `json:"penalty_sec,omitempty"`
	PenaltyPoints int        `json:"penalty_points,omitempty"`
	OpensAt       *time.Time `json:"opens_at,omitempty"`
}

type bonusDTO struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Type    string `json:"type"`
	Points  int    `json:"points"`
	Done    bool   `json:"done"`
	Pending bool   `json:"pending"`
}

type levelDTO struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Task        string          `json:"task"`
	Media       []string        `json:"media,omitempty"`
	Points      int             `json:"points"`
	Open        bool            `json:"open"`
	OpenedAt    time.Time       `json:"opened_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Required    int             `json:"required"`
	Solved      int             `json:"solved"`
	Answers     []answerSlotDTO `json:"answers"`
	Hints       []hintDTO       `json:"hints"`
	Bonuses     []bonusDTO      `json:"bonuses,omitempty"`
}

func clientAnswerType(t scenario.AnswerType) string {
	switch t {
	case scenario.AnswerCode, scenario.AnswerRegex, scenario.AnswerChoice:
		return "code"
	}
	return string(t)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func levelView(sl *scenario.Level, ls *core.LevelState, team *core.Team) levelDTO {
	d := levelDTO{
		ID: sl.ID, Title: sl.Title, Task: sl.Task, Media: sl.Media, Points: sl.Points,
		Open: ls.IsOpen(), OpenedAt: ls.OpenedAt, CompletedAt: timePtr(ls.CompletedAt),
		Required: sl.Require.Needed(len(sl.Answers)), Solved: ls.SolvedCount(),
		Answers: []answerSlotDTO{}, Hints: []hintDTO{},
	}
	for i, a := range sl.Answers {
		slot := answerSlotDTO{Index: i, Type: clientAnswerType(a.Type), Solved: ls.Solved[i], Pending: ls.Pending[i] != 0}
		if a.Type == scenario.AnswerChoice {
			slot.Options = a.Options
		}
		d.Answers = append(d.Answers, slot)
	}
	for i, h := range sl.Hints {
		hd := hintDTO{Index: i, Opened: ls.Hints[i], OnRequest: h.OnRequest}
		if hd.Opened {
			hd.Text = h.Text
		}
		if h.Penalty != nil {
			hd.PenaltySec = int64(h.Penalty.Time.D() / time.Second)
			hd.PenaltyPoints = h.Penalty.Points
		}
		if h.IsAuto() && !hd.Opened && ls.IsOpen() {
			hd.OpensAt = timePtr(ls.OpenedAt.Add(h.After.D()))
		}
		d.Hints = append(d.Hints, hd)
	}
	for _, b := range sl.Bonuses {
		d.Bonuses = append(d.Bonuses, bonusDTO{
			ID: b.ID, Title: b.Title, Type: clientAnswerType(b.Answer.Type), Points: b.Points,
			Done: team.Bonuses[b.ID], Pending: team.PendingBonus[b.ID] != 0,
		})
	}
	return d
}

// openedLevels — открытые уровни команды по времени открытия.
func openedLevels(g *core.Game, team *core.Team) []levelDTO {
	out := []levelDTO{}
	for i := range g.Scenario.Levels {
		sl := &g.Scenario.Levels[i]
		if ls := team.Levels[sl.ID]; ls != nil {
			out = append(out, levelView(sl, ls, team))
		}
	}
	slices.SortStableFunc(out, func(a, b levelDTO) int { return a.OpenedAt.Compare(b.OpenedAt) })
	return out
}

type playerDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Captain bool   `json:"captain"`
}

func playersDTO(g *core.Game, team *core.Team) []playerDTO {
	out := []playerDTO{}
	for _, p := range g.ActivePlayers(team.ID) {
		out = append(out, playerDTO{ID: p.ID, Name: p.Name, Captain: p.ID == team.CaptainID})
	}
	return out
}

type teamDTO struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	InviteCode string      `json:"invite_code,omitempty"`
	Players    []playerDTO `json:"players"`
	CaptainID  string      `json:"captain_id,omitempty"`
}

func teamView(g *core.Game, team *core.Team) teamDTO {
	return teamDTO{
		ID: team.ID, Name: team.Name, Status: string(team.Status), InviteCode: team.InviteCode,
		Players: playersDTO(g, team), CaptainID: team.CaptainID,
	}
}

type playerStateDTO struct {
	Seq       int64  `json:"seq"`
	RunStatus string `json:"run_status"`
	Title     string `json:"title"`

	Team        teamDTO    `json:"team"`
	PlayerID    string     `json:"player_id"`
	IsCaptain   bool       `json:"is_captain"`
	StartAt     *time.Time `json:"start_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	TimeLimitAt *time.Time `json:"time_limit_at,omitempty"`
	Points      int        `json:"points"`
	PenaltySec  int64      `json:"penalty_sec"`
	ElapsedSec  int64      `json:"elapsed_sec"`
	LevelsDone  int        `json:"levels_done"`
	LevelsTotal int        `json:"levels_total"`

	Levels       []levelDTO   `json:"levels"`
	Messages     []messageDTO `json:"messages"`
	HintRequests string       `json:"hint_requests"`
	Leaderboard  struct {
		Visible bool `json:"visible"`
		Frozen  bool `json:"frozen"`
	} `json:"leaderboard"`
}

func playerStateView(g *core.Game, title, playerID string, team *core.Team, now time.Time) playerStateDTO {
	d := playerStateDTO{
		Seq: g.Seq(), RunStatus: string(g.Status), Title: title,
		Team: teamView(g, team), PlayerID: playerID, IsCaptain: team.CaptainID == playerID,
		StartedAt: timePtr(team.StartedAt), FinishedAt: timePtr(team.FinishedAt),
		Points: team.Points, PenaltySec: int64(team.PenaltyTime / time.Second),
		ElapsedSec: int64(g.Elapsed(team, now) / time.Second),
		LevelsDone: team.LevelsDone, LevelsTotal: len(g.Scenario.Levels),
		Levels:       openedLevels(g, team),
		Messages:     messagesDTO(g.Messages),
		HintRequests: string(g.Scenario.Participants.HintRequests),
	}
	if team.Status == core.TeamStatusScheduled {
		d.StartAt = timePtr(team.ScheduledAt)
	}
	if limit := g.Scenario.Timing.TimeLimit.D(); limit > 0 && !team.StartedAt.IsZero() {
		d.TimeLimitAt = timePtr(team.StartedAt.Add(limit))
	}
	d.Leaderboard.Visible = g.LeaderboardVisible(team.ID)
	d.Leaderboard.Frozen = g.Leaderboard.Frozen && g.Status != core.RunStatusFinished
	return d
}

// --- ведущий ---

type currentLevelDTO struct {
	LevelID     string     `json:"level_id"`
	Title       string     `json:"title"`
	OpenedAt    time.Time  `json:"opened_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Solved      int        `json:"solved"`
	Required    int        `json:"required"`
	HintsOpened int        `json:"hints_opened"`
	HintsTotal  int        `json:"hints_total"`
}

type dashTeamDTO struct {
	teamDTO
	Contact      string            `json:"contact,omitempty"`
	Slot         int               `json:"slot"`
	ScheduledAt  *time.Time        `json:"scheduled_at,omitempty"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	Route        []string          `json:"route,omitempty"`
	Points       int               `json:"points"`
	PenaltySec   int64             `json:"penalty_sec"`
	ElapsedSec   int64             `json:"elapsed_sec"`
	LevelsDone   int               `json:"levels_done"`
	LastActivity *time.Time        `json:"last_activity,omitempty"`
	Current      []currentLevelDTO `json:"current"` // открытые сейчас
	Levels       []currentLevelDTO `json:"levels"`  // все открытые когда-либо, по времени открытия
}

type dashboardDTO struct {
	Seq          int64  `json:"seq"`
	RunStatus    string `json:"run_status"`
	LevelsTotal  int    `json:"levels_total"`
	Registration struct {
		Open     bool `json:"open"`
		MaxTeams int  `json:"max_teams"`
	} `json:"registration"`
	Leaderboard struct {
		Visibility string `json:"visibility"`
		Override   *bool  `json:"override"`
		Frozen     bool   `json:"frozen"`
	} `json:"leaderboard"`
	PendingModeration int           `json:"pending_moderation"`
	Teams             []dashTeamDTO `json:"teams"`
	Withdrawn         []teamDTO     `json:"withdrawn,omitempty"`
	Standings         []standingDTO `json:"standings"`
	Messages          []messageDTO  `json:"messages"`
}

func dashboardView(g *core.Game, now time.Time) dashboardDTO {
	d := dashboardDTO{
		Seq: g.Seq(), RunStatus: string(g.Status), LevelsTotal: len(g.Scenario.Levels),
		PendingModeration: len(g.PendingModeration()),
		Standings:         standingsDTO(g.Standings(now)),
		Messages:          messagesDTO(g.Messages),
		Teams:             []dashTeamDTO{},
	}
	d.Registration.Open = !g.Registration.Closed
	d.Registration.MaxTeams = g.Registration.MaxTeams
	d.Leaderboard.Visibility = string(g.Scenario.Leaderboard.Visibility)
	d.Leaderboard.Override = g.Leaderboard.Override
	d.Leaderboard.Frozen = g.Leaderboard.Frozen

	for _, id := range g.TeamOrder {
		t := g.Teams[id]
		if t.Status == core.TeamStatusWithdrawn {
			d.Withdrawn = append(d.Withdrawn, teamView(g, t))
			continue
		}
		dt := dashTeamDTO{
			teamDTO: teamView(g, t), Contact: t.Contact, Slot: t.Slot,
			ScheduledAt: timePtr(t.ScheduledAt), StartedAt: timePtr(t.StartedAt), FinishedAt: timePtr(t.FinishedAt),
			FinishReason: t.FinishReason, Route: t.Route, Points: t.Points,
			PenaltySec: int64(t.PenaltyTime / time.Second), ElapsedSec: int64(g.Elapsed(t, now) / time.Second),
			LevelsDone: t.LevelsDone, LastActivity: timePtr(t.LastActivity),
			Current: []currentLevelDTO{}, Levels: []currentLevelDTO{},
		}
		if dt.Players == nil {
			dt.Players = []playerDTO{}
		}
		for _, sl := range g.Scenario.Levels {
			ls := t.Levels[sl.ID]
			if ls == nil {
				continue
			}
			opened := 0
			for _, h := range ls.Hints {
				if h {
					opened++
				}
			}
			lv := currentLevelDTO{
				LevelID: sl.ID, Title: cmp.Or(sl.Title, sl.ID), OpenedAt: ls.OpenedAt, CompletedAt: timePtr(ls.CompletedAt),
				Solved: ls.SolvedCount(), Required: sl.Require.Needed(len(sl.Answers)),
				HintsOpened: opened, HintsTotal: len(sl.Hints),
			}
			dt.Levels = append(dt.Levels, lv)
			if ls.IsOpen() {
				dt.Current = append(dt.Current, lv)
			}
		}
		slices.SortStableFunc(dt.Levels, func(a, b currentLevelDTO) int { return a.OpenedAt.Compare(b.OpenedAt) })
		d.Teams = append(d.Teams, dt)
	}
	return d
}

type moderationDTO struct {
	Seq        int64     `json:"seq"`
	At         time.Time `json:"at"`
	TeamID     string    `json:"team_id"`
	TeamName   string    `json:"team_name"`
	PlayerName string    `json:"player_name,omitempty"`
	LevelID    string    `json:"level_id"`
	LevelTitle string    `json:"level_title"`
	Target     string    `json:"target"` // название бонуса или «Ответ N»
	Kind       string    `json:"kind"`
	Value      string    `json:"value,omitempty"`
	MediaURL   string    `json:"media_url,omitempty"`
}

func moderationView(g *core.Game) []moderationDTO {
	out := []moderationDTO{}
	for _, it := range g.PendingModeration() {
		d := moderationDTO{
			Seq: it.Seq, At: it.At, TeamID: it.TeamID, LevelID: it.LevelID,
			Kind: string(it.Kind), Value: it.Value,
		}
		if t := g.Teams[it.TeamID]; t != nil {
			d.TeamName = t.Name
		}
		if p := g.Players[it.PlayerID]; p != nil {
			d.PlayerName = p.Name
		}
		for _, sl := range g.Scenario.Levels {
			if sl.ID != it.LevelID {
				continue
			}
			d.LevelTitle = sl.Title
			if it.BonusID != "" {
				for _, b := range sl.Bonuses {
					if b.ID == it.BonusID {
						d.Target = "Бонус: " + cmp.Or(b.Title, b.ID)
					}
				}
			} else {
				d.Target = "Ответ " + strconv.Itoa(it.AnswerIdx+1)
			}
		}
		if it.MediaKey != "" {
			d.MediaURL = "/admin/v1/media/" + it.MediaKey
		}
		out = append(out, d)
	}
	return out
}
