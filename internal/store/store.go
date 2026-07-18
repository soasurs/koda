// Package store owns SQLite-backed Koda session metadata and ADK history.
package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	adksession "github.com/soasurs/adk/session"
	"github.com/soasurs/adk/session/database"
)

const (
	adkTablePrefix = "adk_"
	appID          = "koda"
)

// Store owns the local database used for Koda session configuration and ADK
// conversation history. Its methods are safe for concurrent use.
type Store struct {
	db          *sqlx.DB
	adkSessions adksession.SessionService
	runLocker   *runLocker
	queries     queries
	now         func() time.Time
	closed      atomic.Bool
}

// DefaultPath returns the default local Koda database path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: find home directory: %w", err)
	}
	return filepath.Join(home, ".koda", "koda.db"), nil
}

// OpenDefault opens the Store at DefaultPath.
func OpenDefault(ctx context.Context) (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	if err := secureDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return Open(ctx, path)
}

// Open opens or creates a Store at path and applies all Koda and ADK schema
// migrations. The caller must call Close when the Store is no longer needed.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("store: path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create database directory: %w", err)
	}

	db, err := sqlx.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}
	closeOnError := func(err error) (*Store, error) {
		db.Close() //nolint:errcheck // Preserve the initialization error.
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("store: ping database: %w", err))
	}

	locker := newRunLocker()
	adkOptions := []database.Option{
		database.WithTablePrefix(adkTablePrefix),
		database.WithRunLocker(locker),
	}
	if err := database.InitSchema(ctx, db, adkOptions...); err != nil {
		return closeOnError(fmt.Errorf("store: initialize ADK schema: %w", err))
	}
	if err := initSchema(ctx, db); err != nil {
		return closeOnError(fmt.Errorf("store: initialize Koda schema: %w", err))
	}
	adkSessions, err := database.NewDatabaseSessionService(db, adkOptions...)
	if err != nil {
		return closeOnError(fmt.Errorf("store: create ADK session service: %w", err))
	}
	if err := secureDatabaseFiles(path); err != nil {
		return closeOnError(err)
	}

	return &Store{
		db:          db,
		adkSessions: adkSessions,
		runLocker:   locker,
		queries:     newQueries(adkTablePrefix),
		now:         time.Now,
	}, nil
}

// Close prevents new operations from starting and then closes the underlying
// database. Ongoing operations that started before Close will complete; the
// call is safe to retry after the first invocation.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.closed.Store(true)
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close database: %w", err)
	}
	return nil
}

func (s *Store) checkClosed() error {
	if s.closed.Load() {
		return errors.New("store is closed")
	}
	return nil
}

// ADKSessionService returns the database-backed ADK SessionService. Before a new
// session is passed to an ADK Runner, call EnsureADKSession for that session
// ID so the ADK ledger exists.
func (s *Store) ADKSessionService() adksession.SessionService {
	return s.adkSessions
}

// EnsureADKSession returns the ADK history session corresponding to id,
// creating it on first use. The Koda session must already exist.
func (s *Store) EnsureADKSession(ctx context.Context, id string) (adksession.Session, error) {
	id, err := normalizeSessionID(id)
	if err != nil {
		return nil, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()

	exists, err := s.sessionExists(ctx, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("store: ensure ADK session %q: %w", id, ErrNotFound)
	}

	sess, err := s.adkSessions.GetSession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: get ADK session %q: %w", id, err)
	}
	if sess != nil {
		return sess, nil
	}
	sess, err = s.adkSessions.CreateSession(ctx, adksession.CreateSessionRequest{
		SessionID: id,
		AppID:     appID,
	})
	if err != nil {
		return nil, fmt.Errorf("store: create ADK session %q: %w", id, err)
	}
	return sess, nil
}

// LockRun acquires the lock shared with the ADK SessionService for one Koda
// session. It serializes full turns and lifecycle operations for that session.
func (s *Store) LockRun(ctx context.Context, id string) (func(), error) {
	id, err := normalizeSessionID(id)
	if err != nil {
		return nil, err
	}
	unlock, err := s.runLocker.LockRun(ctx, adksession.RunLockKey{
		AppID:     appID,
		SessionID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("store: lock run for %q: %w", id, err)
	}
	return unlock, nil
}

// LockRunContext acquires the run lock for id and returns a context that makes
// nested acquisitions by the ADK Runner reentrant. The caller must invoke the
// returned unlock function after all post-run persistence and terminal journal
// work is complete.
func (s *Store) LockRunContext(ctx context.Context, id string) (context.Context, func(), error) {
	id, err := normalizeSessionID(id)
	if err != nil {
		return nil, nil, err
	}
	lockedCtx, unlock, err := s.runLocker.lockContext(ctx, adksession.RunLockKey{
		AppID:     appID,
		SessionID: id,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("store: lock run for %q: %w", id, err)
	}
	return lockedCtx, unlock, nil
}

func sqliteDSN(path string) string {
	path = filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); volume != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := &url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL",
	}
	return u.String()
}

func secureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("store: create database directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("store: secure database directory: %w", err)
	}
	return nil
}

func secureDatabaseFiles(path string) error {
	for _, file := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(file, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("store: secure database file: %w", err)
		}
	}
	return nil
}
