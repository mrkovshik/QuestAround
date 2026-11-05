package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// MediaStore — хранилище загруженных фото.
type MediaStore interface {
	// Save сохраняет изображение и возвращает ключ. Не изображение — ошибка.
	Save(ctx context.Context, runID string, r io.Reader) (key string, err error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, string, error)
}

var errNotImage = newErr(http.StatusUnsupportedMediaType, "not_image", "нужно изображение JPEG, PNG, WebP или GIF")

var imageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// DiskMedia хранит файлы в каталоге: <dir>/<runID>/<id>.<ext>.
type DiskMedia struct{ Dir string }

var keyRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+\.(jpg|png|webp|gif)$`)

func (d DiskMedia) Save(_ context.Context, runID string, r io.Reader) (string, error) {
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

func (d DiskMedia) Open(_ context.Context, key string) (io.ReadSeekCloser, string, error) {
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
