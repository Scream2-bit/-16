package httpapi

import (
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"goshort/internal/shortener"
	"goshort/web"
)

const maxBodySize = 1 << 20

type Handler struct {
	svc     *shortener.Service
	baseURL string
	tmpl    *template.Template
	log     *slog.Logger
}

type shortenRequest struct {
	URL      string `json:"url"`
	Alias    string `json:"alias"`
	TTLHours int    `json:"ttl_hours"`
}

type linkResponse struct {
	Code        string     `json:"code"`
	ShortURL    string     `json:"short_url"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	Clicks      int64      `json:"clicks"`
}

type statsResponse struct {
	linkResponse
	ByDay   []dayItem    `json:"by_day"`
	Sources []sourceItem `json:"sources"`
}

type dayItem struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type sourceItem struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(svc *shortener.Service, baseURL string, log *slog.Logger) (http.Handler, error) {
	tmpl, err := template.New("").Funcs(funcs).ParseFS(web.Templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}

	h := &Handler{svc: svc, baseURL: baseURL, tmpl: tmpl, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /api/shorten", h.apiShorten)
	mux.HandleFunc("GET /api/links/{code}", h.apiStats)
	mux.HandleFunc("GET /stats/{code}", h.statsPage)
	mux.HandleFunc("GET /{code}", h.redirect)

	return recoverer(log, logging(log, mux)), nil
}

func (h *Handler) index(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "index.html", map[string]any{"BaseURL": h.baseURL})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) apiShorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req shortenRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"некорректное тело запроса"})
		return
	}

	link, err := h.svc.Shorten(r.Context(), shortener.ShortenRequest{
		URL:      req.URL,
		Alias:    req.Alias,
		TTLHours: req.TTLHours,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.toResponse(link))
}

func (h *Handler) apiStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats(r.Context(), r.PathValue("code"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	resp := statsResponse{linkResponse: h.toResponse(st.Link)}
	for _, d := range st.ByDay {
		resp.ByDay = append(resp.ByDay, dayItem{Date: d.Day.Format(time.DateOnly), Count: d.Count})
	}
	resp.Sources = []sourceItem{}
	for _, s := range st.Sources {
		resp.Sources = append(resp.Sources, sourceItem{Source: sourceName(s.Source), Count: s.Count})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) statsPage(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Stats(r.Context(), r.PathValue("code"))
	if err != nil {
		h.errorPage(w, err)
		return
	}
	var maxDay int64
	for _, d := range st.ByDay {
		maxDay = max(maxDay, d.Count)
	}
	h.render(w, http.StatusOK, "stats.html", map[string]any{
		"Link":     st.Link,
		"ShortURL": h.baseURL + "/" + st.Link.Code,
		"ByDay":    st.ByDay,
		"MaxDay":   maxDay,
		"Sources":  st.Sources,
		"Expired":  st.Link.Expired(time.Now()),
	})
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request) {
	target, err := h.svc.Resolve(r.Context(), r.PathValue("code"), r.Referer(), r.UserAgent())
	if err != nil {
		h.errorPage(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) toResponse(l shortener.Link) linkResponse {
	return linkResponse{
		Code:        l.Code,
		ShortURL:    h.baseURL + "/" + l.Code,
		OriginalURL: l.OriginalURL,
		CreatedAt:   l.CreatedAt,
		ExpiresAt:   l.ExpiresAt,
		Clicks:      l.Clicks,
	}
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	status := statusOf(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		h.log.Error("request failed", "err", err)
		msg = "внутренняя ошибка сервера"
	}
	writeJSON(w, status, errorResponse{msg})
}

func (h *Handler) errorPage(w http.ResponseWriter, err error) {
	status := statusOf(err)
	title := "Ссылка не найдена"
	text := "Возможно, в адресе допущена опечатка или ссылка была удалена."
	switch status {
	case http.StatusGone:
		title = "Срок действия ссылки истёк"
		text = "Автор ограничил время жизни этой ссылки, и оно закончилось."
	case http.StatusInternalServerError:
		h.log.Error("request failed", "err", err)
		title = "Что-то пошло не так"
		text = "Попробуйте обновить страницу немного позже."
	}
	h.render(w, status, "error.html", map[string]any{"Status": status, "Title": title, "Text": text})
}

func (h *Handler) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.log.Error("render template", "name", name, "err", err)
	}
}

func statusOf(err error) int {
	switch {
	case errors.Is(err, shortener.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, shortener.ErrExpired):
		return http.StatusGone
	case errors.Is(err, shortener.ErrAliasTaken):
		return http.StatusConflict
	case errors.Is(err, shortener.ErrEmptyURL),
		errors.Is(err, shortener.ErrURLTooLong),
		errors.Is(err, shortener.ErrInvalidURL),
		errors.Is(err, shortener.ErrUnsupportedScheme),
		errors.Is(err, shortener.ErrSelfLink),
		errors.Is(err, shortener.ErrInvalidAlias),
		errors.Is(err, shortener.ErrReservedAlias),
		errors.Is(err, shortener.ErrInvalidTTL):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func sourceName(s string) string {
	if s == "" {
		return "Прямой переход"
	}
	return s
}
