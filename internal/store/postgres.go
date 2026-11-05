package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"questline/internal/core"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Postgres — хранилище в PostgreSQL.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres подключается к базе и применяет миграции.
func OpenPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("миграции: %w", err)
	}
	return p, nil
}

func (p *Postgres) Close() { p.pool.Close() }

// migrate применяет migrations/NNN_*.sql по порядку, каждую в своей
// транзакции. Advisory lock защищает от параллельного запуска нескольких инстансов.
func (p *Postgres) migrate(ctx context.Context) error {
	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	slices.Sort(files)

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	const lockID = 7_441_002 // произвольная константа приложения
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version int PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	for _, f := range files {
		name := strings.TrimPrefix(f, "migrations/")
		num, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(num)
		if !ok || err != nil {
			return fmt.Errorf("имя миграции %q: ожидалось NNN_описание.sql", name)
		}
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrationsFS.ReadFile(f)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (p *Postgres) Append(ctx context.Context, runID string, events []core.Event) error {
	if len(events) == 0 {
		return nil
	}
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var last int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) FROM events WHERE run_id = $1`, runID).Scan(&last); err != nil {
			return err
		}
		if err := checkSeq(last, events); err != nil {
			return err
		}
		batch := &pgx.Batch{}
		for _, e := range events {
			payload, err := json.Marshal(e.Payload)
			if err != nil {
				return err
			}
			batch.Queue(`INSERT INTO events (run_id, seq, type, at, team_id, player_id, client_event_id, payload)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				runID, e.Seq, string(e.Type), e.At, e.TeamID, e.PlayerID, e.ClientEventID, payload)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
	// Параллельная запись успела раньше — PRIMARY KEY (run_id, seq).
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (p *Postgres) Load(ctx context.Context, runID string) ([]core.Event, error) {
	rows, err := p.pool.Query(ctx, `SELECT seq, type, at, team_id, player_id, client_event_id, payload
		FROM events WHERE run_id = $1 ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []core.Event
	for rows.Next() {
		var (
			e       core.Event
			typ     string
			payload []byte
		)
		if err := rows.Scan(&e.Seq, &typ, &e.At, &e.TeamID, &e.PlayerID, &e.ClientEventID, &payload); err != nil {
			return nil, err
		}
		e.Type = core.EventType(typ)
		e.At = e.At.UTC()
		if e.Payload, err = core.DecodePayload(e.Type, payload); err != nil {
			return nil, fmt.Errorf("событие %d: %w", e.Seq, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *Postgres) SaveScenarioVersion(ctx context.Context, orgID, slug, source string) (ScenarioVersion, error) {
	sv := ScenarioVersion{ID: newID(), OrgID: orgID, Slug: slug, Source: source}
	// Номер версии считаем в том же запросе; при гонке двух загрузок
	// UNIQUE (org_id, slug, version) отклонит вторую — повторяем.
	for range 3 {
		err := p.pool.QueryRow(ctx, `
			INSERT INTO scenario_versions (id, org_id, slug, version, source)
			SELECT $1, $2, $3, COALESCE(MAX(version), 0) + 1, $4
			FROM scenario_versions WHERE org_id = $2 AND slug = $3
			RETURNING version, created_at`,
			sv.ID, orgID, slug, source).Scan(&sv.Version, &sv.CreatedAt)
		if isUniqueViolation(err) {
			continue
		}
		sv.CreatedAt = sv.CreatedAt.UTC()
		return sv, err
	}
	return ScenarioVersion{}, ErrConflict
}

func (p *Postgres) GetScenarioVersion(ctx context.Context, id string) (ScenarioVersion, error) {
	var sv ScenarioVersion
	err := p.pool.QueryRow(ctx, `SELECT id, org_id, slug, version, source, created_at
		FROM scenario_versions WHERE id = $1`, id).
		Scan(&sv.ID, &sv.OrgID, &sv.Slug, &sv.Version, &sv.Source, &sv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return sv, ErrNotFound
	}
	sv.CreatedAt = sv.CreatedAt.UTC()
	return sv, err
}

const runColumns = `id, org_id, scenario_version_id, title, public_code, seed, created_at`

func scanRun(row pgx.Row) (Run, error) {
	var (
		r    Run
		seed int64
	)
	err := row.Scan(&r.ID, &r.OrgID, &r.ScenarioVersionID, &r.Title, &r.PublicCode, &seed, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	r.Seed = uint64(seed)
	r.CreatedAt = r.CreatedAt.UTC()
	return r, err
}

func (p *Postgres) CreateRun(ctx context.Context, nr NewRun) (Run, error) {
	r := Run{ID: newID(), OrgID: nr.OrgID, ScenarioVersionID: nr.ScenarioVersionID, Title: nr.Title, Seed: newSeed()}
	for range 5 { // коллизия кода маловероятна, но возможна
		r.PublicCode = newPublicCode()
		err := p.pool.QueryRow(ctx, `INSERT INTO runs (id, org_id, scenario_version_id, title, public_code, seed)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at`,
			r.ID, r.OrgID, r.ScenarioVersionID, r.Title, r.PublicCode, int64(r.Seed)).Scan(&r.CreatedAt)
		var pgErr *pgconn.PgError
		switch {
		case isUniqueViolation(err):
			continue
		case errors.As(err, &pgErr) && pgErr.Code == "23503": // foreign key
			return Run{}, ErrNotFound
		case err != nil:
			return Run{}, err
		}
		r.CreatedAt = r.CreatedAt.UTC()
		return r, nil
	}
	return Run{}, ErrConflict
}

func (p *Postgres) GetRun(ctx context.Context, id string) (Run, error) {
	return scanRun(p.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE id = $1`, id))
}

func (p *Postgres) GetRunByCode(ctx context.Context, code string) (Run, error) {
	return scanRun(p.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM runs WHERE public_code = $1`, code))
}

func (p *Postgres) ListRuns(ctx context.Context, orgID string) ([]Run, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+runColumns+` FROM runs WHERE org_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
