package scenario

import (
	"fmt"
	"strings"
)

// Incompatible — новая версия сценария меняет структуру, на которую уже
// ссылается журнал идущей игры (уровни, слоты ответов, подсказки, бонусы).
type Incompatible struct {
	Problems []string
}

func (e *Incompatible) Error() string {
	return "во время игры можно менять только содержимое: " + strings.Join(e.Problems, "; ")
}

// CheckCompatible проверяет, можно ли заменить сценарий old на new в уже
// начатой игре. События журнала ссылаются на уровни по id и на ответы,
// подсказки и бонусы по индексам/id — эта структура должна совпадать.
//
// Менять можно: тексты, медиа, баллы, коды/regex/варианты, координаты,
// радиус и точность, тексты/время/штрафы подсказок, лимит времени,
// настройки таблицы, нормализацию, rate limit, условия unlock.
func CheckCompatible(old, new *Scenario) error {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if old.Participants.Mode != new.Participants.Mode {
		add("participants.mode: %s → %s", old.Participants.Mode, new.Participants.Mode)
	}
	if old.Flow.Mode != new.Flow.Mode {
		add("flow.mode: %s → %s", old.Flow.Mode, new.Flow.Mode)
	}
	if old.Timing.Mode != new.Timing.Mode {
		add("timing.mode: %s → %s", old.Timing.Mode, new.Timing.Mode)
	}

	newLevels := map[string]*Level{}
	for i := range new.Levels {
		newLevels[new.Levels[i].ID] = &new.Levels[i]
	}
	if len(old.Levels) != len(new.Levels) {
		add("число уровней: %d → %d", len(old.Levels), len(new.Levels))
	}
	for i := range old.Levels {
		o := &old.Levels[i]
		n, ok := newLevels[o.ID]
		if !ok {
			add("пропал уровень %q", o.ID)
			continue
		}
		if len(o.Answers) != len(n.Answers) {
			add("уровень %q: число ответов %d → %d", o.ID, len(o.Answers), len(n.Answers))
		} else {
			for k := range o.Answers {
				if o.Answers[k].Type != n.Answers[k].Type {
					add("уровень %q, ответ %d: тип %s → %s", o.ID, k+1, o.Answers[k].Type, n.Answers[k].Type)
				}
			}
		}
		if len(o.Hints) != len(n.Hints) {
			add("уровень %q: число подсказок %d → %d", o.ID, len(o.Hints), len(n.Hints))
		} else {
			for k := range o.Hints {
				if o.Hints[k].OnRequest != n.Hints[k].OnRequest {
					add("уровень %q, подсказка %d: по таймеру ↔ по запросу", o.ID, k+1)
				}
			}
		}
		if len(o.Bonuses) != len(n.Bonuses) {
			add("уровень %q: число бонусов %d → %d", o.ID, len(o.Bonuses), len(n.Bonuses))
		} else {
			for k := range o.Bonuses {
				ob, nb := o.Bonuses[k], n.Bonuses[k]
				if ob.ID != nb.ID || ob.Answer.Type != nb.Answer.Type {
					add("уровень %q: бонус %q (%s) → %q (%s)", o.ID, ob.ID, ob.Answer.Type, nb.ID, nb.Answer.Type)
				}
			}
		}
	}
	if len(p) > 0 {
		return &Incompatible{Problems: p}
	}
	return nil
}
