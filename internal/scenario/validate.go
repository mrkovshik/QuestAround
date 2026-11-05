package scenario

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"questline/internal/routes"
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type validator struct {
	s    *Scenario
	loc  locator
	errs Errors
}

func (v *validator) add(path []any, format string, args ...any) {
	v.errs = append(v.errs, Error{
		Line: v.loc.line(path...),
		Path: renderPath(path),
		Msg:  fmt.Sprintf(format, args...),
	})
}

func p(parts ...any) []any { return parts }

func oneOf[T ~string](v T, allowed ...T) bool { return slices.Contains(allowed, v) }

func list[T ~string](allowed ...T) string {
	s := make([]string, len(allowed))
	for i, a := range allowed {
		s[i] = string(a)
	}
	return strings.Join(s, " | ")
}

func (v *validator) validate() {
	s := v.s

	if s.Version != SchemaVersion {
		v.add(p("version"), "поддерживается только version: %d", SchemaVersion)
	}
	if strings.TrimSpace(s.ID) == "" {
		v.add(p("id"), "не задан id сценария")
	} else if !idRe.MatchString(s.ID) {
		v.add(p("id"), "id %q: только строчные латинские буквы, цифры, - и _", s.ID)
	}
	if strings.TrimSpace(s.Title) == "" {
		v.add(p("title"), "не задано название")
	}

	v.validateParticipants()
	v.validateTiming()
	v.validateStart()
	v.validateScoring()
	v.validateLeaderboard()
	v.validateDefaults()
	v.validateLevels()
	v.validateFlow() // после уровней: нужны id
}

func (v *validator) validateParticipants() {
	pt := v.s.Participants
	if !oneOf(pt.Mode, ParticipantsTeam, ParticipantsSolo) {
		v.add(p("participants", "mode"), "ожидалось %s", list(ParticipantsTeam, ParticipantsSolo))
	}
	if !oneOf(pt.HintRequests, HintRequestsCaptain, HintRequestsAnyone) {
		v.add(p("participants", "hint_requests"), "ожидалось %s", list(HintRequestsCaptain, HintRequestsAnyone))
	}
	ts := pt.TeamSize
	switch {
	case pt.Mode == ParticipantsSolo && (ts.Min != 1 || ts.Max != 1):
		v.add(p("participants", "team_size"), "в режиме solo размер команды всегда 1")
	case ts.Min < 1:
		v.add(p("participants", "team_size", "min"), "минимум 1 игрок")
	case ts.Max < ts.Min:
		v.add(p("participants", "team_size", "max"), "max (%d) меньше min (%d)", ts.Max, ts.Min)
	}
}

func (v *validator) validateTiming() {
	t := v.s.Timing
	switch t.Mode {
	case TimingAsync:
	case TimingSync:
		v.add(p("timing", "mode"), "режим sync (синхронные раунды) пока не поддерживается")
	default:
		v.add(p("timing", "mode"), "ожидалось %s", list(TimingAsync, TimingSync))
	}
}

func (v *validator) validateStart() {
	st := v.s.Start
	if !oneOf(st.Mode, StartSimultaneous, StartStaggered, StartManual) {
		v.add(p("start", "mode"), "ожидалось %s", list(StartSimultaneous, StartStaggered, StartManual))
	}
	if !oneOf(st.Order, OrderRegistration, OrderRandom, OrderManual) {
		v.add(p("start", "order"), "ожидалось %s", list(OrderRegistration, OrderRandom, OrderManual))
	}
	if st.Mode == StartStaggered && st.Interval == 0 {
		v.add(p("start", "interval"), "для staggered нужен interval > 0")
	}
	if st.Mode != StartStaggered && st.Interval != 0 {
		v.add(p("start", "interval"), "interval используется только при mode: staggered")
	}
}

func (v *validator) validateScoring() {
	if !oneOf(v.s.Scoring.Mode, ScoringTime, ScoringPoints, ScoringMixed) {
		v.add(p("scoring", "mode"), "ожидалось %s", list(ScoringTime, ScoringPoints, ScoringMixed))
	}
}

var knownColumns = []string{"rank", "team", "points", "levels", "time"}

