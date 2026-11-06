package httpapi

import (
	"cmp"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/store"
)

const maxPhotoSize = 15 << 20

// runByCode — игра и её актор по публичному коду.
func (s *Server) runByCode(ctx context.Context, code string) (store.Run, *runner.Runner, error) {
	run, err := s.cfg.Store.GetRunByCode(ctx, strings.ToUpper(code))
	if err != nil {
		return run, nil, err
	}
	rn, err := s.cfg.Runners.Get(ctx, run.ID)
	return run, rn, err
}

func runTitle(run store.Run, g *core.Game) string {
	return cmp.Or(run.Title, g.Scenario.Title)
}

// --- публичное ---

type gameInfoDTO struct {
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Status       string `json:"status"`
	Participants struct {
		Mode    string `json:"mode"`
		MinSize int    `json:"min_size"`
		MaxSize int    `json:"max_size"`
	} `json:"participants"`
	Registration struct {
		Open     bool `json:"open"`
		MaxTeams int  `json:"max_teams"`
		Teams    int  `json:"teams"`
	} `json:"registration"`
}

func (s *Server) gameInfo(w http.ResponseWriter, r *http.Request) {
	run, rn, err := s.runByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	var d gameInfoDTO
	err = rn.Read(r.Context(), func(g *core.Game) {
		d.Title = runTitle(run, g)
		d.Description = g.Scenario.Description
		d.Status = string(g.Status)
		pt := g.Scenario.Participants
		d.Participants.Mode = string(pt.Mode)
		d.Participants.MinSize, d.Participants.MaxSize = pt.TeamSize.Min, pt.TeamSize.Max
		d.Registration.Open = !g.Registration.Closed && g.Status == core.RunStatusPending
		d.Registration.MaxTeams = g.Registration.MaxTeams
		d.Registration.Teams = len(g.ActiveTeams())
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type credentialsDTO struct {
	Token      string `json:"token"`
	PlayerID   string `json:"player_id"`
	TeamID     string `json:"team_id"`
	InviteCode string `json:"invite_code,omitempty"`
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamName    string `json:"team_name"`
		CaptainName string `json:"captain_name"`
		Contact     string `json:"contact"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	run, rn, err := s.runByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	secret := newSecret()
	cmd := core.RegisterTeam{
		TeamID: newID(), Name: req.TeamName, Contact: strings.TrimSpace(req.Contact),
		Captain: &core.NewPlayer{ID: newID(), Name: req.CaptainName, TokenHash: hashSecret(secret)},
	}
	for range 5 { // повтор на редкой коллизии инвайт-кода
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
	writeJSON(w, http.StatusCreated, credentialsDTO{
		Token: makeToken(run.ID, cmd.Captain.ID, secret), PlayerID: cmd.Captain.ID,
		TeamID: cmd.TeamID, InviteCode: cmd.InviteCode,
	})
}

func (s *Server) joinTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InviteCode string `json:"invite_code"`
		Name       string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	run, rn, err := s.runByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	secret := newSecret()
	np := core.NewPlayer{ID: newID(), Name: req.Name, TokenHash: hashSecret(secret)}
	out, err := rn.Do(r.Context(), core.JoinTeam{InviteCode: strings.ToUpper(strings.TrimSpace(req.InviteCode)), Player: np})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialsDTO{
		Token: makeToken(run.ID, np.ID, secret), PlayerID: np.ID, TeamID: out.Events[0].TeamID,
	})
}

// --- игрок ---

func (s *Server) runTitle(ctx context.Context, runID string) string {
	run, err := s.cfg.Store.GetRun(ctx, runID)
	if err != nil {
		return ""
	}
	return run.Title
}

func (s *Server) state(w http.ResponseWriter, r *http.Request, p *player) {
	title := s.runTitle(r.Context(), p.RunID)
	var d playerStateDTO
	err := p.Runner.Read(r.Context(), func(g *core.Game) {
		d = playerStateView(g, cmp.Or(title, g.Scenario.Title), p.PlayerID, g.Teams[p.TeamID], s.now())
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type answerResultDTO struct {
	Seq       int64  `json:"seq"`
	Result    string `json:"result"`
	LevelID   string `json:"level_id,omitempty"`
	AnswerIdx *int   `json:"answer_idx,omitempty"`
	BonusID   string `json:"bonus_id,omitempty"`
}

func answerDTO(a *core.AnswerResult) answerResultDTO {
	return answerResultDTO{Seq: a.Seq, Result: string(a.Result), LevelID: a.LevelID, AnswerIdx: a.AnswerIdx, BonusID: a.BonusID}
}

type answerRequest struct {
	ClientEventID string  `json:"client_event_id"`
	LevelID       string  `json:"level_id"`
	Kind          string  `json:"kind"` // code | geo | text
	Value         string  `json:"value"`
	Lat           float64 `json:"lat"`
	Lng           float64 `json:"lng"`
	Accuracy      float64 `json:"accuracy"`
	AnswerIdx     *int    `json:"answer_idx"`
	BonusID       string  `json:"bonus_id"`
}

func (req answerRequest) command(teamID, playerID string) (core.SubmitAnswer, error) {
	kind := core.AnswerKind(req.Kind)
	switch kind {
	case core.KindCode, core.KindGeo, core.KindText:
	default:
		return core.SubmitAnswer{}, newErr(http.StatusBadRequest, "bad_kind", "kind: ожидалось code, geo или text (фото — через /photos)")
	}
	return core.SubmitAnswer{
		TeamID: teamID, PlayerID: playerID, ClientEventID: req.ClientEventID,
		LevelID: req.LevelID, BonusID: req.BonusID, AnswerIdx: req.AnswerIdx,
		Kind: kind, Value: req.Value, Lat: req.Lat, Lng: req.Lng, Accuracy: req.Accuracy,
	}, nil
}

func (s *Server) submitAnswer(w http.ResponseWriter, r *http.Request, p *player) {
	var req answerRequest
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	cmd, err := req.command(p.TeamID, p.PlayerID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out, err := p.Runner.Do(r.Context(), cmd)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, answerDTO(out.Answer))
}

// submitPhoto: multipart — photo (файл), level_id, bonus_id, answer_idx, client_event_id.
func (s *Server) submitPhoto(w http.ResponseWriter, r *http.Request, p *player) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, r, newErr(http.StatusBadRequest, "bad_multipart", "ожидалась форма multipart/form-data"))
		return
	}
	cmd := core.SubmitAnswer{TeamID: p.TeamID, PlayerID: p.PlayerID, Kind: core.KindPhoto}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		switch part.FormName() {
		case "photo":
			key, err := s.cfg.Media.Save(r.Context(), p.RunID, part)
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			cmd.MediaKey = key
		case "level_id", "bonus_id", "answer_idx", "client_event_id":
			raw, err := io.ReadAll(io.LimitReader(part, 256))
			if err != nil {
				s.writeError(w, r, err)
				return
			}
			v := strings.TrimSpace(string(raw))
			switch part.FormName() {
			case "level_id":
				cmd.LevelID = v
			case "bonus_id":
				cmd.BonusID = v
			case "client_event_id":
				cmd.ClientEventID = v
			case "answer_idx":
				if v != "" {
					i, err := strconv.Atoi(v)
					if err != nil {
						s.writeError(w, r, newErr(http.StatusBadRequest, "bad_request", "answer_idx: ожидалось число"))
						return
					}
					cmd.AnswerIdx = &i
				}
			}
		}
		part.Close()
	}
	out, err := p.Runner.Do(r.Context(), cmd)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, answerDTO(out.Answer))
}

func (s *Server) requestHint(w http.ResponseWriter, r *http.Request, p *player) {
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		s.writeError(w, r, core.ErrUnknownHint)
		return
	}
	_, err = p.Runner.Do(r.Context(), core.RequestHint{
		TeamID: p.TeamID, PlayerID: p.PlayerID, LevelID: r.PathValue("level"), HintIdx: idx,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request, p *player) {
	var (
		st      []core.Standing
		visible bool
		frozen  bool
		final   bool
		rule    string
	)
	err := p.Runner.Read(r.Context(), func(g *core.Game) {
		st, visible = g.LeaderboardFor(p.TeamID, s.now())
		frozen = g.Leaderboard.Frozen && g.Status != core.RunStatusFinished
		final = g.Results != nil
		rule = g.ScoringRule()
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if !visible {
		s.writeError(w, r, newErr(http.StatusForbidden, "leaderboard_hidden", "таблица результатов сейчас скрыта"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"frozen": frozen, "final": final, "rule": rule, "standings": standingsDTO(st)})
}

// --- команда ---

func (s *Server) team(w http.ResponseWriter, r *http.Request, p *player) {
	var d teamDTO
	err := p.Runner.Read(r.Context(), func(g *core.Game) { d = teamView(g, g.Teams[p.TeamID]) })
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// teamCommand выполняет команду капитана и возвращает свежий состав.
func (s *Server) teamCommand(w http.ResponseWriter, r *http.Request, p *player, cmd core.Command) {
	if _, err := p.Runner.Do(r.Context(), cmd); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.team(w, r, p)
}

func (s *Server) renameTeam(w http.ResponseWriter, r *http.Request, p *player) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.teamCommand(w, r, p, core.RenameTeam{Actor: core.Actor{PlayerID: p.PlayerID}, TeamID: p.TeamID, Name: req.Name})
}

func (s *Server) removePlayer(w http.ResponseWriter, r *http.Request, p *player) {
	s.teamCommand(w, r, p, core.RemovePlayer{Actor: core.Actor{PlayerID: p.PlayerID}, TeamID: p.TeamID, PlayerID: r.PathValue("player")})
}

func (s *Server) rotateInvite(w http.ResponseWriter, r *http.Request, p *player) {
	var err error
	for range 5 {
		_, err = p.Runner.Do(r.Context(), core.RotateInvite{Actor: core.Actor{PlayerID: p.PlayerID}, TeamID: p.TeamID, InviteCode: newInviteCode()})
		if !errors.Is(err, core.ErrInviteTaken) {
			break
		}
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.team(w, r, p)
}

func (s *Server) transferCaptain(w http.ResponseWriter, r *http.Request, p *player) {
	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := readJSON(w, r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.teamCommand(w, r, p, core.TransferCaptain{Actor: core.Actor{PlayerID: p.PlayerID}, TeamID: p.TeamID, PlayerID: req.PlayerID})
}

func (s *Server) withdrawTeam(w http.ResponseWriter, r *http.Request, p *player) {
	s.teamCommand(w, r, p, core.WithdrawTeam{Actor: core.Actor{PlayerID: p.PlayerID}, TeamID: p.TeamID})
}
