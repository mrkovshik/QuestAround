package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"questaround/internal/core"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// apiError — ошибка с HTTP-статусом и машиночитаемым кодом.
type apiError struct {
	status int
	code   string
	msg    string
}

func (e *apiError) Error() string { return e.msg }

func newErr(status int, code, msg string) *apiError {
	return &apiError{status: status, code: code, msg: msg}
}

var (
	errUnauthorized = newErr(http.StatusUnauthorized, "unauthorized", "нужна авторизация")
	errBadJSON      = newErr(http.StatusBadRequest, "bad_json", "некорректный JSON")
)

// errorTable сопоставляет ошибки домена с HTTP. Клиенту уходят code
// (для логики) и message (по-русски, можно показать человеку).
var errorTable = []struct {
	err    error
	status int
	code   string
}{
	{core.ErrBadCommand, 400, "bad_request"},
	{core.ErrEmptyAnswer, 400, "empty_answer"},
	{core.ErrNoTarget, 400, "no_target"},
	{core.ErrReasonRequired, 400, "reason_required"},
	{core.ErrNotAllowed, 403, "not_allowed"},
	{core.ErrUnknownTeam, 404, "unknown_team"},
	{core.ErrUnknownLevel, 404, "unknown_level"},
	{core.ErrUnknownHint, 404, "unknown_hint"},
	{core.ErrUnknownInvite, 404, "unknown_invite"},
	{core.ErrUnknownPlayer, 404, "unknown_player"},
	{store.ErrNotFound, 404, "not_found"},
	{core.ErrRunStarted, 409, "run_started"},
	{core.ErrRunNotRunning, 409, "run_not_running"},
	{core.ErrRunNotFinished, 409, "run_not_finished"},
	{core.ErrNoTeams, 409, "no_teams"},
	{core.ErrTeamExists, 409, "team_exists"},
	{core.ErrTeamNotPlaying, 409, "team_not_playing"},
	{core.ErrTeamAlreadyStarted, 409, "team_already_started"},
	{core.ErrLevelNotOpen, 409, "level_not_open"},
	{core.ErrAlreadyPending, 409, "already_pending"},
	{core.ErrHintAlreadyOpen, 409, "hint_already_open"},
	{core.ErrNotPending, 409, "not_pending"},
	{core.ErrRegistrationClosed, 409, "registration_closed"},
	{core.ErrRegistrationFull, 409, "registration_full"},
	{core.ErrTeamFull, 409, "team_full"},
	{core.ErrPlayerExists, 409, "player_exists"},
	{core.ErrInviteTaken, 409, "invite_taken"},
	{core.ErrRateLimited, 429, "rate_limited"},
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Details []scenarioIssue `json:"details,omitempty"`
}

type scenarioIssue struct {
	Line    int    `json:"line,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.status, errorBody{Error: ae.code, Message: ae.msg})
		return
	}
	var inc *scenario.Incompatible
	if errors.As(err, &inc) {
		body := errorBody{Error: "scenario_incompatible", Message: "игра уже идёт — в новой версии изменилась структура сценария"}
		for _, p := range inc.Problems {
			body.Details = append(body.Details, scenarioIssue{Message: p})
		}
		writeJSON(w, http.StatusConflict, body)
		return
	}
	var se scenario.Errors
	if errors.As(err, &se) {
		body := errorBody{Error: "invalid_scenario", Message: "в сценарии есть ошибки"}
		for _, e := range se {
			body.Details = append(body.Details, scenarioIssue{Line: e.Line, Path: e.Path, Message: e.Msg})
		}
		writeJSON(w, http.StatusUnprocessableEntity, body)
		return
	}
	for _, m := range errorTable {
		if errors.Is(err, m.err) {
			writeJSON(w, m.status, errorBody{Error: m.code, Message: m.err.Error()})
			return
		}
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{Error: "too_large", Message: "слишком большой запрос"})
		return
	}
	s.log.Error("внутренняя ошибка", "err", err, "method", r.Method, "path", r.URL.Path)
	writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal", Message: "внутренняя ошибка сервера"})
}

const maxJSONBody = 1 << 20

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return err
		}
		if errors.Is(err, io.EOF) {
			return newErr(http.StatusBadRequest, "bad_json", "пустое тело запроса")
		}
		return newErr(http.StatusBadRequest, "bad_json", "некорректный JSON: "+err.Error())
	}
	return nil
}