func (v *validator) validateLeaderboard() {
	lb := v.s.Leaderboard
	if !oneOf(lb.Visibility, VisibilityAlways, VisibilityHidden, VisibilityAfterFinish, VisibilityHost) {
		v.add(p("leaderboard", "visibility"), "ожидалось %s",
			list(VisibilityAlways, VisibilityHidden, VisibilityAfterFinish, VisibilityHost))
	}
	if lb.FreezeBeforeEnd != 0 {
		limit := v.s.Timing.TimeLimit
		if limit == 0 {
			v.add(p("leaderboard", "freeze_before_end"), "заморозка требует timing.time_limit")
		} else if lb.FreezeBeforeEnd >= limit {
			v.add(p("leaderboard", "freeze_before_end"), "должно быть меньше time_limit (%s)", limit)
		}
	}
	for i, c := range lb.Columns {
		if !slices.Contains(knownColumns, c) {
			v.add(p("leaderboard", "columns", i), "неизвестная колонка %q, доступны: %s", c, strings.Join(knownColumns, ", "))
		}
	}
}

func (v *validator) validateDefaults() {
	d := v.s.Defaults
	for i, h := range d.HintAfter {
		if h == 0 {
			v.add(p("defaults", "hint_after", i), "время подсказки должно быть > 0")
		}
	}
	if rl := d.RateLimit; rl != nil {
		if rl.WrongAnswers < 1 {
			v.add(p("defaults", "rate_limit", "wrong_answers"), "должно быть ≥ 1")
		}
		if rl.Per == 0 {
			v.add(p("defaults", "rate_limit", "per"), "должно быть > 0")
		}
	}
	for i, op := range d.AnswerNormalize {
		if !knownNormalize[op] {
			v.add(p("defaults", "answer_normalize", i), "неизвестная операция %q (trim, lower, yo_to_e, collapse_spaces)", op)
		}
	}
}

func (v *validator) validateLevels() {
	s := v.s
	if len(s.Levels) == 0 {
		v.add(p("levels"), "нет ни одного уровня")
		return
	}

	ids := map[string]int{}
	bonusIDs := map[string]bool{}
	var firstPins, lastPins []string

	for i, l := range s.Levels {
		lp := func(rest ...any) []any { return slices.Clip(append(p("levels", i), rest...)) }
		name := l.ID
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}

		switch {
		case l.ID == "":
			v.add(lp(), "уровень %s: не задан id", name)
		case !idRe.MatchString(l.ID):
			v.add(lp("id"), "id %q: только строчные латинские буквы, цифры, - и _", l.ID)
		default:
			if prev, dup := ids[l.ID]; dup {
				v.add(lp("id"), "id %q уже используется уровнем #%d", l.ID, prev+1)
			} else {
				ids[l.ID] = i
			}
		}

		if strings.TrimSpace(l.Task) == "" {
			v.add(lp(), "уровень %s: пустое задание (task)", name)
		}
		if l.Points < 0 {
			v.add(lp("points"), "баллы не могут быть отрицательными")
		}

		switch l.Pin {
		case routes.PinNone:
		case routes.PinFirst:
			firstPins = append(firstPins, name)
		case routes.PinLast:
			lastPins = append(lastPins, name)
		default:
			v.add(lp("pin"), "ожидалось first | last")
		}
		if l.Pin != routes.PinNone && l.Chain != "" {
			v.add(lp("chain"), "закреплённый уровень (pin) не может входить в цепочку")
		}

		// Ответы.
		if len(l.Answers) == 0 {
			v.add(lp(), "уровень %s: нет ни одного ответа (answers)", name)
		}
		for ai, a := range l.Answers {
			v.validateAnswer(a, lp("answers", ai))
		}
		if l.Require.Kind == RequireN {
			if l.Require.N < 1 || l.Require.N > len(l.Answers) {
				v.add(lp("require"), "require: %d, а ответов на уровне %d", l.Require.N, len(l.Answers))
			}
		}

		// Подсказки.
		auto := 0
		for hi, h := range l.Hints {
			hp := lp("hints", hi)
			if strings.TrimSpace(h.Text) == "" {
				v.add(hp, "пустой текст подсказки")
			}
			if h.OnRequest {
				if h.After != 0 {
					v.add(append(hp, "after"), "подсказка не может быть одновременно по таймеру и по запросу")
				}
				if pen := h.Penalty; pen != nil && (pen.Time == 0) == (pen.Points == 0) {
					v.add(append(hp, "penalty"), "укажите ровно одно: time или points")
				}
				if pen := h.Penalty; pen != nil && pen.Points < 0 {
					v.add(append(hp, "penalty", "points"), "штраф не может быть отрицательным")
				}
				continue
			}
			if h.Penalty != nil {
				v.add(append(hp, "penalty"), "штраф бывает только у подсказок on_request")
			}
			if h.After == 0 && auto >= len(v.s.Defaults.HintAfter) {
				v.add(hp, "у подсказки нет after, а в defaults.hint_after только %d значений", len(v.s.Defaults.HintAfter))
			}
			auto++
		}

		// Бонусы.
		for bi, b := range l.Bonuses {
			bp := lp("bonuses", bi)
			switch {
			case b.ID == "":
				v.add(bp, "не задан id бонуса")
			case !idRe.MatchString(b.ID):
				v.add(append(bp, "id"), "id %q: только строчные латинские буквы, цифры, - и _", b.ID)
			case bonusIDs[b.ID]:
				v.add(append(bp, "id"), "id бонуса %q уже используется", b.ID)
			default:
				bonusIDs[b.ID] = true
			}
			if b.Points < 0 {
				v.add(append(bp, "points"), "баллы не могут быть отрицательными")
			}
			v.validateAnswer(b.Answer, append(bp, "answer"))
		}
	}

	if len(firstPins) > 1 {
		v.add(p("levels"), "pin: first может быть только у одного уровня, сейчас: %s", strings.Join(firstPins, ", "))
	}
	if len(lastPins) > 1 {
		v.add(p("levels"), "pin: last может быть только у одного уровня, сейчас: %s", strings.Join(lastPins, ", "))
	}
	v.validateChains()
}

