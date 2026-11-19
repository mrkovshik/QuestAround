package vkbot

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"questaround/internal/core"
	"questaround/internal/scenario"
)

// Команды кнопок.
const (
	cmdStart    = "start"
	cmdTask     = "task"
	cmdHint     = "hint"
	cmdHintTake = "hint_take"
	cmdSliv     = "sliv"
	cmdLoc      = "loc"
	cmdSkip     = "skip"
	cmdHost     = "host"
	cmdSOS      = "sos"
	cmdSOSSend  = "sos_send"
	cmdCancel   = "cancel"
	cmdLeave    = "leave"
	cmdGames    = "games"
	cmdGame     = "game"
	cmdNewTeam  = "new_team"
	cmdJoinTeam = "join_team"
	cmdTeam     = "team"
)

const (
	labelTask = "📋 Задание"
	labelHint = "💡 Подсказка"
	labelSliv = "⏭ Сливы"
	labelHost = "💬 Ведущему"
	labelSOS  = "🆘 Помощь"
)

// mainKeyboard — постоянная клавиатура игрока. Сливы и чат с ведущим — у капитана.
func mainKeyboard(captain bool) *Keyboard {
	kb := &Keyboard{Buttons: [][]Button{
		{textButton(labelTask, "primary", payload{Cmd: cmdTask}), textButton(labelHint, "secondary", payload{Cmd: cmdHint})},
		{locationButton()},
	}}
	if captain {
		kb.Buttons = append(kb.Buttons, []Button{
			textButton(labelSliv, "secondary", payload{Cmd: cmdSliv}),
			textButton(labelHost, "secondary", payload{Cmd: cmdHost}),
		})
	}
	kb.Buttons = append(kb.Buttons, []Button{
		textButton(labelTeam, "secondary", payload{Cmd: cmdTeam}),
		textButton(labelSOS, "negative", payload{Cmd: cmdSOS}),
	})
	return kb
}

func inline(rows ...[]Button) *Keyboard {
	return &Keyboard{Inline: true, Buttons: rows}
}

// --- время и штрафы ---

func (b *Bot) clock(t time.Time) string { return t.In(b.cfg.Location).Format("15:04") }

// dur — «1 ч 05 мин», «12 мин», «40 сек».
func dur(d time.Duration) string {
	if d < time.Minute {
		s := max(int(d.Round(time.Second)/time.Second), 0)
		return strconv.Itoa(s) + " сек"
	}
	d = d.Round(time.Minute)
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	if h == 0 {
		return strconv.Itoa(m) + " мин"
	}
	if m == 0 {
		return strconv.Itoa(h) + " ч"
	}
	return fmt.Sprintf("%d ч %02d мин", h, m)
}

// penaltyText — «штраф 10 мин» / «штраф 5 баллов»; пусто — бесплатно.
func penaltyText(d time.Duration, pts int) string {
	switch {
	case d > 0:
		return "штраф " + dur(d)
	case pts > 0:
		return "штраф " + points(pts)
	}
	return ""
}

func scenarioPenalty(p *scenario.Penalty) string {
	if p == nil {
		return ""
	}
	return penaltyText(p.Time.D(), p.Points)
}

func points(n int) string {
	n10, n100 := n%10, n%100
	switch {
	case n10 == 1 && n100 != 11:
		return strconv.Itoa(n) + " балл"
	case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
		return strconv.Itoa(n) + " балла"
	}
	return strconv.Itoa(n) + " баллов"
}

func adjustText(d time.Duration, pts int) string {
	switch {
	case d > 0:
		return "+" + dur(d) + " штрафа"
	case d < 0:
		return "−" + dur(-d) + " к времени"
	case pts > 0:
		return "+" + points(pts)
	case pts < 0:
		return "−" + points(-pts)
	}
	return "без изменений"
}

func mapsLinks(lat, lng float64) string {
	ll := strconv.FormatFloat(lat, 'f', 6, 64) + "," + strconv.FormatFloat(lng, 'f', 6, 64)
	yll := strconv.FormatFloat(lng, 'f', 6, 64) + "," + strconv.FormatFloat(lat, 'f', 6, 64)
	return "Яндекс Карты: https://yandex.ru/maps/?pt=" + yll + "&z=17\nGoogle Карты: https://maps.google.com/?q=" + ll
}

