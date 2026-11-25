package vkbot

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// Регистрация через VK: «Доступные игры» → игра → создать команду или
// вступить в команду по коду приглашения. Работает при «белых списках»,
// когда сайт с регистрацией недоступен.

const (
	labelGames = "🎲 Доступные игры"
	labelTeam  = "👥 Команда"

	cmdCity      = "city"
	cmdCityReset = "city_reset"

	modeTeamName = "team_name" // ждём название новой команды
	modeTeamCode = "team_code" // ждём код приглашения выбранной команды

	maxTeamName = 40
)

// guestKeyboard — клавиатура того, кто ещё не в игре.
func guestKeyboard() *Keyboard {
	return &Keyboard{Buttons: [][]Button{{textButton(labelGames, "primary", payload{Cmd: cmdGames})}}}
}

// gameInfo — игра для списка и карточки (собирается внутри актора).
type gameInfo struct {
	run      store.Run
	title    string
	city     string
	listed   bool
	desc     string
	status   core.RunStatus
	regOpen  bool // можно создать новую команду
	solo     bool
	teams    []teamInfo
	maxTeams int
	maxSize  int
}

type teamInfo struct {
	id, name, captain string
	players           int
	joinable          bool
}

func readGame(g *core.Game, run store.Run) gameInfo {
	gi := gameInfo{
		run:      run,
		title:    cmp.Or(run.Title, g.Scenario.Title),
		city:     g.Listing.City,
		listed:   g.Listing.Listed,
		desc:     strings.TrimSpace(g.Scenario.Description),
		status:   g.Status,
		solo:     g.Scenario.Participants.Mode == scenario.ParticipantsSolo,
		maxTeams: g.Registration.MaxTeams,
		maxSize:  g.Scenario.Participants.TeamSize.Max,
	}
	active := g.ActiveTeams()
	gi.regOpen = g.Status == core.RunStatusPending && !g.Registration.Closed &&
		(gi.maxTeams == 0 || len(active) < gi.maxTeams)
	for _, t := range active {
		ti := teamInfo{id: t.ID, name: t.Name, players: len(g.ActivePlayers(t.ID))}
		if p := g.Players[t.CaptainID]; p != nil {
			ti.captain = p.Name
		}
		ti.joinable = !gi.solo && t.Status != core.TeamStatusFinished && (gi.maxSize == 0 || ti.players < gi.maxSize)
		gi.teams = append(gi.teams, ti)
	}
	return gi
}

func (gi gameInfo) statusLine() string {
	switch {
	case gi.status == core.RunStatusRunning:
		return "игра идёт — можно вступить в команду"
	case gi.regOpen && gi.maxTeams > 0:
		return fmt.Sprintf("регистрация открыта, команд %d из %d", len(gi.teams), gi.maxTeams)
	case gi.regOpen:
		return fmt.Sprintf("регистрация открыта, команд: %d", len(gi.teams))
	case gi.maxTeams > 0 && len(gi.teams) >= gi.maxTeams:
		return "мест для новых команд нет — можно вступить в команду"
	}
	return "регистрация новых команд закрыта — можно вступить в команду"
}

// availableGames — недавние незавершённые игры, которые ведущий показал в списке.
func (b *Bot) availableGames(ctx context.Context) ([]gameInfo, error) {
	runs, err := b.cfg.Store.ListRuns(ctx, b.cfg.OrgID)
	if err != nil {
		return nil, err
	}
	since := b.now().Add(-b.cfg.SearchWindow)
	var out []gameInfo
	for _, run := range runs {
		if run.CreatedAt.Before(since) {
			continue
		}
		rn, err := b.cfg.Runners.Get(ctx, run.ID)
		if err != nil {
			continue
		}
		var gi gameInfo
		_ = rn.Read(ctx, func(g *core.Game) { gi = readGame(g, run) })
		if !gi.listed || gi.status == core.RunStatusFinished || (!gi.regOpen && !anyJoinable(gi.teams)) {
			continue
		}
		out = append(out, gi)
	}
	return out, nil
}

func anyJoinable(ts []teamInfo) bool {
	for _, t := range ts {
		if t.joinable {
			return true
		}
	}
	return false
}

