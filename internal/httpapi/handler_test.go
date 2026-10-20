package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"goshort/internal/shortener"
	"goshort/internal/storage"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := storage.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	svc := shortener.NewService(db, shortener.Options{SelfHost: "sho.rt"})
	h, err := New(svc, "http://sho.rt", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func shorten(t *testing.T, srv *httptest.Server, body string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/shorten", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var data map[string]any
	json.NewDecoder(resp.Body).Decode(&data)
	return resp, data
}

func TestAPIShorten(t *testing.T) {
	srv := newServer(t)

	resp, data := shorten(t, srv, `{"url":"https://go.dev/blog","alias":"goblog"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("статус %d, ожидался 201", resp.StatusCode)
	}
	if data["short_url"] != "http://sho.rt/goblog" {
		t.Fatalf("short_url = %v", data["short_url"])
	}
}

func TestAPIShortenErrors(t *testing.T) {
	srv := newServer(t)

	cases := []struct {
		body   string
		status int
	}{
		{`{"url":""}`, http.StatusBadRequest},
		{`{"url":"ftp://example.com"}`, http.StatusBadRequest},
		{`{"url":"https://example.com","ttl_hours":-5}`, http.StatusBadRequest},
		{`не json`, http.StatusBadRequest},
		{`{"url":"https://example.com","unknown":1}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		resp, data := shorten(t, srv, c.body)
		if resp.StatusCode != c.status {
			t.Errorf("%s: статус %d, ожидался %d", c.body, resp.StatusCode, c.status)
		}
		if data["error"] == "" || data["error"] == nil {
			t.Errorf("%s: нет текста ошибки", c.body)
		}
	}

	shorten(t, srv, `{"url":"https://example.com","alias":"taken"}`)
	resp, _ := shorten(t, srv, `{"url":"https://example.org","alias":"taken"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("повторный псевдоним: статус %d, ожидался 409", resp.StatusCode)
	}
}

func TestRedirect(t *testing.T) {
	srv := newServer(t)
	shorten(t, srv, `{"url":"https://example.com/target","alias":"jump"}`)

	resp, err := noRedirect().Get(srv.URL + "/jump")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("статус %d, ожидался 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://example.com/target" {
		t.Fatalf("Location = %q", loc)
	}

	resp, err = noRedirect().Get(srv.URL + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("статус %d, ожидался 404", resp.StatusCode)
	}
}

func TestStatsEndpoints(t *testing.T) {
	srv := newServer(t)
	shorten(t, srv, `{"url":"https://example.com/","alias":"stat"}`)

	for range 3 {
		resp, err := noRedirect().Get(srv.URL + "/stat")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	resp, err := http.Get(srv.URL + "/api/links/stat")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var data struct {
		Clicks int64 `json:"clicks"`
		ByDay  []struct {
			Count int64 `json:"count"`
		} `json:"by_day"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Clicks != 3 || len(data.ByDay) != 7 {
		t.Fatalf("clicks=%d, дней=%d", data.Clicks, len(data.ByDay))
	}

	page, err := http.Get(srv.URL + "/stats/stat")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK || !strings.Contains(string(body), "переходов всего") {
		t.Fatalf("страница статистики: статус %d", page.StatusCode)
	}
}

func TestIndexPage(t *testing.T) {
	srv := newServer(t)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("статус %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
}
