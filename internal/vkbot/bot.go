// Package vkbot — бот игроков в сообществе VK. Запасной транспорт для игр
// в России: VK входит в «белый список» и работает при отключениях мобильного
// интернета, когда наш сайт недоступен.
//
// Бот — такой же клиент движка, как HTTP API: игрок вступает в команду по
// инвайту, получает задания сообщениями, отвечает текстом (код) или
// местоположением (гео). События журнала игры (новый уровень, подсказка,
// сообщение ведущего…) бот рассылает VK-игрокам команды сам.
package vkbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"questaround/internal/core"
	"questaround/internal/runner"
	"questaround/internal/scenario"
	"questaround/internal/store"
)

// Media — картинки заданий и фото из сообщений (реализует httpapi.DiskMedia).
type Media interface {
	AssetKey(name string) (string, bool)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, string, error)
	// Save сохраняет фото игрока; не картинка — ошибка.
	Save(ctx context.Context, runID string, r io.Reader) (string, error)
}

type Config struct {
	API     API
	Store   store.Store
	VK      store.VKStore
	Runners *runner.Manager
	Media   Media
	Logger  *slog.Logger
	Clock   runner.Clock // nil — системное время
	OrgID   string       // по умолчанию store.DefaultOrg
	// SearchWindow — в каких играх искать голый инвайт-код (по дате создания,
	// по умолчанию 60 дней).
	SearchWindow time.Duration
	// Location — часовой пояс для времени в сообщениях (по умолчанию time.Local).
	Location *time.Location
	// GroupURL — ссылка «написать сообществу» для приглашений.
	GroupURL string
}

type Bot struct {
	cfg Config
	log *slog.Logger

	mu       sync.Mutex
	modes    map[int64]mode
	watching map[string]bool
	photos   map[string]string // ключ медиа → вложение VK
	runCtx   context.Context
	wg       sync.WaitGroup
}

func New(cfg Config) *Bot {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Clock == nil {
		cfg.Clock = runner.RealClock{}
	}
	if cfg.OrgID == "" {
		cfg.OrgID = store.DefaultOrg
	}
	if cfg.SearchWindow == 0 {
		cfg.SearchWindow = 60 * 24 * time.Hour
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	return &Bot{
		cfg:      cfg,
		log:      cfg.Logger.With("transport", "vk"),
		modes:    map[int64]mode{},
		watching: map[string]bool{},
		photos:   map[string]string{},
	}
}

func (b *Bot) now() time.Time { return b.cfg.Clock.Now() }

// Run слушает Long Poll и рассылает уведомления, пока не отменён ctx.
func (b *Bot) Run(ctx context.Context) error {
	b.mu.Lock()
	b.runCtx = ctx
	b.mu.Unlock()
	defer b.wg.Wait()

	binds, err := b.cfg.VK.ListVK(ctx)
	if err != nil {
		return fmt.Errorf("vk: привязки игроков: %w", err)
	}
	for _, bd := range binds {
		b.watch(bd.RunID)
	}

	var (
		lp      LongPoll
		renew   = true
		backoff = time.Second
	)
	sleep := func() {
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}
		backoff = min(backoff*2, time.Minute)
	}
	b.log.Info("VK-бот запущен")
	for ctx.Err() == nil {
		if renew {
			lp, err = b.cfg.API.LongPollServer(ctx)
			if err != nil {
				if ctx.Err() == nil {
					b.log.Error("long poll: сервер", "err", err)
					sleep()
				}
				continue
			}
			renew = false
		}
		res, err := b.cfg.API.Poll(ctx, lp)
		if err != nil {
			if ctx.Err() == nil {
				b.log.Warn("long poll", "err", err)
				sleep()
			}
			continue
		}
		backoff = time.Second
		switch res.Failed {
		case 0:
		case 1:
			lp.TS = res.TS
			continue
		default:
			renew = true
			continue
		}
		lp.TS = res.TS
		for _, u := range res.Updates {
			if u.Type == "message_new" {
				b.safeHandle(ctx, u.Object.Message)
			}
		}
	}
	return nil
}

func (b *Bot) safeHandle(ctx context.Context, m Message) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("паника при обработке сообщения", "panic", r, "from", m.FromID)
		}
	}()
	b.handle(ctx, m)
}

// --- отправка ---

// out — сообщение к отправке: текст, картинки и клавиатура.
type out struct {
	peer     int64
	text     string
	assets   []string // имена картинок из сценария или ссылки https://
	keys     []string // ключи медиа (фото из сообщений ведущего)
	keyboard *Keyboard
	lat, lng *float64
}

const maxText = 4000