// validateChains: уровни одной цепочки объявлены подряд.
func (v *validator) validateChains() {
	lastIdx := map[string]int{}
	for i, l := range v.s.Levels {
		if l.Chain == "" {
			continue
		}
		if prev, seen := lastIdx[l.Chain]; seen && prev != i-1 {
			v.add(p("levels", i, "chain"), "уровни цепочки %q должны быть объявлены подряд", l.Chain)
		}
		lastIdx[l.Chain] = i
	}
}

func (v *validator) validateAnswer(a Answer, path []any) {
	at := func(rest ...any) []any { return append(append([]any(nil), path...), rest...) }
	norm := v.s.Defaults.AnswerNormalize

	switch a.Type {
	case AnswerCode:
		if len(a.Values) == 0 {
			v.add(at("values"), "нужен хотя бы один код")
		}
		for i, val := range a.Values {
			if Normalize(val, norm) == "" {
				v.add(at("values", i), "пустой код")
			}
		}
	case AnswerRegex:
		if a.Pattern == "" {
			v.add(at("pattern"), "не задан pattern")
		} else if _, err := regexp.Compile(a.Pattern); err != nil {
			v.add(at("pattern"), "некорректное регулярное выражение: %v", err)
		}
	case AnswerChoice:
		if len(a.Options) < 2 {
			v.add(at("options"), "нужно минимум 2 варианта")
		}
		if !slices.Contains(a.Options, a.Correct) {
			v.add(at("correct"), "правильный ответ %q не найден среди options", a.Correct)
		}
	case AnswerGeo:
		if len(a.Point) != 2 {
			v.add(at("point"), "ожидалось [широта, долгота]")
		} else {
			if a.Point[0] < -90 || a.Point[0] > 90 {
				v.add(at("point"), "широта %v вне диапазона [-90, 90]", a.Point[0])
			}
			if a.Point[1] < -180 || a.Point[1] > 180 {
				v.add(at("point"), "долгота %v вне диапазона [-180, 180]", a.Point[1])
			}
		}
		if a.RadiusM <= 0 {
			v.add(at("radius_m"), "радиус должен быть > 0")
		}
		if a.MaxAccuracyM < 0 {
			v.add(at("max_accuracy_m"), "не может быть отрицательным")
		}
	case AnswerPhoto, AnswerText:
	case "":
		v.add(path, "не задан type ответа")
	default:
		v.add(at("type"), "неизвестный тип %q, ожидалось %s", a.Type,
			list(AnswerCode, AnswerRegex, AnswerChoice, AnswerGeo, AnswerPhoto, AnswerText))
	}

	// Поля, не относящиеся к типу, — скорее всего ошибка автора.
	stray := func(set bool, field string, types ...AnswerType) {
		if set && !slices.Contains(types, a.Type) {
			v.add(at(field), "поле %s не используется в ответе типа %s", field, a.Type)
		}
	}
	stray(len(a.Values) > 0, "values", AnswerCode)
	stray(a.Pattern != "", "pattern", AnswerRegex)
	stray(len(a.Options) > 0, "options", AnswerChoice)
	stray(a.Correct != "", "correct", AnswerChoice)
	stray(len(a.Point) > 0, "point", AnswerGeo)
	stray(a.RadiusM != 0, "radius_m", AnswerGeo)
	stray(a.MaxAccuracyM != 0, "max_accuracy_m", AnswerGeo)
}

