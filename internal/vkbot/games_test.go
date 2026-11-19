package vkbot

import (
	"regexp"
	"strings"
	"testing"

	"questaround/internal/core"
)

const carol = int64(303)

var inviteInText = regexp.MustCompile(`код приглашения: ([A-Z0-9]{8})`)

// Регистрация через VK: список игр → игра → создать команду / вступить по коду.
func TestRegistrationInVK(t *testing.T) {
	h := newHarness(t)
	h.bot.cfg.GroupURL = "https://vk.com/write-243408916"

	// Гость видит кнопку «Доступные игры».
	got := h.msg(alice, "Привет")
	if !hasButton(got[0].Keyboard, "Доступные игры") {
		t.Fatal("у гостя нет кнопки «Доступные игры»")
	}

	// Список игр → карточка игры.
	got = h.msg(alice, labelGames, withPayload(payload{Cmd: cmdGames}))
	expect(t, got, "«Пилот» — регистрация открыта, команд: 1")
	card := h.msg(alice, "", withPayload(findButton(t, got, "Пилот")))
	expect(t, card, "Команды:", "«Ёжики» — игроков: 0 из 3", "код приглашения")
	findButton(t, card, "Вступить в «Ёжики»")

	// Создание команды.
	expect(t, h.msg(alice, "", withPayload(findButton(t, card, "Создать команду"))), "Как будет называться команда")
	expect(t, h.msg(alice, strings.Repeat("я", 41)), "Слишком длинное")
	got = h.msg(alice, "Совы")
	expect(t, got, "Вы в игре «Пилот»", "Команда «Совы»", "Вы капитан", "Вступай в команду «Совы» в игре «Пилот»!", "vk.com/write-243408916")
	m := inviteInText.FindStringSubmatch(texts(got))
	if m == nil {
		t.Fatalf("нет кода приглашения:\n%s", texts(got))
	}
	code := m[1]
	var contact string
	_ = h.rn.Read(h.ctx, func(g *core.Game) {
		for _, tm := range g.Teams {
			if tm.Name == "Совы" {
				contact = tm.Contact
			}
		}
	})
	if contact != "https://vk.com/id101" {
		t.Errorf("контакт капитана %q", contact)
	}
	h.sync() // ставим курсор

	// Вступление в выбранную команду: чужой код не подходит, свой — да.
	card = h.msg(bob, "", withPayload(payload{Cmd: cmdGame, Run: h.run.ID}))
	expect(t, card, "«Совы» — игроков: 1 из 3, капитан Игрок Н.")
	expect(t, h.msg(bob, "", withPayload(findButton(t, card, "Вступить в «Совы»"))), "код приглашения команды «Совы»")
	expect(t, h.msg(bob, "ABCDEFGH"), "не подходит к этой команде")
	expect(t, h.msg(bob, strings.ToLower(code)), "Вы в игре «Пилот»", "Команда «Совы»", "игроков: 2")
	h.sync()
	expect(t, h.api.take(alice), "В команду вступил(а) Игрок Н.")

	// Капитан видит состав и приглашение по кнопке «Команда».
	expect(t, h.msg(alice, "", withPayload(payload{Cmd: cmdTeam})), "Игроки:", "Игрок Н. 👑 (вы)", "код приглашения: "+code)
	if got := texts(h.msg(bob, "", withPayload(payload{Cmd: cmdTeam}))); strings.Contains(got, code) {
		t.Error("не-капитан видит код приглашения")
	}

	// Ссылка на игру открывает карточку.
	expect(t, h.msg(carol, "https://quest.example/g/"+h.run.PublicCode), "🎲 «Пилот»", "«Совы»")

	// Регистрация закрыта — создать нельзя, вступить можно.
	h.do(core.SetRegistration{Open: false})
	card = h.msg(carol, "", withPayload(payload{Cmd: cmdGame, Run: h.run.ID}))
	expect(t, card, "Регистрация новых команд закрыта")
	if hasButton(card[0].Keyboard, "Создать команду") {
		t.Error("кнопка создания при закрытой регистрации")
	}
	expect(t, h.msg(carol, "", withPayload(payload{Cmd: cmdNewTeam, Run: h.run.ID})), "закрыта")

	// Отмена ввода кода.
	h.msg(carol, "", withPayload(payload{Cmd: cmdJoinTeam, Run: h.run.ID, Team: "t1"}))
	got = h.msg(carol, "", withPayload(payload{Cmd: cmdCancel}))
	expect(t, got, "Отменено")
	if !hasButton(got[0].Keyboard, "Доступные игры") {
		t.Error("после отмены у гостя нет кнопки игр")
	}

	// Игра идёт — в списке «можно вступить».
	h.do(core.StartRun{})
	expect(t, h.msg(carol, "/игры"), "игра идёт — можно вступить в команду")
}

func TestParseGameLink(t *testing.T) {
	for _, c := range []struct {
		text string
		bare bool
		code string
		ok   bool
	}{
		{"https://q.ru/g/abc234", false, "ABC234", true},
		{"https://q.ru/g/abc234/", false, "ABC234", true},
		{"ABC234", true, "ABC234", true},
		{"ABC234", false, "", false},
		{"Привет", true, "", false},
	} {
		code, ok := parseGameLink(c.text, c.bare)
		if code != c.code || ok != c.ok {
			t.Errorf("%q: %q %v", c.text, code, ok)
		}
	}
}
