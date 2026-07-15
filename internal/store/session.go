package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/soasurs/koda/internal/permission"
)

var (
	// ErrNotFound indicates that a Koda session does not exist or was deleted.
	ErrNotFound = errors.New("session not found")
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Session is Koda's durable configuration and summary for one coding session.
// Conversation events remain owned by the associated ADK session ledger.
type Session struct {
	ID              string
	Title           string
	Workdir         string
	ProviderID      string
	ModelID         string
	ReasoningEffort string
	FileAccess      permission.FileAccess
	ShellAccess     permission.ShellAccess
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ArchivedAt      time.Time
	EventCount      int64
	ContextTokens   int64
	ContextMeasured bool
}

// CreateSessionParams contains the initial configuration for a coding session.
type CreateSessionParams struct {
	ID              string
	Title           string
	Workdir         string
	ProviderID      string
	ModelID         string
	ReasoningEffort string
	FileAccess      permission.FileAccess
	ShellAccess     permission.ShellAccess
}

// UpdateSessionParams contains only the session fields to change. A nil field
// leaves the stored value unchanged; a non-nil empty ReasoningEffort restores
// the model default.
type UpdateSessionParams struct {
	Title           *string
	Workdir         *string
	ProviderID      *string
	ModelID         *string
	ReasoningEffort *string
	FileAccess      *permission.FileAccess
	ShellAccess     *permission.ShellAccess
	Archived        *bool
}

// ListSessionsParams selects one page of Koda sessions. Results are ordered
// by updated time descending, then session ID ascending.
type ListSessionsParams struct {
	Limit    int
	Offset   int64
	Archived bool
}

type sessionRow struct {
	ID              string `db:"id"`
	Title           string `db:"title"`
	Workdir         string `db:"workdir"`
	ProviderID      string `db:"provider_id"`
	ModelID         string `db:"model_id"`
	ReasoningEffort string `db:"reasoning_effort"`
	FileAccess      string `db:"file_access"`
	ShellAccess     string `db:"shell_access"`
	CreatedAt       int64  `db:"created_at"`
	UpdatedAt       int64  `db:"updated_at"`
	ArchivedAt      int64  `db:"archived_at"`
	EventCount      int64  `db:"event_count"`
	ContextTokens   int64  `db:"context_tokens"`
}

// CreateSession creates Koda session metadata. Its ADK history ledger is
// created lazily by EnsureADKSession immediately before the first run.
func (s *Store) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	if err := s.checkClosed(); err != nil {
		return Session{}, err
	}
	params, err := normalizeCreateParams(params)
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC().UnixMilli()
	_, err = s.db.ExecContext(ctx, s.queries.createSession,
		params.ID,
		params.Title,
		params.Workdir,
		params.ProviderID,
		params.ModelID,
		params.ReasoningEffort,
		params.FileAccess,
		params.ShellAccess,
		now,
		now,
	)
	if err != nil {
		return Session{}, fmt.Errorf("store: create session: %w", err)
	}
	return s.GetSession(ctx, params.ID)
}

// GetSession returns the active session identified by id.
func (s *Store) GetSession(ctx context.Context, id string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return Session{}, err
	}
	row := new(sessionRow)
	err = s.db.GetContext(ctx, row, s.queries.getSession, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, fmt.Errorf("store: get session %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return Session{}, fmt.Errorf("store: get session %q: %w", id, err)
	}
	return sessionFromRow(*row), nil
}

// ListSessions returns one page of active sessions and their unpaginated total.
func (s *Store) ListSessions(ctx context.Context, params ListSessionsParams) ([]Session, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	params, err := normalizeListParams(params)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := s.db.GetContext(ctx, &total, s.queries.countSessions, params.Archived); err != nil {
		return nil, 0, fmt.Errorf("store: count sessions: %w", err)
	}

	rows := make([]sessionRow, 0)
	err = s.db.SelectContext(ctx, &rows, s.queries.listSessions, params.Archived, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list sessions: %w", err)
	}
	sessions := make([]Session, len(rows))
	for i, row := range rows {
		sessions[i] = sessionFromRow(row)
	}
	return sessions, total, nil
}

