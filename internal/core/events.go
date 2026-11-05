package core

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType — тип события игрового журнала.
type EventType string

const (
	EvRunStarted               EventType = "run_started"
	EvRunFinished              EventType = "run_finished"
	EvTeamRegistered           EventType = "team_registered"
	EvTeamStartScheduled       EventType = "team_start_scheduled"
	EvRouteAssigned            EventType = "route_assigned"
	EvTeamStarted              EventType = "team_started"
	EvLevelOpened              EventType = "level_opened"
	EvAnswerSubmitted          EventType = "answer_submitted"
	EvModerationResolved       EventType = "moderation_resolved"
	EvHintOpened               EventType = "hint_opened"
	EvBonusCompleted           EventType = "bonus_completed"
	EvLevelCompleted           EventType = "level_completed"
	EvTeamFinished             EventType = "team_finished"
	EvAdminOverride            EventType = "admin_override"
	EvLeaderboardVisibilitySet EventType = "leaderboard_visibility_set"
	EvLeaderboardFrozen        EventType = "leaderboard_frozen"
	EvBroadcast                EventType = "broadcast"
)

// Event — запись журнала. Состояние игры — свёртка событий через Apply.
//
// Seq назначает движок (1, 2, 3… в пределах Run); хранилище обязано
// сохранить его как есть — на Seq ссылаются, например, решения модерации.
type Event struct {
	Seq           int64
	Type          EventType
	At            time.Time
	TeamID        string
	PlayerID      string
	ClientEventID string
	Payload       Payload
}

// Payload — данные конкретного типа события.
type Payload interface{ Type() EventType }

type RunStarted struct{}

type RunFinished struct {
	Reason string `json:"reason"` // all_finished | admin
}

type TeamRegistered struct {
	Name string `json:"name"`
}

type TeamStartScheduled struct {
	StartAt time.Time `json:"start_at"`
	Slot    int       `json:"slot"`
}

type RouteAssigned struct {
	Levels []string `json:"levels"`
}

type TeamStarted struct{}

type LevelOpened struct {
	LevelID string `json:"level_id"`
	Reason  string `json:"reason"` // start | next | unlock
}

// AnswerSubmitted — попытка ответа и результат проверки.
type AnswerSubmitted struct {
	LevelID  string     `json:"level_id,omitempty"` // уровень, к которому отнесён ответ
	Kind     AnswerKind `json:"kind"`
	Value    string     `json:"value,omitempty"`
	Lat      float64    `json:"lat,omitempty"`
	Lng      float64    `json:"lng,omitempty"`
	Accuracy float64    `json:"accuracy,omitempty"`
	MediaKey string     `json:"media_key,omitempty"`

	Result Result `json:"result"`
	// Что именно совпало (для correct/duplicate/pending):
	AnswerIdx *int   `json:"answer_idx,omitempty"`
	BonusID   string `json:"bonus_id,omitempty"`
}

type ModerationResolved struct {
	Ref      int64  `json:"ref"` // Seq события answer_submitted
	Approved bool   `json:"approved"`
	Comment  string `json:"comment,omitempty"`
}

type HintOpened struct {
	LevelID       string        `json:"level_id"`
	HintIdx       int           `json:"hint_idx"`
	Trigger       string        `json:"trigger"` // timer | request | admin
	PenaltyTime   time.Duration `json:"penalty_time,omitempty"`
	PenaltyPoints int           `json:"penalty_points,omitempty"`
}

type BonusCompleted struct {
	LevelID string `json:"level_id"`
	BonusID string `json:"bonus_id"`
	Points  int    `json:"points"`
}

type LevelCompleted struct {
	LevelID string `json:"level_id"`
	Points  int    `json:"points"`
}

type TeamFinished struct {
	Reason string `json:"reason"` // completed | time_limit | admin
}

type AdminOverride struct {
	Action  OverrideAction `json:"action"`
	LevelID string         `json:"level_id,omitempty"`
	HintIdx int            `json:"hint_idx,omitempty"`
	Time    time.Duration  `json:"time,omitempty"`
	Points  int            `json:"points,omitempty"`
	Reason  string         `json:"reason"`
}

type LeaderboardVisibilitySet struct {
	Visible *bool `json:"visible"` // nil — вернуть поведение из сценария
}

type LeaderboardFrozen struct{}

type Broadcast struct {
	Text string `json:"text"`
}

