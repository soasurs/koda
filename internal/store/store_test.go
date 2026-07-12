package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/soasurs/adk/session/event"
)

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
		SafeMode:        true,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Title != "First session" || created.Workdir != "/workspace/b" || !created.SafeMode {
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
	safeMode := false
	updated, err := store.UpdateSession(t.Context(), "session-b", UpdateSessionParams{
		Title:           &title,
		Workdir:         &workdir,
		ProviderID:      &providerID,
		ModelID:         &modelID,
		ReasoningEffort: &emptyEffort,
		SafeMode:        &safeMode,
	})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if updated.Title != title || updated.Workdir != workdir || updated.ProviderID != providerID ||
		updated.ModelID != modelID || updated.ReasoningEffort != "" || updated.SafeMode {
		t.Fatalf("UpdateSession() = %+v, want updated values", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("UpdateSession().UpdatedAt = %v, want after %v", updated.UpdatedAt, created.UpdatedAt)
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
