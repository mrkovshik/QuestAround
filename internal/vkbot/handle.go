package vkbot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// mode — бот ждёт от пользователя особого сообщения (не ответа на задание).
type mode struct {
	kind   string // cmdHost | cmdSOS | modeTeamName | modeTeamCode
	until  time.Time
	runID  string // для регистрации: выбранная игра
	teamID string // и команда
}

const (
	hostModeTTL = 10 * time.Minute
	sosModeTTL  = 30 * time.Minute
)

func (b *Bot) getMode(user int64) string { return b.modeOf(user).kind }

func (b *Bot) modeOf(user int64) mode {
	b.mu.Lock()
	defer b.mu.Unlock()
	m, ok := b.modes[user]
	if !ok || b.now().After(m.until) {
		delete(b.modes, user)
		return mode{}
	}
	return m
}

func (b *Bot) setModeFull(user int64, m mode, ttl time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	m.until = b.now().Add(ttl)
	b.modes[user] = m
}

func (b *Bot) setMode(user int64, kind string, ttl time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if kind == "" {
		delete(b.modes, user)
		return
	}
	b.modes[user] = mode{kind: kind, until: b.now().Add(ttl)}
}

// slash — текстовые команды для тех, у кого не видно кнопок.
var slash = map[string]string{
	"/start": cmdStart, "/help": cmdStart, "/старт": cmdStart, "/помощь_по_боту": cmdStart, "начать": cmdStart,
	"/задание": cmdTask, "/task": cmdTask,
	"/подсказка": cmdHint, "/hint": cmdHint,
	"/сливы": cmdSliv, "/слив": cmdSliv,
	"/ведущему": cmdHost, "/host": cmdHost,
	"/помощь": cmdSOS, "/sos": cmdSOS, "/сос": cmdSOS,
	"/отмена": cmdCancel, "/cancel": cmdCancel,
	"/выход": cmdLeave, "/leave": cmdLeave,
	"/игры": cmdGames, "/games": cmdGames, "игры": cmdGames,
	"/команда": cmdTeam, "/team": cmdTeam,
}

// HandleMessage обрабатывает входящее сообщение (экспортировано для тестов).
func (b *Bot) HandleMessage(ctx context.Context, m Message) { b.handle(ctx, m) }

