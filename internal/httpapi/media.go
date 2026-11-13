package httpapi

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// MediaStore — хранилище фото игроков и библиотека картинок для сценариев.
//
// Ключи неугадываемы (случайные или хеш содержимого), поэтому файлы отдаются
// по прямой ссылке /m/{key} без авторизации: ссылку получают только те, кому её показали.
type MediaStore interface {
	// Save сохраняет изображение и возвращает ключ. Не изображение — ошибка.
	Save(ctx context.Context, runID string, r io.Reader) (key string, err error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, string, error)

	// PutAsset кладёт картинку в библиотеку под именем (то же имя — замена).
	PutAsset(ctx context.Context, name string, r io.Reader) (Asset, error)
	Assets(ctx context.Context) ([]Asset, error)
	DeleteAsset(ctx context.Context, name string) error
	// AssetKey — ключ файла для имени из сценария.
	AssetKey(name string) (string, bool)
	// DeleteRun стирает все фото игры.
	DeleteRun(ctx context.Context, runID string) error
}

// Asset — картинка в библиотеке сценариев.
type Asset struct {
	Name string    `json:"name"`
	Key  string    `json:"key"`
	Size int64     `json:"size"`
	At   time.Time `json:"at"`
}

// mediaURL — публичная ссылка на файл.
func mediaURL(key string) string {
	if key == "" {
		return ""
	}
	return "/m/" + key
}

var errNotImage = newErr(http.StatusUnsupportedMediaType, "not_image", "нужно изображение JPEG, PNG, WebP или GIF")

var imageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// DiskMedia хранит файлы в каталоге: <dir>/<runID>/<id>.<ext>;
// библиотека — <dir>/assets/<sha256>.<ext> и <dir>/assets/index.json (имя → файл).
type DiskMedia struct {
	Dir string

	mu     sync.Mutex
	index  map[string]Asset // nil — ещё не загружен
	loaded bool
}

var keyRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+\.(jpg|png|webp|gif)$`)

func (d *DiskMedia) Save(_ context.Context, runID string, r io.Reader) (string, error) {
	br := bufio.NewReaderSize(r, 512)
	head, err := br.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	ext, ok := imageExt[http.DetectContentType(head)]
	if !ok {
		return "", errNotImage
	}
	key := runID + "/" + newID() + ext
	if !keyRe.MatchString(key) {
		return "", fmt.Errorf("некорректный ключ медиа %q", key)
	}
	path := filepath.Join(d.Dir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op после успешного Rename
	if _, err := io.Copy(tmp, br); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return key, os.Rename(tmp.Name(), path)
}

func (d *DiskMedia) Open(_ context.Context, key string) (io.ReadSeekCloser, string, error) {
	if !keyRe.MatchString(key) {
		return nil, "", errNotFoundMedia
	}
	f, err := os.Open(filepath.Join(d.Dir, filepath.FromSlash(key)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", errNotFoundMedia
	}
	if err != nil {
		return nil, "", err
	}
	ctype := "application/octet-stream"
	for ct, ext := range imageExt {
		if filepath.Ext(key) == ext {
			ctype = ct
		}
	}
	return f, ctype, nil
}

var errNotFoundMedia = newErr(http.StatusNotFound, "not_found", "файл не найден")

// --- библиотека картинок ---

const maxAssetSize = 10 << 20

func (d *DiskMedia) indexPath() string { return filepath.Join(d.Dir, "assets", "index.json") }

// loadLocked читает индекс с диска один раз.
func (d *DiskMedia) loadLocked() error {
	if d.loaded {
		return nil
	}
	d.index = map[string]Asset{}
	data, err := os.ReadFile(d.indexPath())
	if errors.Is(err, os.ErrNotExist) {
		d.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &d.index); err != nil {
		return fmt.Errorf("assets/index.json: %w", err)
	}
	d.loaded = true
	return nil
}

func (d *DiskMedia) saveIndexLocked() error {
	data, err := json.MarshalIndent(d.index, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.indexPath())
}

func (d *DiskMedia) PutAsset(_ context.Context, name string, r io.Reader) (Asset, error) {
	br := bufio.NewReaderSize(r, 512)
	head, err := br.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return Asset{}, err
	}
	ext, ok := imageExt[http.DetectContentType(head)]
	if !ok {
		return Asset{}, errNotImage
	}
	dir := filepath.Join(d.Dir, "assets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Asset{}, err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return Asset{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(br, maxAssetSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Asset{}, err
	}
	if n > maxAssetSize {
		return Asset{}, newErr(http.StatusRequestEntityTooLarge, "too_large", "картинка больше 10 МБ")
	}
	key := "assets/" + hex.EncodeToString(h.Sum(nil))[:24] + ext
	if err := os.Rename(tmp.Name(), filepath.Join(d.Dir, filepath.FromSlash(key))); err != nil {
		return Asset{}, err
	}
	a := Asset{Name: name, Key: key, Size: n, At: time.Now().UTC().Truncate(time.Second)}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.loadLocked(); err != nil {
		return Asset{}, err
	}
	d.index[name] = a
	return a, d.saveIndexLocked()
}

func (d *DiskMedia) Assets(context.Context) ([]Asset, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.loadLocked(); err != nil {
		return nil, err
	}
	out := make([]Asset, 0, len(d.index))
	for _, a := range d.index {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b Asset) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// DeleteAsset убирает имя из библиотеки. Сам файл остаётся: на него могут ссылаться
// уже открытые у игроков экраны, а места он занимает немного.
func (d *DiskMedia) DeleteAsset(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.loadLocked(); err != nil {
		return err
	}
	if _, ok := d.index[name]; !ok {
		return errNotFoundMedia
	}
	delete(d.index, name)
	return d.saveIndexLocked()
}

func (d *DiskMedia) AssetKey(name string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.loadLocked() != nil {
		return "", false
	}
	a, ok := d.index[name]
	return a.Key, ok
}

func (d *DiskMedia) DeleteRun(_ context.Context, runID string) error {
	if !idRe.MatchString(runID) || runID == "assets" {
		return fmt.Errorf("плохой id игры %q", runID)
	}
	return os.RemoveAll(filepath.Join(d.Dir, runID))
}

var idRe = regexp.MustCompile(`^[a-z0-9]+$`)
