package config

import (
	"flag"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr       string
	BaseURL    string
	DBPath     string
	CodeLength int
}

func Load() Config {
	cfg := Config{
		Addr:       env("GOSHORT_ADDR", ":8080"),
		BaseURL:    env("GOSHORT_BASE_URL", "http://localhost:8080"),
		DBPath:     env("GOSHORT_DB", "goshort.db"),
		CodeLength: 6,
	}
	if v, err := strconv.Atoi(os.Getenv("GOSHORT_CODE_LEN")); err == nil {
		cfg.CodeLength = v
	}

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "адрес HTTP-сервера")
	flag.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "внешний адрес сервиса")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "путь к файлу базы данных")
	flag.IntVar(&cfg.CodeLength, "code-len", cfg.CodeLength, "длина короткого кода (4-12)")
	flag.Parse()

	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.CodeLength < 4 || cfg.CodeLength > 12 {
		cfg.CodeLength = 6
	}
	return cfg
}

func (c Config) Host() string {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
