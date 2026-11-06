package core

import (
	"cmp"
	"slices"
	"time"

	"questaround/internal/scenario"
)

// AnswerKind — что прислал игрок.
type AnswerKind string

const (
	KindCode  AnswerKind = "code"  // набранный текст: проверяется по code/regex/choice
	KindGeo   AnswerKind = "geo"   // координаты
	KindPhoto AnswerKind = "photo" // фото → модерация
	KindText  AnswerKind = "text"  // свободный текст → модерация
)

// Result — итог проверки ответа.
type Result string

const (
	ResultCorrect    Result = "correct"
	ResultWrong      Result = "wrong"
	ResultDuplicate  Result = "duplicate"  // уже засчитано раньше
	ResultPending    Result = "pending"    // ждёт модерации
	ResultInaccurate Result = "inaccurate" // GPS слишком неточный
)

type OverrideAction string

const (
	OverrideCompleteLevel OverrideAction = "complete_level"
	OverrideOpenHint      OverrideAction = "open_hint"
	OverrideAdjustTime    OverrideAction = "adjust_time"
	OverrideAdjustPoints  OverrideAction = "adjust_points"
	OverrideFinishTeam    OverrideAction = "finish_team"
)

type RunStatus string

const (
	RunStatusPending  RunStatus = "pending"
	RunStatusRunning  RunStatus = "running"
	RunStatusFinished RunStatus = "finished"
)

type TeamStatus string

const (
	TeamStatusRegistered TeamStatus = "registered"
	TeamStatusScheduled  TeamStatus = "scheduled"
	TeamStatusPlaying    TeamStatus = "playing"
	TeamStatusFinished   TeamStatus = "finished"
	TeamStatusWithdrawn  TeamStatus = "withdrawn" // снялась до старта
)

// Game — состояние одного Run: свёртка журнала событий.
// Не потокобезопасно: доступ из одной горутины (актор Run).
type Game struct {
	RunID    string
	Scenario *scenario.Scenario
	Seed     uint64 // для случайного порядка старта и routes: random

	Status     RunStatus
	StartedAt  time.Time
	FinishedAt time.Time

	Teams        map[string]*Team
	TeamOrder    []string // порядок регистрации
	Players      map[string]*Player
	Registration Registration

	Leaderboard LeaderboardState
	Messages    []Message

	seq      int64
	levelIdx map[string]int
	seen     map[string]map[string]AnswerResult // team → client_event_id → результат
	pending  map[int64]*PendingItem             // модерация: Seq → заявка
	invites  map[string]string                  // инвайт-код → команда
}

// Registration — настройки регистрации проведения (не сценария).
type Registration struct {
	Closed   bool // по умолчанию открыта
	MaxTeams int  // 0 — без ограничения
}

// Player — игрок. Токен хранится только в виде хэша.
type Player struct {
	ID        string
	TeamID    string
	Name      string
	TokenHash string
	JoinedAt  time.Time
	Removed   bool
}

type LeaderboardState struct {
	Override *bool // ручное переключение ведущим; nil — по сценарию
	Frozen   bool
	FrozenAt time.Time
	Snapshot []Standing
}

type Message struct {
	At   time.Time
	Text string
}

type Team struct {
	ID         string
	Name       string
	Status     TeamStatus
	CaptainID  string
	InviteCode string
	Contact    string   // контакт капитана для ведущего
	PlayerIDs  []string // в порядке вступления, включая удалённых

	Slot         int
	ScheduledAt  time.Time
	Route        []string // для linear
	StartedAt    time.Time
	FinishedAt   time.Time
	FinishReason string

	Levels        map[string]*LevelState // только открытые
	Bonuses       map[string]bool        // засчитанные бонусы
	PendingBonus  map[string]int64       // бонус → Seq заявки на модерации
	Points        int
	PenaltyTime   time.Duration
	LevelsDone    int
	LastActivity  time.Time
	wrongAttempts []time.Time // для rate limit
}

type LevelState struct {
	OpenedAt    time.Time
	CompletedAt time.Time
	Solved      []bool        // по индексу answers
	Pending     map[int]int64 // answer idx → Seq заявки на модерации
	Hints       []bool        // открытые подсказки
}

func (l *LevelState) IsOpen() bool { return l != nil && l.CompletedAt.IsZero() }

