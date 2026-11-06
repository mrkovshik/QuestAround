package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

const maxScenarioSize = 1 << 20

// --- сценарии ---

type scenarioVersionDTO struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"`
}

// uploadScenario: тело — YAML. Невалидный сценарий → 422 со списком ошибок по строкам.
func (s *Server) uploadScenario(w http.ResponseWriter, r *http.Request) {
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxScenarioSize))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	sc, err := scenario.Parse(src)
	if err != nil {
		var se scenario.Errors
		if !errors.As(err, &se) {
			// синтаксис/структура: одна ошибка без номера строки в нашем формате
			err = scenario.Errors{{Msg: err.Error()}}
		}
		s.writeError(w, r, err)
		return
	}
	sv, err := s.cfg.Store.SaveScenarioVersion(r.Context(), store.DefaultOrg, sc.ID, string(src))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, scenarioVersionDTO{
		ID: sv.ID, Slug: sv.Slug, Version: sv.Version, Title: sc.Title, CreatedAt: sv.CreatedAt,
	})
}

func (s *Server) listScenarios(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.ListScenarioVersions(r.Context(), store.DefaultOrg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out := make([]scenarioVersionDTO, 0, len(list))
	for _, sv := range list {
		out = append(out, scenarioVersionDTO{ID: sv.ID, Slug: sv.Slug, Version: sv.Version, CreatedAt: sv.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getScenario(w http.ResponseWriter, r *http.Request) {
	sv, err := s.cfg.Store.GetScenarioVersion(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	d := scenarioVersionDTO{ID: sv.ID, Slug: sv.Slug, Version: sv.Version, CreatedAt: sv.CreatedAt, Source: sv.Source}
	if sc, err := scenario.Parse([]byte(sv.Source)); err == nil {
		d.Title = sc.Title
	}
	writeJSON(w, http.StatusOK, d)
}

// --- игры ---

type runDTO struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	PublicCode        string    `json:"public_code"`
	ScenarioVersionID string    `json:"scenario_version_id"`
	CreatedAt         time.Time `json:"created_at"`
	Status            string    `json:"status,omitempty"`
	Teams             int       `json:"teams"`
}

func (s *Server) runView(r *http.Request, run store.Run) runDTO {
	d := runDTO{ID: run.ID, Title: run.Title, PublicCode: run.PublicCode, ScenarioVersionID: run.ScenarioVersionID, CreatedAt: run.CreatedAt}
	if rn, err := s.cfg.Runners.Get(r.Context(), run.ID); err == nil {
		_ = rn.Read(r.Context(), func(g *core.Game) {
			d.Status = string(g.Status)
			d.Teams = len(g.ActiveTeams())
			if d.Title == "" {
				d.Title = g.Scenario.Title
			}
		})
	}
	return d
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScenarioVersionID string `json:"scenario_version_id"`
		Title             string `json:"title"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	run, err := s.cfg.Store.CreateRun(r.Context(), store.NewRun{
		OrgID: store.DefaultOrg, ScenarioVersionID: req.ScenarioVersionID, Title: strings.TrimSpace(req.Title),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.runView(r, run))
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.cfg.Store.ListRuns(r.Context(), store.DefaultOrg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out := make([]runDTO, 0, len(runs))
	for _, run := range runs {
		out = append(out, s.runView(r, run))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.cfg.Store.GetRun(r.Context(), r.PathValue("run"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.runView(r, run))
}

func (s *Server) runner(r *http.Request) (*runner.Runner, error) {
	return s.cfg.Runners.Get(r.Context(), r.PathValue("run"))
}

// command выполняет команду ведущего и отвечает свежим дашбордом.
func (s *Server) command(w http.ResponseWriter, r *http.Request, cmd core.Command) {
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if _, err := rn.Do(r.Context(), cmd); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeDashboard(w, r, rn)
}

func (s *Server) writeDashboard(w http.ResponseWriter, r *http.Request, rn *runner.Runner) {
	var d dashboardDTO
	if err := rn.Read(r.Context(), func(g *core.Game) { d = dashboardView(g, s.now()) }); err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeDashboard(w, r, rn)
}

func (s *Server) setRegistration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Open     bool `json:"open"`
		MaxTeams int  `json:"max_teams"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.SetRegistration{Open: req.Open, MaxTeams: req.MaxTeams})
}

func (s *Server) hostCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Contact string `json:"contact"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	cmd := core.RegisterTeam{TeamID: newID(), Name: req.Name, Contact: req.Contact, ByHost: true}
	for range 5 {
		cmd.InviteCode = newInviteCode()
		_, err = rn.Do(r.Context(), cmd)
		if !errors.Is(err, core.ErrInviteTaken) {
			break
		}
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"team_id": cmd.TeamID, "invite_code": cmd.InviteCode})
}

var host = core.Actor{Host: true}

func (s *Server) hostRenameTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.RenameTeam{Actor: host, TeamID: r.PathValue("team"), Name: req.Name})
}

func (s *Server) hostWithdrawTeam(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, core.WithdrawTeam{Actor: host, TeamID: r.PathValue("team")})
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, core.StartRun{})
}

func (s *Server) finishRun(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, core.FinishRun{})
}

func (s *Server) startTeam(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, core.StartTeam{TeamID: r.PathValue("team")})
}

// duration в JSON — строкой Go: "5m", "-2m30s".
type duration time.Duration

func (d *duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = duration(v)
	return nil
}

func (s *Server) override(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action  string   `json:"action"`
		LevelID string   `json:"level_id"`
		HintIdx int      `json:"hint_idx"`
		Time    duration `json:"time"`
		Points  int      `json:"points"`
		Reason  string   `json:"reason"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.Override{
		TeamID: r.PathValue("team"), Action: core.OverrideAction(req.Action), LevelID: req.LevelID,
		HintIdx: req.HintIdx, Time: time.Duration(req.Time), Points: req.Points, Reason: req.Reason, By: "host",
	})
}

// hostSubmitAnswer — ведущий вводит ответ за команду (например, без связи).
func (s *Server) hostSubmitAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	cmd, err := req.command(r.PathValue("team"), "host")
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out, err := rn.Do(r.Context(), cmd)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, answerDTO(out.Answer))
}

func (s *Server) moderationQueue(w http.ResponseWriter, r *http.Request) {
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var q []moderationDTO
	if err := rn.Read(r.Context(), func(g *core.Game) { q = moderationView(g) }); err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) moderate(w http.ResponseWriter, r *http.Request) {
	seq, err := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	if err != nil {
		s.writeError(w, r, core.ErrNotPending)
		return
	}
	var req struct {
		Approved bool   `json:"approved"`
		Comment  string `json:"comment"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if _, err := rn.Do(r.Context(), core.Moderate{Ref: seq, Approved: req.Approved, Comment: req.Comment, By: "host"}); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.moderationQueue(w, r)
}

func (s *Server) setLeaderboard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Visible *bool `json:"visible"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.SetLeaderboard{Visible: req.Visible})
}

func (s *Server) broadcast(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.command(w, r, core.SendBroadcast{Text: req.Text})
}

// events — журнал игры целиком (разбор спорных ситуаций, аналитика).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if _, err := s.cfg.Store.GetRun(r.Context(), r.PathValue("run")); err != nil {
		s.writeError(w, r, err)
		return
	}
	ev, err := s.cfg.Store.Load(r.Context(), r.PathValue("run"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if ev == nil {
		ev = []core.Event{}
	}
	writeJSON(w, http.StatusOK, ev)
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	f, ctype, err := s.cfg.Media.Open(r.Context(), r.PathValue("key"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", time.Time{}, f)
}