// short обрезает подпись кнопки (VK — до 40 символов).
func short(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// --- уровень ---

// levelPosition — «Уровень 2 из 5» для linear, иначе пусто.
func levelPosition(t *core.Team, levelID string) string {
	if i := slices.Index(t.Route, levelID); i >= 0 {
		return fmt.Sprintf("Уровень %d из %d", i+1, len(t.Route))
	}
	return ""
}

// levelMessage — задание уровня со всем, что нужно знать игроку.
func (b *Bot) levelMessage(g *core.Game, t *core.Team, sl *scenario.Level, ls *core.LevelState) out {
	now := b.now()
	var sb strings.Builder
	head := "📍 "
	if pos := levelPosition(t, sl.ID); pos != "" {
		head += pos + " · "
	}
	head += "«" + cmp.Or(sl.Title, sl.ID) + "»"
	sb.WriteString(head)
	if lim := g.Scenario.Timing.TimeLimit.D(); lim > 0 && !t.StartedAt.IsZero() {
		if left := t.StartedAt.Add(lim).Sub(now); left > 0 {
			sb.WriteString("\n⏳ До конца игры: " + dur(left))
		}
	}
	sb.WriteString("\n\n" + strings.TrimSpace(sl.Task))

	// Ответы.
	need := sl.Require.Needed(len(sl.Answers))
	if need > 1 || need < len(sl.Answers) {
		fmt.Fprintf(&sb, "\n\nНужно ответов: %d из %d, засчитано %d.", need, len(sl.Answers), ls.SolvedCount())
	}
	var choiceRows [][]Button
	kinds := map[string]bool{}
	for i, a := range sl.Answers {
		if i < len(ls.Solved) && ls.Solved[i] {
			continue
		}
		switch a.Type {
		case scenario.AnswerChoice:
			kinds["code"] = true
			for _, opt := range a.Options {
				if len(choiceRows) < 6 {
					choiceRows = append(choiceRows, []Button{plainButton(short(opt, 40))})
				}
			}
		case scenario.AnswerCode, scenario.AnswerRegex:
			kinds["code"] = true
		default:
			kinds[string(a.Type)] = true
		}
	}
	if kinds["code"] {
		sb.WriteString("\n\n✍️ Ответ — пришлите его сообщением.")
	}
	if kinds["geo"] {
		sb.WriteString("\n\n📍 Ответ — место: на точке нажмите «Отправить местоположение» (или скрепка → Место).")
	}
	if kinds["photo"] {
		sb.WriteString("\n\n📷 Ответ — фото: через VK пока не принимается. Отправьте в приложении, когда будет связь, или покажите ведущему.")
	}
	if kinds["text"] {
		sb.WriteString("\n\n📝 Ответ — свободный текст: капитан отправляет его через «Ведущему».")
	}

	// Подсказки.
	for i, h := range sl.Hints {
		opened := i < len(ls.Hints) && ls.Hints[i]
		switch {
		case opened:
			if h.Text != "" {
				fmt.Fprintf(&sb, "\n\n💡 Подсказка %d: %s", i+1, h.Text)
			} else {
				fmt.Fprintf(&sb, "\n\n💡 Подсказка %d открыта (картинка).", i+1)
			}
		case h.OnRequest:
			line := fmt.Sprintf("\n\n💡 Подсказка %d — по запросу", i+1)
			if pen := scenarioPenalty(h.Penalty); pen != "" {
				line += ", " + pen
			}
			if av := ls.OpenedAt.Add(h.AvailableAfter.D()); h.AvailableAfter > 0 && av.After(now) {
				line += ", можно взять в " + b.clock(av)
			}
			if h.IsAuto() {
				line += "; сама откроется в " + b.clock(ls.OpenedAt.Add(h.After.D()))
			}
			sb.WriteString(line + ".")
		case h.IsAuto():
			fmt.Fprintf(&sb, "\n\n💡 Подсказка %d откроется в %s.", i+1, b.clock(ls.OpenedAt.Add(h.After.D())))
		}
	}

	// Сливы.
	if ls.LocationRevealed && sl.Location != nil {
		sb.WriteString("\n\n📍 Локация: " + cmp.Or(sl.Location.Text, "точка на карте"))
		if pt := sl.LocationPoint(); pt != nil {
			sb.WriteString("\n" + mapsLinks(pt[0], pt[1]))
		}
	}

	// Бонусы.
	for _, bn := range sl.Bonuses {
		line := "\n\n⭐ Бонус «" + cmp.Or(bn.Title, bn.ID) + "»"
		switch {
		case bn.Time > 0:
			line += " (−" + dur(bn.Time.D()) + ")"
		case bn.Points > 0:
			line += " (+" + points(bn.Points) + ")"
		}
		switch {
		case t.Bonuses[bn.ID]:
			line += " — засчитан ✓"
		case t.PendingBonus[bn.ID] != 0:
			line += " — на проверке"
		case bn.Answer.Type.NeedsModeration():
			line += " — ответ через приложение или ведущего"
		case bn.Answer.Type == scenario.AnswerGeo:
			line += " — отправьте местоположение на точке"
		default:
			line += " — пришлите ответ сообщением"
		}
		sb.WriteString(line)
	}

	o := out{text: sb.String(), assets: slices.Clone(sl.Media)}
	for i, h := range sl.Hints {
		if i < len(ls.Hints) && ls.Hints[i] {
			o.assets = append(o.assets, h.Media...)
		}
	}
	if len(choiceRows) > 0 {
		o.keyboard = inline(choiceRows...)
	}
	return o
}

// openLevels — открытые незавершённые уровни команды по времени открытия.
func openLevels(g *core.Game, t *core.Team) []*scenario.Level {
	var out []*scenario.Level
	for i := range g.Scenario.Levels {
		sl := &g.Scenario.Levels[i]
		if t.Levels[sl.ID].IsOpen() {
			out = append(out, sl)
		}
	}
	slices.SortStableFunc(out, func(a, b *scenario.Level) int {
		return t.Levels[a.ID].OpenedAt.Compare(t.Levels[b.ID].OpenedAt)
	})
	return out
}

// statusMessages — «что сейчас происходит» для игрока: ожидание старта,
// текущие задания или итог.
func (b *Bot) statusMessages(g *core.Game, teamID string, captain bool) []out {
	t := g.Teams[teamID]
	kb := mainKeyboard(captain)
	switch {
	case t.Status == core.TeamStatusWithdrawn:
		return []out{{text: "Команда снялась с игры."}}

	case t.Status == core.TeamStatusFinished || g.Status == core.RunStatusFinished:
		text := "🏁 Игра для вашей команды окончена."
		if g.Results != nil {
			for _, st := range g.Results.Standings {
				if st.TeamID == teamID {
					text += fmt.Sprintf(" Место: %d из %d.", st.Rank, len(g.Results.Standings))
				}
			}
			if g.Results.Message != "" {
				text += "\n\n" + g.Results.Message
			}
		}
		return []out{{text: text, keyboard: kb}}

	case t.Status == core.TeamStatusPlaying:
		levels := openLevels(g, t)
		if len(levels) == 0 {
			return []out{{text: "Открытых заданий нет — ждём решения ведущего.", keyboard: kb}}
		}
		var outs []out
		for _, sl := range levels {
			o := b.levelMessage(g, t, sl, t.Levels[sl.ID])
			if o.keyboard == nil {
				o.keyboard = kb
			}
			outs = append(outs, o)
		}
		return outs
	}

	text := "⏳ Игра ещё не началась."
	if t.Status == core.TeamStatusScheduled && !t.ScheduledAt.IsZero() {
		text = "⏳ Старт вашей команды в " + b.clock(t.ScheduledAt) + "."
	}
	text += " Задание придёт сюда сообщением."
	return []out{{text: text, keyboard: kb}}
}

// teamLine — «Команда «Ёжики» · капитан Иван П. · игроков: 3».
func teamLine(g *core.Game, t *core.Team) string {
	captain := ""
	if p := g.Players[t.CaptainID]; p != nil {
		captain = p.Name
	}
	s := "Команда «" + t.Name + "»"
	if captain != "" {
		s += " · капитан " + captain
	}
	return s + " · игроков: " + strconv.Itoa(len(g.ActivePlayers(t.ID)))
}
