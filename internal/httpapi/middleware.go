package httpapi

import (
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"goshort/internal/shortener"
)

var funcs = template.FuncMap{
	"date": func(t time.Time) string {
		return t.Local().Format("02.01.2006 15:04")
	},
	"day": func(d shortener.DayStat) string {
		return d.Day.Format("02.01")
	},
	"percent": func(v, total int64) int64 {
		if total == 0 {
			return 0
		}
		return v * 100 / total
	},
	"source": sourceName,
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).Round(time.Microsecond),
		)
	})
}

func recoverer(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", "err", rec, "path", r.URL.Path)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
