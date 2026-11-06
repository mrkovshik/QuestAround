// Package scenario описывает YAML-формат сценария игры: типы, парсинг,
// значения по умолчанию и валидацию с номерами строк в ошибках.
package scenario

import "questaround/internal/routes"

// SchemaVersion — поддерживаемая версия схемы сценария.
const SchemaVersion = 1

type Scenario struct {
	Version      int          `yaml:"version"`
	ID           string       `yaml:"id"`
	Title        string       `yaml:"title"`
	Description  string       `yaml:"description"`
	Participants Participants `yaml:"participants"`
	Timing       Timing       `yaml:"timing"`
	Start        Start        `yaml:"start"`
	Flow         Flow         `yaml:"flow"`
	Scoring      Scoring      `yaml:"scoring"`
	Leaderboard  Leaderboard  `yaml:"leaderboard"`
	Defaults     Defaults     `yaml:"defaults"`
	Levels       []Level      `yaml:"levels"`
}

// --- участники ---

type ParticipantMode string

const (
	ParticipantsTeam ParticipantMode = "team"
	ParticipantsSolo ParticipantMode = "solo"
)

type HintRequests string

const (
	HintRequestsCaptain HintRequests = "captain"
	HintRequestsAnyone  HintRequests = "anyone"
)

type Participants struct {
	Mode         ParticipantMode `yaml:"mode"`
	TeamSize     TeamSize        `yaml:"team_size"`
	HintRequests HintRequests    `yaml:"hint_requests"` // кто может брать штрафные подсказки
}

type TeamSize struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// --- время ---

type TimingMode string

const (
	TimingAsync TimingMode = "async"
	TimingSync  TimingMode = "sync" // синхронные раунды (квиз) — после MVP
)

type Timing struct {
	Mode      TimingMode `yaml:"mode"`
	TimeLimit Duration   `yaml:"time_limit"` // от старта команды; 0 — без лимита
}

// --- старт ---

type StartMode string

const (
	StartSimultaneous StartMode = "simultaneous"
	StartStaggered    StartMode = "staggered"
	StartManual       StartMode = "manual"
)

type StartOrder string

const (
	OrderRegistration StartOrder = "registration"
	OrderRandom       StartOrder = "random"
	OrderManual       StartOrder = "manual"
)

type Start struct {
	Mode     StartMode  `yaml:"mode"`
	Interval Duration   `yaml:"interval"` // для staggered
	Order    StartOrder `yaml:"order"`
}

// --- flow ---

type FlowMode string

const (
	FlowLinear FlowMode = "linear"
	FlowStorm  FlowMode = "storm"
	FlowGraph  FlowMode = "graph"
)

type Flow struct {
	Mode      FlowMode    `yaml:"mode"`
	Routes    routes.Mode `yaml:"routes"`     // только для linear
	RouteList [][]string  `yaml:"route_list"` // для routes: explicit
}

// --- очки ---

type ScoringMode string

const (
	ScoringTime   ScoringMode = "time"
	ScoringPoints ScoringMode = "points"
	ScoringMixed  ScoringMode = "mixed"
)

type Scoring struct {
	Mode ScoringMode `yaml:"mode"`
}

// --- лидерборд ---

type Visibility string

const (
	VisibilityAlways      Visibility = "always"
	VisibilityHidden      Visibility = "hidden"
	VisibilityAfterFinish Visibility = "after_finish"
	VisibilityHost        Visibility = "host"
)

type Leaderboard struct {
	Visibility      Visibility `yaml:"visibility"`
	FreezeBeforeEnd Duration   `yaml:"freeze_before_end"`
	Columns         []string   `yaml:"columns"`
}

// --- значения по умолчанию для уровней ---

type Defaults struct {
	// HintAfter — расписание автоподсказок: i-я автоподсказка уровня без
	// явного after открывается через HintAfter[i] после открытия уровня.
	HintAfter       []Duration `yaml:"hint_after"`
	RateLimit       *RateLimit `yaml:"rate_limit"`
	AnswerNormalize []string   `yaml:"answer_normalize"`
}

type RateLimit struct {
	WrongAnswers int      `yaml:"wrong_answers"`
	Per          Duration `yaml:"per"`
}

// --- уровни ---

type Level struct {
	ID      string     `yaml:"id"`
	Title   string     `yaml:"title"`
	Task    string     `yaml:"task"`
	Media   []string   `yaml:"media"`
	Points  int        `yaml:"points"`
	Pin     routes.Pin `yaml:"pin"`
	Chain   string     `yaml:"chain"`
	Require Require    `yaml:"require"` // сколько ответов нужно для зачёта
	Answers []Answer   `yaml:"answers"`
	Hints   []Hint     `yaml:"hints"`
	Bonuses []Bonus    `yaml:"bonuses"`
	Unlock  *Unlock    `yaml:"unlock"` // только для flow: graph
}

type AnswerType string

const (
	AnswerCode   AnswerType = "code"
	AnswerRegex  AnswerType = "regex"
	AnswerChoice AnswerType = "choice"
	AnswerGeo    AnswerType = "geo"
	AnswerPhoto  AnswerType = "photo"
	AnswerText   AnswerType = "text"
)

// NeedsModeration — ответ проверяет ведущий вручную.
func (t AnswerType) NeedsModeration() bool {
	return t == AnswerPhoto || t == AnswerText
}

type Answer struct {
	Type AnswerType `yaml:"type"`

	Values  []string `yaml:"values"`  // code
	Pattern string   `yaml:"pattern"` // regex
	Options []string `yaml:"options"` // choice
	Correct string   `yaml:"correct"` // choice: значение из options

	Point        []float64 `yaml:"point"`          // geo: [lat, lng]
	RadiusM      float64   `yaml:"radius_m"`       // geo
	MaxAccuracyM float64   `yaml:"max_accuracy_m"` // geo, 0 — не проверять
}

type Hint struct {
	After     Duration `yaml:"after"`      // через сколько после открытия уровня
	OnRequest bool     `yaml:"on_request"` // открывается по запросу
	Penalty   *Penalty `yaml:"penalty"`    // только для on_request
	Text      string   `yaml:"text"`
}

// IsAuto — подсказка по таймеру (явному или из defaults.hint_after).
func (h Hint) IsAuto() bool { return !h.OnRequest }

type Penalty struct {
	Time   Duration `yaml:"time"`
	Points int      `yaml:"points"`
}

type Bonus struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Answer Answer `yaml:"answer"`
	Points int    `yaml:"points"`
}

// Unlock — условие открытия уровня в flow: graph. Задаётся ровно одна форма.
type Unlock struct {
	AllOf []string `yaml:"all_of"`
	AnyOf []string `yaml:"any_of"`
	Count int      `yaml:"count"`
	Of    []string `yaml:"of"`
}

// Deps — все уровни, на которые ссылается условие.
func (u *Unlock) Deps() []string {
	if u == nil {
		return nil
	}
	out := append([]string(nil), u.AllOf...)
	out = append(out, u.AnyOf...)
	return append(out, u.Of...)
}

// RouteLevels — уровни в формате пакета routes.
func (s *Scenario) RouteLevels() []routes.Level {
	out := make([]routes.Level, len(s.Levels))
	for i, l := range s.Levels {
		out[i] = routes.Level{ID: l.ID, Pin: l.Pin, Chain: l.Chain}
	}
	return out
}

// Level возвращает уровень по ID.
func (s *Scenario) Level(id string) (*Level, bool) {
	for i := range s.Levels {
		if s.Levels[i].ID == id {
			return &s.Levels[i], true
		}
	}
	return nil, false
}
