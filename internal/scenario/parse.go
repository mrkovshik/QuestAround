package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"questaround/internal/routes"
)

// ParseFile читает и разбирает сценарий из файла.
func ParseFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse разбирает YAML, подставляет значения по умолчанию и валидирует.
// При проблемах в сценарии возвращает Errors со всеми найденными ошибками.
func Parse(data []byte) (*Scenario, error) { return ParseWith(data, Options{}) }

// Options — проверки, которым нужен внешний мир.
type Options struct {
	// MediaExists — загружена ли картинка с таким именем в библиотеку.
	// nil — имена файлов не проверяются (только формат).
	MediaExists func(name string) bool
	// Strict — проверки, которые действуют только для новых версий сценария
	// (при загрузке), чтобы не ломать уже сохранённые игры: например,
	// баллы в режиме зачёта по времени.
	Strict bool
}

// ParseWith — Parse с дополнительными проверками.
func ParseWith(data []byte, opt Options) (*Scenario, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("синтаксис YAML: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // опечатка в ключе — ошибка, а не молча игнорируемое поле
	var s Scenario
	if err := dec.Decode(&s); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("пустой сценарий")
		}
		return nil, fmt.Errorf("структура сценария: %w", err)
	}

	markExplicitAfter(&root, &s)
	s.applyDefaults()

	v := &validator{s: &s, loc: locator{root: &root}, opt: opt}
	v.validate()
	if len(v.errs) > 0 {
		return nil, v.errs
	}

	s.resolveHints()
	return &s, nil
}

func (s *Scenario) applyDefaults() {
	for i := range s.Levels {
		if rl := s.Levels[i].RateLimit; rl != nil && rl.Action == "" {
			rl.Action = RateLimitBlock
		}
	}
	p := &s.Participants
	if p.Mode == "" {
		p.Mode = ParticipantsTeam
	}
	if p.Mode == ParticipantsSolo {
		if p.TeamSize.Min == 0 {
			p.TeamSize.Min = 1
		}
		if p.TeamSize.Max == 0 {
			p.TeamSize.Max = 1
		}
	} else {
		if p.TeamSize.Min == 0 {
			p.TeamSize.Min = 1
		}
		if p.TeamSize.Max == 0 {
			p.TeamSize.Max = 5
		}
	}
	if p.HintRequests == "" {
		p.HintRequests = HintRequestsCaptain
	}

	if s.Timing.Mode == "" {
		s.Timing.Mode = TimingAsync
	}
	if s.Start.Mode == "" {
		s.Start.Mode = StartSimultaneous
	}
	if s.Start.Order == "" {
		s.Start.Order = OrderRegistration
	}
	if s.Flow.Mode == "" {
		s.Flow.Mode = FlowLinear
	}
	if s.Flow.Routes == "" {
		s.Flow.Routes = routes.ModeSame
	}
	if s.Scoring.Mode == "" {
		s.Scoring.Mode = ScoringPoints
	}
	if s.Leaderboard.Visibility == "" {
		s.Leaderboard.Visibility = VisibilityAlways
	}
	if len(s.Leaderboard.Columns) == 0 {
		s.Leaderboard.Columns = []string{"rank", "team", "points", "levels", "time"}
	}
	if s.Defaults.AnswerNormalize == nil {
		s.Defaults.AnswerNormalize = append([]string(nil), defaultNormalize...)
	}
}

// resolveHints проставляет время автоподсказкам без явного after
// из defaults.hint_after (валидатор уже проверил, что значений хватает).
func (s *Scenario) resolveHints() {
	for li := range s.Levels {
		auto := 0 // номер подсказки по таймеру (без on_request) на уровне
		for hi := range s.Levels[li].Hints {
			h := &s.Levels[li].Hints[hi]
			if h.OnRequest {
				continue
			}
			if h.usesSchedule() {
				h.After = s.Defaults.HintAfter[auto]
			}
			auto++
		}
	}
}

// markExplicitAfter отмечает подсказки, у которых after указан в YAML явно:
// after: 0 («автоматом никогда») отличается от отсутствующего after.
func markExplicitAfter(root *yaml.Node, s *Scenario) {
	doc := root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	levels := mapValue(doc, "levels")
	if levels == nil || levels.Kind != yaml.SequenceNode {
		return
	}
	for li, ln := range levels.Content {
		if li >= len(s.Levels) {
			return
		}
		hints := mapValue(ln, "hints")
		if hints == nil || hints.Kind != yaml.SequenceNode {
			continue
		}
		for hi, hn := range hints.Content {
			if hi < len(s.Levels[li].Hints) && mapValue(hn, "after") != nil {
				s.Levels[li].Hints[hi].AfterSet = true
			}
		}
	}
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
