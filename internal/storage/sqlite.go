package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"goshort/internal/shortener"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const schema = `
CREATE TABLE IF NOT EXISTS links (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	code         TEXT    NOT NULL UNIQUE,
	original_url TEXT    NOT NULL,
	created_at   TEXT    NOT NULL,
	expires_at   TEXT,
	clicks       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_links_url ON links(original_url);

CREATE TABLE IF NOT EXISTS clicks (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
	clicked_at TEXT    NOT NULL,
	referer    TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_clicks_link ON clicks(link_id, clicked_at);
`

const timeLayout = time.RFC3339

type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) Create(ctx context.Context, link *shortener.Link) error {
	var expires any
	if link.ExpiresAt != nil {
		expires = link.ExpiresAt.UTC().Format(timeLayout)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (code, original_url, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		link.Code, link.OriginalURL, link.CreatedAt.UTC().Format(timeLayout), expires)
	if err != nil {
		var se *sqlite.Error
		if errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return shortener.ErrCodeExists
		}
		return err
	}
	link.ID, err = res.LastInsertId()
	return err
}

func (s *SQLite) GetByCode(ctx context.Context, code string) (shortener.Link, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, code, original_url, created_at, expires_at, clicks FROM links WHERE code = ?`, code)
	return scanLink(row)
}

func (s *SQLite) FindPermanentByURL(ctx context.Context, url string) (shortener.Link, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, code, original_url, created_at, expires_at, clicks FROM links
		 WHERE original_url = ? AND expires_at IS NULL
		 ORDER BY id LIMIT 1`, url)
	return scanLink(row)
}

func (s *SQLite) RegisterClick(ctx context.Context, c shortener.Click) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO clicks (link_id, clicked_at, referer) VALUES (?, ?, ?)`,
		c.LinkID, c.ClickedAt.UTC().Format(timeLayout), c.Referer); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE links SET clicks = clicks + 1 WHERE id = ?`, c.LinkID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) ClicksByDay(ctx context.Context, linkID int64, since time.Time) ([]shortener.DayStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT substr(clicked_at, 1, 10) AS day, COUNT(*) FROM clicks
		 WHERE link_id = ? AND clicked_at >= ?
		 GROUP BY day ORDER BY day`,
		linkID, since.UTC().Format(timeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []shortener.DayStat
	for rows.Next() {
		var day string
		var st shortener.DayStat
		if err := rows.Scan(&day, &st.Count); err != nil {
			return nil, err
		}
		if st.Day, err = time.Parse(time.DateOnly, day); err != nil {
			return nil, err
		}
		res = append(res, st)
	}
	return res, rows.Err()
}

func (s *SQLite) TopSources(ctx context.Context, linkID int64, limit int) ([]shortener.SourceStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT referer, COUNT(*) AS cnt FROM clicks
		 WHERE link_id = ?
		 GROUP BY referer ORDER BY cnt DESC, referer LIMIT ?`, linkID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []shortener.SourceStat
	for rows.Next() {
		var st shortener.SourceStat
		if err := rows.Scan(&st.Source, &st.Count); err != nil {
			return nil, err
		}
		res = append(res, st)
	}
	return res, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLink(row scanner) (shortener.Link, error) {
	var (
		l       shortener.Link
		created string
		expires sql.NullString
	)
	err := row.Scan(&l.ID, &l.Code, &l.OriginalURL, &created, &expires, &l.Clicks)
	if errors.Is(err, sql.ErrNoRows) {
		return shortener.Link{}, shortener.ErrNotFound
	}
	if err != nil {
		return shortener.Link{}, err
	}
	if l.CreatedAt, err = time.Parse(timeLayout, created); err != nil {
		return shortener.Link{}, err
	}
	if expires.Valid {
		t, err := time.Parse(timeLayout, expires.String)
		if err != nil {
			return shortener.Link{}, err
		}
		l.ExpiresAt = &t
	}
	return l, nil
}
