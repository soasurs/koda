package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/soasurs/adk/model"
	adksession "github.com/soasurs/adk/session"
	sessionevent "github.com/soasurs/adk/session/event"
)

var (
	// ErrUndoConflict indicates that the latest undoable turn changed after a
	// client loaded the conversation history.
	ErrUndoConflict = errors.New("undo turn conflict")
)

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
	Turns               []*adksession.Turn
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
	turns, err := s.recoverAndListTurns(ctx, id)
	if err != nil {
		return ConversationHistory{}, err
	}

	persisted := make([]sessionevent.Event, 0)
	if err := s.db.SelectContext(ctx, &persisted, s.queries.listHistoryEvents, id); err != nil {
		return ConversationHistory{}, fmt.Errorf("store: list history for %q: %w", id, err)
	}
	history := ConversationHistory{Events: make([]model.Event, len(persisted))}
	visibleTurnIDs := make(map[string]struct{})
	for index := range persisted {
		history.Events[index] = persisted[index].ToModel()
		visibleTurnIDs[persisted[index].TurnID] = struct{}{}
		if persisted[index].ArchivedAt > 0 {
			history.CompactedEventCount++
		} else if persisted[index].Role == string(model.RoleUser) {
			history.UndoableTurnID = persisted[index].TurnID
		}
	}
	allEventTurnIDs := make([]string, 0)
	if err := s.db.SelectContext(ctx, &allEventTurnIDs, s.queries.listAllEventTurnIDs, id); err != nil {
		return ConversationHistory{}, fmt.Errorf("store: list event turns for %q: %w", id, err)
	}
	hadEvents := make(map[string]struct{}, len(allEventTurnIDs))
	for _, turnID := range allEventTurnIDs {
		hadEvents[turnID] = struct{}{}
	}
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		_, visible := visibleTurnIDs[turn.ID]
		_, deleted := hadEvents[turn.ID]
		if visible || !deleted {
			history.Turns = append(history.Turns, turn)
		}
	}
	current, err := s.getCurrentCompaction(ctx, id)
	if err != nil {
		return ConversationHistory{}, err
	}
	history.CurrentCompaction = current
	return history, nil
}

// ProjectionHistory contains active durable facts for context projection.
type ProjectionHistory struct {
	Events []*sessionevent.Event
	Turns  []*adksession.Turn
}

// ListProjectionHistory returns active Events and Turns in one locked snapshot.
// Any running Turns left by an earlier process are lazily marked abandoned.
func (s *Store) ListProjectionHistory(ctx context.Context, id string) (ProjectionHistory, error) {
	if err := ctx.Err(); err != nil {
		return ProjectionHistory{}, err
	}
	id, err := normalizeSessionID(id)
	if err != nil {
		return ProjectionHistory{}, err
	}
	unlock, err := s.LockRun(ctx, id)
	if err != nil {
		return ProjectionHistory{}, err
	}
	defer unlock()
	if _, err := s.GetSession(ctx, id); err != nil {
		return ProjectionHistory{}, err
	}
	turns, err := s.recoverAndListTurns(ctx, id)
	if err != nil {
		return ProjectionHistory{}, err
	}
	sess, err := s.adkSessions.GetSession(ctx, id)
	if err != nil {
		return ProjectionHistory{}, fmt.Errorf("store: get ADK session %q: %w", id, err)
	}
	if sess == nil {
		return ProjectionHistory{Turns: turns}, nil
	}
	events, err := sess.ListEvents(ctx)
	if err != nil {
		return ProjectionHistory{}, fmt.Errorf("store: list projection events for %q: %w", id, err)
	}
	return ProjectionHistory{Events: events, Turns: turns}, nil
}

func (s *Store) recoverAndListTurns(ctx context.Context, id string) ([]*adksession.Turn, error) {
	sess, err := s.adkSessions.GetSession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("store: get ADK session %q: %w", id, err)
	}
	if sess == nil {
		return nil, nil
	}
	turns, ok := sess.(adksession.TurnStore)
	if !ok {
		return nil, errors.New("store: ADK session does not support durable turns")
	}
	if err := turns.InterruptRunningTurns(ctx, adksession.TurnReasonAbandoned); err != nil {
		return nil, fmt.Errorf("store: recover abandoned turns for %q: %w", id, err)
	}
	result, err := turns.ListTurns(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list turns for %q: %w", id, err)
	}
	return result, nil
}

// ListEvents returns all active ADK events in conversation order. Failed and
// interrupted Turns may intentionally contribute an incomplete event prefix.
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
