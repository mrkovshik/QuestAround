// Package routes строит порядок прохождения уровней для каждой команды
// в линейном flow: одинаковый, со сдвигом (ротация), случайный или явный.
//
// Пакет не зависит от формата сценария: на вход — плоский список уровней
// с признаками pin и chain, на выход — список ID уровней для каждой команды.
package routes

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Pin закрепляет уровень в начале или в конце маршрута у всех команд.
type Pin string

const (
	PinNone  Pin = ""
	PinFirst Pin = "first"
	PinLast  Pin = "last"
)

// Mode — способ построения маршрутов.
type Mode string

const (
	ModeSame     Mode = "same"     // у всех одинаковый порядок
	ModeRotate   Mode = "rotate"   // циклический сдвиг, команды равномерно разнесены по кругу
	ModeRandom   Mode = "random"   // случайный порядок для каждой команды (детерминирован seed)
	ModeExplicit Mode = "explicit" // маршруты заданы явно, команды распределяются по кругу
)

// Level — уровень в том порядке, в котором он объявлен в сценарии.
type Level struct {
	ID    string
	Pin   Pin
	Chain string // уровни с одинаковым Chain идут подряд и двигаются как один блок
}

// Options — параметры построения.
type Options struct {
	Mode     Mode
	Teams    int
	Seed     uint64     // для ModeRandom
	Explicit [][]string // для ModeExplicit
}

// Build возвращает маршрут (список ID уровней) для каждой из opt.Teams команд.
// Индекс в результате — порядковый номер команды (слот старта).
func Build(levels []Level, opt Options) ([][]string, error) {
	if opt.Teams <= 0 {
		return nil, errors.New("число команд должно быть больше нуля")
	}
	if len(levels) == 0 {
		return nil, errors.New("нет уровней")
	}

	if opt.Mode == ModeExplicit {
		return buildExplicit(levels, opt)
	}

	head, tail, blocks := split(levels)
	n := len(blocks)
	out := make([][]string, opt.Teams)

	for i := range opt.Teams {
		order := make([]int, n)
		switch opt.Mode {
		case ModeSame, "":
			for k := range order {
				order[k] = k
			}
		case ModeRotate:
			// floor(i*n/teams): при teams <= n стартовые блоки максимально
			// разнесены по кругу; при teams > n повторы распределяются равномерно.
			offset := 0
			if n > 0 {
				offset = i * n / opt.Teams
			}
			for k := range order {
				order[k] = (offset + k) % n
			}
		case ModeRandom:
			r := rand.New(rand.NewPCG(opt.Seed, uint64(i)))
			order = r.Perm(n)
		default:
			return nil, fmt.Errorf("неизвестный режим маршрутов %q", opt.Mode)
		}

		route := make([]string, 0, len(levels))
		route = append(route, head...)
		for _, b := range order {
			route = append(route, blocks[b]...)
		}
		route = append(route, tail...)
		out[i] = route
	}
	return out, nil
}

func buildExplicit(levels []Level, opt Options) ([][]string, error) {
	if len(opt.Explicit) == 0 {
		return nil, errors.New("режим explicit: список маршрутов пуст")
	}
	for i, r := range opt.Explicit {
		if problems := Validate(levels, r); len(problems) > 0 {
			return nil, fmt.Errorf("маршрут %d: %s", i+1, problems[0])
		}
	}
	out := make([][]string, opt.Teams)
	for i := range opt.Teams {
		src := opt.Explicit[i%len(opt.Explicit)]
		out[i] = append([]string(nil), src...)
	}
	return out, nil
}

// split делит уровни на закреплённые в начале, закреплённые в конце
// и подвижные блоки (одиночные уровни или цепочки).
// Предполагается, что сценарий уже провалидирован: цепочки непрерывны
// и не содержат закреплённых уровней.
func split(levels []Level) (head, tail []string, blocks [][]string) {
	for i := 0; i < len(levels); {
		l := levels[i]
		switch l.Pin {
		case PinFirst:
			head = append(head, l.ID)
			i++
			continue
		case PinLast:
			tail = append(tail, l.ID)
			i++
			continue
		}
		block := []string{l.ID}
		j := i + 1
		if l.Chain != "" {
			for j < len(levels) && levels[j].Chain == l.Chain && levels[j].Pin == PinNone {
				block = append(block, levels[j].ID)
				j++
			}
		}
		blocks = append(blocks, block)
		i = j
	}
	return head, tail, blocks
}

// Validate проверяет, что route — корректный маршрут для levels:
// содержит каждый уровень ровно один раз, соблюдает pin и порядок цепочек.
// Возвращает список проблем (пустой — маршрут корректен).
func Validate(levels []Level, route []string) []string {
	var problems []string

	known := make(map[string]Level, len(levels))
	for _, l := range levels {
		known[l.ID] = l
	}

	pos := make(map[string]int, len(route))
	for i, id := range route {
		if _, ok := known[id]; !ok {
			problems = append(problems, fmt.Sprintf("неизвестный уровень %q", id))
			continue
		}
		if _, dup := pos[id]; dup {
			problems = append(problems, fmt.Sprintf("уровень %q встречается дважды", id))
			continue
		}
		pos[id] = i
	}
	for _, l := range levels {
		if _, ok := pos[l.ID]; !ok {
			problems = append(problems, fmt.Sprintf("нет уровня %q", l.ID))
		}
	}
	if len(problems) > 0 {
		return problems
	}

	for _, l := range levels {
		switch l.Pin {
		case PinFirst:
			if pos[l.ID] != 0 {
				problems = append(problems, fmt.Sprintf("уровень %q закреплён первым", l.ID))
			}
		case PinLast:
			if pos[l.ID] != len(route)-1 {
				problems = append(problems, fmt.Sprintf("уровень %q закреплён последним", l.ID))
			}
		}
	}

	// Цепочки: члены идут подряд в объявленном порядке.
	chains := map[string][]string{}
	var chainOrder []string
	for _, l := range levels {
		if l.Chain == "" {
			continue
		}
		if _, seen := chains[l.Chain]; !seen {
			chainOrder = append(chainOrder, l.Chain)
		}
		chains[l.Chain] = append(chains[l.Chain], l.ID)
	}
	for _, name := range chainOrder {
		members := chains[name]
		start := pos[members[0]]
		for k, id := range members {
			if pos[id] != start+k {
				problems = append(problems, fmt.Sprintf("цепочка %q должна идти подряд: %v", name, members))
				break
			}
		}
	}
	return problems
}
