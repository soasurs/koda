package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/session/event"

	"github.com/soasurs/koda/internal/permission"
)

func TestOpenMigratesVersionTwoStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "koda.db")
	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		t.Fatalf("open version two database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE koda_schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	for version := 1; version <= 2; version++ {
		for _, statement := range migrationSQL(version) {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("apply old migration v%d: %v", version, err)
			}
		}
		if _, err := db.Exec(`INSERT INTO koda_schema_migrations (version) VALUES ($1)`, version); err != nil {
			t.Fatalf("record old migration v%d: %v", version, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO koda_sessions (
			id, workdir, provider_id, model_id, file_access, shell_access,
			created_at, updated_at
		) VALUES ('session-1', '/workspace', 'openai', 'gpt-5',
			'workspace_write', 'approval_required', 1, 1)
	`); err != nil {
		t.Fatalf("insert old session: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close version two database: %v", err)
	}

	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open(version two) error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	got, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(migrated) error = %v", err)
	}
	if got.CompactionGeneration != 0 || got.CurrentCompactionID != 0 {
		t.Fatalf("GetSession(migrated) = %+v", got)
	}
	var applied int
	if err := store.db.GetContext(t.Context(), &applied, `
		SELECT COUNT(*) FROM koda_schema_migrations WHERE version = 3
	`); err != nil {
		t.Fatalf("find migration v3: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration v3 count = %d, want 1", applied)
	}
}

func TestOpenInitializesSchemasAndSecuresDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "koda.db")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	for _, table := range []string{
		"adk_sessions",
		"adk_events",
		"adk_schema_migrations",
		"koda_sessions",
		"koda_session_compactions",
		"koda_schema_migrations",
	} {
		var count int
		if err := store.db.GetContext(t.Context(), &count, `
			SELECT COUNT(*)
			FROM sqlite_master
			WHERE type = 'table' AND name = $1
		`, table); err != nil {
			t.Fatalf("find table %q: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d, want 1", table, count)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(database) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions = %o, want 600", got)
	}
}

func TestStoreSessionLifecycle(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	created, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID:              "session-b",
		Title:           " First session ",
		Workdir:         " /workspace/b ",
		ProviderID:      "openai",
		ModelID:         "gpt-5",
		ReasoningEffort: " high ",
		FileAccess:      permission.FileAccessWorkspaceWrite,
		ShellAccess:     permission.ShellAccessUnrestricted,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Title != "First session" || created.Workdir != "/workspace/b" ||
		created.FileAccess != permission.FileAccessWorkspaceWrite ||
		created.ShellAccess != permission.ShellAccessUnrestricted {
		t.Fatalf("CreateSession() = %+v, want normalized values", created)
	}
	if created.EventCount != 0 {
		t.Fatalf("CreateSession().EventCount = %d, want 0", created.EventCount)
	}

	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-a", Workdir: "/workspace/a", ProviderID: "anthropic", ModelID: "claude",
	}); err != nil {
		t.Fatalf("CreateSession(second) error = %v", err)
	}

	sessions, total, err := store.ListSessions(t.Context(), ListSessionsParams{})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if total != 2 || len(sessions) != 2 {
		t.Fatalf("ListSessions() = %d sessions, total %d; want 2, 2", len(sessions), total)
	}
	if sessions[0].ID != "session-a" || sessions[1].ID != "session-b" {
		t.Fatalf("ListSessions() IDs = %q, %q; want session-a, session-b", sessions[0].ID, sessions[1].ID)
	}

	now = now.Add(time.Minute)
	title := "Updated"
	workdir := "/workspace/updated"
	providerID := "openai-responses"
	modelID := "gpt-5.1"
	emptyEffort := ""
	fileAccess := permission.FileAccessUnrestricted
	shellAccess := permission.ShellAccessApprovalRequired
	updated, err := store.UpdateSession(t.Context(), "session-b", UpdateSessionParams{
		Title:           &title,
		Workdir:         &workdir,
		ProviderID:      &providerID,
		ModelID:         &modelID,
		ReasoningEffort: &emptyEffort,
		FileAccess:      &fileAccess,
		ShellAccess:     &shellAccess,
	})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if updated.Title != title || updated.Workdir != workdir || updated.ProviderID != providerID ||
		updated.ModelID != modelID || updated.ReasoningEffort != "" ||
		updated.FileAccess != fileAccess || updated.ShellAccess != shellAccess {
		t.Fatalf("UpdateSession() = %+v, want updated values", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdateSession().UpdatedAt = %v, want after %v", updated.UpdatedAt, created.UpdatedAt)
	}

	now = now.Add(time.Minute)
	archived := true
	archivedSession, err := store.UpdateSession(t.Context(), "session-b", UpdateSessionParams{Archived: &archived})
	if err != nil {
		t.Fatalf("UpdateSession(archive) error = %v", err)
	}
	if archivedSession.ArchivedAt.IsZero() {
		t.Fatalf("UpdateSession(archive).ArchivedAt is zero")
	}
	if _, total, err := store.ListSessions(t.Context(), ListSessionsParams{}); err != nil || total != 1 {
		t.Fatalf("ListSessions(active after archive) total = %d, error = %v; want 1, nil", total, err)
	}
	archivedSessions, archivedTotal, err := store.ListSessions(t.Context(), ListSessionsParams{Archived: true})
	if err != nil {
		t.Fatalf("ListSessions(archived) error = %v", err)
	}
	if archivedTotal != 1 || len(archivedSessions) != 1 || archivedSessions[0].ID != "session-b" {
		t.Fatalf("ListSessions(archived) = %+v, total %d; want session-b only", archivedSessions, archivedTotal)
	}

	now = now.Add(time.Minute)
	archived = false
	restored, err := store.UpdateSession(t.Context(), "session-b", UpdateSessionParams{Archived: &archived})
	if err != nil {
		t.Fatalf("UpdateSession(restore) error = %v", err)
	}
	if !restored.ArchivedAt.IsZero() {
		t.Fatalf("UpdateSession(restore).ArchivedAt = %v, want zero", restored.ArchivedAt)
	}

	sessions, total, err = store.ListSessions(t.Context(), ListSessionsParams{Limit: 1})
	if err != nil {
		t.Fatalf("ListSessions(updated) error = %v", err)
	}
	if total != 2 || len(sessions) != 1 || sessions[0].ID != "session-b" {
		t.Fatalf("ListSessions(updated) = %+v, total %d; want session-b first", sessions, total)
	}

	if err := store.DeleteSession(t.Context(), "session-b"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := store.GetSession(t.Context(), "session-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession(deleted) error = %v, want ErrNotFound", err)
	}
	sessions, total, err = store.ListSessions(t.Context(), ListSessionsParams{})
	if err != nil {
		t.Fatalf("ListSessions(deleted) error = %v", err)
	}
	if total != 1 || len(sessions) != 1 || sessions[0].ID != "session-a" {
		t.Fatalf("ListSessions(deleted) = %+v, total %d; want session-a only", sessions, total)
	}
}

func TestEnsureADKSessionAndDelete(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := store.EnsureADKSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("EnsureADKSession(first) error = %v", err)
	}
	second, err := store.EnsureADKSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("EnsureADKSession(second) error = %v", err)
	}
	if first.GetSessionID() != "session-1" || second.GetSessionID() != "session-1" {
		t.Fatalf("EnsureADKSession() IDs = %q, %q; want session-1", first.GetSessionID(), second.GetSessionID())
	}
	if err := first.CreateEvent(t.Context(), &event.Event{
		EventID: 1, TurnID: "turn-1", Role: string(model.RoleUser), Content: "hello",
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	if err := store.DeleteSession(t.Context(), "session-1"); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	adkSession, err := store.ADKSessionService().GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("ADKSessionService().GetSession() error = %v", err)
	}
	if adkSession != nil {
		t.Fatalf("ADK session after delete = %v, want nil", adkSession)
	}
	var activeEvents int
	if err := store.db.GetContext(t.Context(), &activeEvents, `SELECT COUNT(*) FROM adk_events WHERE session_id = $1 AND deleted_at = 0`, "session-1"); err != nil {
		t.Fatalf("count active events: %v", err)
	}
	if activeEvents != 0 {
		t.Fatalf("active events after delete = %d, want 0", activeEvents)
	}
	if _, err := store.EnsureADKSession(t.Context(), "session-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EnsureADKSession(deleted) error = %v, want ErrNotFound", err)
	}
}

func TestSessionEventCountIncludesOnlyActiveADKEvents(t *testing.T) {
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
	createdAt := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC).UnixMilli()
	if err := adkSession.CreateEvent(t.Context(), &event.Event{
		EventID:   1,
		TurnID:    "turn-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}

	got, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.EventCount != 1 {
		t.Fatalf("GetSession().EventCount = %d, want 1", got.EventCount)
	}

	if err := adkSession.ArchiveEventsBefore(t.Context(), 0); err != nil {
		t.Fatalf("ArchiveEventsBefore() error = %v", err)
	}
	got, err = store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(after archive) error = %v", err)
	}
	if got.EventCount != 0 {
		t.Fatalf("GetSession(after archive).EventCount = %d, want 0", got.EventCount)
	}
}

func TestSessionContextTokensUseLatestMeasuredEvent(t *testing.T) {
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
	createdAt := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	for _, value := range []struct {
		id               int64
		promptTokens     int64
		completionTokens int64
	}{
		{id: 1, promptTokens: 120_000, completionTokens: 5_000},
		{id: 2, promptTokens: 90_000, completionTokens: 7_000},
		{id: 3},
	} {
		if err := adkSession.CreateEvent(t.Context(), &event.Event{
			EventID:          value.id,
			TurnID:           "turn-1",
			Role:             string(model.RoleAssistant),
			Content:          "answer",
			PromptTokens:     value.promptTokens,
			CompletionTokens: value.completionTokens,
			CreatedAt:        createdAt.Add(time.Duration(value.id) * time.Millisecond).UnixMilli(),
			UpdatedAt:        createdAt.Add(time.Duration(value.id) * time.Millisecond).UnixMilli(),
		}); err != nil {
			t.Fatalf("CreateEvent(%d) error = %v", value.id, err)
		}
	}

	got, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if !got.ContextMeasured || got.ContextTokens != 97_000 {
		t.Fatalf("GetSession() context = %d, measured %t; want 97000, true", got.ContextTokens, got.ContextMeasured)
	}

	if err := adkSession.DeleteEvent(t.Context(), 2); err != nil {
		t.Fatalf("DeleteEvent() error = %v", err)
	}
	got, err = store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(after delete) error = %v", err)
	}
	if !got.ContextMeasured || got.ContextTokens != 125_000 {
		t.Fatalf("GetSession(after delete) context = %d, measured %t; want 125000, true", got.ContextTokens, got.ContextMeasured)
	}
}

func TestListEventsAndUndoLastMessage(t *testing.T) {
	store := openTestStore(t)
	createdAt := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
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
	events := []*event.Event{
		{
			EventID: 1, TurnID: "turn-1", Role: string(model.RoleUser),
			Parts:     event.Parts{{Type: model.ContentPartTypeText, Text: "first question"}},
			CreatedAt: createdAt.UnixMilli(), UpdatedAt: createdAt.UnixMilli(),
		},
		{
			EventID: 2, TurnID: "turn-1", Role: string(model.RoleAssistant), Content: "first answer",
			CreatedAt: createdAt.Add(time.Millisecond).UnixMilli(), UpdatedAt: createdAt.Add(time.Millisecond).UnixMilli(),
		},
		{
			EventID: 3, TurnID: "turn-2", Role: string(model.RoleUser),
			Parts: event.Parts{
				{Type: model.ContentPartTypeText, Text: "second question"},
				{Type: model.ContentPartTypeImageURL, ImageURL: "https://example.com/diagram.png", ImageDetail: model.ImageDetailHigh},
			},
			CreatedAt: createdAt.Add(2 * time.Millisecond).UnixMilli(), UpdatedAt: createdAt.Add(2 * time.Millisecond).UnixMilli(),
		},
		{
			EventID: 4, TurnID: "turn-2", Role: string(model.RoleAssistant), Content: "second answer",
			CreatedAt: createdAt.Add(3 * time.Millisecond).UnixMilli(), UpdatedAt: createdAt.Add(3 * time.Millisecond).UnixMilli(),
		},
	}
	for _, event := range events {
		if err := adkSession.CreateEvent(t.Context(), event); err != nil {
			t.Fatalf("CreateEvent(%d) error = %v", event.EventID, err)
		}
	}

	listed, err := store.ListEvents(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(listed) != 4 || listed[0].ID != 1 || listed[1].ID != 2 || listed[2].ID != 3 || listed[3].ID != 4 {
		t.Fatalf("ListEvents() = %+v; want all 4 events", listed)
	}

	undone, err := store.UndoLastMessage(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("UndoLastMessage() error = %v", err)
	}
	if undone.TurnID != "turn-2" || undone.DeletedEventCount != 2 || len(undone.Input.Parts) != 2 ||
		undone.Input.Parts[0].Text != "second question" || undone.Input.Parts[1].ImageURL != "https://example.com/diagram.png" {
		t.Fatalf("UndoLastMessage() = %+v", undone)
	}

	listed, err = store.ListEvents(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("ListEvents(after undo) error = %v", err)
	}
	if len(listed) != 2 || listed[0].TurnID != "turn-1" || listed[1].TurnID != "turn-1" {
		t.Fatalf("ListEvents(after undo) = %+v; want turn-1 only", listed)
	}
	updated, err := store.GetSession(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("GetSession(after undo) error = %v", err)
	}
	if updated.EventCount != 2 || !updated.UpdatedAt.Equal(createdAt.Add(time.Hour)) {
		t.Fatalf("GetSession(after undo) = %+v", updated)
	}

	if _, err := store.UndoLastMessage(t.Context(), "session-1"); err != nil {
		t.Fatalf("UndoLastMessage(second) error = %v", err)
	}
	undone, err = store.UndoLastMessage(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("UndoLastMessage(empty) error = %v", err)
	}
	if undone.TurnID != "" || undone.DeletedEventCount != 0 || len(undone.Input.Parts) != 0 {
		t.Fatalf("UndoLastMessage(empty) = %+v, want empty result", undone)
	}
}

func TestEventHistoryValidationAndEmptyLedger(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	events, err := store.ListEvents(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("ListEvents(empty ledger) error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("ListEvents(empty ledger) = %+v", events)
	}
	if _, err := store.ListEvents(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListEvents(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := store.UndoLastMessage(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UndoLastMessage(missing) error = %v, want ErrNotFound", err)
	}
}

func TestSessionValidation(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	}); err == nil {
		t.Fatal("CreateSession(empty ID) error = nil, want error")
	}
	if _, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "", ProviderID: "openai", ModelID: "gpt-5",
	}); err == nil {
		t.Fatal("CreateSession(empty workdir) error = nil, want error")
	}
	if _, _, err := store.ListSessions(t.Context(), ListSessionsParams{Offset: -1}); err == nil {
		t.Fatal("ListSessions(negative offset) error = nil, want error")
	}
}

func TestRunLockerHonorsContextAndSeparatesSessions(t *testing.T) {
	store := openTestStore(t)
	unlock, err := store.LockRun(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("LockRun(first) error = %v", err)
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.LockRun(ctx, "session-1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("LockRun(same session) error = %v, want context deadline exceeded", err)
	}
	otherUnlock, err := store.LockRun(t.Context(), "session-2")
	if err != nil {
		t.Fatalf("LockRun(other session) error = %v", err)
	}
	otherUnlock()
}

func TestLockRunContextIsReentrantAndRollbackTurnRestoresSession(t *testing.T) {
	store := openTestStore(t)
	createdAt := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return createdAt }
	created, err := store.CreateSession(t.Context(), CreateSessionParams{
		ID: "session-1", Workdir: "/workspace", ProviderID: "openai", ModelID: "gpt-5",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	lockedCtx, unlock, err := store.LockRunContext(t.Context(), "session-1")
	if err != nil {
		t.Fatalf("LockRunContext() error = %v", err)
	}
	defer unlock()
	nestedUnlock, err := store.LockRun(lockedCtx, "session-1")
	if err != nil {
		t.Fatalf("LockRun(reentrant) error = %v", err)
	}
	nestedUnlock()
	adkSession, err := store.EnsureADKSession(lockedCtx, "session-1")
	if err != nil {
		t.Fatalf("EnsureADKSession() error = %v", err)
	}
	if err := adkSession.CreateEvent(lockedCtx, &event.Event{
		EventID: 1, TurnID: "turn-1", Role: string(model.RoleUser), Content: "hello",
		CreatedAt: createdAt.UnixMilli(), UpdatedAt: createdAt.UnixMilli(),
	}); err != nil {
		t.Fatalf("CreateEvent() error = %v", err)
	}
	store.now = func() time.Time { return createdAt.Add(time.Hour) }
	title := "Generated title"
	if _, err := store.UpdateSession(lockedCtx, "session-1", UpdateSessionParams{Title: &title}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if err := store.RollbackTurn(lockedCtx, "session-1", "turn-1", created); err != nil {
		t.Fatalf("RollbackTurn() error = %v", err)
	}
	listed, err := store.ListEvents(lockedCtx, "session-1")
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListEvents() = %+v; want empty", listed)
	}
	got, err := store.GetSession(lockedCtx, "session-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.Title != created.Title || !got.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("session after rollback = %+v, want title %q and UpdatedAt %v", got, created.Title, created.UpdatedAt)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "koda.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}
