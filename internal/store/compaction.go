package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrCompactionConflict indicates that a compaction was prepared from a
	// stale session generation.
	ErrCompactionConflict = errors.New("compaction generation conflict")
)

// Compaction is one immutable durable context-compaction generation. The
// segment summary describes only the newly archived events, while the state
// snapshot contains the complete working state carried into later runs.
type Compaction struct {
	ID                   int64
	SessionID            string
	Generation           int64
	PreviousCompactionID int64
	StartEventID         int64
	BoundaryEventID      int64
	SegmentSummary       string
	StateSnapshot        string
	SourceTokens         int64
	EstimatedTokensAfter int64
	ModelID              string
	CreatedAt            time.Time
}

// CommitCompactionParams contains a prepared compaction and the generation it
// was based on. BoundaryEventID is the first active event to retain; zero
// archives every active event.
type CommitCompactionParams struct {
	ExpectedGeneration   int64
	StartEventID         int64
	BoundaryEventID      int64
	SegmentSummary       string
	StateSnapshot        string
	SourceTokens         int64
	EstimatedTokensAfter int64
	ModelID              string
}

type compactionRow struct {
	ID                   int64  `db:"id"`
	SessionID            string `db:"session_id"`
	Generation           int64  `db:"generation"`
	PreviousCompactionID int64  `db:"previous_compaction_id"`
	StartEventID         int64  `db:"start_event_id"`
	BoundaryEventID      int64  `db:"boundary_event_id"`
	SegmentSummary       string `db:"segment_summary"`
	StateSnapshot        string `db:"state_snapshot"`
	SourceTokens         int64  `db:"source_tokens"`
	EstimatedTokensAfter int64  `db:"estimated_tokens_after"`
	ModelID              string `db:"model_id"`
	CreatedAt            int64  `db:"created_at"`
}

type compactionSessionState struct {
	CurrentCompactionID int64 `db:"current_compaction_id"`
	Generation          int64 `db:"compaction_generation"`
}

type eventPosition struct {
	EventID   int64 `db:"event_id"`
	CreatedAt int64 `db:"created_at"`
}

// GetCurrentCompaction returns the active compaction generation for id. It
// returns nil when the session has not been compacted.
func (s *Store) GetCurrentCompaction(ctx context.Context, id string) (*Compaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return nil, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var currentID int64
	err = s.db.GetContext(ctx, &currentID, `
		SELECT current_compaction_id
		FROM koda_sessions
		WHERE id = $1 AND deleted_at = 0
	`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: get current compaction for %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get current compaction ID for %q: %w", id, err)
	}
	if currentID == 0 {
		return nil, nil
	}

	row := new(compactionRow)
	err = s.db.GetContext(ctx, row, `
		SELECT id, session_id, generation, previous_compaction_id,
			start_event_id, boundary_event_id, segment_summary, state_snapshot,
			source_tokens, estimated_tokens_after, model_id, created_at
		FROM koda_session_compactions
		WHERE id = $1 AND session_id = $2 AND deleted_at = 0
	`, currentID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: current compaction %d for %q is missing", currentID, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get current compaction for %q: %w", id, err)
	}
	result := compactionFromRow(*row)
	return &result, nil
}

// ListCompactions returns every active compaction generation for id in
// generation order. It waits for an active run so callers observe a complete
// generation chain.
func (s *Store) ListCompactions(ctx context.Context, id string) ([]Compaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return nil, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if _, err := s.GetSession(ctx, id); err != nil {
		return nil, err
	}

	rows := make([]compactionRow, 0)
	err = s.db.SelectContext(ctx, &rows, `
		SELECT id, session_id, generation, previous_compaction_id,
			start_event_id, boundary_event_id, segment_summary, state_snapshot,
			source_tokens, estimated_tokens_after, model_id, created_at
		FROM koda_session_compactions
		WHERE session_id = $1 AND deleted_at = 0
		ORDER BY generation ASC
	`, id)
	if err != nil {
		return nil, fmt.Errorf("store: list compactions for %q: %w", id, err)
	}
	result := make([]Compaction, len(rows))
	for index, row := range rows {
		result[index] = compactionFromRow(row)
	}
	return result, nil
}

