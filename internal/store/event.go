package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/soasurs/adk/model"
	sessionevent "github.com/soasurs/adk/session/event"
)

var (
	// ErrUndoConflict indicates that the latest undoable turn changed after a
	// client loaded the conversation history.
	ErrUndoConflict = errors.New("undo turn conflict")
)

// RollbackTurn removes a turn that the ADK Runner completed but the Run RPC
// could not acknowledge. previous restores the session title and ordering
// timestamp when completion metadata had already been updated.
func (s *Store) RollbackTurn(ctx context.Context, id, turnID string, previous Session) error {
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
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return errors.New("store: turn ID must not be empty")
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return err
	}
	defer unlock()

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin rollback turn %q: %w", turnID, err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit below completes the transaction.
	if _, err := tx.ExecContext(ctx, s.queries.deleteTurnEvents, s.now().UTC().UnixMilli(), id, turnID); err != nil {
		return fmt.Errorf("store: rollback turn %q: %w", turnID, err)
	}
	if !previous.UpdatedAt.IsZero() {
		if _, err := tx.ExecContext(ctx, s.queries.restoreSession, previous.Title, previous.UpdatedAt.UTC().UnixMilli(), id); err != nil {
			return fmt.Errorf("store: restore session %q metadata: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit rollback turn %q: %w", turnID, err)
	}
	return nil
}

// UndoLastMessageResult describes the most recently removed user turn.
type UndoLastMessageResult struct {
	TurnID            string
	DeletedEventCount int64
	Input             model.Content
}

// ConversationHistory contains every non-deleted event, including the
// compacted prefix retained for display, and the current mutation boundary.
type ConversationHistory struct {
	Events              []model.Event
	CompactedEventCount int64
	CurrentCompaction   *Compaction
	UndoableTurnID      string
}

// ListHistory returns complete visible history and its current compaction and
// undo boundaries. It waits for any active turn so the snapshot is consistent.
func (s *Store) ListHistory(ctx context.Context, id string) (ConversationHistory, error) {
	if err := ctx.Err(); err != nil {
		return ConversationHistory{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return ConversationHistory{}, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return ConversationHistory{}, err
	}
	defer unlock()
	if _, err := s.GetSession(ctx, id); err != nil {
		return ConversationHistory{}, err
	}

	persisted := make([]sessionevent.Event, 0)
	if err := s.db.SelectContext(ctx, &persisted, s.queries.listHistoryEvents, id); err != nil {
		return ConversationHistory{}, fmt.Errorf("store: list history for %q: %w", id, err)
	}
	history := ConversationHistory{Events: make([]model.Event, len(persisted))}
	for index := range persisted {
		history.Events[index] = persisted[index].ToModel()
		if persisted[index].ArchivedAt > 0 {
			history.CompactedEventCount++
		} else if persisted[index].Role == string(model.RoleUser) {
			history.UndoableTurnID = persisted[index].TurnID
		}
	}
	current, err := s.getCurrentCompaction(ctx, id)
	if err != nil {
		return ConversationHistory{}, err
	}
	history.CurrentCompaction = current
	return history, nil
}

// ListEvents returns all active ADK events in conversation order.
// It waits for any active turn so callers never observe an incomplete ledger.
func (s *Store) ListEvents(ctx context.Context, id string) ([]model.Event, error) {
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

	persisted := make([]sessionevent.Event, 0)
	if err := s.db.SelectContext(ctx, &persisted, s.queries.listEvents, id); err != nil {
		return nil, fmt.Errorf("store: list events for %q: %w", id, err)
	}
	events := make([]model.Event, len(persisted))
	for index := range persisted {
		events[index] = persisted[index].ToModel()
	}
	return events, nil
}

// UndoLastMessage deletes the most recent active user turn. It returns an
// empty result when the session has no active user message.
func (s *Store) UndoLastMessage(ctx context.Context, id, expectedTurnID string) (UndoLastMessageResult, error) {
	if err := ctx.Err(); err != nil {
		return UndoLastMessageResult{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return UndoLastMessageResult{}, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return UndoLastMessageResult{}, err
	}
	defer unlock()
	if _, err := s.GetSession(ctx, id); err != nil {
		return UndoLastMessageResult{}, err
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: begin undo for %q: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit below completes the transaction.

	userEvent := new(sessionevent.Event)
	err = tx.GetContext(ctx, userEvent, s.queries.latestUserEvent, id, string(model.RoleUser))
	if errors.Is(err, sql.ErrNoRows) {
		if strings.TrimSpace(expectedTurnID) != "" {
			return UndoLastMessageResult{}, fmt.Errorf("store: expected undo turn %q, but no active user turn remains: %w", strings.TrimSpace(expectedTurnID), ErrUndoConflict)
		}
		return UndoLastMessageResult{}, nil
	}
	if err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: find last user event for %q: %w", id, err)
	}
	if userEvent.TurnID == "" {
		return UndoLastMessageResult{}, fmt.Errorf("store: last user event %d has no turn ID", userEvent.EventID)
	}
	expectedTurnID = strings.TrimSpace(expectedTurnID)
	if expectedTurnID != "" && userEvent.TurnID != expectedTurnID {
		return UndoLastMessageResult{}, fmt.Errorf("store: expected undo turn %q, current turn is %q: %w", expectedTurnID, userEvent.TurnID, ErrUndoConflict)
	}

	deletedAt := s.now().UTC().UnixMilli()
	result, err := tx.ExecContext(ctx, s.queries.deleteTurnEvents, deletedAt, id, userEvent.TurnID)
	if err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: delete turn %q: %w", userEvent.TurnID, err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: count deleted turn events: %w", err)
	}
	if deleted == 0 {
		return UndoLastMessageResult{}, fmt.Errorf("store: delete turn %q: no active events", userEvent.TurnID)
	}
	if _, err := tx.ExecContext(ctx, s.queries.touchSession, deletedAt, id); err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: touch session %q after undo: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return UndoLastMessageResult{}, fmt.Errorf("store: commit undo for %q: %w", id, err)
	}
	return UndoLastMessageResult{
		TurnID:            userEvent.TurnID,
		DeletedEventCount: deleted,
		Input:             userEvent.ToModel().Content,
	}, nil
}
