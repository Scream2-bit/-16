package shortener_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"goshort/internal/shortener"
	"goshort/internal/storage"
)

const chrome = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/129.0 Safari/537.36"

type clock struct {
	t time.Time
}

func (c *clock) Now() time.Time { return c.t }

func newService(t *testing.T) (*shortener.Service, *clock) {
	t.Helper()
	db, err := storage.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	c := &clock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	svc := shortener.NewService(db, shortener.Options{SelfHost: "sho.rt", Now: c.Now})
	return svc, c
}

func TestShortenAndResolve(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	link, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "go.dev/doc/effective_go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(link.Code) != 6 {
		t.Fatalf("длина кода %d, ожидалось 6", len(link.Code))
	}

	target, err := svc.Resolve(ctx, link.Code, "", chrome)
	if err != nil {
		t.Fatal(err)
	}
	if target != "https://go.dev/doc/effective_go" {
		t.Fatalf("переход на %q", target)
	}
}

func TestShortenSameURLReturnsSameCode(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	a, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://example.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://EXAMPLE.com/a"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != b.Code {
		t.Fatalf("для одинаковой ссылки получены разные коды: %s и %s", a.Code, b.Code)
	}
}

func TestShortenWithAlias(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	link, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://example.com", Alias: "promo"})
	if err != nil {
		t.Fatal(err)
	}
	if link.Code != "promo" {
		t.Fatalf("код %q, ожидался promo", link.Code)
	}

	_, err = svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://example.org", Alias: "promo"})
	if !errors.Is(err, shortener.ErrAliasTaken) {
		t.Fatalf("ошибка %v, ожидалась ErrAliasTaken", err)
	}
}

func TestShortenInvalidInput(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	cases := []struct {
		req  shortener.ShortenRequest
		want error
	}{
		{shortener.ShortenRequest{URL: ""}, shortener.ErrEmptyURL},
		{shortener.ShortenRequest{URL: "ftp://example.com"}, shortener.ErrUnsupportedScheme},
		{shortener.ShortenRequest{URL: "https://sho.rt/abc"}, shortener.ErrSelfLink},
		{shortener.ShortenRequest{URL: "https://example.com", Alias: "a b"}, shortener.ErrInvalidAlias},
		{shortener.ShortenRequest{URL: "https://example.com", Alias: "api"}, shortener.ErrReservedAlias},
		{shortener.ShortenRequest{URL: "https://example.com", TTLHours: -1}, shortener.ErrInvalidTTL},
		{shortener.ShortenRequest{URL: "https://example.com", TTLHours: 9000}, shortener.ErrInvalidTTL},
	}
	for _, c := range cases {
		if _, err := svc.Shorten(ctx, c.req); !errors.Is(err, c.want) {
			t.Errorf("%+v: ошибка %v, ожидалась %v", c.req, err, c.want)
		}
	}
}

func TestResolveNotFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Resolve(context.Background(), "nothing", "", chrome)
	if !errors.Is(err, shortener.ErrNotFound) {
		t.Fatalf("ошибка %v, ожидалась ErrNotFound", err)
	}
}

func TestLinkExpires(t *testing.T) {
	svc, c := newService(t)
	ctx := context.Background()

	link, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://example.com", TTLHours: 1})
	if err != nil {
		t.Fatal(err)
	}

	c.t = c.t.Add(59 * time.Minute)
	if _, err := svc.Resolve(ctx, link.Code, "", chrome); err != nil {
		t.Fatalf("ссылка должна работать до истечения срока: %v", err)
	}

	c.t = c.t.Add(2 * time.Minute)
	if _, err := svc.Resolve(ctx, link.Code, "", chrome); !errors.Is(err, shortener.ErrExpired) {
		t.Fatalf("ошибка %v, ожидалась ErrExpired", err)
	}
}

func TestStats(t *testing.T) {
	svc, c := newService(t)
	ctx := context.Background()

	link, err := svc.Shorten(ctx, shortener.ShortenRequest{URL: "https://example.com/stats"})
	if err != nil {
		t.Fatal(err)
	}

	svc.Resolve(ctx, link.Code, "https://www.vk.com/feed", chrome)
	svc.Resolve(ctx, link.Code, "https://vk.com/im", chrome)
	svc.Resolve(ctx, link.Code, "", chrome)
	svc.Resolve(ctx, link.Code, "", "TelegramBot (like TwitterBot)")
	c.t = c.t.Add(24 * time.Hour)
	svc.Resolve(ctx, link.Code, "https://t.me/channel", chrome)

	st, err := svc.Stats(ctx, link.Code)
	if err != nil {
		t.Fatal(err)
	}
	if st.Link.Clicks != 4 {
		t.Fatalf("переходов %d, ожидалось 4 (бот не учитывается)", st.Link.Clicks)
	}
	if len(st.ByDay) != 7 {
		t.Fatalf("дней в статистике %d, ожидалось 7", len(st.ByDay))
	}
	if got := st.ByDay[5].Count; got != 3 {
		t.Errorf("переходов за 15.10: %d, ожидалось 3", got)
	}
	if got := st.ByDay[6].Count; got != 1 {
		t.Errorf("переходов за 16.10: %d, ожидалось 1", got)
	}
	if len(st.Sources) == 0 || st.Sources[0].Source != "vk.com" || st.Sources[0].Count != 2 {
		t.Errorf("неверный список источников: %+v", st.Sources)
	}
}