func (l *LevelState) SolvedCount() int {
	n := 0
	for _, s := range l.Solved {
		if s {
			n++
		}
	}
	return n
}

// PendingItem — ответ, ожидающий решения модератора.
type PendingItem struct {
	Seq       int64
	At        time.Time
	TeamID    string
	PlayerID  string
	LevelID   string
	AnswerIdx int    // -1, если это бонус
	BonusID   string // для бонуса
	Kind      AnswerKind
	Value     string
	MediaKey  string
}

// AnswerResult — что вернуть игроку на попытку ответа.
type AnswerResult struct {
	Seq       int64
	Result    Result
	LevelID   string
	AnswerIdx *int
	BonusID   string
}

// NewGame создаёт пустую игру по провалидированному сценарию.
func NewGame(runID string, sc *scenario.Scenario, seed uint64) *Game {
	g := &Game{
		RunID:    runID,
		Scenario: sc,
		Seed:     seed,
		Status:   RunStatusPending,
		Teams:    map[string]*Team{},
		Players:  map[string]*Player{},
		invites:  map[string]string{},
		levelIdx: map[string]int{},
		seen:     map[string]map[string]AnswerResult{},
		pending:  map[int64]*PendingItem{},
	}
	for i, l := range sc.Levels {
		g.levelIdx[l.ID] = i
	}
	return g
}

// Seq — номер последнего применённого события.
func (g *Game) Seq() int64 { return g.seq }

func (g *Game) level(id string) (*scenario.Level, bool) {
	i, ok := g.levelIdx[id]
	if !ok {
		return nil, false
	}
	return &g.Scenario.Levels[i], true
}

// Replay восстанавливает игру из журнала.
func Replay(runID string, sc *scenario.Scenario, seed uint64, events []Event) *Game {
	g := NewGame(runID, sc, seed)
	for _, e := range events {
		g.Apply(e)
	}
	return g
}

