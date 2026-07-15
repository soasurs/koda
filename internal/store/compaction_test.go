package store

import (
	"errors"
	"testing"
	"time"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/session/event"
)

func TestCommitCompactionGenerations(t *testing.T) {
	store := openTestStore(t)
	createdAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return createdAt.Add(time.Hour) }
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	adkSession, err := store.EnsureADKSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("EnsureADKSession() error = %v", err)
	}
	for _, value := range []struct {
		id     int64
		role   model.Role
		prompt int64
		output int64
	}{
		{id: 1, role: model.RoleUser},
		{id: 2, role: model.RoleAssistant, prompt: 80_000, output: 5_000},
		{id: 3, role: model.RoleUser},
		{id: 4, role: model.RoleAssistant, prompt: 120_000, output: 6_000},
	} {
		if err := adkSession.CreateEvent(t.Context(), &event.Event{
			EventID: value.id, TurnID: "turn", Role: string(value.role), Content: "content",
			PromptTokens: value.prompt, CompletionTokens: value.output,
			CreatedAt: createdAt.Add(time.Duration(value.id) * time.Millisecond).UnixMilli(),
			UpdatedAt: createdAt.Add(time.Duration(value.id) * time.Millisecond).UnixMilli(),
		}); err != nil {
			t.Fatalf("CreateEvent(%d) error = %v", value.id, err)
		}
	}

	before, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(before) error = %v", err)
	}
	if !before.ContextMeasured || before.ContextTokens != 126_000 {
		t.Fatalf("GetSession(before) context = %d, measured %t", before.ContextTokens, before.ContextMeasured)
	}

	first, err := store.CommitCompaction(t.Context(), "session-1", CommitCompactionParams{
		ExpectedGeneration:   0,
		StartEventID:         1,
		BoundaryEventID:      3,
		SegmentSummary:       " first segment ",
		StateSnapshot:        " first snapshot ",
		SourceTokens:         126_000,
		EstimatedTokensAfter: 32_000,
		ModelID:              " gpt-5 ",
	})
	if err != nil {
		t.Fatalf("CommitCompaction(first) error = %v", err)
	}
	if first.ID == 0 || first.Generation != 1 || first.PreviousCompactionID != 0 ||
		first.StartEventID != 1 || first.BoundaryEventID != 3 || first.SegmentSummary != "first segment" ||
		first.StateSnapshot != "first snapshot" || first.ModelID != "gpt-5" {
		t.Fatalf("CommitCompaction(first) = %+v", first)
	}

	current, err := store.GetCurrentCompaction(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetCurrentCompaction(first) error = %v", err)
	}
	if current == nil || *current != first {
		t.Fatalf("GetCurrentCompaction(first) = %+v, want %+v", current, first)
	}
	afterFirst, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(after first) error = %v", err)
	}
	if afterFirst.CompactionGeneration != 1 || afterFirst.CurrentCompactionID != first.ID ||
		afterFirst.ContextMeasured || afterFirst.ContextTokens != 0 || afterFirst.EventCount != 2 {
		t.Fatalf("GetSession(after first) = %+v", afterFirst)
	}
	active, err := adkSession.ListEvents(t.Context())
	if err != nil {
		t.Fatalf("ListEvents(after first) error = %v", err)
	}
	archived, err := adkSession.ListArchivedEvents(t.Context())
	if err != nil {
		t.Fatalf("ListArchivedEvents(after first) error = %v", err)
	}
	if len(active) != 2 || active[0].EventID != 3 || len(archived) != 2 || archived[0].EventID != 1 {
		t.Fatalf("events after first compaction: active=%+v archived=%+v", active, archived)
	}

	if err := adkSession.CreateEvent(t.Context(), &event.Event{
		EventID: 5, TurnID: "turn-3", Role: string(model.RoleAssistant), Content: "new answer",
		PromptTokens: 40_000, CompletionTokens: 2_000,
		CreatedAt: createdAt.Add(5 * time.Millisecond).UnixMilli(),
		UpdatedAt: createdAt.Add(5 * time.Millisecond).UnixMilli(),
	}); err != nil {
		t.Fatalf("CreateEvent(5) error = %v", err)
	}
	afterMeasurement, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(after measurement) error = %v", err)
	}
	if !afterMeasurement.ContextMeasured || afterMeasurement.ContextTokens != 42_000 {
		t.Fatalf("GetSession(after measurement) context = %d, measured %t", afterMeasurement.ContextTokens, afterMeasurement.ContextMeasured)
	}

	if _, err := store.CommitCompaction(t.Context(), "session-1", CommitCompactionParams{
		ExpectedGeneration: 0, StartEventID: 3, BoundaryEventID: 5,
		SegmentSummary: "stale", StateSnapshot: "stale", ModelID: "gpt-5",
	}); !errors.Is(err, ErrCompactionConflict) {
		t.Fatalf("CommitCompaction(stale) error = %v, want ErrCompactionConflict", err)
	}
	if _, err := store.CommitCompaction(t.Context(), "session-1", CommitCompactionParams{
		ExpectedGeneration: 1, StartEventID: 3, BoundaryEventID: 99,
		SegmentSummary: "invalid", StateSnapshot: "invalid", ModelID: "gpt-5",
	}); err == nil {
		t.Fatal("CommitCompaction(invalid boundary) error = nil")
	}
	current, err = store.GetCurrentCompaction(t.Context(), "session-1")
	if err != nil || current == nil || current.ID != first.ID {
		t.Fatalf("GetCurrentCompaction(after rejected commits) = %+v, %v", current, err)
	}

	store.now = func() time.Time { return createdAt.Add(2 * time.Hour) }
	second, err := store.CommitCompaction(t.Context(), "session-1", CommitCompactionParams{
		ExpectedGeneration:   1,
		StartEventID:         3,
		BoundaryEventID:      5,
		SegmentSummary:       "second segment",
		StateSnapshot:        "second snapshot",
		SourceTokens:         42_000,
		EstimatedTokensAfter: 12_000,
		ModelID:              "gpt-5",
	})
	if err != nil {
		t.Fatalf("CommitCompaction(second) error = %v", err)
	}
	if second.Generation != 2 || second.PreviousCompactionID != first.ID {
		t.Fatalf("CommitCompaction(second) = %+v", second)
	}
	current, err = store.GetCurrentCompaction(t.Context(), "session-1")
	if err != nil || current == nil || *current != second {
		t.Fatalf("GetCurrentCompaction(second) = %+v, %v; want %+v", current, err, second)
	}
	generations, err := store.ListCompactions(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("ListCompactions() error = %v", err)
	}
	if len(generations) != 2 || generations[0] != first || generations[1] != second {
		t.Fatalf("ListCompactions() = %+v, want [%+v, %+v]", generations, first, second)
	}

	if err := store.DeleteSession(t.Context(), "session-1"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	var remaining int
	if err := store.db.GetContext(t.Context(), &remaining, `
		SELECT COUNT(*) FROM koda_session_compactions
		WHERE session_id = $1 AND deleted_at = 0
	`, "session-1"); err != nil {
		t.Fatalf("count active compactions after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("active compactions after delete = %d, want 0", remaining)
	}
}

func TestCommitCompactionCanArchiveAllActiveEvents(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	adkSession, err := store.EnsureADKSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("EnsureADKSession() error = %v", err)
	}
	if err := adkSession.CreateEvent(t.Context(), &event.Event{
		EventID: 1, TurnID: "turn-1", Role: string(model.RoleUser), Content: "large turn",
		CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}
	if _, err := store.CommitCompaction(t.Context(), "session-1", CommitCompactionParams{
		StartEventID: 1, BoundaryEventID: 0, SegmentSummary: "segment",
		StateSnapshot: "snapshot", ModelID: "gpt-5",
	}); err != nil {
		t.Fatalf("CommitCompaction() error = %v", err)
	}
	active, err := adkSession.ListEvents(t.Context())
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("ListEvents() = %+v, want empty", active)
	}
}
