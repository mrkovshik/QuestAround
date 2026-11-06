// Package runner — актор игры: одна горутина владеет core.Game,
// принимает команды через канал, сохраняет события в журнал до ответа
// клиенту, сам присылает Tick к ближайшему дедлайну и оповещает подписчиков.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"questaround/internal/core"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// ErrStopped — актор остановлен (контекст отменён или фатальная ошибка).
var ErrStopped = errors.New("runner: игра остановлена")

// Config — параметры актора.
type Config struct {
	RunID    string
	Scenario *scenario.Scenario
	Seed     uint64
	Store    store.EventStore
	Clock    Clock        // nil — реальное время
	Logger   *slog.Logger // nil — slog.Default()
	// AppendTimeout — сколько ждать записи в журнал (по умолчанию 5s).
	AppendTimeout time.Duration
}

// Runner — актор одной игры. Безопасен для конкурентного использования.
type Runner struct {
	cfg  Config
	log  *slog.Logger
	reqs chan request
	done chan struct{}

	mu   sync.Mutex
	subs map[*subscription]struct{}
	err  error // причина остановки

	game *core.Game // только в горутине loop
}

type request struct {
	cmd   core.Command
	read  func(*core.Game)
	reply chan reply
}

type reply struct {
	out core.Outcome
	err error
}

// Start загружает журнал, восстанавливает игру и запускает актор.
// Актор работает, пока не отменён ctx.
func Start(ctx context.Context, cfg Config) (*Runner, error) {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.AppendTimeout == 0 {
		cfg.AppendTimeout = 5 * time.Second
	}
	r := &Runner{
		cfg:  cfg,
		log:  cfg.Logger.With("run", cfg.RunID),
		reqs: make(chan request),
		done: make(chan struct{}),
		subs: map[*subscription]struct{}{},
	}
	g, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	r.game = g
	go r.loop(ctx)
	return r, nil
}

func (r *Runner) load(ctx context.Context) (*core.Game, error) {
	events, err := r.cfg.Store.Load(ctx, r.cfg.RunID)
	if err != nil {
		return nil, fmt.Errorf("загрузка журнала: %w", err)
	}
	return core.Replay(r.cfg.RunID, r.cfg.Scenario, r.cfg.Seed, events), nil
}

// Do выполняет команду. Возвращает события, уже сохранённые в журнале.
func (r *Runner) Do(ctx context.Context, cmd core.Command) (core.Outcome, error) {
	rep, err := r.send(ctx, request{cmd: cmd})
	if err != nil {
		return core.Outcome{}, err
	}
	return rep.out, rep.err
}

// Read выполняет fn внутри актора с текущим состоянием игры.
// fn не должна сохранять ссылки на Game или его части и не должна блокироваться.
func (r *Runner) Read(ctx context.Context, fn func(g *core.Game)) error {
	_, err := r.send(ctx, request{read: fn})
	return err
}

func (r *Runner) send(ctx context.Context, req request) (reply, error) {
	req.reply = make(chan reply, 1)
	select {
	case r.reqs <- req:
	case <-r.done:
		return reply{}, r.stopErr()
	case <-ctx.Done():
		return reply{}, ctx.Err()
	}
	// Команда принята — ждём результата даже при отмене ctx клиента:
	// событие могло уже записаться, а ответ нужен для идемпотентности.
	select {
	case rep := <-req.reply:
		return rep, nil
	case <-r.done:
		return reply{}, r.stopErr()
	}
}

// Done закрывается, когда актор остановился.
func (r *Runner) Done() <-chan struct{} { return r.done }

// Err — причина остановки (nil, пока работает).
func (r *Runner) Err() error {
	select {
	case <-r.done:
		return r.stopErr()
	default:
		return nil
	}
}

func (r *Runner) stopErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	return ErrStopped
}

func (r *Runner) loop(ctx context.Context) {
	defer close(r.done)
	for {
		var (
			timer  Timer
			timerC <-chan time.Time
		)
		if dl, ok := r.game.NextDeadline(); ok {
			timer = r.cfg.Clock.NewTimer(dl.Sub(r.cfg.Clock.Now()))
			timerC = timer.C()
		}

		select {
		case <-ctx.Done():
			r.stop(timer, ctx.Err())
			return

		case req := <-r.reqs:
			if req.read != nil {
				req.read(r.game)
				req.reply <- reply{}
			} else {
				out, err := r.handle(req.cmd)
				req.reply <- reply{out: out, err: err}
			}

		case <-timerC:
			if _, err := r.handle(core.Tick{}); err != nil {
				r.log.Error("tick", "err", err)
			}
		}

		if timer != nil {
			timer.Stop()
		}
		if err := r.fatal(); err != nil {
			r.stop(nil, err)
			return
		}
	}
}

func (r *Runner) stop(timer Timer, err error) {
	if timer != nil {
		timer.Stop()
	}
	r.mu.Lock()
	if r.err == nil {
		r.err = fmt.Errorf("%w: %v", ErrStopped, err)
	}
	for s := range r.subs {
		close(s.ch)
		delete(r.subs, s)
	}
	r.mu.Unlock()
}

// fatal — ошибка, после которой актор не может продолжать (состояние
// разошлось с журналом и не восстановилось).
func (r *Runner) fatal() error {
	if r.game == nil {
		return errors.New("состояние игры потеряно")
	}
	return nil
}

func (r *Runner) handle(cmd core.Command) (core.Outcome, error) {
	// Время с точностью Postgres: состояние в памяти и после рестарта совпадает.
	now := r.cfg.Clock.Now().UTC().Truncate(time.Microsecond)
	out, err := r.game.Handle(now, cmd)
	if err != nil || len(out.Events) == 0 {
		return out, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.AppendTimeout)
	defer cancel()
	if err := r.cfg.Store.Append(ctx, r.cfg.RunID, out.Events); err != nil {
		// Состояние в памяти ушло вперёд журнала — откатываемся перечитыванием.
		r.log.Error("запись журнала", "err", err, "cmd", fmt.Sprintf("%T", cmd))
		g, lerr := r.load(ctx)
		if lerr != nil {
			r.log.Error("восстановление после сбоя записи", "err", lerr)
			r.game = nil
		} else {
			r.game = g
		}
		return core.Outcome{}, fmt.Errorf("сохранение: %w", err)
	}
	r.notify(out.Events)
	return out, nil
}
