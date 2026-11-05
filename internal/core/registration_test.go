package core

import "testing"

const regYAML = `
version: 1
id: r
title: r
participants: {team_size: {min: 1, max: 3}}
levels: [{id: a, task: t, answers: [{type: code, values: ["1"]}]}]
`

func np(id string) *NewPlayer {
	return &NewPlayer{ID: id, Name: "Игрок " + id, TokenHash: "h-" + id}
}

func TestSelfRegistrationAndJoin(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(RegisterTeam{TeamID: "t1", Name: " Ёжики ", InviteCode: "INV1", Contact: "@cap", Captain: np("cap")})
	team := h.team("t1")
	h.eq("имя обрезано", team.Name, "Ёжики")
	h.eq("капитан", team.CaptainID, "cap")
	h.eq("контакт", team.Contact, "@cap")

	h.fail(RegisterTeam{TeamID: "t2", Name: "x"}, ErrBadCommand) // без капитана — только ведущий
	h.fail(RegisterTeam{TeamID: "t2", Name: "x", InviteCode: "INV1", Captain: np("c2")}, ErrInviteTaken)
	h.fail(RegisterTeam{TeamID: "t2", Name: "x", Captain: np("cap")}, ErrPlayerExists)
	h.fail(RegisterTeam{TeamID: "t1", Name: "x", Captain: np("c2")}, ErrTeamExists)

	h.do(JoinTeam{InviteCode: "INV1", Player: *np("p1")})
	h.do(JoinTeam{InviteCode: "INV1", Player: *np("p2")})
	h.fail(JoinTeam{InviteCode: "INV1", Player: *np("p3")}, ErrTeamFull)
	h.fail(JoinTeam{InviteCode: "nope", Player: *np("p3")}, ErrUnknownInvite)
	h.fail(JoinTeam{InviteCode: "INV1", Player: NewPlayer{ID: "p3", Name: " "}}, ErrBadCommand)
	h.eq("состав", len(h.g.ActivePlayers("t1")), 3)
	h.eq("p1 не капитан", h.team("t1").CaptainID, "cap")
}

func TestHostTeamFirstJoinerIsCaptain(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(RegisterTeam{TeamID: "t1", Name: "Корпоратив", InviteCode: "HOST1", ByHost: true})
	h.eq("без капитана", h.team("t1").CaptainID, "")
	h.do(JoinTeam{InviteCode: "HOST1", Player: *np("p1")})
	h.do(JoinTeam{InviteCode: "HOST1", Player: *np("p2")})
	h.eq("первый — капитан", h.team("t1").CaptainID, "p1")
}

func TestCaptainActions(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(RegisterTeam{TeamID: "t1", Name: "A", InviteCode: "I1", Captain: np("cap")})
	h.do(JoinTeam{InviteCode: "I1", Player: *np("p1")})
	h.do(RegisterTeam{TeamID: "t2", Name: "B", InviteCode: "I2", Captain: np("other")})

	capA := Actor{PlayerID: "cap"}
	h.fail(RenameTeam{Actor: Actor{PlayerID: "p1"}, TeamID: "t1", Name: "X"}, ErrNotAllowed)
	h.fail(RenameTeam{Actor: Actor{PlayerID: "other"}, TeamID: "t1", Name: "X"}, ErrNotAllowed)
	h.do(RenameTeam{Actor: capA, TeamID: "t1", Name: "Альфа"})
	h.eq("имя", h.team("t1").Name, "Альфа")

	h.fail(RemovePlayer{Actor: capA, TeamID: "t1", PlayerID: "cap"}, ErrNotAllowed)
	h.fail(RemovePlayer{Actor: capA, TeamID: "t1", PlayerID: "other"}, ErrUnknownPlayer)

	h.do(RotateInvite{Actor: capA, TeamID: "t1", InviteCode: "I1-new"})
	h.fail(JoinTeam{InviteCode: "I1", Player: *np("late")}, ErrUnknownInvite)
	h.fail(RotateInvite{Actor: capA, TeamID: "t1", InviteCode: "I2"}, ErrInviteTaken)

	h.do(TransferCaptain{Actor: capA, TeamID: "t1", PlayerID: "p1"})
	h.eq("новый капитан", h.team("t1").CaptainID, "p1")
	h.fail(RenameTeam{Actor: capA, TeamID: "t1", Name: "Y"}, ErrNotAllowed)
	h.do(RemovePlayer{Actor: Actor{PlayerID: "p1"}, TeamID: "t1", PlayerID: "cap"})
	h.eq("состав", len(h.g.ActivePlayers("t1")), 1)

	h.do(RenameTeam{Actor: Actor{Host: true}, TeamID: "t2", Name: "Бета"})
	h.eq("ведущий переименовал", h.team("t2").Name, "Бета")
}

func TestWithdrawAndRegistrationSettings(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(SetRegistration{Open: true, MaxTeams: 2})
	h.do(RegisterTeam{TeamID: "t1", Name: "A", InviteCode: "I1", Captain: np("c1")})
	h.do(RegisterTeam{TeamID: "t2", Name: "B", InviteCode: "I2", Captain: np("c2")})
	h.fail(RegisterTeam{TeamID: "t3", Name: "C", Captain: np("c3")}, ErrRegistrationFull)
	h.do(RegisterTeam{TeamID: "t3", Name: "C", ByHost: true}) // ведущему лимит не мешает

	h.do(WithdrawTeam{Actor: Actor{PlayerID: "c2"}, TeamID: "t2"})
	h.fail(JoinTeam{InviteCode: "I2", Player: *np("x")}, ErrUnknownInvite)
	h.eq("активных", len(h.g.ActiveTeams()), 2)

	h.do(SetRegistration{Open: false})
	h.fail(RegisterTeam{TeamID: "t4", Name: "D", Captain: np("c4")}, ErrRegistrationClosed)

	h.do(StartRun{})
	h.eq("снявшаяся не стартовала", h.team("t2").Status, TeamStatusWithdrawn)
	h.fail(WithdrawTeam{Actor: Actor{PlayerID: "c1"}, TeamID: "t1"}, ErrRunStarted)
	h.eq("в таблице без снявшихся", len(h.g.Standings(h.now)), 2)

	// Вступить можно и во время игры.
	h.do(JoinTeam{InviteCode: "I1", Player: *np("late")})

	h.code("t1", "1")
	h.code("t3", "1")
	h.eq("игра закончилась без снявшейся", h.g.Status, RunStatusFinished)
	h.fail(JoinTeam{InviteCode: "I1", Player: *np("late2")}, ErrTeamNotPlaying)
}
