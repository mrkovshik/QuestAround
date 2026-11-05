package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"questline/internal/httpapi"
	"questline/internal/runner"
	"questline/internal/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", env("QUESTLINE_ADDR", ":8080"), "адрес HTTP-сервера")
	dsn := fs.String("db", env("QUESTLINE_DATABASE_URL", ""), "Postgres DSN; пусто — хранение в памяти (только для разработки)")
	mediaDir := fs.String("media", env("QUESTLINE_MEDIA_DIR", "./data/media"), "каталог для фото")
	adminToken := fs.String("admin-token", env("QUESTLINE_ADMIN_TOKEN", ""), "токен ведущего (обязателен)")
	debug := fs.Bool("debug", os.Getenv("QUESTLINE_DEBUG") != "", "подробные логи")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*adminToken) < 16 {
		return errors.New("задайте QUESTLINE_ADMIN_TOKEN (не короче 16 символов)")
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if *dsn == "" {
		log.Warn("QUESTLINE_DATABASE_URL не задан — данные хранятся в памяти и пропадут при перезапуске")
		st = store.NewMemory()
	} else {
		pg, err := store.OpenPostgres(ctx, *dsn)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		defer pg.Close()
		st = pg
	}
	if err := os.MkdirAll(*mediaDir, 0o755); err != nil {
		return err
	}

	runnersCtx, stopRunners := context.WithCancel(context.Background())
	defer stopRunners()
	api := httpapi.New(httpapi.Config{
		Store:      st,
		Runners:    runner.NewManager(runnersCtx, st, runner.RealClock{}, log),
		Media:      httpapi.DiskMedia{Dir: *mediaDir},
		AdminToken: *adminToken,
		Logger:     log,
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second, // загрузка фото с плохой связью
		WriteTimeout:      30 * time.Second, // SSE снимает дедлайн сам
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("сервер запущен", "addr", *addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("остановка…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stopRunners() // закрывает SSE-подписки, чтобы Shutdown не ждал их
	return srv.Shutdown(shutdownCtx)
}
