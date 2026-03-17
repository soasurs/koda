package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/soasurs/koda/internal/config"
)

type SessionMeta struct {
	SessionID int64  `db:"session_id"`
	Title     string `db:"title"`
	WorkDir   string `db:"workdir"`
	CreatedAt int64  `db:"created_at"`
	UpdatedAt int64  `db:"updated_at"`
}

type sessionCatalog interface {
	CreateSession(context.Context, SessionMeta) error
	TouchSession(context.Context, int64, string) error
	SetTitle(context.Context, int64, string) error
	ListSessions(context.Context) ([]SessionMeta, error)
	DeleteSession(context.Context, int64) error
}

func newSessionCatalog(cfg *config.Config) (sessionCatalog, error) {
	if cfg.NoSession {
		return newMemorySessionCatalog(), nil
	}

	dir := filepath.Join(os.Getenv("HOME"), ".koda")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("session catalog: create ~/.koda dir: %w", err)
	}

	dbPath := filepath.Join(dir, "sessions.db")
	db, err := sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("session catalog: open sessions.db: %w", err)
	}
	if err := migrateSessionCatalog(db); err != nil {
		return nil, fmt.Errorf("session catalog: migrate schema: %w", err)
	}
	return &sqliteSessionCatalog{db: db}, nil
}

type sqliteSessionCatalog struct {
	db *sqlx.DB
}

func (c *sqliteSessionCatalog) CreateSession(ctx context.Context, meta SessionMeta) error {
	now := time.Now().UnixMilli()
	if meta.CreatedAt == 0 {
		meta.CreatedAt = now
	}
	if meta.UpdatedAt == 0 {
		meta.UpdatedAt = meta.CreatedAt
	}
	_, err := c.db.ExecContext(
		ctx,
		`INSERT INTO koda_sessions (session_id, title, workdir, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		meta.SessionID,
		normalizeSessionTitle(meta.Title),
		strings.TrimSpace(meta.WorkDir),
		meta.CreatedAt,
		meta.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("session catalog: create session: %w", err)
	}
	return nil
}

func (c *sqliteSessionCatalog) TouchSession(ctx context.Context, sessionID int64, title string) error {
	trimmedTitle := strings.TrimSpace(title)
	_, err := c.db.ExecContext(
		ctx,
		`UPDATE koda_sessions
		 SET updated_at = ?,
		     title = CASE WHEN (trim(title) = '' OR title = 'New Session') AND ? <> '' THEN ? ELSE title END
		 WHERE session_id = ?`,
		time.Now().UnixMilli(),
		trimmedTitle,
		normalizeSessionTitle(trimmedTitle),
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("session catalog: touch session: %w", err)
	}
	return nil
}

func (c *sqliteSessionCatalog) SetTitle(ctx context.Context, sessionID int64, title string) error {
	_, err := c.db.ExecContext(ctx,
		`UPDATE koda_sessions SET title = ? WHERE session_id = ?`,
		normalizeSessionTitle(title), sessionID,
	)
	if err != nil {
		return fmt.Errorf("session catalog: set title: %w", err)
	}
	return nil
}

func (c *sqliteSessionCatalog) ListSessions(ctx context.Context) ([]SessionMeta, error) {
	var result []SessionMeta
	if err := c.db.SelectContext(ctx, &result, `SELECT session_id, title, workdir, created_at, updated_at FROM koda_sessions ORDER BY updated_at DESC, created_at DESC, session_id DESC`); err != nil {
		return nil, fmt.Errorf("session catalog: list sessions: %w", err)
	}
	return result, nil
}

func (c *sqliteSessionCatalog) DeleteSession(ctx context.Context, sessionID int64) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM koda_sessions WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("session catalog: delete session: %w", err)
	}
	return nil
}

type memorySessionCatalog struct {
	sessions map[int64]SessionMeta
}

func newMemorySessionCatalog() sessionCatalog {
	return &memorySessionCatalog{sessions: map[int64]SessionMeta{}}
}

func (c *memorySessionCatalog) CreateSession(ctx context.Context, meta SessionMeta) error {
	now := time.Now().UnixMilli()
	if meta.CreatedAt == 0 {
		meta.CreatedAt = now
	}
	if meta.UpdatedAt == 0 {
		meta.UpdatedAt = meta.CreatedAt
	}
	meta.Title = normalizeSessionTitle(meta.Title)
	meta.WorkDir = strings.TrimSpace(meta.WorkDir)
	c.sessions[meta.SessionID] = meta
	return nil
}

func (c *memorySessionCatalog) TouchSession(ctx context.Context, sessionID int64, title string) error {
	meta, ok := c.sessions[sessionID]
	if !ok {
		return nil
	}
	if (strings.TrimSpace(meta.Title) == "" || meta.Title == "New Session") && strings.TrimSpace(title) != "" {
		meta.Title = normalizeSessionTitle(title)
	}
	meta.UpdatedAt = time.Now().UnixMilli()
	c.sessions[sessionID] = meta
	return nil
}

func (c *memorySessionCatalog) SetTitle(ctx context.Context, sessionID int64, title string) error {
	meta, ok := c.sessions[sessionID]
	if !ok {
		return nil
	}
	meta.Title = normalizeSessionTitle(title)
	c.sessions[sessionID] = meta
	return nil
}

func (c *memorySessionCatalog) ListSessions(ctx context.Context) ([]SessionMeta, error) {
	result := make([]SessionMeta, 0, len(c.sessions))
	for _, meta := range c.sessions {
		result = append(result, meta)
	}
	slices.SortFunc(result, func(a, b SessionMeta) int {
		switch {
		case a.UpdatedAt > b.UpdatedAt:
			return -1
		case a.UpdatedAt < b.UpdatedAt:
			return 1
		case a.CreatedAt > b.CreatedAt:
			return -1
		case a.CreatedAt < b.CreatedAt:
			return 1
		case a.SessionID > b.SessionID:
			return -1
		case a.SessionID < b.SessionID:
			return 1
		default:
			return 0
		}
	})
	return result, nil
}

func (c *memorySessionCatalog) DeleteSession(ctx context.Context, sessionID int64) error {
	delete(c.sessions, sessionID)
	return nil
}

func migrateSessionCatalog(db *sqlx.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS koda_sessions (
			session_id INTEGER PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			workdir TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`INSERT INTO koda_sessions (session_id, title, workdir, created_at, updated_at)
		 SELECT session_id, '', '', created_at, updated_at
		 FROM sessions
		 WHERE deleted_at = 0
		   AND session_id NOT IN (SELECT session_id FROM koda_sessions)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSessionTitle(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return "New Session"
	}
	title := strings.Join(fields, " ")
	const maxTitleLen = 72
	if len(title) > maxTitleLen {
		return title[:maxTitleLen-3] + "..."
	}
	return title
}