func (RunStarted) Type() EventType               { return EvRunStarted }
func (RunFinished) Type() EventType              { return EvRunFinished }
func (TeamRegistered) Type() EventType           { return EvTeamRegistered }
func (TeamStartScheduled) Type() EventType       { return EvTeamStartScheduled }
func (RouteAssigned) Type() EventType            { return EvRouteAssigned }
func (TeamStarted) Type() EventType              { return EvTeamStarted }
func (LevelOpened) Type() EventType              { return EvLevelOpened }
func (AnswerSubmitted) Type() EventType          { return EvAnswerSubmitted }
func (ModerationResolved) Type() EventType       { return EvModerationResolved }
func (HintOpened) Type() EventType               { return EvHintOpened }
func (BonusCompleted) Type() EventType           { return EvBonusCompleted }
func (LevelCompleted) Type() EventType           { return EvLevelCompleted }
func (TeamFinished) Type() EventType             { return EvTeamFinished }
func (AdminOverride) Type() EventType            { return EvAdminOverride }
func (LeaderboardVisibilitySet) Type() EventType { return EvLeaderboardVisibilitySet }
func (LeaderboardFrozen) Type() EventType        { return EvLeaderboardFrozen }
func (Broadcast) Type() EventType                { return EvBroadcast }

var payloadFactory = map[EventType]func() Payload{
	EvRunStarted:               func() Payload { return &RunStarted{} },
	EvRunFinished:              func() Payload { return &RunFinished{} },
	EvTeamRegistered:           func() Payload { return &TeamRegistered{} },
	EvTeamStartScheduled:       func() Payload { return &TeamStartScheduled{} },
	EvRouteAssigned:            func() Payload { return &RouteAssigned{} },
	EvTeamStarted:              func() Payload { return &TeamStarted{} },
	EvLevelOpened:              func() Payload { return &LevelOpened{} },
	EvAnswerSubmitted:          func() Payload { return &AnswerSubmitted{} },
	EvModerationResolved:       func() Payload { return &ModerationResolved{} },
	EvHintOpened:               func() Payload { return &HintOpened{} },
	EvBonusCompleted:           func() Payload { return &BonusCompleted{} },
	EvLevelCompleted:           func() Payload { return &LevelCompleted{} },
	EvTeamFinished:             func() Payload { return &TeamFinished{} },
	EvAdminOverride:            func() Payload { return &AdminOverride{} },
	EvLeaderboardVisibilitySet: func() Payload { return &LeaderboardVisibilitySet{} },
	EvLeaderboardFrozen:        func() Payload { return &LeaderboardFrozen{} },
	EvBroadcast:                func() Payload { return &Broadcast{} },
}

// DecodePayload восстанавливает payload по типу события (для хранилища).
func DecodePayload(t EventType, data []byte) (Payload, error) {
	f, ok := payloadFactory[t]
	if !ok {
		return nil, fmt.Errorf("неизвестный тип события %q", t)
	}
	p := f()
	if len(data) > 0 {
		if err := json.Unmarshal(data, p); err != nil {
			return nil, fmt.Errorf("событие %s: %w", t, err)
		}
	}
	// Payload храним по значению — разыменовываем указатель.
	return deref(p), nil
}

func deref(p Payload) Payload {
	switch v := p.(type) {
	case *RunStarted:
		return *v
	case *RunFinished:
		return *v
	case *TeamRegistered:
		return *v
	case *TeamStartScheduled:
		return *v
	case *RouteAssigned:
		return *v
	case *TeamStarted:
		return *v
	case *LevelOpened:
		return *v
	case *AnswerSubmitted:
		return *v
	case *ModerationResolved:
		return *v
	case *HintOpened:
		return *v
	case *BonusCompleted:
		return *v
	case *LevelCompleted:
		return *v
	case *TeamFinished:
		return *v
	case *AdminOverride:
		return *v
	case *LeaderboardVisibilitySet:
		return *v
	case *LeaderboardFrozen:
		return *v
	case *Broadcast:
		return *v
	}
	return p
}

type eventJSON struct {
	Seq           int64           `json:"seq"`
	Type          EventType       `json:"type"`
	At            time.Time       `json:"at"`
	TeamID        string          `json:"team_id,omitempty"`
	PlayerID      string          `json:"player_id,omitempty"`
	ClientEventID string          `json:"client_event_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

func (e Event) MarshalJSON() ([]byte, error) {
	p, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(eventJSON{
		Seq: e.Seq, Type: e.Type, At: e.At, TeamID: e.TeamID,
		PlayerID: e.PlayerID, ClientEventID: e.ClientEventID, Payload: p,
	})
}

func (e *Event) UnmarshalJSON(data []byte) error {
	var raw eventJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p, err := DecodePayload(raw.Type, raw.Payload)
	if err != nil {
		return err
	}
	*e = Event{
		Seq: raw.Seq, Type: raw.Type, At: raw.At, TeamID: raw.TeamID,
		PlayerID: raw.PlayerID, ClientEventID: raw.ClientEventID, Payload: p,
	}
	return nil
}
