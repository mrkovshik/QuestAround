package vkbot

import (
	"encoding/json"
	"testing"

	"questaround/internal/core"
)

const photoYAML = `
version: 1
id: vk-photo
title: Фото
participants: {team_size: {min: 1, max: 3}}
levels:
  - id: p1
    title: Селфи
    task: Сделайте фото у фонтана
    answers: [{type: photo}]
    bonuses:
      - {id: b1, title: Командное фото, answer: {type: photo}, points: 5}
  - id: p2
    title: Финиш
    task: Кодовое слово
    answers: [{type: code, values: ["финиш"]}]
`

// withPhoto — сообщение с фото (и подписью); orig — ссылка на оригинал.
func withPhoto(orig string) func(*Message) {
	return func(m *Message) {
		var a Attachment
		raw := `{"type":"photo","photo":{"sizes":[{"url":"https://sun9-1.userapi.com/s.jpg","width":130,"height":100},` +
			`{"url":"https://sun9-1.userapi.com/x.jpg","width":1280,"height":960}],"orig_photo":{"url":"` + orig + `"}}}`
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			panic(err)
		}
		m.Attaches = append(m.Attaches, a)
	}
}

func TestPhotoAnswers(t *testing.T) {
	h := newHarnessYAML(t, photoYAML)
	h.msg(alice, "ABCDEFGH")
	h.sync()
	h.do(core.StartRun{})
	h.sync()
	expect(t, h.api.take(alice), "Ответ — фото: пришлите его сюда", "Бонус «Командное фото» (+5 баллов) — пришлите фото")

	// Фото с подписью: подпись — не код; два адресата — бот спрашивает.
	got := h.msg(alice, "вот мы", withPhoto("https://sun9-1.userapi.com/orig1.jpg"))
	expect(t, got, "К чему это фото?")
	toLevel := findButton(t, got, "К уровню «Селфи»")
	findButton(t, got, "К бонусу «Командное фото»")
	if len(h.api.downloads) != 1 || h.api.downloads[0] != "https://sun9-1.userapi.com/orig1.jpg" {
		t.Fatalf("скачано: %v", h.api.downloads)
	}
	expect(t, h.msg(alice, "", withPayload(toLevel)), "Фото отправлено ведущему")

	// Уровень ждёт проверки — второе фото само уходит к бонусу.
	expect(t, h.msg(alice, "", withPhoto("https://sun9-1.userapi.com/orig2.jpg")), "Фото отправлено ведущему")
	// Больше фото никто не ждёт.
	expect(t, h.msg(alice, "", withPhoto("https://sun9-1.userapi.com/orig3.jpg")), "фото не нужно ни одному заданию")

	var levelSeq int64
	var photos []*core.Photo
	_ = h.rn.Read(h.ctx, func(g *core.Game) {
		tm := g.Teams["t1"]
		levelSeq = tm.Levels["p1"].Pending[0]
		photos = g.Photos
		if tm.PendingBonus["b1"] == 0 {
			t.Error("бонус не ждёт проверки")
		}
	})
	if levelSeq == 0 || len(photos) != 2 || photos[0].MediaKey == "" {
		t.Fatalf("на проверке: уровень %d, фото %+v", levelSeq, photos)
	}

	// Ведущий принимает — команде приходит решение и следующий уровень.
	h.do(core.Moderate{Ref: levelSeq, Approved: true, Comment: "Отлично!"})
	h.sync()
	expect(t, h.api.take(alice), "✅ Фото к уровню «Селфи» принято.", "Комментарий: Отлично!", "«Финиш»")

	// Текст — по-прежнему код.
	expect(t, h.msg(alice, "финиш"), "Верно")
}

func TestImageURL(t *testing.T) {
	var a Attachment
	_ = json.Unmarshal([]byte(`{"type":"photo","photo":{"sizes":[{"url":"s","width":10,"height":10},{"url":"big","width":100,"height":80},{"url":"mid","width":50,"height":50}]}}`), &a)
	if got := a.imageURL(); got != "big" {
		t.Fatalf("без orig_photo взяли %q", got)
	}
	_ = json.Unmarshal([]byte(`{"type":"doc","doc":{"url":"d","ext":"PNG"}}`), &a)
	if a.imageURL() != "d" {
		t.Fatal("картинка-документ не распознана")
	}
	_ = json.Unmarshal([]byte(`{"type":"doc","doc":{"url":"d","ext":"pdf"}}`), &a)
	if a.imageURL() != "" {
		t.Fatal("pdf принят за картинку")
	}
	for host, ok := range map[string]bool{"sun9-1.userapi.com": true, "vk.com": true, "evil.com": false, "userapi.com.evil.com": false} {
		if vkHost(host) != ok {
			t.Errorf("vkHost(%q) != %v", host, ok)
		}
	}
}