// CommitCompaction atomically records a new immutable generation, archives
// the selected active-history prefix, advances the session generation, and
// invalidates context usage measured before the compaction.
func (s *Store) CommitCompaction(ctx context.Context, id string, params CommitCompactionParams) (Compaction, error) {
	if err := ctx.Err(); err != nil {
		return Compaction{}, err
	}
	if err := s.checkClosed(); err != nil {
		return Compaction{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return Compaction{}, err
	}
	params, err = normalizeCommitCompactionParams(params)
	if err != nil {
		return Compaction{}, err
	}
	lockedCtx, unlock, err := s.LockRunContext(ctx, id)
	if err != nil {
		return Compaction{}, err
	}
	defer unlock()

	tx, err := s.db.BeginTxx(lockedCtx, nil)
	if err != nil {
		return Compaction{}, fmt.Errorf("store: begin compaction for %q: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit below completes the transaction.

	state := new(compactionSessionState)
	err = tx.GetContext(lockedCtx, state, `
		SELECT current_compaction_id, compaction_generation
		FROM koda_sessions
		WHERE id = $1 AND deleted_at = 0
	`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Compaction{}, fmt.Errorf("store: compact session %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return Compaction{}, fmt.Errorf("store: load compaction state for %q: %w", id, err)
	}
	if state.Generation != params.ExpectedGeneration {
		return Compaction{}, fmt.Errorf("store: compact session %q at generation %d, current generation %d: %w", id, params.ExpectedGeneration, state.Generation, ErrCompactionConflict)
	}

	first := new(eventPosition)
	err = tx.GetContext(lockedCtx, first, `
		SELECT event_id, created_at
		FROM adk_events
		WHERE session_id = $1 AND deleted_at = 0 AND archived_at = 0
		ORDER BY created_at ASC, event_id ASC
		LIMIT 1
	`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Compaction{}, fmt.Errorf("store: compact session %q: no active events", id)
	}
	if err != nil {
		return Compaction{}, fmt.Errorf("store: find first active event for %q: %w", id, err)
	}
	if first.EventID != params.StartEventID {
		return Compaction{}, fmt.Errorf("store: compact session %q starts at event %d, first active event is %d", id, params.StartEventID, first.EventID)
	}

	var compactedEvents int64
	if params.BoundaryEventID == 0 {
		if err := tx.GetContext(lockedCtx, &compactedEvents, `
			SELECT COUNT(*)
			FROM adk_events
			WHERE session_id = $1 AND deleted_at = 0 AND archived_at = 0
		`, id); err != nil {
			return Compaction{}, fmt.Errorf("store: count active events for %q: %w", id, err)
		}
	} else {
		boundary := new(eventPosition)
		err = tx.GetContext(lockedCtx, boundary, `
			SELECT event_id, created_at
			FROM adk_events
			WHERE session_id = $1 AND event_id = $2
				AND deleted_at = 0 AND archived_at = 0
		`, id, params.BoundaryEventID)
		if errors.Is(err, sql.ErrNoRows) {
			return Compaction{}, fmt.Errorf("store: compaction boundary event %d for %q is not active", params.BoundaryEventID, id)
		}
		if err != nil {
			return Compaction{}, fmt.Errorf("store: load compaction boundary for %q: %w", id, err)
		}
		if boundary.CreatedAt < first.CreatedAt || (boundary.CreatedAt == first.CreatedAt && boundary.EventID <= first.EventID) {
			return Compaction{}, fmt.Errorf("store: compaction boundary event %d must follow start event %d", boundary.EventID, first.EventID)
		}
		if err := tx.GetContext(lockedCtx, &compactedEvents, `
			SELECT COUNT(*)
			FROM adk_events
			WHERE session_id = $1 AND deleted_at = 0 AND archived_at = 0
				AND (created_at < $2 OR (created_at = $2 AND event_id < $3))
		`, id, boundary.CreatedAt, boundary.EventID); err != nil {
			return Compaction{}, fmt.Errorf("store: count compacted events for %q: %w", id, err)
		}
	}
	if compactedEvents == 0 {
		return Compaction{}, fmt.Errorf("store: compact session %q: no events precede the boundary", id)
	}

	var usageMinEventID int64
	if err := tx.GetContext(lockedCtx, &usageMinEventID, `
		SELECT MAX(event_id)
		FROM adk_events
		WHERE session_id = $1 AND deleted_at = 0
	`, id); err != nil {
		return Compaction{}, fmt.Errorf("store: find context usage watermark for %q: %w", id, err)
	}

	now := s.now().UTC()
	generation := state.Generation + 1
	result, err := tx.ExecContext(lockedCtx, `
		INSERT INTO koda_session_compactions (
			session_id, generation, previous_compaction_id,
			start_event_id, boundary_event_id, segment_summary, state_snapshot,
			source_tokens, estimated_tokens_after, model_id, created_at, deleted_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 0)
	`, id, generation, state.CurrentCompactionID, params.StartEventID, params.BoundaryEventID,
		params.SegmentSummary, params.StateSnapshot, params.SourceTokens,
		params.EstimatedTokensAfter, params.ModelID, now.UnixMilli())
	if err != nil {
		return Compaction{}, fmt.Errorf("store: insert compaction generation %d for %q: %w", generation, id, err)
	}
	compactionID, err := result.LastInsertId()
	if err != nil {
		return Compaction{}, fmt.Errorf("store: get compaction ID for %q: %w", id, err)
	}

	var archivedResult sql.Result
	if params.BoundaryEventID == 0 {
		archivedResult, err = tx.ExecContext(lockedCtx, `
			UPDATE adk_events
			SET archived_at = $1
			WHERE session_id = $2 AND deleted_at = 0 AND archived_at = 0
		`, now.UnixMilli(), id)
	} else {
		var boundary eventPosition
		if err := tx.GetContext(lockedCtx, &boundary, `
			SELECT event_id, created_at FROM adk_events
			WHERE session_id = $1 AND event_id = $2 AND deleted_at = 0 AND archived_at = 0
		`, id, params.BoundaryEventID); err != nil {
			return Compaction{}, fmt.Errorf("store: reload compaction boundary for %q: %w", id, err)
		}
		archivedResult, err = tx.ExecContext(lockedCtx, `
			UPDATE adk_events
			SET archived_at = $1
			WHERE session_id = $2 AND deleted_at = 0 AND archived_at = 0
				AND (created_at < $3 OR (created_at = $3 AND event_id < $4))
		`, now.UnixMilli(), id, boundary.CreatedAt, boundary.EventID)
	}
	if err != nil {
		return Compaction{}, fmt.Errorf("store: archive compacted events for %q: %w", id, err)
	}
	archived, err := archivedResult.RowsAffected()
	if err != nil {
		return Compaction{}, fmt.Errorf("store: count archived events for %q: %w", id, err)
	}
	if archived != compactedEvents {
		return Compaction{}, fmt.Errorf("store: archived %d events for %q, expected %d", archived, id, compactedEvents)
	}

	updated, err := tx.ExecContext(lockedCtx, `
		UPDATE koda_sessions
		SET current_compaction_id = $1,
			compaction_generation = $2,
			context_usage_min_event_id = $3,
			last_compaction_attempt_usage = 0,
			consecutive_compaction_failures = 0,
			updated_at = $4
		WHERE id = $5 AND deleted_at = 0 AND compaction_generation = $6
	`, compactionID, generation, usageMinEventID, now.UnixMilli(), id, params.ExpectedGeneration)
	if err != nil {
		return Compaction{}, fmt.Errorf("store: advance compaction generation for %q: %w", id, err)
	}
	rows, err := updated.RowsAffected()
	if err != nil {
		return Compaction{}, fmt.Errorf("store: count updated compaction generation for %q: %w", id, err)
	}
	if rows != 1 {
		return Compaction{}, fmt.Errorf("store: advance compaction generation for %q: %w", id, ErrCompactionConflict)
	}
	if err := tx.Commit(); err != nil {
		return Compaction{}, fmt.Errorf("store: commit compaction generation %d for %q: %w", generation, id, err)
	}

	return Compaction{
		ID:                   compactionID,
		SessionID:            id,
		Generation:           generation,
		PreviousCompactionID: state.CurrentCompactionID,
		StartEventID:         params.StartEventID,
		BoundaryEventID:      params.BoundaryEventID,
		SegmentSummary:       params.SegmentSummary,
		StateSnapshot:        params.StateSnapshot,
		SourceTokens:         params.SourceTokens,
		EstimatedTokensAfter: params.EstimatedTokensAfter,
		ModelID:              params.ModelID,
		CreatedAt:            now,
	}, nil
}

func normalizeCommitCompactionParams(params CommitCompactionParams) (CommitCompactionParams, error) {
	params.SegmentSummary = strings.TrimSpace(params.SegmentSummary)
	params.StateSnapshot = strings.TrimSpace(params.StateSnapshot)
	params.ModelID = strings.TrimSpace(params.ModelID)
	if params.ExpectedGeneration < 0 {
		return CommitCompactionParams{}, errors.New("store: expected compaction generation must not be negative")
	}
	if params.StartEventID <= 0 {
		return CommitCompactionParams{}, errors.New("store: compaction start event ID must be positive")
	}
	if params.BoundaryEventID < 0 {
		return CommitCompactionParams{}, errors.New("store: compaction boundary event ID must not be negative")
	}
	if params.SegmentSummary == "" {
		return CommitCompactionParams{}, errors.New("store: compaction segment summary must not be empty")
	}
	if params.StateSnapshot == "" {
		return CommitCompactionParams{}, errors.New("store: compaction state snapshot must not be empty")
	}
	if params.SourceTokens < 0 {
		return CommitCompactionParams{}, errors.New("store: compaction source tokens must not be negative")
	}
	if params.EstimatedTokensAfter < 0 {
		return CommitCompactionParams{}, errors.New("store: compaction estimated tokens must not be negative")
	}
	if params.ModelID == "" {
		return CommitCompactionParams{}, errors.New("store: compaction model ID must not be empty")
	}
	return params, nil
}

func compactionFromRow(row compactionRow) Compaction {
	return Compaction{
		ID:                   row.ID,
		SessionID:            row.SessionID,
		Generation:           row.Generation,
		PreviousCompactionID: row.PreviousCompactionID,
		StartEventID:         row.StartEventID,
		BoundaryEventID:      row.BoundaryEventID,
		SegmentSummary:       row.SegmentSummary,
		StateSnapshot:        row.StateSnapshot,
		SourceTokens:         row.SourceTokens,
		EstimatedTokensAfter: row.EstimatedTokensAfter,
		ModelID:              row.ModelID,
		CreatedAt:            time.UnixMilli(row.CreatedAt).UTC(),
	}
}
