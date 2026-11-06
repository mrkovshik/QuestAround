// Package store — хранение журнала событий и справочников (сценарии, игры).
//
// Две реализации с одним контрактом: Memory (тесты, локальная разработка)
// и Postgres (прод). Контракт проверяется общим набором тестов.
package store

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"questaround/internal/core"
)

var (
	// ErrConflict — журнал изменился параллельно: Seq первого события
	// не продолжает сохранённый журнал. Повторять запись нельзя —
	// нужно перечитать журнал.
	ErrConflict = errors.New("store: конфликт последовательности событий")
	ErrNotFound = errors.New("store: не найдено")
)

// DefaultOrg — организация по умолчанию (создаётся миграцией).
const DefaultOrg = "default"

// EventStore — append-only журнал событий игры.
type EventStore interface {
	// Append атомарно дописывает события. events[0].Seq должен быть
	// ровно на 1 больше последнего сохранённого, Seq идут подряд.
	Append(ctx context.Context, runID string, events []core.Event) error
	// Load возвращает журнал по возрастанию Seq.
	Load(ctx context.Context, runID string) ([]core.Event, error)
}

// ScenarioVersion — неизменяемая версия исходника сценария.
type ScenarioVersion struct {
	ID        string
	OrgID     string
	Slug      string // id сценария из YAML
	Version   int    // 1, 2, 3… в пределах (OrgID, Slug)
	Source    string // YAML как загружен
	CreatedAt time.Time
}

// Run — проведение игры.
type Run struct {
	ID                string
	OrgID             string
	ScenarioVersionID string
	Title             string
	PublicCode        string // короткий код для ссылки /g/{code}
	Seed              uint64
	CreatedAt         time.Time
}

// NewRun — параметры создания игры; ID, код и seed генерирует хранилище.
type NewRun struct {
	OrgID             string
	ScenarioVersionID string
	Title             string
}

// Store — всё хранилище.
type Store interface {
	EventStore

	// SaveScenarioVersion сохраняет новую версию сценария (Version = последняя + 1).
	SaveScenarioVersion(ctx context.Context, orgID, slug, source string) (ScenarioVersion, error)
	GetScenarioVersion(ctx context.Context, id string) (ScenarioVersion, error)
	// ListScenarioVersions — все версии организации, новые сверху, без исходника.
	ListScenarioVersions(ctx context.Context, orgID string) ([]ScenarioVersion, error)

	CreateRun(ctx context.Context, r NewRun) (Run, error)
	GetRun(ctx context.Context, id string) (Run, error)
	GetRunByCode(ctx context.Context, code string) (Run, error)
	// SetRunScenario меняет версию сценария игры (проверка «до старта» — на вызывающем).
	SetRunScenario(ctx context.Context, runID, scenarioVersionID string) error
	ListRuns(ctx context.Context, orgID string) ([]Run, error)
}

// checkSeq проверяет, что events продолжают журнал, где последний Seq = last.
func checkSeq(last int64, events []core.Event) error {
	for i, e := range events {
		if e.Seq != last+int64(i)+1 {
			return ErrConflict
		}
	}
	return nil
}

const idAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// newID — случайный идентификатор (16 символов, ~80 бит).
func newID() string { return randString(16, idAlphabet) }

// codeAlphabet — без похожих символов (0/O, 1/I/L), чтобы код легко диктовать.
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newPublicCode() string { return randString(6, codeAlphabet) }

func randString(n int, alphabet string) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand не падает на поддерживаемых ОС
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func newSeed() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	var s uint64
	for _, x := range b {
		s = s<<8 | uint64(x)
	}
	return s
}
