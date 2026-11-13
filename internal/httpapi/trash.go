package httpapi

import (
	"context"
	"net/http"
	"time"

	"questaround/internal/core"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// Корзина игр: удалённая игра скрыта и недоступна игрокам,
// через TrashRetention стирается окончательно вместе с фото.

const defaultTrashRetention = 30 * 24 * time.Hour

func (s *Server) retention() time.Duration {
	if s.cfg.TrashRetention > 0 {
		return s.cfg.TrashRetention
	}
	return defaultTrashRetention
}

func (s *Server) deleteRun(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Runners.Delete(r.Context(), r.PathValue("run"), s.now()); err != nil {
		s.writeError(w, r, err)
		return
	}
	s.log.Info("игра удалена в корзину", "run", r.PathValue("run"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) restoreRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("run")
	if err := s.cfg.Runners.Restore(r.Context(), id); err != nil {
		s.writeError(w, r, err)
		return
	}
	run, err := s.cfg.Store.GetRun(r.Context(), id)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.runView(r, run))
}

func (s *Server) purgeRun(w http.ResponseWriter, r *http.Request) {
	if err := s.purge(r.Context(), r.PathValue("run")); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) purge(ctx context.Context, runID string) error {
	if err := s.cfg.Runners.Purge(ctx, runID); err != nil {
		return err
	}
	if err := s.cfg.Media.DeleteRun(ctx, runID); err != nil {
		s.log.Warn("фото удалённой игры не стёрлись", "run", runID, "err", err)
	}
	s.log.Info("игра стёрта окончательно", "run", runID)
	return nil
}

type trashDTO struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	PublicCode string    `json:"public_code"`
	CreatedAt  time.Time `json:"created_at"`
	DeletedAt  time.Time `json:"deleted_at"`
	PurgeAt    time.Time `json:"purge_at"`
	Teams      int       `json:"teams"`
	Started    bool      `json:"started"`
}

func (s *Server) listTrash(w http.ResponseWriter, r *http.Request) {
	runs, err := s.cfg.Store.ListDeletedRuns(r.Context(), store.DefaultOrg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out := make([]trashDTO, 0, len(runs))
	for _, run := range runs {
		d := trashDTO{ID: run.ID, Title: run.Title, PublicCode: run.PublicCode, CreatedAt: run.CreatedAt,
			DeletedAt: *run.DeletedAt, PurgeAt: run.DeletedAt.Add(s.retention())}
		// Игра в корзине не поднимается — что в ней было, считаем по журналу.
		if evs, err := s.cfg.Store.Load(r.Context(), run.ID); err == nil {
			for _, e := range evs {
				switch e.Type {
				case core.EvTeamRegistered:
					d.Teams++
				case core.EvRunStarted:
					d.Started = true
				}
			}
		}
		if d.Title == "" {
			if sv, err := s.cfg.Store.GetScenarioVersion(r.Context(), run.ScenarioVersionID); err == nil {
				d.Title = sv.Slug
				if sc, err := scenario.Parse([]byte(sv.Source)); err == nil {
					d.Title = sc.Title
				}
			}
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, out)
}

// PurgeExpired стирает игры, пролежавшие в корзине дольше срока.
func (s *Server) PurgeExpired(ctx context.Context) {
	runs, err := s.cfg.Store.ListDeletedRuns(ctx, store.DefaultOrg)
	if err != nil {
		s.log.Warn("очистка корзины", "err", err)
		return
	}
	limit := s.now().Add(-s.retention())
	for _, run := range runs {
		if run.DeletedAt.Before(limit) {
			if err := s.purge(ctx, run.ID); err != nil {
				s.log.Warn("очистка корзины", "run", run.ID, "err", err)
			}
		}
	}
}

// RunJanitor раз в час чистит корзину, пока жив ctx.
func (s *Server) RunJanitor(ctx context.Context) {
	s.PurgeExpired(ctx)
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.PurgeExpired(ctx)
		}
	}
}
