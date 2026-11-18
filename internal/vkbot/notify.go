package vkbot

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/store"
)

// watch запускает рассылку уведомлений по игре (один раз на игру).
func (b *Bot) watch(runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watching[runID] || b.runCtx == nil {
		return
	}
	b.watching[runID] = true
	ctx := b.runCtx
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer func() {
			b.mu.Lock()
			delete(b.watching, runID)
			b.mu.Unlock()
		}()
		b.watchLoop(ctx, runID)
	}()
}

func (b *Bot) watchLoop(ctx context.Context, runID string) {
	for ctx.Err() == nil {
		rn, err := b.cfg.Runners.Get(ctx, runID)
		if errors.Is(err, store.ErrNotFound) {
			return // игра удалена
		}
		if err != nil {
			b.log.Error("vk: актор игры", "run", runID, "err", err)
			if !sleepCtx(ctx, 5*time.Second) {
				return
			}
			continue
		}
		ch, cancel := rn.Subscribe("")
		b.syncRun(ctx, runID, rn)
	listen:
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					break listen
				}
				b.syncRun(ctx, runID, rn)
			case <-ctx.Done():
				cancel()
				return
			}
		}
		cancel()
		// Актор остановился (замена сценария, удаление) — поднимем заново.
		if !sleepCtx(ctx, time.Second) {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// syncRun рассылает уведомления по событиям, появившимся после курсора.
func (b *Bot) syncRun(ctx context.Context, runID string, rn *runner.Runner) {
	if err := b.SyncRun(ctx, runID, rn); err != nil && ctx.Err() == nil {
		b.log.Error("vk: рассылка", "run", runID, "err", err)
	}
}

// SyncRun — один проход рассылки (экспортирован для тестов).
func (b *Bot) SyncRun(ctx context.Context, runID string, rn *runner.Runner) error {
	cursor, ok, err := b.cfg.VK.VKCursor(ctx, runID)
	if err != nil {
		return err
	}
	events, err := b.cfg.Store.Load(ctx, runID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	last := events[len(events)-1].Seq
	if !ok {
		// Первый запуск по игре — старое не рассылаем.
		return b.cfg.VK.SetVKCursor(ctx, runID, last)
	}
	if last <= cursor {
		return nil
	}
	binds, err := b.cfg.VK.ListVK(ctx)
	if err != nil {
		return err
	}
	var users []store.VKBinding
	for _, bd := range binds {
		if bd.RunID == runID {
			users = append(users, bd)
		}
	}

	bySeq := make(map[int64]core.Event, len(events))
	for _, e := range events {
		bySeq[e.Seq] = e
	}
	var (
		outs    []out
		removed []int64
	)
	err = rn.Read(ctx, func(g *core.Game) {
		n := notifier{b: b, g: g, users: users, bySeq: bySeq}
		for _, e := range events {
			if e.Seq > cursor {
				n.event(e)
			}
		}
		outs, removed = n.outs, n.removed
	})
	if err != nil {
		return err
	}
	for _, o := range outs {
		b.send(ctx, o)
	}
	for _, u := range removed {
		if err := b.cfg.VK.UnbindVK(ctx, u); err != nil {
			b.log.Error("vk: отвязка", "user", u, "err", err)
		}
	}
	return b.cfg.VK.SetVKCursor(ctx, runID, last)
}

// notifier превращает события в сообщения; работает внутри актора.
type notifier struct {
	b       *Bot
	g       *core.Game
	users   []store.VKBinding
	bySeq   map[int64]core.Event
	outs    []out
	removed []int64
}

// team — VK-пользователи команды (кроме удалённых игроков).
func (n *notifier) team(teamID string) []store.VKBinding {
	var out []store.VKBinding
	for _, u := range n.users {
		if p := n.g.Players[u.PlayerID]; p != nil && !p.Removed && p.TeamID == teamID {
			out = append(out, u)
		}
	}
	return out
}

func (n *notifier) all() []store.VKBinding {
	var out []store.VKBinding
	for _, u := range n.users {
		if p := n.g.Players[u.PlayerID]; p != nil && !p.Removed {
			out = append(out, u)
		}
	}
	return out
}

func (n *notifier) toTeam(teamID string, o out) {
	for _, u := range n.team(teamID) {
		o.peer = u.UserID
		n.outs = append(n.outs, o)
	}
}

func (n *notifier) levelTitle(id string) string {
	if sl, ok := n.g.Scenario.Level(id); ok {
		return cmp.Or(sl.Title, sl.ID)
	}
	return id
}

func (n *notifier) event(e core.Event) {
	g, b := n.g, n.b
	switch p := e.Payload.(type) {
	case core.TeamStartScheduled:
		if p.StartAt.Sub(b.now()) > time.Minute {
			n.toTeam(e.TeamID, out{text: "⏱ Старт вашей команды в " + b.clock(p.StartAt) + "."})
		}

	case core.TeamStarted:
		n.toTeam(e.TeamID, out{text: "🚀 Старт! Задание — следующим сообщением."})

	case core.LevelOpened:
		t := g.Teams[e.TeamID]
		sl, ok := g.Scenario.Level(p.LevelID)
		ls := t.Levels[p.LevelID]
		if !ok || ls == nil {
			return
		}
		for _, u := range n.team(e.TeamID) {
			o := b.levelMessage(g, t, sl, ls)
			o.peer = u.UserID
			if o.keyboard == nil {
				o.keyboard = mainKeyboard(t.CaptainID == u.PlayerID)
			}
			n.outs = append(n.outs, o)
		}

	case core.HintOpened:
		sl, ok := g.Scenario.Level(p.LevelID)
		if !ok || p.HintIdx >= len(sl.Hints) {
			return
		}
		h := sl.Hints[p.HintIdx]
		text := fmt.Sprintf("💡 Подсказка %d — «%s»", p.HintIdx+1, cmp.Or(sl.Title, sl.ID))
		if pen := penaltyText(p.PenaltyTime, p.PenaltyPoints); pen != "" {
			text += " (" + pen + ")"
		}
		if h.Text != "" {
			text += "\n\n" + h.Text
		}
		n.toTeam(e.TeamID, out{text: text, assets: h.Media})

	case core.LevelCompleted:
		if p.Skipped {
			n.toTeam(e.TeamID, out{text: "⏭ Уровень «" + n.levelTitle(p.LevelID) + "» слит."})
		} else {
			n.toTeam(e.TeamID, out{text: "✅ Уровень «" + n.levelTitle(p.LevelID) + "» пройден."})
		}

	case core.BonusCompleted:
		n.toTeam(e.TeamID, out{text: "⭐ Бонус «" + n.bonusTitle(p.LevelID, p.BonusID) + "» засчитан."})

	case core.ModerationResolved:
		ref, ok := n.bySeq[p.Ref]
		a, isAnswer := ref.Payload.(core.AnswerSubmitted)
		if !ok || !isAnswer {
			return
		}
		what := "Ответ"
		if a.Kind == core.KindPhoto {
			what = "Фото"
		}
		target := "к уровню «" + n.levelTitle(a.LevelID) + "»"
		if a.BonusID != "" {
			target = "к бонусу «" + n.bonusTitle(a.LevelID, a.BonusID) + "»"
		}
		text := "✅ " + what + " " + target + " принято."
		if !p.Approved {
			text = "❌ " + what + " " + target + " не принято."
		}
		if p.Comment != "" {
			text += "\nКомментарий: " + p.Comment
		}
		n.toTeam(ref.TeamID, out{text: text})

	case core.LocationRevealed:
		sl, ok := g.Scenario.Level(p.LevelID)
		if !ok || sl.Location == nil {
			return
		}
		text := "📍 Локация уровня «" + cmp.Or(sl.Title, sl.ID) + "»"
		if pen := penaltyText(p.PenaltyTime, p.PenaltyPoints); pen != "" {
			text += " (" + pen + ")"
		}
		o := out{}
		if sl.Location.Text != "" {
			text += "\n" + sl.Location.Text
		}
		if pt := sl.LocationPoint(); pt != nil {
			lat, lng := pt[0], pt[1]
			o.lat, o.lng = &lat, &lng
			text += "\n" + mapsLinks(lat, lng)
		}
		o.text = text
		n.toTeam(e.TeamID, o)

	case core.TeamFinished:
		text := "🏁 Финиш! Ваша команда прошла игру."
		switch p.Reason {
		case "time_limit":
			text = "🏁 Время вышло — игра для вашей команды окончена."
		case "admin":
			text = "🏁 Ведущий завершил игру для вашей команды."
		}
		n.toTeam(e.TeamID, out{text: text})

	case core.AdminOverride:
		switch p.Action {
		case core.OverrideAdjustTime, core.OverrideAdjustPoints:
			text := "⚖️ Решение ведущего: " + adjustText(p.Time, p.Points)
			if p.Reason != "" {
				text += "\nПричина: " + p.Reason
			}
			n.toTeam(e.TeamID, out{text: text})
		}

	case core.Broadcast:
		o := out{text: "📢 " + p.Text}
		if p.MediaKey != "" {
			o.keys = []string{p.MediaKey}
		}
		for _, u := range n.all() {
			o.peer = u.UserID
			n.outs = append(n.outs, o)
		}

	case core.ChatPosted:
		if !p.FromHost {
			return
		}
		o := out{text: "💬 Ведущий: " + p.Text}
		if p.MediaKey != "" {
			o.keys = []string{p.MediaKey}
		}
		n.toTeam(e.TeamID, o)

	case core.SOSResolved:
		for _, a := range g.SOS {
			if a.Seq == p.Ref {
				text := "🆘 Ведущий принял сигнал."
				if p.Note != "" {
					text += "\n" + p.Note
				}
				n.toTeam(a.TeamID, out{text: text})
			}
		}

	case core.ResultsApproved:
		for _, t := range g.ActiveTeams() {
			text := "🏆 Итоги утверждены."
			for _, st := range p.Standings {
				if st.TeamID == t.ID {
					text += " Ваше место: " + strconv.Itoa(st.Rank) + " из " + strconv.Itoa(len(p.Standings)) + "."
				}
			}
			if p.Message != "" {
				text += "\n\n" + p.Message
			}
			n.toTeam(t.ID, out{text: text})
		}

	case core.RunFinished:
		for _, u := range n.all() {
			n.outs = append(n.outs, out{peer: u.UserID, text: "Игра завершена. Спасибо, что играли!"})
		}

	case core.CaptainChanged:
		t := g.Teams[e.TeamID]
		name := ""
		if pl := g.Players[p.PlayerID]; pl != nil {
			name = pl.Name
		}
		for _, u := range n.team(e.TeamID) {
			text := "👑 Капитан команды теперь — " + name + "."
			if u.PlayerID == p.PlayerID {
				text = "👑 Теперь вы капитан команды."
			}
			n.outs = append(n.outs, out{peer: u.UserID, text: text, keyboard: mainKeyboard(t.CaptainID == u.PlayerID)})
		}

	case core.PlayerRemoved:
		for _, u := range n.users {
			if u.PlayerID == p.PlayerID {
				n.outs = append(n.outs, out{peer: u.UserID, text: "Вас убрали из команды. Чтобы играть снова, пришлите приглашение.", keyboard: emptyKeyboard()})
				n.removed = append(n.removed, u.UserID)
			}
		}
	}
}

func (n *notifier) bonusTitle(levelID, bonusID string) string {
	if sl, ok := n.g.Scenario.Level(levelID); ok {
		for _, bn := range sl.Bonuses {
			if bn.ID == bonusID {
				return cmp.Or(bn.Title, bn.ID)
			}
		}
	}
	return bonusID
}
