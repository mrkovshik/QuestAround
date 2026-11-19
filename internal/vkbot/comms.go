package vkbot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"questaround/internal/core"
	"questaround/internal/runner"
)

// --- чат с ведущим ---

func (b *Bot) startHost(ctx context.Context, user int64, s *session) {
	if !s.captain {
		b.reply(ctx, user, "Писать ведущему может только капитан. Если что-то случилось — «🆘 Помощь».")
		return
	}
	b.setMode(user, cmdHost, hostModeTTL)
	b.send(ctx, out{peer: user, text: "Напишите сообщение ведущему одним сообщением. Можно отправить местоположение.",
		keyboard: inline([]Button{textButton("Отмена", "secondary", payload{Cmd: cmdCancel})})})
}

func (b *Bot) postHost(ctx context.Context, user int64, s *session, m Message) {
	text := strings.TrimSpace(m.Text)
	if m.Geo != nil {
		c := m.Geo.Coordinates
		text = strings.TrimSpace(text + "\n📍 " + strconv.FormatFloat(c.Latitude, 'f', 6, 64) + ", " + strconv.FormatFloat(c.Longitude, 'f', 6, 64))
	}
	if text == "" {
		b.reply(ctx, user, "Пустое сообщение не отправить — напишите текст.")
		return
	}
	_, err := s.rn.Do(ctx, core.PostChat{TeamID: s.teamID, PlayerID: s.bind.PlayerID, Text: text})
	if err != nil {
		b.reply(ctx, user, "⚠️ "+humanErr(err)+".")
		return
	}
	b.setMode(user, "", 0)
	b.reply(ctx, user, "💬 Отправлено ведущему. Ответ придёт сюда.")
}

// --- тревога ---

func (b *Bot) startSOS(ctx context.Context, user int64, s *session) {
	b.setMode(user, cmdSOS, sosModeTTL)
	contact := ""
	_ = s.rn.Read(ctx, func(g *core.Game) { contact = g.HostContact })
	text := "🆘 Опишите, что случилось, и/или нажмите «Отправить местоположение» — ведущий сразу увидит сигнал."
	if contact != "" {
		text += "\n\nЕсли нет интернета — звоните ведущему: " + contact
	}
	b.send(ctx, out{peer: user, text: text, keyboard: inline(
		[]Button{textButton("🆘 Отправить сигнал без описания", "negative", payload{Cmd: cmdSOSSend})},
		[]Button{textButton("Отмена", "secondary", payload{Cmd: cmdCancel})},
	)})
}

func (b *Bot) raiseSOS(ctx context.Context, user int64, s *session, text string, geo *Geo) {
	cmd := core.RaiseSOS{TeamID: s.teamID, PlayerID: s.bind.PlayerID, Text: strings.TrimSpace(text)}
	if geo != nil {
		lat, lng := geo.Coordinates.Latitude, geo.Coordinates.Longitude
		cmd.Lat, cmd.Lng = &lat, &lng
	}
	if cmd.Text == "" && geo == nil {
		b.reply(ctx, user, "Опишите, что случилось, или отправьте местоположение.")
		return
	}
	if _, err := s.rn.Do(ctx, cmd); err != nil {
		b.reply(ctx, user, "⚠️ Сигнал не отправлен: "+humanErr(err)+". Позвоните ведущему.")
		return
	}
	// Режим не снимаем: следующие сообщения тоже уйдут ведущему как сигнал
	// (например, сначала место, потом описание).
	b.setMode(user, cmdSOS, sosModeTTL)
	b.send(ctx, out{peer: user, text: "🆘 Сигнал отправлен ведущему. Можно дописать подробности или отправить место — тоже уйдёт ведущему.",
		keyboard: inline([]Button{textButton("Вернуться к игре", "primary", payload{Cmd: cmdCancel})})})
}

// --- мелочи ---

// humanErr — текст ошибки для игрока (ошибки движка уже по-русски).
func humanErr(err error) string {
	var ve *ErrVK
	switch {
	case errors.Is(err, runner.ErrStopped), errors.As(err, &ve):
		return "сервер занят, попробуйте ещё раз через минуту"
	case errors.Is(err, core.ErrRateLimited), errors.Is(err, core.ErrNotAllowed),
		errors.Is(err, core.ErrTeamNotPlaying), errors.Is(err, core.ErrNoTarget),
		errors.Is(err, core.ErrNotAvailableYet), errors.Is(err, core.ErrHintAlreadyOpen),
		errors.Is(err, core.ErrUnknownInvite), errors.Is(err, core.ErrTeamFull),
		errors.Is(err, core.ErrLevelNotOpen), errors.Is(err, core.ErrAlreadyPending),
		errors.Is(err, core.ErrLocationRevealed), errors.Is(err, core.ErrEmptyAnswer),
		errors.Is(err, core.ErrRegistrationClosed), errors.Is(err, core.ErrRunNotRunning):
		return capitalize(err.Error())
	case errors.Is(err, core.ErrRegistrationFull), errors.Is(err, core.ErrRunStarted):
		return capitalize(err.Error())
	}
	return "не получилось, попробуйте ещё раз"
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

const idAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i := range buf {
		buf[i] = idAlphabet[int(buf[i])%len(idAlphabet)]
	}
	return string(buf)
}

// randomHash — хэш случайного токена: VK-игрок входит не по токену,
// но в журнале у игрока должен быть хэш, а войти с ним в приложение нельзя.
func randomHash() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	h := sha256.Sum256([]byte(base64.RawURLEncoding.EncodeToString(b)))
	return hex.EncodeToString(h[:])
}
