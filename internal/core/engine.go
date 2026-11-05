package core

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"questline/internal/routes"
	"questline/internal/scenario"
)

// tx накапливает события одной команды. Каждое событие сразу применяется,
// чтобы следующие решения видели актуальное состояние.
//
// Инвариант обработчиков: все проверки — до первого emit. Ошибка после
// emit оставила бы состояние частично изменённым.
type tx struct {
	g   *Game
	now time.Time
	out []Event
}

func (t *tx) emit(teamID, playerID, clientID string, p Payload) Event {
	e := Event{
		Seq:           t.g.seq + 1,
		Type:          p.Type(),
		At:            t.now,
		TeamID:        teamID,
		PlayerID:      playerID,
		ClientEventID: clientID,
		Payload:       p,
	}
	t.g.Apply(e)
	t.out = append(t.out, e)
	return e
}

// Handle обрабатывает команду в момент now.
// При ошибке состояние не меняется и событий нет.
func (g *Game) Handle(now time.Time, cmd Command) (Outcome, error) {
	t := &tx{g: g, now: now}
	var (
		ans *AnswerResult
		err error
	)
	switch c := cmd.(type) {
	case RegisterTeam:
		err = g.registerTeam(t, c)
	case StartRun:
		err = g.startRun(t)
	case StartTeam:
		err = g.startTeamCmd(t, c)
	case SubmitAnswer:
		ans, err = g.submit(t, c)
	case RequestHint:
		err = g.requestHint(t, c)
	case Moderate:
		err = g.moderate(t, c)
	case Override:
		err = g.override(t, c)
	case SetLeaderboard:
		t.emit("", "", "", LeaderboardVisibilitySet{Visible: c.Visible})
	case SendBroadcast:
		if strings.TrimSpace(c.Text) == "" {
			err = ErrBadCommand
		} else {
			t.emit("", "", "", Broadcast{Text: c.Text})
		}
	case FinishRun:
		err = g.finishRun(t)
	case Tick:
		g.tick(t)
	default:
		err = fmt.Errorf("%w: %T", ErrBadCommand, cmd)
	}
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Events: t.out, Answer: ans}, nil
}

// --- регистрация и старт ---

func (g *Game) registerTeam(t *tx, c RegisterTeam) error {
	if g.Status != RunStatusPending {
		return ErrRunStarted
	}
	if c.TeamID == "" || strings.TrimSpace(c.Name) == "" {
		return ErrBadCommand
	}
	if _, ok := g.Teams[c.TeamID]; ok {
		return ErrTeamExists
	}
	t.emit(c.TeamID, "", "", TeamRegistered{Name: c.Name})
	return nil
}

func (g *Game) startRun(t *tx) error {
	if g.Status != RunStatusPending {
		return ErrRunStarted
	}
	if len(g.TeamOrder) == 0 {
		return ErrNoTeams
	}
	sc := g.Scenario
	order := g.slotOrder()

	var rs [][]string
	if sc.Flow.Mode == scenario.FlowLinear {
		var err error
		rs, err = routes.Build(sc.RouteLevels(), routes.Options{
			Mode:     sc.Flow.Routes,
			Teams:    len(order),
			Seed:     g.Seed,
			Explicit: sc.Flow.RouteList,
		})
		if err != nil {
			return fmt.Errorf("маршруты: %w", err)
		}
	}

	t.emit("", "", "", RunStarted{})
	for slot, id := range order {
		if rs != nil {
			t.emit(id, "", "", RouteAssigned{Levels: rs[slot]})
		}
	}

	switch sc.Start.Mode {
	case scenario.StartSimultaneous:
		for _, id := range order {
			g.startTeam(t, g.Teams[id])
		}
	case scenario.StartStaggered:
		for slot, id := range order {
			at := t.now.Add(time.Duration(slot) * sc.Start.Interval.D())
			t.emit(id, "", "", TeamStartScheduled{StartAt: at, Slot: slot})
		}
		g.tickStarts(t) // первый слот стартует сразу
	case scenario.StartManual:
		// ведущий стартует команды сам
	}
	return nil
}

