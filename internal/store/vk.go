package store

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// VKBinding — пользователь VK играет игроком PlayerID в игре RunID.
type VKBinding struct {
	UserID    int64
	RunID     string
	PlayerID  string
	CreatedAt time.Time
}

// VKStore — данные VK-бота. Привязка у пользователя одна: новая заменяет старую.
type VKStore interface {
	BindVK(ctx context.Context, b VKBinding) error
	// GetVK — привязка пользователя; ErrNotFound, если нет или игра в корзине.
	GetVK(ctx context.Context, userID int64) (VKBinding, error)
	UnbindVK(ctx context.Context, userID int64) error
	// ListVK — все привязки к живым играм (не в корзине).
	ListVK(ctx context.Context) ([]VKBinding, error)

	// VKCursor — Seq последнего события игры, по которому бот уже разослал
	// уведомления; 0 и ok=false — курсора ещё нет.
	VKCursor(ctx context.Context, runID string) (seq int64, ok bool, err error)
	SetVKCursor(ctx context.Context, runID string, seq int64) error
}

// --- Memory ---

func (m *Memory) BindVK(_ context.Context, b VKBinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[b.RunID]; !ok {
		return ErrNotFound
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	m.vk[b.UserID] = b
	return nil
}

func (m *Memory) GetVK(_ context.Context, userID int64) (VKBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.vk[userID]
	if !ok {
		return VKBinding{}, ErrNotFound
	}
	if r, ok := m.runs[b.RunID]; !ok || r.DeletedAt != nil {
		return VKBinding{}, ErrNotFound
	}
	return b, nil
}

func (m *Memory) UnbindVK(_ context.Context, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vk, userID)
	return nil
}

func (m *Memory) ListVK(_ context.Context) ([]VKBinding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []VKBinding
	for _, b := range m.vk {
		if r, ok := m.runs[b.RunID]; ok && r.DeletedAt == nil {
			out = append(out, b)
		}
	}
	slices.SortFunc(out, func(a, b VKBinding) int { return cmp.Compare(a.UserID, b.UserID) })
	return out, nil
}

func (m *Memory) VKCursor(_ context.Context, runID string) (int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq, ok := m.vkCursors[runID]
	return seq, ok, nil
}

func (m *Memory) SetVKCursor(_ context.Context, runID string, seq int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[runID]; !ok {
		return ErrNotFound
	}
	m.vkCursors[runID] = seq
	return nil
}

// purgeVK — часть PurgeRun (вызывается под m.mu).
func (m *Memory) purgeVK(runID string) {
	for id, b := range m.vk {
		if b.RunID == runID {
			delete(m.vk, id)
		}
	}
	delete(m.vkCursors, runID)
}

// --- Postgres ---

func (p *Postgres) BindVK(ctx context.Context, b VKBinding) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO vk_players (user_id, run_id, player_id) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET run_id = EXCLUDED.run_id, player_id = EXCLUDED.player_id, created_at = now()`,
		b.UserID, b.RunID, b.PlayerID)
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

func (p *Postgres) GetVK(ctx context.Context, userID int64) (VKBinding, error) {
	var b VKBinding
	err := p.pool.QueryRow(ctx, `
		SELECT v.user_id, v.run_id, v.player_id, v.created_at
		FROM vk_players v JOIN runs r ON r.id = v.run_id
		WHERE v.user_id = $1 AND r.deleted_at IS NULL`, userID).
		Scan(&b.UserID, &b.RunID, &b.PlayerID, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

func (p *Postgres) UnbindVK(ctx context.Context, userID int64) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM vk_players WHERE user_id = $1`, userID)
	return err
}

func (p *Postgres) ListVK(ctx context.Context) ([]VKBinding, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT v.user_id, v.run_id, v.player_id, v.created_at
		FROM vk_players v JOIN runs r ON r.id = v.run_id
		WHERE r.deleted_at IS NULL ORDER BY v.user_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (VKBinding, error) {
		var b VKBinding
		err := row.Scan(&b.UserID, &b.RunID, &b.PlayerID, &b.CreatedAt)
		return b, err
	})
}

func (p *Postgres) VKCursor(ctx context.Context, runID string) (int64, bool, error) {
	var seq int64
	err := p.pool.QueryRow(ctx, `SELECT seq FROM vk_cursors WHERE run_id = $1`, runID).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return seq, err == nil, err
}

func (p *Postgres) SetVKCursor(ctx context.Context, runID string, seq int64) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO vk_cursors (run_id, seq) VALUES ($1, $2)
		ON CONFLICT (run_id) DO UPDATE SET seq = EXCLUDED.seq`, runID, seq)
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}