// Apply применяет событие к состоянию. Не проверяет инварианты и не падает:
// события — уже свершившиеся факты, проверки делаются при их создании.
func (g *Game) Apply(e Event) {
	g.seq = e.Seq
	t := g.Teams[e.TeamID]

	switch p := e.Payload.(type) {
	case RunStarted:
		g.Status = RunStatusRunning
		g.StartedAt = e.At

	case RunFinished:
		g.Status = RunStatusFinished
		g.FinishedAt = e.At

	case TeamRegistered:
		g.Teams[e.TeamID] = &Team{
			ID:           e.TeamID,
			Name:         p.Name,
			InviteCode:   p.InviteCode,
			Contact:      p.Contact,
			Status:       TeamStatusRegistered,
			Levels:       map[string]*LevelState{},
			Bonuses:      map[string]bool{},
			PendingBonus: map[string]int64{},
		}
		g.TeamOrder = append(g.TeamOrder, e.TeamID)
		if p.InviteCode != "" {
			g.invites[p.InviteCode] = e.TeamID
		}

	case PlayerJoined:
		g.Players[e.PlayerID] = &Player{
			ID: e.PlayerID, TeamID: e.TeamID, Name: p.Name,
			TokenHash: p.TokenHash, JoinedAt: e.At,
		}
		t.PlayerIDs = append(t.PlayerIDs, e.PlayerID)
		if p.Captain {
			t.CaptainID = e.PlayerID
		}

	case PlayerRemoved:
		g.Players[p.PlayerID].Removed = true

	case CaptainChanged:
		t.CaptainID = p.PlayerID

	case InviteRotated:
		delete(g.invites, t.InviteCode)
		t.InviteCode = p.InviteCode
		g.invites[p.InviteCode] = t.ID

	case TeamRenamed:
		t.Name = p.Name

	case TeamWithdrawn:
		t.Status = TeamStatusWithdrawn
		delete(g.invites, t.InviteCode)

	case RegistrationSet:
		g.Registration = Registration{Closed: !p.Open, MaxTeams: p.MaxTeams}

	case TeamStartScheduled:
		t.Status = TeamStatusScheduled
		t.Slot = p.Slot
		t.ScheduledAt = p.StartAt

	case RouteAssigned:
		t.Route = append([]string(nil), p.Levels...)

	case TeamStarted:
		t.Status = TeamStatusPlaying
		t.StartedAt = e.At
		t.LastActivity = e.At

	case LevelOpened:
		sl, _ := g.level(p.LevelID)
		t.Levels[p.LevelID] = &LevelState{
			OpenedAt: e.At,
			Solved:   make([]bool, len(sl.Answers)),
			Pending:  map[int]int64{},
			Hints:    make([]bool, len(sl.Hints)),
		}

	case AnswerSubmitted:
		t.LastActivity = e.At
		res := AnswerResult{Seq: e.Seq, Result: p.Result, LevelID: p.LevelID, AnswerIdx: p.AnswerIdx, BonusID: p.BonusID}
		if e.ClientEventID != "" {
			if g.seen[e.TeamID] == nil {
				g.seen[e.TeamID] = map[string]AnswerResult{}
			}
			g.seen[e.TeamID][e.ClientEventID] = res
		}
		switch p.Result {
		case ResultCorrect:
			if p.AnswerIdx != nil {
				t.Levels[p.LevelID].Solved[*p.AnswerIdx] = true
			}
			// бонус засчитывается отдельным событием bonus_completed
		case ResultWrong:
			if p.Kind == KindCode {
				t.wrongAttempts = append(t.wrongAttempts, e.At)
			}
		case ResultPending:
			item := &PendingItem{
				Seq: e.Seq, At: e.At, TeamID: e.TeamID, PlayerID: e.PlayerID,
				LevelID: p.LevelID, AnswerIdx: -1, BonusID: p.BonusID,
				Kind: p.Kind, Value: p.Value, MediaKey: p.MediaKey,
			}
			if p.AnswerIdx != nil {
				item.AnswerIdx = *p.AnswerIdx
				t.Levels[p.LevelID].Pending[*p.AnswerIdx] = e.Seq
			} else {
				t.PendingBonus[p.BonusID] = e.Seq
			}
			g.pending[e.Seq] = item
		}

	case ModerationResolved:
		item := g.pending[p.Ref]
		if item == nil {
			return
		}
		delete(g.pending, p.Ref)
		t = g.Teams[item.TeamID]
		if item.AnswerIdx >= 0 {
			ls := t.Levels[item.LevelID]
			delete(ls.Pending, item.AnswerIdx)
			if p.Approved {
				ls.Solved[item.AnswerIdx] = true
			}
		} else {
			delete(t.PendingBonus, item.BonusID)
		}
		if res, ok := g.seenBySeq(item.TeamID, p.Ref); ok {
			if p.Approved {
				res.Result = ResultCorrect
			} else {
				res.Result = ResultWrong
			}
			g.seen[item.TeamID][res.key] = res.AnswerResult
		}

	case HintOpened:
		t.Levels[p.LevelID].Hints[p.HintIdx] = true
		t.PenaltyTime += p.PenaltyTime
		t.Points -= p.PenaltyPoints

	case BonusCompleted:
		t.Bonuses[p.BonusID] = true
		t.Points += p.Points

	case LevelCompleted:
		t.Levels[p.LevelID].CompletedAt = e.At
		t.Points += p.Points
		t.LevelsDone++

	case TeamFinished:
		t.Status = TeamStatusFinished
		t.FinishedAt = e.At
		t.FinishReason = p.Reason

	case AdminOverride:
		switch p.Action {
		case OverrideAdjustTime:
			t.PenaltyTime += p.Time
		case OverrideAdjustPoints:
			t.Points += p.Points
		}

	case LeaderboardVisibilitySet:
		g.Leaderboard.Override = p.Visible

	case LeaderboardFrozen:
		g.Leaderboard.Frozen = true
		g.Leaderboard.FrozenAt = e.At
		g.Leaderboard.Snapshot = g.Standings(e.At)

	case Broadcast:
		g.Messages = append(g.Messages, Message{At: e.At, Text: p.Text})
	}
}

type seenEntry struct {
	AnswerResult
	key string
}

func (g *Game) seenBySeq(teamID string, seq int64) (seenEntry, bool) {
	for k, r := range g.seen[teamID] {
		if r.Seq == seq {
			return seenEntry{AnswerResult: r, key: k}, true
		}
	}
	return seenEntry{}, false
}

// PendingModeration — очередь модерации по времени поступления.
func (g *Game) PendingModeration() []PendingItem {
	out := make([]PendingItem, 0, len(g.pending))
	for _, p := range g.pending {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b PendingItem) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}
