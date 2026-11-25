package vkbot

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func pngBytes(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestToJPEG(t *testing.T) {
	// Прозрачное — на белом фоне.
	out, ok := toJPEG(pngBytes(t, 10, 10, color.NRGBA{0, 0, 0, 0}))
	if !ok {
		t.Fatal("не перекодировалось")
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := img.At(5, 5).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Fatalf("прозрачный пиксель не белый: %d %d %d", r>>8, g>>8, b>>8)
	}
	// Большое уменьшается, полупрозрачное смешивается с белым.
	out, ok = toJPEG(pngBytes(t, 3000, 600, color.NRGBA{255, 0, 0, 128}))
	if !ok {
		t.Fatal("не перекодировалось")
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out))
	if cfg.Width > 2560 {
		t.Fatalf("ширина %d", cfg.Width)
	}
	img, _ = jpeg.Decode(bytes.NewReader(out))
	if r, g, _, _ := img.At(10, 10).RGBA(); r>>8 < 240 || g>>8 < 110 || g>>8 > 145 {
		t.Fatalf("полупрозрачный красный на белом: r=%d g=%d", r>>8, g>>8)
	}
	if _, ok := toJPEG([]byte("не картинка")); ok {
		t.Fatal("мусор перекодировался")
	}
}

// Сервер загрузки отказывает на оригинал — уходит JPEG.
type flakyUpload struct {
	fakeAPI
	names []string
}

func (f *flakyUpload) UploadPhoto(ctx context.Context, peer int64, name string, r io.Reader) (string, error) {
	f.names = append(f.names, name)
	if strings.HasSuffix(name, ".png") {
		return "", errors.New("загрузка фото: HTTP 405")
	}
	return f.fakeAPI.UploadPhoto(ctx, peer, name, r)
}

type pngMedia struct{ data []byte }

func (m pngMedia) AssetKey(string) (string, bool)                          { return "assets/x.png", true }
func (m pngMedia) Save(context.Context, string, io.Reader) (string, error) { return "", nil }
func (m pngMedia) Open(context.Context, string) (io.ReadSeekCloser, string, error) {
	return nopCloser{bytes.NewReader(m.data)}, "image/png", nil
}

func TestPhotoFallbackToJPEG(t *testing.T) {
	api := &flakyUpload{}
	b := New(Config{API: api, Media: pngMedia{pngBytes(t, 20, 20, color.NRGBA{0, 128, 0, 255})},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	att := b.photo(context.Background(), 5, "assets/x.png")
	if att == "" {
		t.Fatal("картинка не загрузилась")
	}
	if want := []string{"x.png", "x.png", "x.jpg"}; strings.Join(api.names, ",") != strings.Join(want, ",") {
		t.Fatalf("попытки: %v", api.names)
	}
	// Второй раз — из кэша.
	if b.photo(context.Background(), 6, "assets/x.png") != att || len(api.names) != 3 {
		t.Fatal("кэш не сработал")
	}
}
