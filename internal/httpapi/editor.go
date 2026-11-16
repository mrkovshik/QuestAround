package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"questaround/internal/routes"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// Конструктор сценариев: проверка без сохранения и превью маршрутов.
// Сохранение — тем же POST /admin/v1/scenarios, что и загрузка YAML.

const maxPreviewTeams = 50

// parseUploaded читает YAML из тела и разбирает его с теми же проверками,
// что и при сохранении новой версии.
func (s *Server) parseUploaded(w http.ResponseWriter, r *http.Request) ([]byte, *scenario.Scenario, error) {
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxScenarioSize))
	if err != nil {
		return nil, nil, err
	}
	sc, err := scenario.ParseWith(src, scenario.Options{Strict: true, MediaExists: func(n string) bool {
		_, ok := s.cfg.Media.AssetKey(n)
		return ok
	}})
	if err != nil {
		var se scenario.Errors
		if !errors.As(err, &se) {
			// синтаксис/структура: одна ошибка без номера строки в нашем формате
			err = scenario.Errors{{Msg: err.Error()}}
		}
		return nil, nil, err
	}
	return src, sc, nil
}

type validateDTO struct {
	OK     bool   `json:"ok"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Levels int    `json:"levels"`
	// NextVersion — какой номер получит версия при сохранении (1 — новый сценарий).
	NextVersion int `json:"next_version"`
}

// validateScenario — POST /admin/v1/scenarios/validate: как загрузка, но без сохранения.
func (s *Server) validateScenario(w http.ResponseWriter, r *http.Request) {
	_, sc, err := s.parseUploaded(w, r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	next := 1
	list, err := s.cfg.Store.ListScenarioVersions(r.Context(), store.DefaultOrg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	for _, sv := range list {
		if sv.Slug == sc.ID && sv.Version >= next {
			next = sv.Version + 1
		}
	}
	writeJSON(w, http.StatusOK, validateDTO{OK: true, ID: sc.ID, Title: sc.Title, Levels: len(sc.Levels), NextVersion: next})
}

type routesPreviewDTO struct {
	Mode   string     `json:"mode"`
	Routes [][]string `json:"routes"`
}

// previewRoutes — POST /admin/v1/scenarios/routes?teams=N: маршруты команд,
// как их построит игра при старте. Для random — пример с фиксированным seed.
func (s *Server) previewRoutes(w http.ResponseWriter, r *http.Request) {
	teams, err := strconv.Atoi(r.URL.Query().Get("teams"))
	if err != nil || teams < 1 || teams > maxPreviewTeams {
		s.writeError(w, r, newErr(http.StatusBadRequest, "bad_request", "teams: число от 1 до "+strconv.Itoa(maxPreviewTeams)))
		return
	}
	_, sc, err := s.parseUploaded(w, r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if sc.Flow.Mode != scenario.FlowLinear {
		writeJSON(w, http.StatusOK, routesPreviewDTO{Mode: string(sc.Flow.Mode)})
		return
	}
	rs, err := routes.Build(sc.RouteLevels(), routes.Options{
		Mode:     sc.Flow.Routes,
		Teams:    teams,
		Seed:     1,
		Explicit: sc.Flow.RouteList,
	})
	if err != nil {
		s.writeError(w, r, newErr(http.StatusBadRequest, "bad_request", "маршруты: "+err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, routesPreviewDTO{Mode: string(sc.Flow.Routes), Routes: rs})
}