func (b *Bot) handle(ctx context.Context, m Message) {
	if m.PeerID != m.FromID || m.FromID <= 0 {
		return // беседы и сообщения от сообществ не обслуживаем
	}
	user := m.FromID
	text := strings.TrimSpace(m.Text)
	p := parsePayload(m.Payload)
	if p.Cmd == "" {
		p.Cmd = slash[strings.ToLower(text)]
	}

	s, err := b.session(ctx, user)
	if err != nil {
		b.log.Error("vk: сессия", "user", user, "err", err)
		b.reply(ctx, user, "Что-то пошло не так на сервере. Попробуйте ещё раз через минуту.")
		return
	}

	// Регистрация: список игр, карточка игры, создание команды, вступление.
	switch p.Cmd {
	case cmdGames:
		b.gamesList(ctx, user)
		return
	case cmdGame:
		b.gameCardByID(ctx, user, p.Run)
		return
	case cmdNewTeam:
		b.startNewTeam(ctx, user, s, p.Run)
		return
	case cmdJoinTeam:
		b.startJoinTeam(ctx, user, p.Run, p.Team)
		return
	}
	if md := b.modeOf(user); md.kind == modeTeamName || md.kind == modeTeamCode {
		switch {
		case p.Cmd == cmdCancel:
			b.setMode(user, "", 0)
			if s != nil {
				b.send(ctx, out{peer: user, text: "Отменено.", keyboard: mainKeyboard(s.captain)})
			} else {
				b.send(ctx, out{peer: user, text: "Отменено.", keyboard: guestKeyboard()})
			}
			return
		case p.Cmd == "" && md.kind == modeTeamName:
			run, err := b.cfg.Store.GetRun(ctx, md.runID)
			rn, err2 := b.cfg.Runners.Get(ctx, md.runID)
			if err != nil || err2 != nil {
				b.setMode(user, "", 0)
				b.reply(ctx, user, "Игра не найдена.")
				return
			}
			b.register(ctx, user, run, rn, text)
			return
		case p.Cmd == "" && md.kind == modeTeamCode:
			b.joinChosenTeam(ctx, user, s, md, text)
			return
		}
		b.setMode(user, "", 0) // другая команда — выходим из регистрации
	}

	// Код переноса из приложения: тот же игрок, что играл на сайте.
	if code, ok := parseTransfer(text); ok && p.Cmd == "" {
		b.transfer(ctx, user, code)
		return
	}
	// Приглашение работает всегда: так можно перейти в другую игру или команду.
	if inv, ok := parseInvite(text, s == nil); ok && p.Cmd == "" {
		b.join(ctx, user, s, inv)
		return
	}
	if code, ok := parseGameLink(text, s == nil); ok && p.Cmd == "" {
		if b.gameCardByCode(ctx, user, code) {
			return
		}
	}
	if s == nil {
		b.greet(ctx, user)
		return
	}

	switch p.Cmd {
	case cmdStart, cmdTask:
		b.setMode(user, "", 0)
		b.sendStatus(ctx, user, s)
		return
	case cmdHint:
		b.hintMenu(ctx, user, s)
		return
	case cmdHintTake:
		b.do(ctx, user, s, core.RequestHint{TeamID: s.teamID, PlayerID: s.bind.PlayerID, LevelID: p.Level, HintIdx: p.Idx}, "")
		return
	case cmdSliv:
		b.slivMenu(ctx, user, s)
		return
	case cmdLoc:
		b.do(ctx, user, s, core.RevealLocation{TeamID: s.teamID, PlayerID: s.bind.PlayerID, LevelID: p.Level}, "")
		return
	case cmdSkip:
		b.do(ctx, user, s, core.SkipLevel{TeamID: s.teamID, PlayerID: s.bind.PlayerID, LevelID: p.Level}, "")
		return
	case cmdHost:
		b.startHost(ctx, user, s)
		return
	case cmdSOS:
		b.startSOS(ctx, user, s)
		return
	case cmdSOSSend:
		b.raiseSOS(ctx, user, s, "Сигнал из VK без описания", nil)
		return
	case cmdCancel:
		b.setMode(user, "", 0)
		b.send(ctx, out{peer: user, text: "Хорошо, продолжаем игру.", keyboard: mainKeyboard(s.captain)})
		return
	case cmdLeave:
		b.leave(ctx, user)
		return
	case cmdTeam:
		b.teamCard(ctx, user, s)
		return
	}

	switch b.getMode(user) {
	case cmdHost:
		b.postHost(ctx, user, s, m)
		return
	case cmdSOS:
		b.raiseSOS(ctx, user, s, text, m.Geo)
		return
	}

	switch {
	case m.Geo != nil:
		c := m.Geo.Coordinates
		b.answer(ctx, user, s, m, core.SubmitAnswer{Kind: core.KindGeo, Lat: c.Latitude, Lng: c.Longitude})
	case text != "":
		b.answer(ctx, user, s, m, core.SubmitAnswer{Kind: core.KindCode, Value: text})
	case hasPhoto(m):
		b.reply(ctx, user, "📷 Фото через VK пока не принимаются. Отправьте его в приложении, когда появится связь, или покажите ведущему — он засчитает вручную.")
	default:
		b.reply(ctx, user, "Пришлите ответ текстом или местоположение. Кнопки — внизу, «📋 Задание» покажет текущее задание.")
	}
}

func hasPhoto(m Message) bool {
	for _, a := range m.Attaches {
		if a.Type == "photo" {
			return true
		}
	}
	return false
}

// --- вход в игру ---

type invite struct {
	runCode string // пусто — искать по всем недавним играм
	code    string
}

var (
	inviteLinkRe = regexp.MustCompile(`(?i)/g/([a-z0-9]+)/join/([a-z0-9]+)`)
	codeRe       = regexp.MustCompile(`^[A-Z0-9]+$`)
)

