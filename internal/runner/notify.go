package runner

import "questaround/internal/core"

// subscription получает Seq последнего изменения, касающегося команды
// (или любого изменения, если teamID пуст — для ведущего).
//
// Канал с буфером 1 и «последнее значение побеждает»: подписчику нужно
// знать только, что состояние изменилось, — он сам перечитает его.
type subscription struct {
	teamID string
	ch     chan int64
}

// Subscribe подписывает на изменения: teamID — изменения этой команды
// и общие (старт, сообщения ведущего, лидерборд); пустой teamID — все.
// Канал закрывается при остановке актора или вызове cancel.
func (r *Runner) Subscribe(teamID string) (<-chan int64, func()) {
	s := &subscription{teamID: teamID, ch: make(chan int64, 1)}
	r.mu.Lock()
	if r.err != nil {
		r.mu.Unlock()
		close(s.ch)
		return s.ch, func() {}
	}
	r.subs[s] = struct{}{}
	r.mu.Unlock()

	cancel := func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, ok := r.subs[s]; ok {
			delete(r.subs, s)
			close(s.ch)
		}
	}
	return s.ch, cancel
}

func (r *Runner) notify(events []core.Event) {
	teams := map[string]bool{}
	global := false
	for _, e := range events {
		if e.TeamID == "" {
			global = true
		} else {
			teams[e.TeamID] = true
		}
	}
	seq := events[len(events)-1].Seq

	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs {
		if s.teamID == "" || global || teams[s.teamID] {
			push(s.ch, seq)
		}
	}
}

// push кладёт значение, вытесняя непрочитанное. Пишет только актор
// (под r.mu), поэтому после вытеснения место гарантированно есть.
func push(ch chan int64, v int64) {
	select {
	case ch <- v:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- v
	}
}
