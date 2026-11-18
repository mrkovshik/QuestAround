package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVK(t *testing.T) {
	forEachStore(t, func(t *testing.T, s Store) {
		ctx := context.Background()
		vs, ok := s.(VKStore)
		if !ok {
			t.Fatal("хранилище не реализует VKStore")
		}
		r1, r2 := newRun(t, s), newRun(t, s)
		user := time.Now().UnixNano() // уникален между запусками на одной базе

		if _, err := vs.GetVK(ctx, user); !errors.Is(err, ErrNotFound) {
			t.Fatalf("до привязки: %v", err)
		}
		if err := vs.BindVK(ctx, VKBinding{UserID: user, RunID: "нет-такой", PlayerID: "p"}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("привязка к несуществующей игре: %v", err)
		}
		if err := vs.BindVK(ctx, VKBinding{UserID: user, RunID: r1.ID, PlayerID: "p1"}); err != nil {
			t.Fatal(err)
		}
		// Новая привязка заменяет старую.
		if err := vs.BindVK(ctx, VKBinding{UserID: user, RunID: r2.ID, PlayerID: "p2"}); err != nil {
			t.Fatal(err)
		}
		b, err := vs.GetVK(ctx, user)
		if err != nil || b.RunID != r2.ID || b.PlayerID != "p2" {
			t.Fatalf("GetVK: %+v, %v", b, err)
		}
		list, _ := vs.ListVK(ctx)
		n := 0
		for _, x := range list {
			if x.UserID == user {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("в списке %d привязок пользователя, ожидалась 1", n)
		}

		// Игра в корзине — привязка не видна.
		del := at
		if err := s.SetRunDeleted(ctx, r2.ID, &del); err != nil {
			t.Fatal(err)
		}
		if _, err := vs.GetVK(ctx, user); !errors.Is(err, ErrNotFound) {
			t.Fatalf("игра в корзине: %v", err)
		}
		if err := s.SetRunDeleted(ctx, r2.ID, nil); err != nil {
			t.Fatal(err)
		}

		// Курсор.
		if _, ok, err := vs.VKCursor(ctx, r2.ID); ok || err != nil {
			t.Fatalf("курсор до записи: %v %v", ok, err)
		}
		if err := vs.SetVKCursor(ctx, r2.ID, 5); err != nil {
			t.Fatal(err)
		}
		if err := vs.SetVKCursor(ctx, r2.ID, 7); err != nil {
			t.Fatal(err)
		}
		if seq, ok, err := vs.VKCursor(ctx, r2.ID); seq != 7 || !ok || err != nil {
			t.Fatalf("курсор: %d %v %v", seq, ok, err)
		}

		// Очистка игры стирает привязки и курсор.
		if err := s.PurgeRun(ctx, r2.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := vs.GetVK(ctx, user); !errors.Is(err, ErrNotFound) {
			t.Fatalf("после очистки игры: %v", err)
		}
		if _, ok, _ := vs.VKCursor(ctx, r2.ID); ok {
			t.Fatal("курсор пережил очистку игры")
		}

		if err := vs.BindVK(ctx, VKBinding{UserID: user, RunID: r1.ID, PlayerID: "p1"}); err != nil {
			t.Fatal(err)
		}
		if err := vs.UnbindVK(ctx, user); err != nil {
			t.Fatal(err)
		}
		if _, err := vs.GetVK(ctx, user); !errors.Is(err, ErrNotFound) {
			t.Fatalf("после отвязки: %v", err)
		}
	})
}