// parseInvite узнаёт приглашение: ссылку …/g/КОД/join/ИНВАЙТ всегда,
// а «КОД ИНВАЙТ» или голый инвайт — только если пользователь ещё не играет
// (иначе это может быть ответ на задание).
func parseInvite(text string, bare bool) (invite, bool) {
	if m := inviteLinkRe.FindStringSubmatch(text); m != nil {
		return invite{runCode: strings.ToUpper(m[1]), code: strings.ToUpper(m[2])}, true
	}
	if !bare {
		return invite{}, false
	}
	f := strings.FieldsFunc(strings.ToUpper(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	switch {
	case len(f) == 1 && len(f[0]) == 8 && codeRe.MatchString(f[0]):
		return invite{code: f[0]}, true
	case len(f) == 2 && len(f[0]) == 6 && len(f[1]) == 8 && codeRe.MatchString(f[0]+f[1]):
		return invite{runCode: f[0], code: f[1]}, true
	}
	return invite{}, false
}

var transferRe = regexp.MustCompile(`(?i)\bQA[\s-]?([A-Z0-9]{4})[\s-]?([A-Z0-9]{4})\b`)

// parseTransfer узнаёт код переноса QA-XXXX-XXXX (с дефисами, пробелами или без).
func parseTransfer(text string) (string, bool) {
	m := transferRe.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return "QA-" + strings.ToUpper(m[1]) + "-" + strings.ToUpper(m[2]), true
}

func (b *Bot) greet(ctx context.Context, user int64) {
	b.send(ctx, out{peer: user, keyboard: guestKeyboard(), text: "Привет! Это бот городских квестов QuestAround.\n\n" +
		"🎲 «Доступные игры» — записаться в игру: создать команду или вступить в команду по коду.\n\n" +
		"Уже играете в приложении? Нажмите там «Играть через VK» и пришлите сюда код вида QA-XXXX-XXXX — " +
		"продолжите тем же игроком.\n\n" +
		"Новый игрок? Пришлите приглашение своей команды — ссылку вида …/g/ABC123/join/XXXXXXXX " +
		"или код приглашения из 8 символов. Его даёт капитан (в приложении: «Команда» → «Пригласить»)."})
}

// transfer привязывает VK к игроку, который уже играет в приложении.
func (b *Bot) transfer(ctx context.Context, user int64, code string) {
	runs, err := b.cfg.Store.ListRuns(ctx, b.cfg.OrgID)
	if err != nil {
		b.reply(ctx, user, "Что-то пошло не так на сервере. Попробуйте ещё раз через минуту.")
		return
	}
	since := b.now().Add(-b.cfg.SearchWindow)
	for _, run := range runs {
		if run.CreatedAt.Before(since) {
			continue
		}
		rn, err := b.cfg.Runners.Get(ctx, run.ID)
		if err != nil {
			continue
		}
		playerID := ""
		_ = rn.Read(ctx, func(g *core.Game) {
			if p, ok := g.PlayerByTransferCode(code); ok {
				playerID = p.ID
			}
		})
		if playerID == "" {
			continue
		}
		if err := b.cfg.VK.BindVK(ctx, store.VKBinding{UserID: user, RunID: run.ID, PlayerID: playerID}); err != nil {
			b.log.Error("vk: привязка по коду переноса", "user", user, "err", err)
			b.reply(ctx, user, "Что-то пошло не так на сервере. Пришлите код ещё раз.")
			return
		}
		b.setMode(user, "", 0)
		b.watch(run.ID)
		b.welcome(ctx, user, run, rn, playerID, true)
		return
	}
	b.reply(ctx, user, "❌ Код не подошёл. Проверьте его в приложении («Связь» → «Играть через VK») или спросите у ведущего.")
}

// findInvite — игра и команда по приглашению.
func (b *Bot) findInvite(ctx context.Context, inv invite) (store.Run, *runner.Runner, error) {
	if inv.runCode != "" {
		run, err := b.cfg.Store.GetRunByCode(ctx, inv.runCode)
		if err != nil {
			return run, nil, core.ErrUnknownInvite
		}
		rn, err := b.cfg.Runners.Get(ctx, run.ID)
		return run, rn, err
	}
	runs, err := b.cfg.Store.ListRuns(ctx, b.cfg.OrgID)
	if err != nil {
		return store.Run{}, nil, err
	}
	since := b.now().Add(-b.cfg.SearchWindow)
	for _, run := range runs {
		if run.CreatedAt.Before(since) {
			continue
		}
		rn, err := b.cfg.Runners.Get(ctx, run.ID)
		if err != nil {
			continue
		}
		found := false
		_ = rn.Read(ctx, func(g *core.Game) {
			if t, ok := g.TeamByInvite(inv.code); ok {
				found = g.Status != core.RunStatusFinished && t.Status != core.TeamStatusFinished
			}
		})
		if found {
			return run, rn, nil
		}
	}
	return store.Run{}, nil, core.ErrUnknownInvite
}

func (b *Bot) join(ctx context.Context, user int64, cur *session, inv invite) {
	run, rn, err := b.findInvite(ctx, inv)
	if err != nil {
		b.reply(ctx, user, "❌ "+humanErr(err)+". Проверьте ссылку или код у капитана.")
		return
	}
	// Уже в этой команде — просто покажем, что происходит.
	if cur != nil && cur.bind.RunID == run.ID {
		same := false
		_ = rn.Read(ctx, func(g *core.Game) {
			t, ok := g.TeamByInvite(inv.code)
			same = ok && t.ID == cur.teamID
		})
		if same {
			b.reply(ctx, user, "Вы уже в этой команде.")
			b.sendStatus(ctx, user, cur)
			return
		}
	}

	name := b.playerName(ctx, user)
	np := core.NewPlayer{ID: newID(), Name: name, TokenHash: randomHash()}
	_, err = rn.Do(ctx, core.JoinTeam{InviteCode: inv.code, Player: np})
	if err != nil {
		b.reply(ctx, user, "❌ "+humanErr(err)+".")
		return
	}
	if err := b.cfg.VK.BindVK(ctx, store.VKBinding{UserID: user, RunID: run.ID, PlayerID: np.ID}); err != nil {
		b.log.Error("vk: привязка", "user", user, "err", err)
		b.reply(ctx, user, "Что-то пошло не так на сервере. Пришлите приглашение ещё раз.")
		return
	}
	b.setMode(user, "", 0)
	b.watch(run.ID)

	b.welcome(ctx, user, run, rn, np.ID, false)
}

// welcome — приветствие игрока в VK и текущее состояние игры.
func (b *Bot) welcome(ctx context.Context, user int64, run store.Run, rn *runner.Runner, playerID string, transferred bool) {
	var (
		welcome string
		outs    []out
		captain bool
	)
	_ = rn.Read(ctx, func(g *core.Game) {
		p := g.Players[playerID]
		t := g.Teams[p.TeamID]
		captain = t.CaptainID == playerID
		title := run.Title
		if title == "" {
			title = g.Scenario.Title
		}
		if transferred {
			welcome = "🔄 " + p.Name + ", вы продолжаете игру «" + title + "» в VK.\n" + teamLine(g, t) + "."
		} else {
			welcome = "🎉 Вы в игре «" + title + "».\n" + teamLine(g, t) + "."
		}
		if captain {
			welcome += "\nВы капитан: вам доступны сливы и чат с ведущим."
		}
		if g.HostContact != "" {
			welcome += "\nЭкстренная связь с ведущим: " + g.HostContact
		}
		welcome += "\n\nОтветы присылайте сюда: код — текстом, место — кнопкой «Отправить местоположение»."
		outs = b.statusMessages(g, p.TeamID, captain)
	})
	b.send(ctx, out{peer: user, text: welcome, keyboard: mainKeyboard(captain)})
	for _, o := range outs {
		o.peer = user
		b.send(ctx, o)
	}
}

func (b *Bot) playerName(ctx context.Context, user int64) string {
	first, last, err := b.cfg.API.UserName(ctx, user)
	if err != nil || strings.TrimSpace(first) == "" {
		return "Игрок VK " + strconv.FormatInt(user%10000, 10)
	}
	name := strings.TrimSpace(first)
	if r, _ := utf8.DecodeRuneInString(strings.TrimSpace(last)); r != utf8.RuneError {
		name += " " + string(r) + "."
	}
	return name
}

func (b *Bot) leave(ctx context.Context, user int64) {
	b.setMode(user, "", 0)
	if err := b.cfg.VK.UnbindVK(ctx, user); err != nil {
		b.log.Error("vk: отвязка", "user", user, "err", err)
	}
	b.send(ctx, out{peer: user, keyboard: guestKeyboard(),
		text: "Вы вышли из игры в VK. Чтобы вернуться, пришлите приглашение команды или выберите игру в «Доступных играх»."})
}

// --- игровые действия ---

func (b *Bot) sendStatus(ctx context.Context, user int64, s *session) {
	var outs []out
	if err := s.rn.Read(ctx, func(g *core.Game) { outs = b.statusMessages(g, s.teamID, s.captain) }); err != nil {
		b.reply(ctx, user, "Не удалось получить задание, попробуйте ещё раз.")
		return
	}
	for _, o := range outs {
		o.peer = user
		b.send(ctx, o)
	}
}

func (b *Bot) answer(ctx context.Context, user int64, s *session, m Message, cmd core.SubmitAnswer) {
	cmd.TeamID, cmd.PlayerID = s.teamID, s.bind.PlayerID
	cmd.ClientEventID = "vk-" + strconv.FormatInt(m.ID, 10) // Long Poll может прислать повтор
	res, err := s.rn.Do(ctx, cmd)
	if err != nil {
		if errors.Is(err, core.ErrRateLimited) {
			if until := b.blockedUntil(ctx, s); !until.IsZero() {
				b.reply(ctx, user, "⛔ Слишком много неверных кодов — ввод на этом задании закрыт до "+b.clock(until)+".")
				return
			}
		}
		b.reply(ctx, user, "⚠️ "+humanErr(err)+".")
		return
	}
	a := res.Answer
	if a == nil {
		return
	}
	geo := cmd.Kind == core.KindGeo
	var text string
	switch a.Result {
	case core.ResultCorrect:
		text = "✅ Верно!"
		if geo {
			text = "✅ Точка засчитана!"
		}
		_ = s.rn.Read(ctx, func(g *core.Game) {
			sl, ok := g.Scenario.Level(a.LevelID)
			ls := g.Teams[s.teamID].Levels[a.LevelID]
			if a.BonusID != "" || !ok || !ls.IsOpen() {
				return
			}
			need := sl.Require.Needed(len(sl.Answers))
			if got := ls.SolvedCount(); got < need {
				text += fmt.Sprintf(" Засчитано %d из %d.", got, need)
			}
		})
	case core.ResultWrong:
		text = "❌ Неверно."
		if geo {
			text = "❌ Не то место — до точки ещё далеко."
		}
		if h := a.RateLimit; h != nil {
			if h.Action == scenario.RateLimitBlock {
				text += " Слишком много неверных кодов — ввод на этом задании закрыт до " + b.clock(h.Until) + "."
			} else {
				text += " Перебор кодов: " + penaltyText(h.PenaltyTime, h.PenaltyPoints) + "."
			}
		}
	case core.ResultDuplicate:
		text = "Этот ответ уже засчитан."
	case core.ResultPending:
		text = "⏳ Отправлено на проверку ведущему."
	case core.ResultInaccurate:
		text = "📡 Местоположение слишком неточное. Выйдите на открытое место и отправьте ещё раз."
	default:
		return
	}
	b.reply(ctx, user, text)
}

// blockedUntil — до какого времени закрыт ввод кодов на открытом уровне команды (своя защита уровня).
func (b *Bot) blockedUntil(ctx context.Context, s *session) time.Time {
	var until time.Time
	_ = s.rn.Read(ctx, func(g *core.Game) {
		for _, ls := range g.Teams[s.teamID].Levels {
			if ls.IsOpen() && ls.BlockedUntil.After(until) {
				until = ls.BlockedUntil
			}
		}
	})
	if until.Before(time.Now()) {
		return time.Time{}
	}
	return until
}

// do выполняет команду игрока; ok — текст при успехе (пусто — промолчать:
// результат придёт уведомлением о событии).
func (b *Bot) do(ctx context.Context, user int64, s *session, cmd core.Command, ok string) {
	if _, err := s.rn.Do(ctx, cmd); err != nil {
		b.reply(ctx, user, "⚠️ "+humanErr(err)+".")
		return
	}
	if ok != "" {
		b.reply(ctx, user, ok)
	}
}

func (b *Bot) hintMenu(ctx context.Context, user int64, s *session) {
	now := b.now()
	var (
		lines []string
		rows  [][]Button
	)
	err := s.rn.Read(ctx, func(g *core.Game) {
		t := g.Teams[s.teamID]
		if t.Status != core.TeamStatusPlaying {
			lines = append(lines, "Команда сейчас не играет.")
			return
		}
		mayRequest := s.captain || g.Scenario.Participants.HintRequests == "anyone"
		levels := openLevels(g, t)
		for _, sl := range levels {
			ls := t.Levels[sl.ID]
			prefix := ""
			if len(levels) > 1 {
				prefix = "«" + short(sl.Title, 16) + "» "
			}
			for i, h := range sl.Hints {
				opened := i < len(ls.Hints) && ls.Hints[i]
				switch {
				case opened:
					lines = append(lines, fmt.Sprintf("💡 %sПодсказка %d: %s", prefix, i+1, cmpText(h.Text, "(картинка — в сообщении с подсказкой)")))
				case h.OnRequest:
					av := ls.OpenedAt.Add(h.AvailableAfter.D())
					pen := scenarioPenalty(h.Penalty)
					switch {
					case av.After(now):
						lines = append(lines, fmt.Sprintf("💡 %sПодсказку %d можно взять через %s.", prefix, i+1, dur(av.Sub(now))))
					case mayRequest && len(rows) < 6:
						label := fmt.Sprintf("%sПодсказка %d", prefix, i+1)
						if pen != "" {
							label += " · " + pen
						}
						rows = append(rows, []Button{textButton(short(label, 40), "primary", payload{Cmd: cmdHintTake, Level: sl.ID, Idx: i})})
					default:
						lines = append(lines, fmt.Sprintf("💡 %sПодсказку %d может взять капитан.", prefix, i+1))
					}
				case h.IsAuto():
					opens := ls.OpenedAt.Add(h.After.D())
					lines = append(lines, fmt.Sprintf("💡 %sПодсказка %d откроется через %s.", prefix, i+1, dur(opens.Sub(now))))
				}
			}
		}
	})
	if err != nil {
		b.reply(ctx, user, "Не удалось получить подсказки, попробуйте ещё раз.")
		return
	}
	if len(lines) == 0 && len(rows) == 0 {
		b.reply(ctx, user, "Подсказок к текущему заданию нет.")
		return
	}
	text := strings.Join(lines, "\n")
	if len(rows) > 0 {
		if text != "" {
			text += "\n\n"
		}
		text += "Взять подсказку по запросу — кнопка ниже (штраф указан на кнопке)."
	}
	o := out{peer: user, text: text}
	if len(rows) > 0 {
		o.keyboard = inline(rows...)
	}
	b.send(ctx, o)
}

func cmpText(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (b *Bot) slivMenu(ctx context.Context, user int64, s *session) {
	if !s.captain {
		b.reply(ctx, user, "Сливать задания может только капитан.")
		return
	}
	now := b.now()
	var (
		lines []string
		rows  [][]Button
	)
	_ = s.rn.Read(ctx, func(g *core.Game) {
		t := g.Teams[s.teamID]
		if t.Status != core.TeamStatusPlaying {
			return
		}
		levels := openLevels(g, t)
		for _, sl := range levels {
			ls := t.Levels[sl.ID]
			prefix := ""
			if len(levels) > 1 {
				prefix = "«" + short(sl.Title, 14) + "» "
			}
			if loc := sl.Location; loc != nil && !ls.LocationRevealed {
				if av := ls.OpenedAt.Add(loc.AvailableAfter.D()); av.After(now) {
					lines = append(lines, fmt.Sprintf("📍 %sСлив локации — через %s.", prefix, dur(av.Sub(now))))
				} else if len(rows) < 6 {
					rows = append(rows, []Button{textButton(short(prefix+"Слить локацию"+penaltySuffix(loc.Penalty), 40), "secondary", payload{Cmd: cmdLoc, Level: sl.ID})})
				}
			}
			if sk := sl.Skip; sk != nil {
				if av := ls.OpenedAt.Add(sk.AvailableAfter.D()); av.After(now) {
					lines = append(lines, fmt.Sprintf("⏭ %sСлив уровня — через %s.", prefix, dur(av.Sub(now))))
				} else if len(rows) < 6 {
					rows = append(rows, []Button{textButton(short(prefix+"Слить уровень"+penaltySuffix(sk.Penalty), 40), "negative", payload{Cmd: cmdSkip, Level: sl.ID})})
				}
			}
		}
	})
	if len(lines) == 0 && len(rows) == 0 {
		b.reply(ctx, user, "У текущего задания сливов нет.")
		return
	}
	text := strings.Join(lines, "\n")
	if len(rows) > 0 {
		if text != "" {
			text += "\n\n"
		}
		text += "Нажатие на кнопку сразу сливает — штраф указан на кнопке."
	}
	o := out{peer: user, text: text}
	if len(rows) > 0 {
		o.keyboard = inline(rows...)
	}
	b.send(ctx, o)
}

func penaltySuffix(p *scenario.Penalty) string {
	if s := scenarioPenalty(p); s != "" {
		return " · " + s
	}
	return " · без штрафа"
}
