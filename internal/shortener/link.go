package shortener

import (
	"context"
	"errors"
	"time"
)

var (
	ErrEmptyURL          = errors.New("ссылка не указана")
	ErrURLTooLong        = errors.New("длина ссылки превышает 2048 символов")
	ErrInvalidURL        = errors.New("некорректный формат ссылки")
	ErrUnsupportedScheme = errors.New("поддерживаются только протоколы http и https")
	ErrSelfLink          = errors.New("нельзя сокращать ссылки на сам сервис")
	ErrInvalidAlias      = errors.New("псевдоним должен содержать от 3 до 32 символов: латинские буквы, цифры, «-» и «_»")
	ErrReservedAlias     = errors.New("этот псевдоним зарезервирован")
	ErrAliasTaken        = errors.New("псевдоним уже занят")
	ErrInvalidTTL        = errors.New("срок действия должен быть от 0 до 8760 часов")
	ErrNotFound          = errors.New("ссылка не найдена")
	ErrExpired           = errors.New("срок действия ссылки истёк")
	ErrCodeExists        = errors.New("код уже существует")
	ErrCodeGeneration    = errors.New("не удалось сгенерировать уникальный код")
)

type Link struct {
	ID          int64
	Code        string
	OriginalURL string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	Clicks      int64
}

func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && !now.Before(*l.ExpiresAt)
}

type Click struct {
	LinkID    int64
	ClickedAt time.Time
	Referer   string
}

type DayStat struct {
	Day   time.Time
	Count int64
}

type SourceStat struct {
	Source string
	Count  int64
}

type Stats struct {
	Link    Link
	ByDay   []DayStat
	Sources []SourceStat
}

type Repository interface {
	Create(ctx context.Context, link *Link) error
	GetByCode(ctx context.Context, code string) (Link, error)
	FindPermanentByURL(ctx context.Context, url string) (Link, error)
	RegisterClick(ctx context.Context, click Click) error
	ClicksByDay(ctx context.Context, linkID int64, since time.Time) ([]DayStat, error)
	TopSources(ctx context.Context, linkID int64, limit int) ([]SourceStat, error)
}
