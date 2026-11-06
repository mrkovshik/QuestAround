package core

import "testing"

func f(v float64) *float64 { return &v }

func TestChat(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(RegisterTeam{TeamID: "t1", Name: "A", InviteCode: "I1", Captain: np("cap")})
	h.do(JoinTeam{InviteCode: "I1", Player: *np("p1")})

	h.do(PostChat{TeamID: "t1", PlayerID: "cap", Text: " Мы у моста, где вход? "})
	h.fail(PostChat{TeamID: "t1", PlayerID: "p1", Text: "а я не капитан"}, ErrNotAllowed)
	h.fail(PostChat{TeamID: "t1", PlayerID: "cap", Text: "   "}, ErrBadCommand)
	h.do(PostChat{TeamID: "t1", FromHost: true, Text: "Слева от лестницы"})

	chat := h.team("t1").Chat
	h.eq("сообщений", len(chat), 2)
	h.eq("текст обрезан", chat[0].Text, "Мы у моста, где вход?")
	h.eq("от ведущего", chat[1].FromHost, true)
	h.eq("кто", chat[1].PlayerID, "host")
}

func TestSOS(t *testing.T) {
	h := newHarness(t, regYAML)
	h.do(RegisterTeam{TeamID: "t1", Name: "A", InviteCode: "I1", Captain: np("cap")})
	h.do(JoinTeam{InviteCode: "I1", Player: *np("p1")})
	h.do(SetHostContact{Text: "+7 900 000-00-00"})
	h.eq("контакт", h.g.HostContact, "+7 900 000-00-00")

	// Тревога — от любого игрока, в любой момент, с координатами и без.
	out := h.do(RaiseSOS{TeamID: "t1", PlayerID: "p1", Lat: f(44.63), Lng: f(39.13), Accuracy: f(12), Text: "упал с велосипеда"})
	h.do(RaiseSOS{TeamID: "t1", PlayerID: "cap"})
	h.fail(RaiseSOS{TeamID: "t1", PlayerID: "p1", Lat: f(1)}, ErrBadCommand)
	h.fail(RaiseSOS{TeamID: "t1", PlayerID: "nobody"}, ErrUnknownPlayer)
	h.eq("активных", len(h.g.ActiveSOS()), 2)

	h.do(ResolveSOS{Ref: out.Events[0].Seq, Note: "еду"})
	h.do(ResolveSOS{Ref: out.Events[0].Seq}) // повтор — без ошибки и без события
	h.fail(ResolveSOS{Ref: 999}, ErrNotPending)
	h.eq("активных после ответа", len(h.g.ActiveSOS()), 1)
	h.eq("заметка", h.g.SOS[0].Note, "еду")
	h.eq("координаты", *h.g.SOS[0].Lat, 44.63)
}
