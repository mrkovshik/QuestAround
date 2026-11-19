package vkbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// API — то, что бот использует из VK API. В тестах подменяется.
type API interface {
	// LongPollServer — адрес и ключ Bots Long Poll.
	LongPollServer(ctx context.Context) (LongPoll, error)
	// Poll ждёт события (до ~25 с). Failed != 0 — нужно обновить ts или ключ.
	Poll(ctx context.Context, lp LongPoll) (PollResult, error)
	Send(ctx context.Context, m Outgoing) error
	// UploadPhoto загружает картинку для сообщений и возвращает вложение "photo…".
	// peerID — получатель: с токеном сообщества без него VK не даёт адрес загрузки.
	// Загруженное вложение можно отправлять и другим собеседникам.
	UploadPhoto(ctx context.Context, peerID int64, filename string, r io.Reader) (string, error)
	// UserName — имя и фамилия пользователя.
	UserName(ctx context.Context, userID int64) (first, last string, err error)
}

type LongPoll struct {
	Server string
	Key    string
	TS     string
}

type PollResult struct {
	TS      string
	Updates []Update
	// Failed: 1 — взять новый TS из ответа; 2 — ключ истёк; 3 — всё заново.
	Failed int
}

// Update — событие Long Poll. Бот обрабатывает только message_new.
type Update struct {
	Type   string `json:"type"`
	Object struct {
		Message Message `json:"message"`
	} `json:"object"`
}

type Message struct {
	ID       int64  `json:"id"`
	FromID   int64  `json:"from_id"`
	PeerID   int64  `json:"peer_id"`
	Text     string `json:"text"`
	Payload  string `json:"payload"`
	Geo      *Geo   `json:"geo"`
	Attaches []struct {
		Type string `json:"type"`
	} `json:"attachments"`
}

type Geo struct {
	Coordinates struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"coordinates"`
}

// Outgoing — исходящее сообщение.
type Outgoing struct {
	PeerID      int64
	Text        string
	Attachments []string
	Keyboard    *Keyboard
	// Lat/Lng — точка на карте в сообщении (слив локации).
	Lat, Lng *float64
}

// ErrVK — ошибка, которую вернул VK API.
type ErrVK struct {
	Code int
	Msg  string
}

func (e *ErrVK) Error() string { return fmt.Sprintf("vk: %d %s", e.Code, e.Msg) }

const apiVersion = "5.199"

// Client — VK API с токеном сообщества.
type Client struct {
	Token   string
	GroupID int64
	BaseURL string       // по умолчанию https://api.vk.com/method/
	HTTP    *http.Client // по умолчанию — с таймаутом 40 с

	mu   sync.Mutex
	last time.Time // ограничение частоты: VK разрешает 20 запросов в секунду
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 40 * time.Second}
}

// throttle выдерживает ≥60 мс между вызовами методов.
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.last.Add(60 * time.Millisecond))
	if wait < 0 {
		wait = 0
	}
	c.last = time.Now().Add(wait)
	c.mu.Unlock()
	if wait == 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// call вызывает метод и раскладывает поле response в out.
func (c *Client) call(ctx context.Context, method string, params url.Values, out any) error {
	if err := c.throttle(ctx); err != nil {
		return err
	}
	params.Set("access_token", c.Token)
	params.Set("v", apiVersion)
	base := c.BaseURL
	if base == "" {
		base = "https://api.vk.com/method/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+method, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var body struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Code int    `json:"error_code"`
			Msg  string `json:"error_msg"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return fmt.Errorf("vk %s: %w", method, err)
	}
	if body.Error != nil {
		return &ErrVK{Code: body.Error.Code, Msg: body.Error.Msg}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body.Response, out)
}

func (c *Client) LongPollServer(ctx context.Context) (LongPoll, error) {
	var r struct {
		Server string `json:"server"`
		Key    string `json:"key"`
		TS     any    `json:"ts"`
	}
	err := c.call(ctx, "groups.getLongPollServer", url.Values{"group_id": {strconv.FormatInt(c.GroupID, 10)}}, &r)
	return LongPoll{Server: r.Server, Key: r.Key, TS: fmt.Sprint(r.TS)}, err
}