// UpdateSession applies params to an active session and returns the result.
func (s *Store) UpdateSession(ctx context.Context, id string, params UpdateSessionParams) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	if err := s.checkClosed(); err != nil {
		return Session{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return Session{}, err
	}
	if params.empty() {
		return s.GetSession(ctx, id)
	}

	current, err := s.GetSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	updated, err := applyUpdate(current, params)
	if err != nil {
		return Session{}, err
	}
	updated.UpdatedAt = s.now().UTC()
	if params.Archived != nil {
		if *params.Archived {
			updated.ArchivedAt = updated.UpdatedAt
		} else {
			updated.ArchivedAt = time.Time{}
		}
	}
	var archivedAt int64
	if !updated.ArchivedAt.IsZero() {
		archivedAt = updated.ArchivedAt.UnixMilli()
	}
	_, err = s.db.ExecContext(ctx, s.queries.updateSession,
		updated.Title,
		updated.Workdir,
		updated.ProviderID,
		updated.ModelID,
		updated.ReasoningEffort,
		updated.FileAccess,
		updated.ShellAccess,
		archivedAt,
		updated.UpdatedAt.UnixMilli(),
		id,
	)
	if err != nil {
		return Session{}, fmt.Errorf("store: update session %q: %w", id, err)
	}
	return s.GetSession(ctx, id)
}

// TouchSession records that a session completed an operation that changes its
// observable history, such as a successfully committed agent turn.
func (s *Store) TouchSession(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.queries.touchSession, s.now().UTC().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("store: touch session %q: %w", id, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: touch session %q: rows affected: %w", id, err)
	}
	if updated == 0 {
		return fmt.Errorf("store: touch session %q: %w", id, ErrNotFound)
	}
	return nil
}

// DeleteSession atomically soft-deletes the Koda session, its ADK ledger, and
// every active or archived event in that ledger.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.checkClosed(); err != nil {
		return err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin delete session %q: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit below completes the transaction.

	deletedAt := s.now().UTC().UnixMilli()
	result, err := tx.ExecContext(ctx, s.queries.deleteSession, deletedAt, id)
	if err != nil {
		return fmt.Errorf("store: delete session %q: %w", id, err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete session %q: rows affected: %w", id, err)
	}
	if deleted == 0 {
		return fmt.Errorf("store: delete session %q: %w", id, ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, s.queries.deleteADKSession, deletedAt, id); err != nil {
		return fmt.Errorf("store: delete ADK session %q: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, s.queries.deleteEvents, deletedAt, id); err != nil {
		return fmt.Errorf("store: delete ADK events for %q: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit delete session %q: %w", id, err)
	}
	return nil
}

func (s *Store) sessionExists(ctx context.Context, id string) (bool, error) {
	var exists int
	err := s.db.GetContext(ctx, &exists, s.queries.sessionExists, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: find session %q: %w", id, err)
	}
	return true, nil
}

func normalizeCreateParams(params CreateSessionParams) (CreateSessionParams, error) {
	var err error
	params.ID, err = normalizeSessionID(params.ID)
	if err != nil {
		return CreateSessionParams{}, err
	}
	params.Title = strings.TrimSpace(params.Title)
	params.Workdir = strings.TrimSpace(params.Workdir)
	params.ProviderID = strings.TrimSpace(params.ProviderID)
	params.ModelID = strings.TrimSpace(params.ModelID)
	params.ReasoningEffort = strings.TrimSpace(params.ReasoningEffort)
	if params.FileAccess == "" {
		params.FileAccess = permission.DefaultFileAccess
	}
	if params.ShellAccess == "" {
		params.ShellAccess = permission.DefaultShellAccess
	}
	if params.Workdir == "" {
		return CreateSessionParams{}, errors.New("store: workdir must not be empty")
	}
	if params.ProviderID == "" {
		return CreateSessionParams{}, errors.New("store: provider ID must not be empty")
	}
	if params.ModelID == "" {
		return CreateSessionParams{}, errors.New("store: model ID must not be empty")
	}
	if !params.FileAccess.Valid() {
		return CreateSessionParams{}, fmt.Errorf("store: invalid file access %q", params.FileAccess)
	}
	if !params.ShellAccess.Valid() {
		return CreateSessionParams{}, fmt.Errorf("store: invalid shell access %q", params.ShellAccess)
	}
	return params, nil
}

func normalizeSessionID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("store: session ID must not be empty")
	}
	return id, nil
}

