package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"questaround/internal/core"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// Manager поднимает акторы игр по требованию и держит по одному на игру.
type Manager struct {
	ctx    context.Context
	store  store.Store
	clock  Clock
	logger *slog.Logger

	mu      sync.Mutex
	runners map[string]*managed
}

type managed struct {
	r      *Runner
	cancel context.CancelFunc
}

// NewManager — акторы живут, пока не отменён ctx.
func NewManager(ctx context.Context, st store.Store, clock Clock, logger *slog.Logger) *Manager {
	return &Manager{ctx: ctx, store: st, clock: clock, logger: logger, runners: map[string]*managed{}}
}

// Get возвращает актор игры, при необходимости загрузив её из хранилища.
// Остановившийся актор (например, после фатальной ошибки) поднимается заново.
func (m *Manager) Get(ctx context.Context, runID string) (*Runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mr, ok := m.runners[runID]; ok && mr.r.Err() == nil {
		return mr.r, nil
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
	rctx, cancel := context.WithCancel(m.ctx)
	r, err := Start(rctx, Config{
		RunID:    run.ID,
		Scenario: sc,
		Seed:     run.Seed,
		Store:    m.store,
		Clock:    m.clock,
		Logger:   m.logger,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	m.runners[runID] = &managed{r: r, cancel: cancel}
	return r, nil
}

// stop останавливает актор игры (подписчики SSE отключатся и переподключатся).
// Вызывать под m.mu.
func (m *Manager) stop(runID string) {
	mr, ok := m.runners[runID]
	if !ok {
		return
	}
	mr.cancel()
	<-mr.r.Done()
	delete(m.runners, runID)
}

// ChangeScenario привязывает игру к другой версии сценария. Можно только
// до старта: зарегистрированные команды и игроки сохраняются — события
// регистрации не зависят от уровней. После старта — core.ErrRunStarted.
func (m *Manager) ChangeScenario(ctx context.Context, runID, versionID string) error {
	sv, err := m.store.GetScenarioVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if _, err := scenario.Parse([]byte(sv.Source)); err != nil {
		return fmt.Errorf("сценарий: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.store.GetRun(ctx, runID); err != nil {
		return err
	}
	// Останавливаем актор, чтобы между проверкой и заменой никто не нажал «Старт»;
	// статус берём из журнала — он источник правды.
	m.stop(runID)
	events, err := m.store.Load(ctx, runID)
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.Type == core.EvRunStarted {
			return core.ErrRunStarted // актор поднимется заново при следующем обращении
		}
	}
	return m.store.SetRunScenario(ctx, runID, versionID)
}
