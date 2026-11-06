package core

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Связь во время игры: чат команды с ведущим, сигнал тревоги (SOS),
// контакт ведущего для звонка без интернета.

const maxChatLen = 1000

// --- события ---

const (
	EvChatPosted     EventType = "chat_posted"
	EvSOSRaised      EventType = "sos_raised"
	EvSOSResolved    EventType = "sos_resolved"
	EvHostContactSet EventType = "host_contact_set"
)

// ChatPosted — сообщение в канале команды с организаторами (TeamID в событии).
type ChatPosted struct {
	FromHost bool   `json:"from_host,omitempty"`
	Text     string `json:"text"`
}

// SOSRaised — игрок просит помощи. Координаты — если телефон их отдал.
type SOSRaised struct {
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
	Accuracy *float64 `json:"accuracy,omitempty"`
	Text     string   `json:"text,omitempty"`
}

type SOSResolved struct {
	Ref  int64  `json:"ref"` // Seq события sos_raised
	Note string `json:"note,omitempty"`
}

type HostContactSet struct {
	Text string `json:"text"`
}

func (ChatPosted) Type() EventType     { return EvChatPosted }
func (SOSRaised) Type() EventType      { return EvSOSRaised }
func (SOSResolved) Type() EventType    { return EvSOSResolved }
func (HostContactSet) Type() EventType { return EvHostContactSet }

func init() {
	payloadFactory[EvChatPosted] = func() Payload { return &ChatPosted{} }
	payloadFactory[EvSOSRaised] = func() Payload { return &SOSRaised{} }
	payloadFactory[EvSOSResolved] = func() Payload { return &SOSResolved{} }
	payloadFactory[EvHostContactSet] = func() Payload { return &HostContactSet{} }
}

// --- состояние ---

type ChatMessage struct {
	Seq      int64
	At       time.Time
	FromHost bool
	PlayerID string
	Text     string
}

type SOSAlert struct {
	Seq        int64
	At         time.Time
	TeamID     string
	PlayerID   string
	Lat, Lng   *float64
	Accuracy   *float64
	Text       string
	ResolvedAt time.Time
	Note       string
}

func (a *SOSAlert) Active() bool { return a.ResolvedAt.IsZero() }

// applyComms — часть Apply для событий связи. Возвращает false, если событие не отсюда.
func (g *Game) applyComms(e Event) bool {
	switch p := e.Payload.(type) {
	case ChatPosted:
		t := g.Teams[e.TeamID]
		t.Chat = append(t.Chat, ChatMessage{Seq: e.Seq, At: e.At, FromHost: p.FromHost, PlayerID: e.PlayerID, Text: p.Text})
	case SOSRaised:
		g.SOS = append(g.SOS, &SOSAlert{Seq: e.Seq, At: e.At, TeamID: e.TeamID, PlayerID: e.PlayerID,
			Lat: p.Lat, Lng: p.Lng, Accuracy: p.Accuracy, Text: p.Text})
	case SOSResolved:
		for _, a := range g.SOS {
			if a.Seq == p.Ref {
				a.ResolvedAt, a.Note = e.At, p.Note
			}
		}
	case HostContactSet:
		g.HostContact = p.Text
	default:
		return false
	}
	return true
}

// --- команды ---

// PostChat — сообщение в канал команды. От игрока — только капитан.
type PostChat struct {
	TeamID   string
	PlayerID string // пусто, если пишет ведущий
	FromHost bool
	Text     string
}

// RaiseSOS — сигнал тревоги от любого игрока команды.
type RaiseSOS struct {
	TeamID   string
	PlayerID string
	Lat, Lng *float64
	Accuracy *float64
	Text     string
}

// ResolveSOS — ведущий принял сигнал.
type ResolveSOS struct {
	Ref  int64
	Note string
}

// SetHostContact — телефон/мессенджер ведущего, показывается игрокам.
type SetHostContact struct {
	Text string
}

func (PostChat) isCommand()       {}
func (RaiseSOS) isCommand()       {}
func (ResolveSOS) isCommand()     {}
func (SetHostContact) isCommand() {}

// handleComms — часть Handle. Возвращает handled=false для чужих команд.
func (g *Game) handleComms(t *tx, cmd Command) (handled bool, err error) {
	switch c := cmd.(type) {
	case PostChat:
		return true, g.postChat(t, c)
	case RaiseSOS:
		return true, g.raiseSOS(t, c)
	case ResolveSOS:
		return true, g.resolveSOS(t, c)
	case SetHostContact:
		text := strings.TrimSpace(c.Text)
		if utf8.RuneCountInString(text) > 200 {
			return true, ErrBadCommand
		}
		t.emit("", "host", "", HostContactSet{Text: text})
		return true, nil
	}
	return false, nil
}

func (g *Game) postChat(t *tx, c PostChat) error {
	team, ok := g.Teams[c.TeamID]
	if !ok {
		return ErrUnknownTeam
	}
	text := strings.TrimSpace(c.Text)
	if text == "" || utf8.RuneCountInString(text) > maxChatLen {
		return ErrBadCommand
	}
	if !c.FromHost {
		if team.CaptainID == "" || team.CaptainID != c.PlayerID {
			return ErrNotAllowed
		}
	}
	by := c.PlayerID
	if c.FromHost {
		by = "host"
	}
	t.emit(team.ID, by, "", ChatPosted{FromHost: c.FromHost, Text: text})
	return nil
}

func (g *Game) raiseSOS(t *tx, c RaiseSOS) error {
	team, ok := g.Teams[c.TeamID]
	if !ok {
		return ErrUnknownTeam
	}
	if p := g.Players[c.PlayerID]; p == nil || p.Removed || p.TeamID != team.ID {
		return ErrUnknownPlayer
	}
	if (c.Lat == nil) != (c.Lng == nil) {
		return ErrBadCommand
	}
	text := strings.TrimSpace(c.Text)
	if utf8.RuneCountInString(text) > maxChatLen {
		return ErrBadCommand
	}
	t.emit(team.ID, c.PlayerID, "", SOSRaised{Lat: c.Lat, Lng: c.Lng, Accuracy: c.Accuracy, Text: text})
	return nil
}

func (g *Game) resolveSOS(t *tx, c ResolveSOS) error {
	for _, a := range g.SOS {
		if a.Seq == c.Ref {
			if !a.Active() {
				return nil
			}
			t.emit(a.TeamID, "host", "", SOSResolved{Ref: c.Ref, Note: strings.TrimSpace(c.Note)})
			return nil
		}
	}
	return ErrNotPending
}

// ActiveSOS — неразобранные сигналы, старые сверху.
func (g *Game) ActiveSOS() []*SOSAlert {
	var out []*SOSAlert
	for _, a := range g.SOS {
		if a.Active() {
			out = append(out, a)
		}
	}
	return out
}