func normalizeListParams(params ListSessionsParams) (ListSessionsParams, error) {
	if params.Limit < 0 {
		return ListSessionsParams{}, errors.New("store: list limit must not be negative")
	}
	if params.Offset < 0 {
		return ListSessionsParams{}, errors.New("store: list offset must not be negative")
	}
	if params.Limit == 0 {
		params.Limit = defaultListLimit
	}
	if params.Limit > maxListLimit {
		params.Limit = maxListLimit
	}
	return params, nil
}

func (p UpdateSessionParams) empty() bool {
	return p.Title == nil &&
		p.Workdir == nil &&
		p.ProviderID == nil &&
		p.ModelID == nil &&
		p.ReasoningEffort == nil &&
		p.FileAccess == nil &&
		p.ShellAccess == nil &&
		p.Archived == nil
}

func applyUpdate(current Session, params UpdateSessionParams) (Session, error) {
	if params.Title != nil {
		current.Title = strings.TrimSpace(*params.Title)
	}
	if params.Workdir != nil {
		current.Workdir = strings.TrimSpace(*params.Workdir)
	}
	if params.ProviderID != nil {
		current.ProviderID = strings.TrimSpace(*params.ProviderID)
	}
	if params.ModelID != nil {
		current.ModelID = strings.TrimSpace(*params.ModelID)
	}
	if params.ReasoningEffort != nil {
		current.ReasoningEffort = strings.TrimSpace(*params.ReasoningEffort)
	}
	if params.FileAccess != nil {
		current.FileAccess = *params.FileAccess
	}
	if params.ShellAccess != nil {
		current.ShellAccess = *params.ShellAccess
	}
	if current.Workdir == "" {
		return Session{}, errors.New("store: workdir must not be empty")
	}
	if current.ProviderID == "" {
		return Session{}, errors.New("store: provider ID must not be empty")
	}
	if current.ModelID == "" {
		return Session{}, errors.New("store: model ID must not be empty")
	}
	if !current.FileAccess.Valid() {
		return Session{}, fmt.Errorf("store: invalid file access %q", current.FileAccess)
	}
	if !current.ShellAccess.Valid() {
		return Session{}, fmt.Errorf("store: invalid shell access %q", current.ShellAccess)
	}
	return current, nil
}

func sessionFromRow(row sessionRow) Session {
	session := Session{
		ID:              row.ID,
		Title:           row.Title,
		Workdir:         row.Workdir,
		ProviderID:      row.ProviderID,
		ModelID:         row.ModelID,
		ReasoningEffort: row.ReasoningEffort,
		FileAccess:      permission.FileAccess(row.FileAccess),
		ShellAccess:     permission.ShellAccess(row.ShellAccess),
		CreatedAt:       time.UnixMilli(row.CreatedAt).UTC(),
		UpdatedAt:       time.UnixMilli(row.UpdatedAt).UTC(),
		EventCount:      row.EventCount,
		ContextTokens:   row.ContextTokens,
		ContextMeasured: row.ContextTokens > 0,
	}
	if row.ArchivedAt > 0 {
		session.ArchivedAt = time.UnixMilli(row.ArchivedAt).UTC()
	}
	return session
}
