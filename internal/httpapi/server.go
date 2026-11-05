// Package httpapi — HTTP API игрока (/api/v1) и ведущего (/admin/v1),
// SSE-стримы изменений и загрузка фото.
package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"questline/internal/runner"
	"questline/internal/store"
)

// Config — зависимости сервера.
type Config struct {
	Store      store.Store
	Runners    *runner.Manager
	Media      MediaStore
	AdminToken string
	Logger     *slog.Logger
	Clock      runner.Clock // для расчёта времени в ответах; nil — системное
	// SSEKeepAlive — интервал пустых комментариев в SSE (по умолчанию 25s).
	SSEKeepAlive time.Duration
	// Static — фронтенд (PWA); отдаётся на все пути вне API. nil — не отдаётся.
	Static http.Handler
}

type Server struct {
	cfg   Config
	log   *slog.Logger
	clock runner.Clock
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = runner.RealClock{}
	}
	if cfg.SSEKeepAlive == 0 {
		cfg.SSEKeepAlive = 25 * time.Second
	}
	return &Server{cfg: cfg, log: cfg.Logger, clock: cfg.Clock}
}

func (s *Server) now() time.Time { return s.clock.Now().UTC().Truncate(time.Microsecond) }

// Handler — все маршруты.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Публичные: инфо об игре, регистрация.
	mux.HandleFunc("GET /api/v1/games/{code}", s.gameInfo)
	mux.HandleFunc("POST /api/v1/games/{code}/teams", s.createTeam)
	mux.HandleFunc("POST /api/v1/games/{code}/join", s.joinTeam)

	// Игрок (Bearer-токен игрока).
	p := func(h playerHandler) http.Handler { return s.playerAuth(h) }
	mux.Handle("GET /api/v1/state", p(s.state))
	mux.Handle("POST /api/v1/answers", p(s.submitAnswer))
	mux.Handle("POST /api/v1/photos", p(s.submitPhoto))
	mux.Handle("POST /api/v1/levels/{level}/hints/{idx}", p(s.requestHint))
	mux.Handle("GET /api/v1/leaderboard", p(s.leaderboard))
	mux.Handle("GET /api/v1/stream", p(s.playerStream))
	mux.Handle("GET /api/v1/team", p(s.team))
	mux.Handle("PATCH /api/v1/team", p(s.renameTeam))
	mux.Handle("DELETE /api/v1/team/players/{player}", p(s.removePlayer))
	mux.Handle("POST /api/v1/team/invite/rotate", p(s.rotateInvite))
	mux.Handle("POST /api/v1/team/captain", p(s.transferCaptain))
	mux.Handle("POST /api/v1/team/withdraw", p(s.withdrawTeam))

	// Ведущий (Bearer-токен ведущего).
	a := func(h http.HandlerFunc) http.Handler { return s.adminAuth(h) }
	mux.Handle("POST /admin/v1/scenarios", a(s.uploadScenario))
	mux.Handle("GET /admin/v1/scenarios", a(s.listScenarios))
	mux.Handle("GET /admin/v1/scenarios/{id}", a(s.getScenario))
	mux.Handle("POST /admin/v1/runs", a(s.createRun))
	mux.Handle("GET /admin/v1/runs", a(s.listRuns))
	mux.Handle("GET /admin/v1/runs/{run}", a(s.getRun))
	mux.Handle("PUT /admin/v1/runs/{run}/registration", a(s.setRegistration))
	mux.Handle("POST /admin/v1/runs/{run}/teams", a(s.hostCreateTeam))
	mux.Handle("PATCH /admin/v1/runs/{run}/teams/{team}", a(s.hostRenameTeam))
	mux.Handle("POST /admin/v1/runs/{run}/teams/{team}/withdraw", a(s.hostWithdrawTeam))
	mux.Handle("POST /admin/v1/runs/{run}/teams/{team}/start", a(s.startTeam))
	mux.Handle("POST /admin/v1/runs/{run}/teams/{team}/override", a(s.override))
	mux.Handle("POST /admin/v1/runs/{run}/teams/{team}/answers", a(s.hostSubmitAnswer))
	mux.Handle("POST /admin/v1/runs/{run}/start", a(s.startRun))
	mux.Handle("POST /admin/v1/runs/{run}/finish", a(s.finishRun))
	mux.Handle("GET /admin/v1/runs/{run}/dashboard", a(s.dashboard))
	mux.Handle("GET /admin/v1/runs/{run}/stream", a(s.adminStream))
	mux.Handle("GET /admin/v1/runs/{run}/moderation", a(s.moderationQueue))
	mux.Handle("POST /admin/v1/runs/{run}/moderation/{seq}", a(s.moderate))
	mux.Handle("PUT /admin/v1/runs/{run}/leaderboard", a(s.setLeaderboard))
	mux.Handle("POST /admin/v1/runs/{run}/broadcast", a(s.broadcast))
	mux.Handle("GET /admin/v1/runs/{run}/events", a(s.events))
	mux.Handle("GET /admin/v1/media/{key...}", a(s.media))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	notFound := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "not_found", Message: "нет такого метода API"})
	}
	mux.HandleFunc("/api/", notFound)
	mux.HandleFunc("/admin/v1/", notFound)
	if s.cfg.Static != nil {
		mux.Handle("/", s.cfg.Static)
	}

	return s.recoverer(s.accessLog(mux))
}

func (s *Server) adminAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := bearer(r)
		if s.cfg.AdminToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.AdminToken)) != 1 {
			s.writeError(w, r, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(t)
	}
	// EventSource в браузере не умеет заголовки — токен в query для SSE.
	if strings.HasSuffix(r.URL.Path, "/stream") {
		return r.URL.Query().Get("token")
	}
	return ""
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic", "value", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal", Message: "внутренняя ошибка сервера"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap — чтобы http.ResponseController видел Flush и дедлайны (нужно SSE).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", time.Since(start))
	})
}
