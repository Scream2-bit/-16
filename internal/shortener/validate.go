package shortener

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

const MaxURLLength = 2048

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

var reserved = map[string]bool{
	"api":    true,
	"static": true,
	"stats":  true,
	"health": true,
	"admin":  true,
}

func NormalizeURL(raw, selfHost string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrEmptyURL
	}
	if len(raw) > MaxURLLength {
		return "", ErrURLTooLong
	}
	if !strings.Contains(raw, "://") {
		if i := strings.Index(raw, ":"); i > 0 && !strings.ContainsAny(raw[:i], "./") && !isPort(raw[i+1:]) {
			return "", ErrUnsupportedScheme
		}
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidURL
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrUnsupportedScheme
	}

	host := strings.ToLower(u.Hostname())
	if host == "" || u.User != nil {
		return "", ErrInvalidURL
	}
	if net.ParseIP(host) == nil && !validDomain(host) {
		return "", ErrInvalidURL
	}
	u.Host = strings.ToLower(u.Host)
	if selfHost != "" && strings.EqualFold(u.Host, selfHost) {
		return "", ErrSelfLink
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func validDomain(host string) bool {
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" || len(part) > 63 || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return false
		}
	}
	return true
}

func isPort(s string) bool {
	end := strings.IndexAny(s, "/?#")
	if end >= 0 {
		s = s[:end]
	}
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func ValidateAlias(alias string) error {
	if !aliasRe.MatchString(alias) {
		return ErrInvalidAlias
	}
	if reserved[strings.ToLower(alias)] {
		return ErrReservedAlias
	}
	return nil
}