func (b *Bot) gamesList(ctx context.Context, user int64) {
	b.setMode(user, "", 0)
	games, err := b.availableGames(ctx)
	if err != nil {
		b.reply(ctx, user, "Не удалось получить список игр, попробуйте ещё раз.")
		return
	}
	if len(games) == 0 {
		b.send(ctx, out{peer: user, text: "Сейчас нет игр, в которые можно записаться. Следите за анонсами сообщества!"})
		return
	}

	// Город: если игры в нескольких городах — сначала спрашиваем.
	var cities []string
	for _, gi := range games {
		if gi.city != "" && !slices.Contains(cities, gi.city) {
			cities = append(cities, gi.city)
		}
	}
	slices.Sort(cities)
	city := b.cityOf(user)
	if city != "" && !slices.Contains(cities, city) {
		city = ""
	}
	if city == "" && len(cities) > 1 {
		var rows [][]Button
		for _, c := range cities {
			if len(rows) < 6 {
				rows = append(rows, []Button{textButton(short("📍 "+c, 40), "primary", payload{Cmd: cmdCity, City: c})})
			}
		}
		b.send(ctx, out{peer: user, text: "В каком вы городе?", keyboard: inline(rows...)})
		return
	}
	if city != "" {
		games = slices.DeleteFunc(games, func(gi gameInfo) bool { return gi.city != "" && gi.city != city })
	}

	var sb strings.Builder
	sb.WriteString("🎲 Доступные игры")
	if city != "" {
		sb.WriteString(" — " + city)
	} else if len(cities) == 1 {
		sb.WriteString(" — " + cities[0])
	}
	sb.WriteString(":")
	var rows [][]Button
	for i, gi := range games {
		fmt.Fprintf(&sb, "\n\n%d. «%s» — %s", i+1, gi.title, gi.statusLine())
		if len(rows) < 5 {
			rows = append(rows, []Button{textButton(short(fmt.Sprintf("%d. %s", i+1, gi.title), 40), "primary", payload{Cmd: cmdGame, Run: gi.run.ID})})
		}
	}
	if len(games) == 0 {
		sb.WriteString("\n\nВ этом городе сейчас нет игр.")
	} else {
		sb.WriteString("\n\nВыберите игру — покажу команды.")
	}
	if len(cities) > 1 {
		rows = append(rows, []Button{textButton("🌍 Другой город", "secondary", payload{Cmd: cmdCityReset})})
	}
	o := out{peer: user, text: sb.String()}
	if len(rows) > 0 {
		o.keyboard = inline(rows...)
	}
	b.send(ctx, o)
}

func (b *Bot) cityOf(user int64) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cities[user]
}

// setCity запоминает город пользователя (в памяти: после перезапуска спросим снова).
func (b *Bot) setCity(user int64, city string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if city == "" {
		delete(b.cities, user)
	} else {
		b.cities[user] = city
	}
}

// gameCard — игра с командами и кнопками «Создать команду» / «Вступить».
func (b *Bot) gameCard(ctx context.Context, user int64, run store.Run) {
	b.setMode(user, "", 0)
	rn, err := b.cfg.Runners.Get(ctx, run.ID)
	if err != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	var gi gameInfo
	_ = rn.Read(ctx, func(g *core.Game) { gi = readGame(g, run) })
	if gi.status == core.RunStatusFinished {
		b.reply(ctx, user, "Игра «"+gi.title+"» уже завершена. Посмотрите «Доступные игры».")
		return
	}

	var sb strings.Builder
	sb.WriteString("🎲 «" + gi.title + "»")
	if gi.city != "" {
		sb.WriteString("\n📍 " + gi.city)
	}
	if gi.desc != "" {
		d := gi.desc
		if utf8.RuneCountInString(d) > 600 {
			d = string([]rune(d)[:600]) + "…"
		}
		sb.WriteString("\n" + d)
	}
	sb.WriteString("\n\n" + capitalize(gi.statusLine()) + ".")
	if gi.solo {
		sb.WriteString("\nИгра одиночная.")
	}
	var rows [][]Button
	if gi.regOpen {
		label := "➕ Создать команду"
		if gi.solo {
			label = "➕ Участвовать"
		}
		rows = append(rows, []Button{textButton(label, "positive", payload{Cmd: cmdNewTeam, Run: run.ID})})
	}
	if !gi.solo {
		if len(gi.teams) == 0 {
			sb.WriteString("\n\nКоманд пока нет — создайте первую!")
		} else {
			sb.WriteString("\n\nКоманды:")
		}
		for _, t := range gi.teams {
			line := fmt.Sprintf("\n• «%s» — игроков: %d", t.name, t.players)
			if gi.maxSize > 0 {
				line = fmt.Sprintf("\n• «%s» — игроков: %d из %d", t.name, t.players, gi.maxSize)
			}
			if t.captain != "" {
				line += ", капитан " + t.captain
			}
			if !t.joinable {
				line += " (мест нет)"
			}
			sb.WriteString(line)
			if t.joinable && len(rows) < 6 {
				rows = append(rows, []Button{textButton(short("👥 Вступить в «"+t.name+"»", 40), "secondary", payload{Cmd: cmdJoinTeam, Run: run.ID, Team: t.id})})
			}
		}
		if anyJoinable(gi.teams) {
			sb.WriteString("\n\nЧтобы вступить, понадобится код приглашения — его даёт капитан.")
		}
	}
	o := out{peer: user, text: sb.String()}
	if len(rows) > 0 {
		o.keyboard = inline(rows...)
	}
	b.send(ctx, o)
}

