package store

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"questline/internal/core"
)

// Memory — хранилище в памяти. Для тестов и локального запуска без Postgres.
// События хранятся в виде JSON, как в Postgres, — чтобы ловить те же
// проблемы сериализации.
type Memory struct {
	mu        sync.Mutex
	events    map[string][][]byte
	scenarios map[string]ScenarioVersion
	runs      map[string]Run
}

func NewMemory() *Memory {
	return &Memory{
		events:    map[string][][]byte{},
		scenarios: map[string]ScenarioVersion{},
		runs:      map[string]Run{},
	}
}

func (m *Memory) Append(_ context.Context, runID string, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	encoded := make([][]byte, len(events))
	for i, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		encoded[i] = b
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := checkSeq(int64(len(m.events[runID])), events); err != nil {
		return err
	}
	m.events[runID] = append(m.events[runID], encoded...)
	return nil
}

func (m *Memory) Load(_ context.Context, runID string) ([]core.Event, error) {
	m.mu.Lock()
	raw := slices.Clone(m.events[runID])
	m.mu.Unlock()
	out := make([]core.Event, len(raw))
	for i, b := range raw {
		if err := json.Unmarshal(b, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m *Memory) SaveScenarioVersion(_ context.Context, orgID, slug, source string) (ScenarioVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := 0
	for _, s := range m.scenarios {
		if s.OrgID == orgID && s.Slug == slug && s.Version > v {
			v = s.Version
		}
	}
	sv := ScenarioVersion{
		ID: newID(), OrgID: orgID, Slug: slug, Version: v + 1,
		Source: source, CreatedAt: now(),
	}
	m.scenarios[sv.ID] = sv
	return sv, nil
}

func (m *Memory) GetScenarioVersion(_ context.Context, id string) (ScenarioVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sv, ok := m.scenarios[id]
	if !ok {
		return ScenarioVersion{}, ErrNotFound
	}
	return sv, nil
}

func (m *Memory) ListScenarioVersions(_ context.Context, orgID string) ([]ScenarioVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ScenarioVersion
	for _, sv := range m.scenarios {
		if sv.OrgID == orgID {
			sv.Source = ""
			out = append(out, sv)
		}
	}
	slices.SortFunc(out, func(a, b ScenarioVersion) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return b.Version - a.Version
	})
	return out, nil
}

func (m *Memory) CreateRun(_ context.Context, nr NewRun) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.scenarios[nr.ScenarioVersionID]; !ok {
		return Run{}, ErrNotFound
	}
	r := Run{
		ID: newID(), OrgID: nr.OrgID, ScenarioVersionID: nr.ScenarioVersionID,
		Title: nr.Title, Seed: newSeed(), CreatedAt: now(),
	}
	for {
		r.PublicCode = newPublicCode()
		if !m.codeTaken(r.PublicCode) {
			break
		}
	}
	m.runs[r.ID] = r
	return r, nil
}

func (m *Memory) codeTaken(code string) bool {
	for _, r := range m.runs {
		if r.PublicCode == code {
			return true
		}
	}
	return false
}

func (m *Memory) GetRun(_ context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return r, nil
}

func (m *Memory) GetRunByCode(_ context.Context, code string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.PublicCode == code {
			return r, nil
		}
	}
	return Run{}, ErrNotFound
}

func (m *Memory) ListRuns(_ context.Context, orgID string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, r := range m.runs {
		if r.OrgID == orgID {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Run) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// now — время с точностью Postgres (микросекунды), чтобы реализации совпадали.
func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
