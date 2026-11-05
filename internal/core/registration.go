package core

import (
	"errors"
	"strings"
)

var (
	ErrRegistrationClosed = errors.New("регистрация закрыта")
	ErrRegistrationFull   = errors.New("мест больше нет")
	ErrUnknownInvite      = errors.New("приглашение недействительно")
	ErrTeamFull           = errors.New("в команде нет мест")
	ErrUnknownPlayer      = errors.New("игрок не найден")
	ErrPlayerExists       = errors.New("игрок с таким id уже есть")
	ErrInviteTaken        = errors.New("инвайт-код уже занят")
)

// Actor — кто выполняет действие с командой: капитан (PlayerID) или ведущий.
type Actor struct {
	PlayerID string
	Host     bool
}

// NewPlayer — данные нового игрока. ID и токен генерирует транспорт,
// в журнал попадает только хэш токена.
type NewPlayer struct {
	ID        string
	Name      string
	TokenHash string
}

func (p NewPlayer) valid() bool {
	return p.ID != "" && strings.TrimSpace(p.Name) != "" && p.TokenHash != ""
}

// RegisterTeam — создать команду до старта игры.
// Самостоятельная регистрация передаёт Captain; ведущий — ByHost и без капитана
// (капитаном станет первый вступивший).
type RegisterTeam struct {
	TeamID     string
	Name       string
	InviteCode string
	Contact    string
	Captain    *NewPlayer
	ByHost     bool
}

// JoinTeam — вступить в команду по инвайту (до старта и во время игры).
type JoinTeam struct {
	InviteCode string
	Player     NewPlayer
}

type RenameTeam struct {
	Actor  Actor
	TeamID string
	Name   string
}

type RemovePlayer struct {
	Actor    Actor
	TeamID   string
	PlayerID string
}

type TransferCaptain struct {
	Actor    Actor
	TeamID   string
	PlayerID string
}

type RotateInvite struct {
	Actor      Actor
	TeamID     string
	InviteCode string
}

// WithdrawTeam — снять команду до старта (после старта — Override finish_team).
type WithdrawTeam struct {
	Actor  Actor
	TeamID string
}

// SetRegistration — открыть/закрыть регистрацию, лимит команд (0 — без лимита).
type SetRegistration struct {
	Open     bool
	MaxTeams int
}

func (JoinTeam) isCommand()        {}
func (RenameTeam) isCommand()      {}
func (RemovePlayer) isCommand()    {}
func (TransferCaptain) isCommand() {}
func (RotateInvite) isCommand()    {}
func (WithdrawTeam) isCommand()    {}
func (SetRegistration) isCommand() {}

// ActivePlayers — игроки команды, кроме удалённых.
func (g *Game) ActivePlayers(teamID string) []*Player {
	t := g.Teams[teamID]
	if t == nil {
		return nil
	}
	var out []*Player
	for _, id := range t.PlayerIDs {
		if p := g.Players[id]; !p.Removed {
			out = append(out, p)
		}
	}
	return out
}

// TeamByInvite — команда по инвайт-коду.
func (g *Game) TeamByInvite(code string) (*Team, bool) {
	t, ok := g.Teams[g.invites[code]]
	return t, ok
}

// ActiveTeams — команды, кроме снявшихся, в порядке регистрации.
func (g *Game) ActiveTeams() []*Team {
	out := make([]*Team, 0, len(g.TeamOrder))
	for _, id := range g.TeamOrder {
		if t := g.Teams[id]; t.Status != TeamStatusWithdrawn {
			out = append(out, t)
		}
	}
	return out
}

func (g *Game) registerTeam(t *tx, c RegisterTeam) error {
	if g.Status != RunStatusPending {
		return ErrRunStarted
	}
	if c.TeamID == "" || strings.TrimSpace(c.Name) == "" {
		return ErrBadCommand
	}
	if _, ok := g.Teams[c.TeamID]; ok {
		return ErrTeamExists
	}
	if !c.ByHost {
		if g.Registration.Closed {
			return ErrRegistrationClosed
		}
		if m := g.Registration.MaxTeams; m > 0 && len(g.ActiveTeams()) >= m {
			return ErrRegistrationFull
		}
		if c.Captain == nil {
			return ErrBadCommand
		}
	}
	if c.InviteCode != "" {
		if _, taken := g.invites[c.InviteCode]; taken {
			return ErrInviteTaken
		}
	}
	if c.Captain != nil {
		if !c.Captain.valid() {
			return ErrBadCommand
		}
		if _, ok := g.Players[c.Captain.ID]; ok {
			return ErrPlayerExists
		}
	}

	t.emit(c.TeamID, "", "", TeamRegistered{Name: strings.TrimSpace(c.Name), InviteCode: c.InviteCode, Contact: c.Contact})
	if c.Captain != nil {
		t.emit(c.TeamID, c.Captain.ID, "", PlayerJoined{Name: strings.TrimSpace(c.Captain.Name), TokenHash: c.Captain.TokenHash, Captain: true})
	}
	return nil
}