var gameLinkRe = regexp.MustCompile(`(?i)/g/([a-z0-9]{6})(?:[/?#\s]|$)`)

// parseGameLink — ссылка на игру …/g/КОД (без /join), а голый код из 6
// символов — только у того, кто ещё не играет.
func parseGameLink(text string, bare bool) (string, bool) {
	if m := gameLinkRe.FindStringSubmatch(text); m != nil {
		return strings.ToUpper(m[1]), true
	}
	if bare {
		t := strings.ToUpper(strings.TrimSpace(text))
		if len(t) == 6 && codeRe.MatchString(t) {
			return t, true
		}
	}
	return "", false
}

func (b *Bot) gameCardByCode(ctx context.Context, user int64, code string) bool {
	run, err := b.cfg.Store.GetRunByCode(ctx, code)
	if err != nil {
		return false
	}
	b.gameCard(ctx, user, run)
	return true
}

func (b *Bot) gameCardByID(ctx context.Context, user int64, runID string) {
	run, err := b.cfg.Store.GetRun(ctx, runID)
	if err != nil || run.DeletedAt != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	b.gameCard(ctx, user, run)
}

// --- создание команды ---

func (b *Bot) startNewTeam(ctx context.Context, user int64, cur *session, runID string) {
	run, err := b.cfg.Store.GetRun(ctx, runID)
	if err != nil || run.DeletedAt != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	rn, err := b.cfg.Runners.Get(ctx, run.ID)
	if err != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	var gi gameInfo
	_ = rn.Read(ctx, func(g *core.Game) { gi = readGame(g, run) })
	if !gi.regOpen {
		b.reply(ctx, user, "❌ Регистрация новых команд в «"+gi.title+"» закрыта.")
		return
	}
	if gi.solo {
		b.register(ctx, user, run, rn, b.playerName(ctx, user))
		return
	}
	b.setModeFull(user, mode{kind: modeTeamName, runID: run.ID}, hostModeTTL)
	text := "Как будет называться команда? Напишите название одним сообщением (до 40 символов)."
	if cur != nil && cur.bind.RunID != run.ID {
		text += "\nВы перейдёте из текущей игры в эту."
	}
	b.send(ctx, out{peer: user, text: text, keyboard: inline([]Button{textButton("Отмена", "secondary", payload{Cmd: cmdCancel})})})
}

// register создаёт команду с пользователем-капитаном и привязывает его.
func (b *Bot) register(ctx context.Context, user int64, run store.Run, rn *runner.Runner, teamName string) {
	teamName = strings.TrimSpace(teamName)
	if teamName == "" {
		b.reply(ctx, user, "Напишите название команды текстом.")
		return
	}
	if utf8.RuneCountInString(teamName) > maxTeamName {
		b.reply(ctx, user, "Слишком длинное название — не больше 40 символов. Напишите покороче.")
		return
	}
	captain := core.NewPlayer{ID: newID(), Name: b.playerName(ctx, user), TokenHash: randomHash()}
	cmd := core.RegisterTeam{
		TeamID: newID(), Name: teamName, Captain: &captain,
		Contact: "https://vk.com/id" + strconv.FormatInt(user, 10),
	}
	var err error
	for range 5 { // повтор на редкой коллизии инвайт-кода
		cmd.InviteCode = newInviteCode()
		if _, err = rn.Do(ctx, cmd); !errors.Is(err, core.ErrInviteTaken) {
			break
		}
	}
	if err != nil {
		b.setMode(user, "", 0)
		b.reply(ctx, user, "❌ "+humanErr(err)+".")
		return
	}
	if err := b.cfg.VK.BindVK(ctx, store.VKBinding{UserID: user, RunID: run.ID, PlayerID: captain.ID}); err != nil {
		b.log.Error("vk: привязка капитана", "user", user, "err", err)
		b.reply(ctx, user, "Команда создана, но что-то пошло не так на сервере. Напишите ведущему.")
		return
	}
	b.setMode(user, "", 0)
	b.watch(run.ID)
	b.welcome(ctx, user, run, rn, captain.ID, false)
	var gi gameInfo
	_ = rn.Read(ctx, func(g *core.Game) { gi = readGame(g, run) })
	if !gi.solo {
		b.reply(ctx, user, "👇 Перешлите следующее сообщение своей команде — по нему игроки вступят.")
		b.reply(ctx, user, b.inviteText(gi.title, teamName, cmd.InviteCode))
	}
}