// slotOrder — порядок команд на старте.
func (g *Game) slotOrder() []string {
	order := append([]string(nil), g.TeamOrder...)
	if g.Scenario.Start.Order == scenario.OrderRandom {
		r := rand.New(rand.NewPCG(g.Seed, 0x5107))
		r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	// OrderManual: пока порядок регистрации; ведущий сможет переставить до старта.
	return order
}

func (g *Game) startTeamCmd(t *tx, c StartTeam) error {
	if g.Status != RunStatusRunning {
		return ErrRunNotRunning
	}
	team, ok := g.Teams[c.TeamID]
	if !ok {
		return ErrUnknownTeam
	}
	if team.Status != TeamStatusRegistered && team.Status != TeamStatusScheduled {
		return ErrTeamAlreadyStarted
	}
	g.startTeam(t, team)
	return nil
}

func (g *Game) startTeam(t *tx, team *Team) {
	t.emit(team.ID, "", "", TeamStarted{})
	sc := g.Scenario
	switch sc.Flow.Mode {
	case scenario.FlowLinear:
		if len(team.Route) > 0 {
			g.openLevel(t, team, team.Route[0], "start")
		}
	case scenario.FlowStorm:
		for _, l := range sc.Levels {
			g.openLevel(t, team, l.ID, "start")
		}
	case scenario.FlowGraph:
		for _, l := range sc.Levels {
			if l.Unlock == nil {
				g.openLevel(t, team, l.ID, "start")
			}
		}
	}
}

func (g *Game) openLevel(t *tx, team *Team, levelID, reason string) {
	t.emit(team.ID, "", "", LevelOpened{LevelID: levelID, Reason: reason})
}

// --- ответы ---

func (g *Game) playingTeam(id string) (*Team, error) {
	team, ok := g.Teams[id]
	if !ok {
		return nil, ErrUnknownTeam
	}
	if team.Status != TeamStatusPlaying {
		return nil, ErrTeamNotPlaying
	}
	return team, nil
}

func (g *Game) submit(t *tx, c SubmitAnswer) (*AnswerResult, error) {
	team, err := g.playingTeam(c.TeamID)
	if err != nil {
		return nil, err
	}
	if c.ClientEventID != "" {
		if r, ok := g.seen[team.ID][c.ClientEventID]; ok {
			return &r, nil // повтор из офлайн-очереди: тот же результат, без событий
		}
	}

	p := AnswerSubmitted{Kind: c.Kind}
	switch c.Kind {
	case KindCode:
		if strings.TrimSpace(c.Value) == "" {
			return nil, ErrEmptyAnswer
		}
		if g.rateLimited(team, t.now) {
			return nil, ErrRateLimited
		}
		levels, err := g.candidateLevels(team, c.LevelID)
		if err != nil {
			return nil, err
		}
		p.Value = c.Value
		g.matchCode(team, levels, c.Value, &p)

	case KindGeo:
		levels, err := g.candidateLevels(team, c.LevelID)
		if err != nil {
			return nil, err
		}
		p.Lat, p.Lng, p.Accuracy = c.Lat, c.Lng, c.Accuracy
		g.matchGeo(team, levels, c, &p)

	case KindPhoto, KindText:
		if err := g.matchModerated(team, c, &p); err != nil {
			return nil, err
		}

	default:
		return nil, ErrBadCommand
	}

	e := t.emit(team.ID, c.PlayerID, c.ClientEventID, p)
	res := AnswerResult{Seq: e.Seq, Result: p.Result, LevelID: p.LevelID, AnswerIdx: p.AnswerIdx, BonusID: p.BonusID}

	if p.Result == ResultCorrect {
		if p.AnswerIdx != nil {
			g.tryCompleteLevel(t, team, p.LevelID)
		} else if p.BonusID != "" {
			g.completeBonus(t, team, p.LevelID, p.BonusID)
		}
	}
	return &res, nil
}

// candidateLevels — открытые (в т.ч. пройденные) уровни, среди которых
// искать ответ: указанный или все в порядке сценария.
func (g *Game) candidateLevels(team *Team, levelID string) ([]string, error) {
	if levelID != "" {
		if _, ok := g.level(levelID); !ok {
			return nil, ErrUnknownLevel
		}
		if team.Levels[levelID] == nil {
			return nil, ErrLevelNotOpen
		}
		return []string{levelID}, nil
	}
	var out []string
	for _, l := range g.Scenario.Levels {
		if team.Levels[l.ID] != nil {
			out = append(out, l.ID)
		}
	}
	return out, nil
}

func ptr(i int) *int { return &i }

// matchCode: сначала незасчитанные ответы открытых уровней, затем бонусы,
// затем проверка на повтор уже засчитанного.
func (g *Game) matchCode(team *Team, levels []string, value string, p *AnswerSubmitted) {
	norm := g.Scenario.Defaults.AnswerNormalize

	for _, id := range levels {
		ls := team.Levels[id]
		if !ls.IsOpen() {
			continue
		}
		sl, _ := g.level(id)
		for i, a := range sl.Answers {
			if isTextAnswer(a.Type) && !ls.Solved[i] && matchText(a, value, norm) {
				p.Result, p.LevelID, p.AnswerIdx = ResultCorrect, id, ptr(i)
				return
			}
		}
	}
	for _, id := range levels {
		sl, _ := g.level(id)
		for _, b := range sl.Bonuses {
			if isTextAnswer(b.Answer.Type) && !team.Bonuses[b.ID] && matchText(b.Answer, value, norm) {
				p.Result, p.LevelID, p.BonusID = ResultCorrect, id, b.ID
				return
			}
		}
	}
	for _, id := range levels {
		ls := team.Levels[id]
		sl, _ := g.level(id)
		for i, a := range sl.Answers {
			if isTextAnswer(a.Type) && ls.Solved[i] && matchText(a, value, norm) {
				p.Result, p.LevelID, p.AnswerIdx = ResultDuplicate, id, ptr(i)
				return
			}
		}
		for _, b := range sl.Bonuses {
			if isTextAnswer(b.Answer.Type) && team.Bonuses[b.ID] && matchText(b.Answer, value, norm) {
				p.Result, p.LevelID, p.BonusID = ResultDuplicate, id, b.ID
				return
			}
		}
	}
	p.Result = ResultWrong
	if len(levels) == 1 {
		p.LevelID = levels[0]
	}
}

func (g *Game) matchGeo(team *Team, levels []string, c SubmitAnswer, p *AnswerSubmitted) {
	inaccurate := false
	for _, id := range levels {
		ls := team.Levels[id]
		if !ls.IsOpen() {
			continue
		}
		sl, _ := g.level(id)
		for i, a := range sl.Answers {
			if a.Type != scenario.AnswerGeo || ls.Solved[i] {
				continue
			}
			switch matchGeo(a, c.Lat, c.Lng, c.Accuracy) {
			case geoHit:
				p.Result, p.LevelID, p.AnswerIdx = ResultCorrect, id, ptr(i)
				return
			case geoInaccurate:
				inaccurate = true
			}
		}
	}
	for _, id := range levels {
		sl, _ := g.level(id)
		for _, b := range sl.Bonuses {
			if b.Answer.Type != scenario.AnswerGeo || team.Bonuses[b.ID] {
				continue
			}
			switch matchGeo(b.Answer, c.Lat, c.Lng, c.Accuracy) {
			case geoHit:
				p.Result, p.LevelID, p.BonusID = ResultCorrect, id, b.ID
				return
			case geoInaccurate:
				inaccurate = true
			}
		}
	}
	if inaccurate {
		p.Result = ResultInaccurate
	} else {
		p.Result = ResultWrong
	}
	if len(levels) == 1 {
		p.LevelID = levels[0]
	}
}

func (g *Game) matchModerated(team *Team, c SubmitAnswer, p *AnswerSubmitted) error {
	want := scenario.AnswerPhoto
	if c.Kind == KindText {
		want = scenario.AnswerText
	}
	if c.Kind == KindPhoto && c.MediaKey == "" {
		return ErrEmptyAnswer
	}
	if c.Kind == KindText && strings.TrimSpace(c.Value) == "" {
		return ErrEmptyAnswer
	}
	if c.LevelID == "" {
		return ErrNoTarget
	}
	sl, ok := g.level(c.LevelID)
	if !ok {
		return ErrUnknownLevel
	}
	ls := team.Levels[c.LevelID]
	if ls == nil {
		return ErrLevelNotOpen
	}
	p.LevelID, p.Value, p.MediaKey = c.LevelID, c.Value, c.MediaKey

	switch {
	case c.BonusID != "":
		i := slices.IndexFunc(sl.Bonuses, func(b scenario.Bonus) bool { return b.ID == c.BonusID })
		if i < 0 || sl.Bonuses[i].Answer.Type != want {
			return ErrNoTarget
		}
		p.BonusID = c.BonusID
		switch {
		case team.Bonuses[c.BonusID]:
			p.Result = ResultDuplicate
		case team.PendingBonus[c.BonusID] != 0:
			return ErrAlreadyPending
		default:
			p.Result = ResultPending
		}

	case c.AnswerIdx != nil:
		i := *c.AnswerIdx
		if i < 0 || i >= len(sl.Answers) || sl.Answers[i].Type != want {
			return ErrNoTarget
		}
		if !ls.IsOpen() {
			return ErrLevelNotOpen
		}
		p.AnswerIdx = ptr(i)
		switch {
		case ls.Solved[i]:
			p.Result = ResultDuplicate
		case ls.Pending[i] != 0:
			return ErrAlreadyPending
		default:
			p.Result = ResultPending
		}

	default:
		if !ls.IsOpen() {
			return ErrNoTarget
		}
		for i, a := range sl.Answers {
			if a.Type == want && !ls.Solved[i] && ls.Pending[i] == 0 {
				p.AnswerIdx, p.Result = ptr(i), ResultPending
				return nil
			}
		}
		return ErrNoTarget
	}
	return nil
}

func (g *Game) rateLimited(team *Team, now time.Time) bool {
	rl := g.Scenario.Defaults.RateLimit
	if rl == nil {
		return false
	}
	from := now.Add(-rl.Per.D())
	n := 0
	for _, at := range team.wrongAttempts {
		if at.After(from) {
			n++
		}
	}
	return n >= rl.WrongAnswers
}

// --- прохождение уровней ---

func (g *Game) tryCompleteLevel(t *tx, team *Team, levelID string) {
	ls := team.Levels[levelID]
	if team.Status != TeamStatusPlaying || !ls.IsOpen() {
		return
	}
	sl, _ := g.level(levelID)
	if ls.SolvedCount() < sl.Require.Needed(len(sl.Answers)) {
		return
	}
	g.completeLevel(t, team, levelID)
}

func (g *Game) completeLevel(t *tx, team *Team, levelID string) {
	sl, _ := g.level(levelID)
	t.emit(team.ID, "", "", LevelCompleted{LevelID: levelID, Points: sl.Points})
	g.advance(t, team, levelID)
}

func (g *Game) completeBonus(t *tx, team *Team, levelID, bonusID string) {
	if team.Bonuses[bonusID] {
		return
	}
	sl, _ := g.level(levelID)
	for _, b := range sl.Bonuses {
		if b.ID == bonusID {
			t.emit(team.ID, "", "", BonusCompleted{LevelID: levelID, BonusID: bonusID, Points: b.Points})
			return
		}
	}
}

// advance открывает следующие уровни после прохождения levelID или финиширует.
func (g *Game) advance(t *tx, team *Team, levelID string) {
	sc := g.Scenario
	switch sc.Flow.Mode {
	case scenario.FlowLinear:
		i := slices.Index(team.Route, levelID)
		if i >= 0 && i+1 < len(team.Route) {
			g.openLevel(t, team, team.Route[i+1], "next")
			return
		}
		g.finishTeam(t, team, "completed", true)

	case scenario.FlowStorm:
		if team.LevelsDone == len(sc.Levels) {
			g.finishTeam(t, team, "completed", true)
		}

	case scenario.FlowGraph:
		for _, l := range sc.Levels {
			if team.Levels[l.ID] == nil && g.unlocked(team, l.Unlock) {
				g.openLevel(t, team, l.ID, "unlock")
			}
		}
		if team.LevelsDone == len(sc.Levels) {
			g.finishTeam(t, team, "completed", true)
			return
		}
		for _, ls := range team.Levels {
			if ls.IsOpen() {
				return
			}
		}
		// открытых уровней нет и ничего не открылось — тупик, финиш
		g.finishTeam(t, team, "completed", true)
	}
}

func (g *Game) unlocked(team *Team, u *scenario.Unlock) bool {
	if u == nil {
		return true
	}
	done := func(id string) bool {
		ls := team.Levels[id]
		return ls != nil && !ls.CompletedAt.IsZero()
	}
	count := func(ids []string) int {
		n := 0
		for _, id := range ids {
			if done(id) {
				n++
			}
		}
		return n
	}
	switch {
	case len(u.AllOf) > 0:
		return count(u.AllOf) == len(u.AllOf)
	case len(u.AnyOf) > 0:
		return count(u.AnyOf) > 0
	default:
		return count(u.Of) >= u.Count
	}
}

func (g *Game) finishTeam(t *tx, team *Team, reason string, autoFinishRun bool) {
	t.emit(team.ID, "", "", TeamFinished{Reason: reason})
	if !autoFinishRun || g.Status != RunStatusRunning {
		return
	}
	for _, id := range g.TeamOrder {
		if g.Teams[id].Status != TeamStatusFinished {
			return
		}
	}
	t.emit("", "", "", RunFinished{Reason: "all_finished"})
}

// --- подсказки, модерация, ведущий ---

func (g *Game) requestHint(t *tx, c RequestHint) error {
	team, err := g.playingTeam(c.TeamID)
	if err != nil {
		return err
	}
	sl, ok := g.level(c.LevelID)
	if !ok {
		return ErrUnknownLevel
	}
	ls := team.Levels[c.LevelID]
	if !ls.IsOpen() {
		return ErrLevelNotOpen
	}
	if c.HintIdx < 0 || c.HintIdx >= len(sl.Hints) {
		return ErrUnknownHint
	}
	h := sl.Hints[c.HintIdx]
	if !h.OnRequest {
		return ErrNotAllowed // откроется по таймеру
	}
	if ls.Hints[c.HintIdx] {
		return ErrHintAlreadyOpen
	}
	if g.Scenario.Participants.HintRequests == scenario.HintRequestsCaptain && !c.IsCaptain {
		return ErrNotAllowed
	}
	p := HintOpened{LevelID: c.LevelID, HintIdx: c.HintIdx, Trigger: "request"}
	if h.Penalty != nil {
		p.PenaltyTime = h.Penalty.Time.D()
		p.PenaltyPoints = h.Penalty.Points
	}
	t.emit(team.ID, c.PlayerID, "", p)
	return nil
}

func (g *Game) moderate(t *tx, c Moderate) error {
	item := g.pending[c.Ref]
	if item == nil {
		return ErrNotPending
	}
	team := g.Teams[item.TeamID]
	it := *item // Apply удалит заявку из очереди
	t.emit(team.ID, c.By, "", ModerationResolved{Ref: c.Ref, Approved: c.Approved, Comment: c.Comment})
	if !c.Approved {
		return nil
	}
	if it.AnswerIdx >= 0 {
		g.tryCompleteLevel(t, team, it.LevelID)
	} else {
		g.completeBonus(t, team, it.LevelID, it.BonusID)
	}
	return nil
}

func (g *Game) override(t *tx, c Override) error {
	if strings.TrimSpace(c.Reason) == "" {
		return ErrReasonRequired
	}
	team, ok := g.Teams[c.TeamID]
	if !ok {
		return ErrUnknownTeam
	}
	p := AdminOverride{Action: c.Action, LevelID: c.LevelID, HintIdx: c.HintIdx, Time: c.Time, Points: c.Points, Reason: c.Reason}

	switch c.Action {
	case OverrideCompleteLevel:
		if team.Status != TeamStatusPlaying {
			return ErrTeamNotPlaying
		}
		if !team.Levels[c.LevelID].IsOpen() {
			return ErrLevelNotOpen
		}
		t.emit(team.ID, c.By, "", p)
		g.completeLevel(t, team, c.LevelID)

	case OverrideOpenHint:
		if team.Status != TeamStatusPlaying {
			return ErrTeamNotPlaying
		}
		sl, ok := g.level(c.LevelID)
		if !ok {
			return ErrUnknownLevel
		}
		ls := team.Levels[c.LevelID]
		if !ls.IsOpen() {
			return ErrLevelNotOpen
		}
		if c.HintIdx < 0 || c.HintIdx >= len(sl.Hints) {
			return ErrUnknownHint
		}
		if ls.Hints[c.HintIdx] {
			return ErrHintAlreadyOpen
		}
		t.emit(team.ID, c.By, "", p)
		t.emit(team.ID, c.By, "", HintOpened{LevelID: c.LevelID, HintIdx: c.HintIdx, Trigger: "admin"})

	case OverrideAdjustTime:
		if c.Time == 0 {
			return ErrBadCommand
		}
		t.emit(team.ID, c.By, "", p)

	case OverrideAdjustPoints:
		if c.Points == 0 {
			return ErrBadCommand
		}
		t.emit(team.ID, c.By, "", p)

	case OverrideFinishTeam:
		if team.Status != TeamStatusPlaying {
			return ErrTeamNotPlaying
		}
		t.emit(team.ID, c.By, "", p)
		g.finishTeam(t, team, "admin", true)

	default:
		return ErrBadCommand
	}
	return nil
}

func (g *Game) finishRun(t *tx) error {
	if g.Status != RunStatusRunning {
		return ErrRunNotRunning
	}
	for _, id := range g.TeamOrder {
		if team := g.Teams[id]; team.Status != TeamStatusFinished {
			g.finishTeam(t, team, "admin", false)
		}
	}
	t.emit("", "", "", RunFinished{Reason: "admin"})
	return nil
}
