package shortener

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	defaultCodeLength = 6
	maxAttempts       = 5
	maxTTLHours       = 24 * 365
	statsDays         = 7
	topSources        = 5
)

var botMarkers = []string{
	"bot", "crawler", "spider", "preview", "facebookexternalhit",
	"whatsapp", "telegram", "slack", "discord", "vkshare",
}

type ShortenRequest struct {
	URL      string
	Alias    string
	TTLHours int
}

type Options struct {
	CodeLength int
	SelfHost   string
	Now        func() time.Time
}

type Service struct {
	repo     Repository
	codeLen  int
	selfHost string
	now      func() time.Time
}

func NewService(repo Repository, opts Options) *Service {
	s := &Service{
		repo:     repo,
		codeLen:  opts.CodeLength,
		selfHost: opts.SelfHost,
		now:      opts.Now,
	}
	if s.codeLen <= 0 {
		s.codeLen = defaultCodeLength
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func (s *Service) Shorten(ctx context.Context, req ShortenRequest) (Link, error) {
	target, err := NormalizeURL(req.URL, s.selfHost)
	if err != nil {
		return Link{}, err
	}
	if req.TTLHours < 0 || req.TTLHours > maxTTLHours {
		return Link{}, ErrInvalidTTL
	}

	now := s.now().UTC().Truncate(time.Second)
	link := Link{OriginalURL: target, CreatedAt: now}
	if req.TTLHours > 0 {
		exp := now.Add(time.Duration(req.TTLHours) * time.Hour)
		link.ExpiresAt = &exp
	}

	alias := strings.TrimSpace(req.Alias)
	if alias != "" {
		if err := ValidateAlias(alias); err != nil {
			return Link{}, err
		}
		link.Code = alias
		if err := s.repo.Create(ctx, &link); err != nil {
			if errors.Is(err, ErrCodeExists) {
				return Link{}, ErrAliasTaken
			}
			return Link{}, err
		}
		return link, nil
	}

	if link.ExpiresAt == nil {
		existing, err := s.repo.FindPermanentByURL(ctx, target)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Link{}, err
		}
	}

	for range maxAttempts {
		code, err := GenerateCode(s.codeLen)
		if err != nil {
			return Link{}, err
		}
		link.Code = code
		err = s.repo.Create(ctx, &link)
		if err == nil {
			return link, nil
		}
		if !errors.Is(err, ErrCodeExists) {
			return Link{}, err
		}
	}
	return Link{}, ErrCodeGeneration
}

func (s *Service) Resolve(ctx context.Context, code, referer, userAgent string) (string, error) {
	link, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if link.Expired(now) {
		return "", ErrExpired
	}
	if !IsBot(userAgent) {
		click := Click{LinkID: link.ID, ClickedAt: now, Referer: refererHost(referer)}
		if err := s.repo.RegisterClick(ctx, click); err != nil {
			return "", err
		}
	}
	return link.OriginalURL, nil
}

func (s *Service) Stats(ctx context.Context, code string) (Stats, error) {
	link, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return Stats{}, err
	}

	today := s.now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, -(statsDays - 1))
	rows, err := s.repo.ClicksByDay(ctx, link.ID, since)
	if err != nil {
		return Stats{}, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Day.Format(time.DateOnly)] = r.Count
	}
	days := make([]DayStat, 0, statsDays)
	for d := since; !d.After(today); d = d.AddDate(0, 0, 1) {
		days = append(days, DayStat{Day: d, Count: counts[d.Format(time.DateOnly)]})
	}

	sources, err := s.repo.TopSources(ctx, link.ID, topSources)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Link: link, ByDay: days, Sources: sources}, nil
}

func IsBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	for _, m := range botMarkers {
		if strings.Contains(ua, m) {
			return true
		}
	}
	return false
}

func refererHost(referer string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