func (c *Client) Poll(ctx context.Context, lp LongPoll) (PollResult, error) {
	u := lp.Server + "?" + url.Values{"act": {"a_check"}, "key": {lp.Key}, "ts": {lp.TS}, "wait": {"25"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return PollResult{}, err
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return PollResult{}, err
	}
	defer res.Body.Close()
	var r struct {
		TS      any      `json:"ts"`
		Updates []Update `json:"updates"`
		Failed  int      `json:"failed"`
	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return PollResult{}, fmt.Errorf("long poll: %w", err)
	}
	out := PollResult{Updates: r.Updates, Failed: r.Failed}
	if r.TS != nil {
		out.TS = fmt.Sprint(r.TS)
	}
	return out, nil
}

func (c *Client) Send(ctx context.Context, m Outgoing) error {
	p := url.Values{
		"peer_id":          {strconv.FormatInt(m.PeerID, 10)},
		"random_id":        {strconv.FormatInt(int64(rand.Int32()), 10)},
		"message":          {m.Text},
		"dont_parse_links": {"1"},
	}
	if len(m.Attachments) > 0 {
		p.Set("attachment", strings.Join(m.Attachments, ","))
	}
	if m.Keyboard != nil {
		kb, err := json.Marshal(m.Keyboard)
		if err != nil {
			return err
		}
		p.Set("keyboard", string(kb))
	}
	if m.Lat != nil && m.Lng != nil {
		p.Set("lat", strconv.FormatFloat(*m.Lat, 'f', 6, 64))
		p.Set("long", strconv.FormatFloat(*m.Lng, 'f', 6, 64))
	}
	return c.call(ctx, "messages.send", p, nil)
}

func (c *Client) UploadPhoto(ctx context.Context, peerID int64, filename string, r io.Reader) (string, error) {
	var srv struct {
		UploadURL string `json:"upload_url"`
	}
	if err := c.call(ctx, "photos.getMessagesUploadServer", url.Values{"peer_id": {strconv.FormatInt(peerID, 10)}}, &srv); err != nil {
		return "", fmt.Errorf("photos.getMessagesUploadServer: %w", err)
	}
	if srv.UploadURL == "" {
		return "", errors.New("photos.getMessagesUploadServer: пустой upload_url")
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("photo", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, r); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.UploadURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var up struct {
		Server int    `json:"server"`
		Photo  string `json:"photo"`
		Hash   string `json:"hash"`
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("загрузка фото: %w", err)
	}
	if err := json.Unmarshal(raw, &up); err != nil {
		return "", fmt.Errorf("загрузка фото: HTTP %d, ответ %.200q", res.StatusCode, raw)
	}
	if up.Photo == "" || up.Photo == "[]" {
		return "", fmt.Errorf("загрузка фото: VK не принял файл, ответ %.200q", raw)
	}
	var saved []struct {
		ID        int64  `json:"id"`
		OwnerID   int64  `json:"owner_id"`
		AccessKey string `json:"access_key"`
	}
	err = c.call(ctx, "photos.saveMessagesPhoto", url.Values{
		"server": {strconv.Itoa(up.Server)}, "photo": {up.Photo}, "hash": {up.Hash},
	}, &saved)
	if err != nil {
		return "", fmt.Errorf("photos.saveMessagesPhoto: %w", err)
	}
	if len(saved) == 0 {
		return "", errors.New("загрузка фото: пустой ответ")
	}
	att := fmt.Sprintf("photo%d_%d", saved[0].OwnerID, saved[0].ID)
	if saved[0].AccessKey != "" {
		att += "_" + saved[0].AccessKey
	}
	return att, nil
}

func (c *Client) UserName(ctx context.Context, userID int64) (string, string, error) {
	var users []struct {
		First string `json:"first_name"`
		Last  string `json:"last_name"`
	}
	if err := c.call(ctx, "users.get", url.Values{"user_ids": {strconv.FormatInt(userID, 10)}}, &users); err != nil {
		return "", "", err
	}
	if len(users) == 0 {
		return "", "", errors.New("пользователь не найден")
	}
	return users[0].First, users[0].Last, nil
}

// --- клавиатуры ---

type Keyboard struct {
	OneTime bool       `json:"one_time"`
	Inline  bool       `json:"inline,omitempty"`
	Buttons [][]Button `json:"buttons"`
}

type Button struct {
	Action ButtonAction `json:"action"`
	Color  string       `json:"color,omitempty"` // primary | secondary | negative | positive (только text)
}

type ButtonAction struct {
	Type    string `json:"type"` // text | location
	Label   string `json:"label,omitempty"`
	Payload string `json:"payload,omitempty"`
}

// textButton — кнопка с командой в payload.
func textButton(label, color string, p payload) Button {
	data, _ := json.Marshal(p)
	return Button{Action: ButtonAction{Type: "text", Label: label, Payload: string(data)}, Color: color}
}

// plainButton — кнопка без payload: нажатие отправляет её текст (варианты ответа).
func plainButton(label string) Button {
	return Button{Action: ButtonAction{Type: "text", Label: label}, Color: "secondary"}
}

// locationButton — «Отправить своё местоположение»; занимает весь ряд.
func locationButton() Button {
	return Button{Action: ButtonAction{Type: "location"}}
}

// payload — команда кнопки.
type payload struct {
	Cmd   string `json:"cmd"`
	Level string `json:"level,omitempty"`
	Idx   int    `json:"idx,omitempty"`
	Run   string `json:"run,omitempty"`
	Team  string `json:"team,omitempty"`
	// Command — стандартная кнопка «Начать» VK присылает {"command":"start"}.
	Command string `json:"command,omitempty"`
}

func parsePayload(s string) payload {
	var p payload
	if s != "" {
		_ = json.Unmarshal([]byte(s), &p)
	}
	if p.Cmd == "" && p.Command == "start" {
		p.Cmd = cmdStart
	}
	return p
}
