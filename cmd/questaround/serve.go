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
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // часовые пояса без tzdata в образе (TZ=Europe/Moscow)

	"questaround/internal/httpapi"
	"questaround/internal/runner"
	"questaround/internal/store"
	"questaround/internal/vkbot"
	"questaround/internal/webui"
)

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", env("QUESTAROUND_ADDR", ":8080"), "адрес HTTP-сервера")
	dsn := fs.String("db", env("QUESTAROUND_DATABASE_URL", ""), "Postgres DSN; пусто — хранение в памяти (только для разработки)")
	mediaDir := fs.String("media", env("QUESTAROUND_MEDIA_DIR", "./data/media"), "каталог для фото")
	adminToken := fs.String("admin-token", env("QUESTAROUND_ADMIN_TOKEN", ""), "токен ведущего (обязателен)")
	trashDays := fs.Int("trash-days", envInt("QUESTAROUND_TRASH_DAYS", 30), "сколько дней удалённая игра лежит в корзине")
	debug := fs.Bool("debug", os.Getenv("QUESTAROUND_DEBUG") != "", "подробные логи")
	vkToken := fs.String("vk-token", env("QUESTAROUND_VK_TOKEN", ""), "токен сообщества VK; пусто — VK-бот выключен")
	vkGroup := fs.Int64("vk-group", int64(envInt("QUESTAROUND_VK_GROUP_ID", 0)), "ID сообщества VK")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(*adminToken) < 16 {
		return errors.New("задайте QUESTAROUND_ADMIN_TOKEN (не короче 16 символов)")
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
		log.Warn("QUESTAROUND_DATABASE_URL не задан — данные хранятся в памяти и пропадут при перезапуске")
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
	runners := runner.NewManager(runnersCtx, st, runner.RealClock{}, log)
	media := &httpapi.DiskMedia{Dir: *mediaDir}
	api := httpapi.New(httpapi.Config{
		Store:      st,
		Runners:    runners,
		Media:      media,
		AdminToken: *adminToken,
		Logger:     log,
		Static:     webui.Handler(),

		TrashRetention: time.Duration(*trashDays) * 24 * time.Hour,
	})
	go api.RunJanitor(runnersCtx)

	if *vkToken != "" {
		if *vkGroup == 0 {
			return errors.New("задайте QUESTAROUND_VK_GROUP_ID вместе с QUESTAROUND_VK_TOKEN")
		}
		bot := vkbot.New(vkbot.Config{
			API:     &vkbot.Client{Token: *vkToken, GroupID: *vkGroup},
			Store:   st,
			VK:      st.(store.VKStore),
			Runners: runners,
			Media:   media,
			Logger:  log,
		})
		go func() {
			if err := bot.Run(runnersCtx); err != nil {
				log.Error("VK-бот остановлен", "err", err)
			}
		}()
	} else {
		log.Info("VK-бот выключен: QUESTAROUND_VK_TOKEN не задан")
	}

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
