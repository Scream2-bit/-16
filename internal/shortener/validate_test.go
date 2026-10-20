package shortener

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"обычная https ссылка", "https://example.com/page?id=1", "https://example.com/page?id=1", nil},
		{"http ссылка", "http://example.com", "http://example.com/", nil},
		{"без протокола", "example.com/news", "https://example.com/news", nil},
		{"пробелы по краям", "  https://go.dev/doc  ", "https://go.dev/doc", nil},
		{"регистр хоста", "HTTPS://Example.COM/Path", "https://example.com/Path", nil},
		{"кириллический путь", "https://ru.wikipedia.org/wiki/Go", "https://ru.wikipedia.org/wiki/Go", nil},
		{"ip адрес с портом", "http://192.168.1.10:8080/admin", "http://192.168.1.10:8080/admin", nil},
		{"пустая строка", "", "", ErrEmptyURL},
		{"только пробелы", "   ", "", ErrEmptyURL},
		{"слишком длинная", "https://example.com/" + strings.Repeat("a", 2100), "", ErrURLTooLong},
		{"протокол ftp", "ftp://files.example.com/file.zip", "", ErrUnsupportedScheme},
		{"javascript", "javascript:alert(1)", "", ErrUnsupportedScheme},
		{"нет домена верхнего уровня", "https://localhost/test", "", ErrInvalidURL},
		{"пробел в домене", "https://exa mple.com", "", ErrInvalidURL},
		{"логин в ссылке", "https://user:pass@example.com", "", ErrInvalidURL},
		{"ссылка на сам сервис", "http://sho.rt/abc123", "", ErrSelfLink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.in, "sho.rt")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ошибка = %v, ожидалась %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("результат = %q, ожидался %q", got, tt.want)
			}
		})
	}
}

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		alias   string
		wantErr error
	}{
		{"my-link", nil},
		{"Promo_2026", nil},
		{"abc", nil},
		{"ab", ErrInvalidAlias},
		{strings.Repeat("x", 33), ErrInvalidAlias},
		{"ссылка", ErrInvalidAlias},
		{"with space", ErrInvalidAlias},
		{"a/b", ErrInvalidAlias},
		{"api", ErrReservedAlias},
		{"STATS", ErrReservedAlias},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			if err := ValidateAlias(tt.alias); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateAlias(%q) = %v, ожидалась %v", tt.alias, err, tt.wantErr)
			}
		})
	}
}

func TestGenerateCode(t *testing.T) {
	seen := make(map[string]bool)
	for range 10000 {
		code, err := GenerateCode(6)
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 {
			t.Fatalf("длина кода %d, ожидалось 6", len(code))
		}
		for _, c := range code {
			if !strings.ContainsRune(alphabet, c) {
				t.Fatalf("недопустимый символ %q в коде %q", c, code)
			}
		}
		if seen[code] {
			t.Fatalf("повтор кода %q", code)
		}
		seen[code] = true
	}
}

func TestIsBot(t *testing.T) {
	bots := []string{
		"TelegramBot (like TwitterBot)",
		"Mozilla/5.0 (compatible; YandexBot/3.0)",
		"WhatsApp/2.23.20.0",
		"facebookexternalhit/1.1",
	}
	for _, ua := range bots {
		if !IsBot(ua) {
			t.Errorf("%q должен определяться как бот", ua)
		}
	}
	if IsBot("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/129.0 Safari/537.36") {
		t.Error("обычный браузер определён как бот")
	}
}
