package vkbot

import (
	"cmp"
	"context"
	"errors"
	"strconv"

	"questaround/internal/core"
	"questaround/internal/scenario"
)

// Фото-ответы через VK: бот скачивает фото из сообщения, сохраняет его
// туда же, куда приложение, и отправляет на модерацию — уровню или бонусу,
// который сейчас ждёт фото. Если таких несколько, спрашивает кнопками.

const (
	cmdPhotoTo = "photo_to"
	modePhoto  = "photo" // фото сохранено, ждём выбор, к чему оно
)

// photoTarget — уровень или бонус, который ждёт фото.
type photoTarget struct {
	levelID, bonusID, label string
}

func photoTargets(g *core.Game, t *core.Team) []photoTarget {
	var out []photoTarget
	for _, sl := range openLevels(g, t) {
		ls := t.Levels[sl.ID]
		title := cmp.Or(sl.Title, sl.ID)
		for i, a := range sl.Answers {
			if a.Type == scenario.AnswerPhoto && !(i < len(ls.Solved) && ls.Solved[i]) && ls.Pending[i] == 0 {
				out = append(out, photoTarget{levelID: sl.ID, label: "К уровню «" + title + "»"})
				break // свободный фото-ответ уровня выберет движок
			}
		}
		for _, bn := range sl.Bonuses {
			if bn.Answer.Type == scenario.AnswerPhoto && !t.Bonuses[bn.ID] && t.PendingBonus[bn.ID] == 0 {
				out = append(out, photoTarget{levelID: sl.ID, bonusID: bn.ID, label: "К бонусу «" + cmp.Or(bn.Title, bn.ID) + "»"})
			}
		}
	}
	return out
}

// firstImage — ссылка на первую картинку в сообщении.
func firstImage(m Message) (url string, count int) {
	for _, a := range m.Attaches {
		if u := a.imageURL(); u != "" {
			if url == "" {
				url = u
			}
			count++
		}
	}
	return url, count
}

func (b *Bot) handlePhoto(ctx context.Context, user int64, s *session, m Message, url string, count int) {
	var targets []photoTarget
	playing := false
	_ = s.rn.Read(ctx, func(g *core.Game) {
		t := g.Teams[s.teamID]
		playing = t.Status == core.TeamStatusPlaying
		if playing {
			targets = photoTargets(g, t)
		}
	})
	switch {
	case !playing:
		b.reply(ctx, user, "Команда сейчас не играет — фото не отправлено.")
		return
	case len(targets) == 0:
		b.reply(ctx, user, "📷 Сейчас фото не нужно ни одному заданию (или уже ждёт проверки). Код присылайте текстом, место — кнопкой «Отправить местоположение».")
		return
	}

	key, err := b.savePhoto(ctx, s.bind.RunID, url)
	if err != nil {
		b.log.Error("vk: фото игрока", "user", user, "err", err)
		b.reply(ctx, user, "⚠️ Не удалось получить фото из VK. Пришлите его ещё раз.")
		return
	}
	note := ""
	if count > 1 {
		note = "\nВ сообщении несколько фото — взял первое."
	}

	if len(targets) == 1 {
		b.submitPhoto(ctx, user, s, m.ID, key, targets[0], note)
		return
	}
	b.setModeFull(user, mode{kind: modePhoto, mediaKey: key, msgID: m.ID}, hostModeTTL)
	var rows [][]Button
	for _, t := range targets {
		if len(rows) == 5 {
			break
		}
		rows = append(rows, []Button{textButton(short(t.label, 40), "primary", payload{Cmd: cmdPhotoTo, Level: t.levelID, Bonus: t.bonusID})})
	}
	rows = append(rows, []Button{textButton("Отмена", "secondary", payload{Cmd: cmdCancel})})
	b.send(ctx, out{peer: user, text: "📷 К чему это фото?" + note, keyboard: inline(rows...)})
}

func (b *Bot) savePhoto(ctx context.Context, runID, url string) (string, error) {
	body, err := b.cfg.API.Download(ctx, url)
	if err != nil {
		return "", err
	}
	defer body.Close()
	return b.cfg.Media.Save(ctx, runID, body)
}

// photoChosen — игрок выбрал кнопкой, к чему фото.
func (b *Bot) photoChosen(ctx context.Context, user int64, s *session, p payload) {
	md := b.modeOf(user)
	if md.kind != modePhoto || md.mediaKey == "" {
		b.reply(ctx, user, "Фото уже не ждёт выбора — пришлите его ещё раз.")
		return
	}
	b.setMode(user, "", 0)
	b.submitPhoto(ctx, user, s, md.msgID, md.mediaKey, photoTarget{levelID: p.Level, bonusID: p.Bonus}, "")
}

func (b *Bot) submitPhoto(ctx context.Context, user int64, s *session, msgID int64, key string, t photoTarget, note string) {
	res, err := s.rn.Do(ctx, core.SubmitAnswer{
		TeamID: s.teamID, PlayerID: s.bind.PlayerID, Kind: core.KindPhoto,
		LevelID: t.levelID, BonusID: t.bonusID, MediaKey: key,
		ClientEventID: "vk-" + strconv.FormatInt(msgID, 10),
	})
	switch {
	case errors.Is(err, core.ErrAlreadyPending):
		b.reply(ctx, user, "⏳ Фото к этому заданию уже ждёт проверки ведущего.")
		return
	case err != nil:
		b.reply(ctx, user, "⚠️ "+humanErr(err)+".")
		return
	}
	switch {
	case res.Answer != nil && res.Answer.Result == core.ResultDuplicate:
		b.reply(ctx, user, "Это задание уже засчитано.")
	default:
		b.reply(ctx, user, "📷 Фото отправлено ведущему на проверку. Решение придёт сюда."+note)
	}
}