func (g *Game) joinTeam(t *tx, c JoinTeam) error {
	team, ok := g.TeamByInvite(c.InviteCode)
	if !ok || c.InviteCode == "" {
		return ErrUnknownInvite
	}
	if g.Status == RunStatusFinished || team.Status == TeamStatusFinished {
		return ErrTeamNotPlaying
	}
	if !c.Player.valid() {
		return ErrBadCommand
	}
	if _, ok := g.Players[c.Player.ID]; ok {
		return ErrPlayerExists
	}
	if len(g.ActivePlayers(team.ID)) >= g.Scenario.Participants.TeamSize.Max {
		return ErrTeamFull
	}
	t.emit(team.ID, c.Player.ID, "", PlayerJoined{
		Name:      strings.TrimSpace(c.Player.Name),
		TokenHash: c.Player.TokenHash,
		Captain:   team.CaptainID == "",
	})
	return nil
}

// teamFor проверяет, что актор — ведущий или капитан команды.
func (g *Game) teamFor(a Actor, teamID string) (*Team, error) {
	team, ok := g.Teams[teamID]
	if !ok {
		return nil, ErrUnknownTeam
	}
	if a.Host {
		return team, nil
	}
	if a.PlayerID == "" || team.CaptainID != a.PlayerID {
		return nil, ErrNotAllowed
	}
	return team, nil
}

func (g *Game) activeMember(team *Team, playerID string) bool {
	p := g.Players[playerID]
	return p != nil && p.TeamID == team.ID && !p.Removed
}

func actorID(a Actor) string {
	if a.Host {
		return "host"
	}
	return a.PlayerID
}

func (g *Game) renameTeam(t *tx, c RenameTeam) error {
	team, err := g.teamFor(c.Actor, c.TeamID)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return ErrBadCommand
	}
	t.emit(team.ID, actorID(c.Actor), "", TeamRenamed{Name: name})
	return nil
}

func (g *Game) removePlayer(t *tx, c RemovePlayer) error {
	team, err := g.teamFor(c.Actor, c.TeamID)
	if err != nil {
		return err
	}
	if !g.activeMember(team, c.PlayerID) {
		return ErrUnknownPlayer
	}
	if c.PlayerID == team.CaptainID {
		return ErrNotAllowed // сначала передать капитанство
	}
	t.emit(team.ID, actorID(c.Actor), "", PlayerRemoved{PlayerID: c.PlayerID})
	return nil
}

func (g *Game) transferCaptain(t *tx, c TransferCaptain) error {
	team, err := g.teamFor(c.Actor, c.TeamID)
	if err != nil {
		return err
	}
	if !g.activeMember(team, c.PlayerID) {
		return ErrUnknownPlayer
	}
	if c.PlayerID == team.CaptainID {
		return nil
	}
	t.emit(team.ID, actorID(c.Actor), "", CaptainChanged{PlayerID: c.PlayerID})
	return nil
}

func (g *Game) rotateInvite(t *tx, c RotateInvite) error {
	team, err := g.teamFor(c.Actor, c.TeamID)
	if err != nil {
		return err
	}
	if team.Status == TeamStatusWithdrawn {
		return ErrTeamNotPlaying
	}
	if c.InviteCode == "" {
		return ErrBadCommand
	}
	if _, taken := g.invites[c.InviteCode]; taken {
		return ErrInviteTaken
	}
	t.emit(team.ID, actorID(c.Actor), "", InviteRotated{InviteCode: c.InviteCode})
	return nil
}

func (g *Game) withdrawTeam(t *tx, c WithdrawTeam) error {
	team, err := g.teamFor(c.Actor, c.TeamID)
	if err != nil {
		return err
	}
	if g.Status != RunStatusPending {
		return ErrRunStarted
	}
	if team.Status == TeamStatusWithdrawn {
		return nil
	}
	t.emit(team.ID, actorID(c.Actor), "", TeamWithdrawn{})
	return nil
}

func (g *Game) setRegistration(t *tx, c SetRegistration) error {
	if c.MaxTeams < 0 {
		return ErrBadCommand
	}
	t.emit("", "host", "", RegistrationSet{Open: c.Open, MaxTeams: c.MaxTeams})
	return nil
}