func (v *validator) validateFlow() {
	s := v.s
	f := s.Flow
	if !oneOf(f.Mode, FlowLinear, FlowStorm, FlowGraph) {
		v.add(p("flow", "mode"), "ожидалось %s", list(FlowLinear, FlowStorm, FlowGraph))
		return
	}
	if !oneOf(f.Routes, routes.ModeSame, routes.ModeRotate, routes.ModeRandom, routes.ModeExplicit) {
		v.add(p("flow", "routes"), "ожидалось %s",
			list(routes.ModeSame, routes.ModeRotate, routes.ModeRandom, routes.ModeExplicit))
	}

	if f.Mode != FlowLinear {
		if f.Routes != routes.ModeSame {
			v.add(p("flow", "routes"), "маршруты задаются только для flow: linear")
		}
		for i, l := range s.Levels {
			if l.Pin != routes.PinNone {
				v.add(p("levels", i, "pin"), "pin используется только во flow: linear")
			}
			if l.Chain != "" {
				v.add(p("levels", i, "chain"), "chain используется только во flow: linear")
			}
		}
	}

	if f.Routes == routes.ModeExplicit {
		if len(f.RouteList) == 0 {
			v.add(p("flow", "route_list"), "для routes: explicit нужен хотя бы один маршрут")
		}
		levels := s.RouteLevels()
		for i, r := range f.RouteList {
			for _, problem := range routes.Validate(levels, r) {
				v.add(p("flow", "route_list", i), "маршрут %d: %s", i+1, problem)
			}
		}
	} else if len(f.RouteList) > 0 {
		v.add(p("flow", "route_list"), "route_list используется только при routes: explicit")
	}

	if f.Mode == FlowGraph {
		v.validateGraph()
	} else {
		for i, l := range s.Levels {
			if l.Unlock != nil {
				v.add(p("levels", i, "unlock"), "unlock используется только во flow: graph")
			}
		}
	}
}

func (v *validator) validateGraph() {
	s := v.s
	known := map[string]bool{}
	for _, l := range s.Levels {
		known[l.ID] = true
	}

	entry := false
	for i, l := range s.Levels {
		u := l.Unlock
		if u == nil {
			entry = true
			continue
		}
		up := p("levels", i, "unlock")

		forms := 0
		if len(u.AllOf) > 0 {
			forms++
		}
		if len(u.AnyOf) > 0 {
			forms++
		}
		if u.Count > 0 || len(u.Of) > 0 {
			forms++
			if u.Count < 1 || u.Count > len(u.Of) {
				v.add(up, "count (%d) должен быть от 1 до числа уровней в of (%d)", u.Count, len(u.Of))
			}
		}
		if forms != 1 {
			v.add(up, "укажите ровно одно: all_of, any_of или count+of")
		}
		for _, dep := range u.Deps() {
			switch {
			case dep == l.ID:
				v.add(up, "уровень не может зависеть от самого себя")
			case !known[dep]:
				v.add(up, "ссылка на несуществующий уровень %q", dep)
			}
		}
	}
	if !entry {
		v.add(p("levels"), "flow: graph — нужен хотя бы один уровень без unlock (стартовый)")
	}

	if cycle := findCycle(s.Levels); cycle != nil {
		v.add(p("levels"), "циклическая зависимость unlock: %s", strings.Join(cycle, " → "))
	}
}

// findCycle ищет цикл в графе зависимостей unlock (DFS с раскраской).
func findCycle(levels []Level) []string {
	deps := map[string][]string{}
	for _, l := range levels {
		deps[l.ID] = l.Unlock.Deps()
	}
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var cycle []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		for _, d := range deps[id] {
			if _, ok := deps[d]; !ok {
				continue // несуществующие ссылки репортятся отдельно
			}
			switch color[d] {
			case grey:
				start := slices.Index(stack, d)
				cycle = append(append([]string(nil), stack[start:]...), d)
				return true
			case white:
				if dfs(d) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	for _, l := range levels {
		if color[l.ID] == white && dfs(l.ID) {
			return cycle
		}
	}
	return nil
}