func (b *Bot) send(ctx context.Context, o out) {
	var atts []string
	text := o.text
	var links []string
	failed := 0
	for _, a := range o.assets {
		if scenario.IsMediaURL(a) {
			links = append(links, a)
			continue
		}
		key, ok := b.cfg.Media.AssetKey(a)
		if !ok {
			b.log.Warn("картинки нет в библиотеке", "name", a)
			failed++
			continue
		}
		if att := b.photo(ctx, o.peer, key); att != "" {
			atts = append(atts, att)
		} else {
			failed++
		}
	}
	for _, k := range o.keys {
		if att := b.photo(ctx, o.peer, k); att != "" {
			atts = append(atts, att)
		} else {
			failed++
		}
	}
	if failed > 0 {
		text += "\n\n🖼 Картинка не загрузилась в VK — если без неё не решить, напишите ведущему."
	}
	if len(links) > 0 {
		text += "\n\n🖼 " + strings.Join(links, "\n🖼 ")
	}
	if r := []rune(text); len(r) > maxText {
		text = string(r[:maxText]) + "…"
	}
	if len(atts) > 10 {
		atts = atts[:10] // предел VK
	}
	err := b.cfg.API.Send(ctx, Outgoing{PeerID: o.peer, Text: text, Attachments: atts, Keyboard: o.keyboard, Lat: o.lat, Lng: o.lng})
	if err != nil {
		var ve *ErrVK
		if errors.As(err, &ve) && (ve.Code == 901 || ve.Code == 902) {
			// Пользователь запретил сообщения от сообщества — ничего не поделать.
			b.log.Info("сообщения запрещены пользователем", "peer", o.peer)
			return
		}
		b.log.Error("отправка сообщения", "err", err, "peer", o.peer)
	}
}

func (b *Bot) reply(ctx context.Context, peer int64, text string) {
	b.send(ctx, out{peer: peer, text: text})
}

// photo загружает картинку в VK (один раз на ключ) и возвращает вложение.
func (b *Bot) photo(ctx context.Context, peer int64, key string) string {
	b.mu.Lock()
	att, ok := b.photos[key]
	b.mu.Unlock()
	if ok {
		return att
	}
	f, _, err := b.cfg.Media.Open(ctx, key)
	if err != nil {
		b.log.Warn("картинка не найдена", "key", key, "err", err)
		return ""
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		b.log.Warn("картинка не читается", "key", key, "err", err)
		return ""
	}
	// Сервер загрузки VK иногда отказывает (405 и т.п.): пробуем ещё раз
	// с новым адресом, затем — перекодированной в JPEG (меньше и надёжнее).
	name := path.Base(key)
	att, err = b.cfg.API.UploadPhoto(ctx, peer, name, bytes.NewReader(data))
	if err != nil {
		b.log.Warn("загрузка картинки в VK, повтор", "key", key, "bytes", len(data), "err", err)
		att, err = b.cfg.API.UploadPhoto(ctx, peer, name, bytes.NewReader(data))
	}
	if err != nil {
		if jpg, ok := toJPEG(data); ok {
			b.log.Warn("загрузка картинки в VK, пробуем JPEG", "key", key, "bytes", len(data), "jpeg_bytes", len(jpg), "err", err)
			att, err = b.cfg.API.UploadPhoto(ctx, peer, strings.TrimSuffix(name, path.Ext(name))+".jpg", bytes.NewReader(jpg))
		}
	}
	if err != nil {
		b.log.Error("загрузка картинки в VK", "key", key, "bytes", len(data), "err", err)
		return ""
	}
	b.mu.Lock()
	b.photos[key] = att
	b.mu.Unlock()
	return att
}

// --- сессия игрока ---

type session struct {
	bind    store.VKBinding
	rn      *runner.Runner
	teamID  string
	captain bool
}

// session — игрок, которым играет пользователь; nil — не привязан
// (или игра удалена, или игрока убрали из команды).
func (b *Bot) session(ctx context.Context, userID int64) (*session, error) {
	bd, err := b.cfg.VK.GetVK(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rn, err := b.cfg.Runners.Get(ctx, bd.RunID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s := &session{bind: bd, rn: rn}
	ok := false
	err = rn.Read(ctx, func(g *core.Game) {
		p := g.Players[bd.PlayerID]
		if p == nil || p.Removed {
			return
		}
		ok = true
		s.teamID = p.TeamID
		s.captain = g.Teams[p.TeamID].CaptainID == p.ID
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	b.watch(bd.RunID)
	return s, nil
}

// toJPEG перекодирует PNG/GIF/JPEG в JPEG: прозрачность — на белом фоне,
// стороны больше 2560 px уменьшаются.
func toJPEG(data []byte) ([]byte, bool) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	bnd := src.Bounds()
	w, h := bnd.Dx(), bnd.Dy()
	if w == 0 || h == 0 {
		return nil, false
	}
	scale := 1
	for max(w, h)/scale > 2560 {
		scale++
	}
	dst := image.NewRGBA(image.Rect(0, 0, w/scale, h/scale))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	if scale == 1 {
		draw.Draw(dst, dst.Bounds(), src, bnd.Min, draw.Over)
	} else {
		// Простое уменьшение по ближайшему пикселю — для картинок задания хватает.
		for y := 0; y < h/scale; y++ {
			for x := 0; x < w/scale; x++ {
				// RGBA премультиплицирован: на белом фоне это v + 255·(1−α).
				c := color.RGBAModel.Convert(src.At(bnd.Min.X+x*scale, bnd.Min.Y+y*scale)).(color.RGBA)
				bg := 255 - uint32(c.A)
				dst.SetRGBA(x, y, color.RGBA{sat(uint32(c.R) + bg), sat(uint32(c.G) + bg), sat(uint32(c.B) + bg), 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func sat(v uint32) uint8 {
	if v > 255 {
		return 255
	}
	return uint8(v)
}
