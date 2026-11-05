package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"questline/internal/scenario"
	"questline/internal/store"
)

// Manager поднимает акторы игр по требованию и держит по одному на игру.
type Manager struct {
	ctx    context.Context
	store  store.Store
	clock  Clock
	logger *slog.Logger

	mu      sync.Mutex
	runners map[string]*Runner
}

// NewManager — акторы живут, пока не отменён ctx.
func NewManager(ctx context.Context, st store.Store, clock Clock, logger *slog.Logger) *Manager {
	return &Manager{ctx: ctx, store: st, clock: clock, logger: logger, runners: map[string]*Runner{}}
}

// Get возвращает актор игры, при необходимости загрузив её из хранилища.
// Остановившийся актор (например, после фатальной ошибки) поднимается заново.
func (m *Manager) Get(ctx context.Context, runID string) (*Runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.runners[runID]; ok && r.Err() == nil {
		return r, nil
	}

	run, err := m.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	sv, err := m.store.GetScenarioVersion(ctx, run.ScenarioVersionID)
	if err != nil {
		return nil, fmt.Errorf("сценарий игры: %w", err)
	}
	sc, err := scenario.Parse([]byte(sv.Source))
	if err != nil {
		return nil, fmt.Errorf("сценарий игры: %w", err)
	}
	r, err := Start(m.ctx, Config{
		RunID:    run.ID,
		Scenario: sc,
		Seed:     run.Seed,
		Store:    m.store,
		Clock:    m.clock,
		Logger:   m.logger,
	})
	if err != nil {
		return nil, err
	}
	m.runners[runID] = r
	return r, nil
}
