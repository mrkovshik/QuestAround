package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"questline/internal/routes"
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
func Parse(data []byte) (*Scenario, error) {
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

	s.applyDefaults()

	v := &validator{s: &s, loc: locator{root: &root}}
	v.validate()
	if len(v.errs) > 0 {
		return nil, v.errs
	}

	s.resolveHints()
	return &s, nil
}

func (s *Scenario) applyDefaults() {
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
		auto := 0
		for hi := range s.Levels[li].Hints {
			h := &s.Levels[li].Hints[hi]
			if !h.IsAuto() {
				continue
			}
			if h.After == 0 {
				h.After = s.Defaults.HintAfter[auto]
			}
			auto++
		}
	}
}
