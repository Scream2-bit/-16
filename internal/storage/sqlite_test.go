package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"goshort/internal/shortener"
)

func openTest(t *testing.T) *SQLite {
	t.Helper()
	db, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGet(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	exp := now.Add(24 * time.Hour)

	link := shortener.Link{Code: "abc123", OriginalURL: "https://example.com/", CreatedAt: now, ExpiresAt: &exp}
	if err := db.Create(ctx, &link); err != nil {
		t.Fatal(err)
	}
	if link.ID == 0 {
		t.Fatal("не присвоен идентификатор")
	}

	got, err := db.GetByCode(ctx, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalURL != link.OriginalURL || !got.CreatedAt.Equal(now) || got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("прочитано %+v", got)
	}
}

func TestCreateDuplicateCode(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()

	first := shortener.Link{Code: "same", OriginalURL: "https://a.com/", CreatedAt: time.Now()}
	if err := db.Create(ctx, &first); err != nil {
		t.Fatal(err)
	}
	second := shortener.Link{Code: "same", OriginalURL: "https://b.com/", CreatedAt: time.Now()}
	if err := db.Create(ctx, &second); !errors.Is(err, shortener.ErrCodeExists) {
		t.Fatalf("ошибка %v, ожидалась ErrCodeExists", err)
	}
}

func TestGetByCodeNotFound(t *testing.T) {
	db := openTest(t)
	if _, err := db.GetByCode(context.Background(), "missing"); !errors.Is(err, shortener.ErrNotFound) {
		t.Fatalf("ошибка %v, ожидалась ErrNotFound", err)
	}
}

func TestRegisterClick(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

	link := shortener.Link{Code: "clk", OriginalURL: "https://example.com/", CreatedAt: day}
	if err := db.Create(ctx, &link); err != nil {
		t.Fatal(err)
	}

	clicks := []shortener.Click{
		{LinkID: link.ID, ClickedAt: day, Referer: "vk.com"},
		{LinkID: link.ID, ClickedAt: day.Add(time.Hour), Referer: "vk.com"},
		{LinkID: link.ID, ClickedAt: day.Add(26 * time.Hour), Referer: ""},
	}
	for _, c := range clicks {
		if err := db.RegisterClick(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.GetByCode(ctx, "clk")
	if err != nil {
		t.Fatal(err)
	}
	if got.Clicks != 3 {
		t.Fatalf("счётчик %d, ожидалось 3", got.Clicks)
	}

	days, err := db.ClicksByDay(ctx, link.ID, day.Truncate(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Count != 2 || days[1].Count != 1 {
		t.Fatalf("статистика по дням: %+v", days)
	}

	src, err := db.TopSources(ctx, link.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(src) != 2 || src[0].Source != "vk.com" || src[0].Count != 2 {
		t.Fatalf("источники: %+v", src)
	}
}
