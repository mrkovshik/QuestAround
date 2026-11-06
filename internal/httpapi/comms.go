package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"questaround/internal/core"
)

// --- DTO ---

type chatDTO struct {
	Seq      int64     `json:"seq"`
	At       time.Time `json:"at"`
	FromHost bool      `json:"from_host"`
	Author   string    `json:"author"`
	Text     string    `json:"text"`
}

func chatView(g *core.Game, team *core.Team) []chatDTO {
	out := make([]chatDTO, 0, len(team.Chat))
	for _, m := range team.Chat {
		author := "Ведущий"
		if !m.FromHost {
			author = "Капитан"
			if p := g.Players[m.PlayerID]; p != nil {
				author = p.Name
			}
		}
		out = append(out, chatDTO{Seq: m.Seq, At: m.At, FromHost: m.FromHost, Author: author, Text: m.Text})
	}
	return out
}

type sosDTO struct {
	Seq          int64      `json:"seq"`
	At           time.Time  `json:"at"`
	TeamID       string     `json:"team_id"`
	TeamName     string     `json:"team_name"`
	PlayerName   string     `json:"player_name"`
	Contact      string     `json:"contact,omitempty"` // контакт капитана
	Lat          *float64   `json:"lat,omitempty"`
	Lng          *float64   `json:"lng,omitempty"`
	Accuracy     *float64   `json:"accuracy,omitempty"`
	Text         string     `json:"text,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	Note         string     `json:"note,omitempty"`
	CurrentLevel string     `json:"current_level,omitempty"`
}

func sosView(g *core.Game, a *core.SOSAlert) sosDTO {
	d := sosDTO{Seq: a.Seq, At: a.At, TeamID: a.TeamID, Lat: a.Lat, Lng: a.Lng, Accuracy: a.Accuracy,
		Text: a.Text, ResolvedAt: timePtr(a.ResolvedAt), Note: a.Note}
	if t := g.Teams[a.TeamID]; t != nil {
		d.TeamName, d.Contact = t.Name, t.Contact
		for _, sl := range g.Scenario.Levels {
			if t.Levels[sl.ID].IsOpen() {
				d.CurrentLevel = sl.Title
				break
			}
		}
	}
	if p := g.Players[a.PlayerID]; p != nil {
		d.PlayerName = p.Name
	}
	return d
}

// sosForTeam — сигналы команды (для экрана игрока), новые сверху.
func sosForTeam(g *core.Game, teamID string) []sosDTO {
	out := []sosDTO{}
	for i := len(g.SOS) - 1; i >= 0; i-- {
		if a := g.SOS[i]; a.TeamID == teamID {
			out = append(out, sosView(g, a))
		}
	}
	return out
}

// --- игрок ---

func (s *Server) postChat(w http.ResponseWriter, r *http.Request, p *player) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	if _, err := p.Runner.Do(r.Context(), core.PostChat{TeamID: p.TeamID, PlayerID: p.PlayerID, Text: req.Text}); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) raiseSOS(w http.ResponseWriter, r *http.Request, p *player) {
	var req struct {
		Lat      *float64 `json:"lat"`
		Lng      *float64 `json:"lng"`
		Accuracy *float64 `json:"accuracy"`
		Text     string   `json:"text"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	out, err := p.Runner.Do(r.Context(), core.RaiseSOS{TeamID: p.TeamID, PlayerID: p.PlayerID,
		Lat: req.Lat, Lng: req.Lng, Accuracy: req.Accuracy, Text: req.Text})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Warn("SOS", "run", p.RunID, "team", p.TeamID, "player", p.PlayerID)
	writeJSON(w, http.StatusCreated, map[string]int64{"seq": out.Events[0].Seq})
}

// --- ведущий ---

func (s *Server) hostChat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.PostChat{TeamID: r.PathValue("team"), FromHost: true, Text: req.Text})
}

func (s *Server) resolveSOS(w http.ResponseWriter, r *http.Request) {
	seq, err := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	if err != nil {
		s.writeError(w, r, core.ErrNotPending)
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.ResolveSOS{Ref: seq, Note: req.Note})
}

func (s *Server) setHostContact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.SetHostContact{Text: req.Text})
}