// inviteText — приглашение для пересылки игрокам.
func (b *Bot) inviteText(game, team, code string) string {
	where := "Напишите сообществу игры в VK"
	if b.cfg.GroupURL != "" {
		where = "Напишите сообществу " + b.cfg.GroupURL
	}
	return "Вступай в команду «" + team + "» в игре «" + game + "»!\n" + where + " код приглашения: " + code
}

const inviteAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // как в приложении

func newInviteCode() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	for i := range buf {
		buf[i] = inviteAlphabet[int(buf[i])%len(inviteAlphabet)]
	}
	return string(buf)
}

// --- вступление в выбранную команду ---

func (b *Bot) startJoinTeam(ctx context.Context, user int64, runID, teamID string) {
	run, err := b.cfg.Store.GetRun(ctx, runID)
	if err != nil || run.DeletedAt != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	rn, err := b.cfg.Runners.Get(ctx, run.ID)
	if err != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	name := ""
	_ = rn.Read(ctx, func(g *core.Game) {
		if t := g.Teams[teamID]; t != nil {
			name = t.Name
		}
	})
	if name == "" {
		b.reply(ctx, user, "Команда не найдена.")
		return
	}
	b.setModeFull(user, mode{kind: modeTeamCode, runID: run.ID, teamID: teamID}, hostModeTTL)
	b.send(ctx, out{peer: user, text: "Пришлите код приглашения команды «" + name + "» — 8 символов, его даёт капитан.",
		keyboard: inline([]Button{textButton("Отмена", "secondary", payload{Cmd: cmdCancel})})})
}

func (b *Bot) joinChosenTeam(ctx context.Context, user int64, cur *session, md mode, text string) {
	code := strings.ToUpper(strings.Join(strings.Fields(text), ""))
	run, err := b.cfg.Store.GetRun(ctx, md.runID)
	if err != nil {
		b.setMode(user, "", 0)
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	rn, err := b.cfg.Runners.Get(ctx, run.ID)
	if err != nil {
		b.reply(ctx, user, "Игра не найдена.")
		return
	}
	match := false
	_ = rn.Read(ctx, func(g *core.Game) {
		t, ok := g.TeamByInvite(code)
		match = ok && t.ID == md.teamID
	})
	if !match {
		b.reply(ctx, user, "❌ Код не подходит к этой команде. Проверьте его у капитана и пришлите ещё раз — или «Отмена».")
		return
	}
	b.setMode(user, "", 0)
	b.join(ctx, user, cur, invite{runCode: run.PublicCode, code: code})
}

// --- карточка своей команды ---

func (b *Bot) teamCard(ctx context.Context, user int64, s *session) {
	var text, title string
	var code, teamName string
	_ = s.rn.Read(ctx, func(g *core.Game) {
		t := g.Teams[s.teamID]
		title = cmp.Or(g.Scenario.Title, "")
		teamName, code = t.Name, t.InviteCode
		var sb strings.Builder
		sb.WriteString("👥 " + teamLine(g, t) + "\nИгроки:")
		for _, p := range g.ActivePlayers(t.ID) {
			sb.WriteString("\n• " + p.Name)
			if p.ID == t.CaptainID {
				sb.WriteString(" 👑")
			}
			if p.ID == s.bind.PlayerID {
				sb.WriteString(" (вы)")
			}
		}
		text = sb.String()
	})
	if run, err := b.cfg.Store.GetRun(ctx, s.bind.RunID); err == nil && run.Title != "" {
		title = run.Title
	}
	b.reply(ctx, user, text)
	if s.captain && code != "" {
		b.reply(ctx, user, "👇 Приглашение для новых игроков — перешлите им:")
		b.reply(ctx, user, b.inviteText(title, teamName, code))
	}
}
