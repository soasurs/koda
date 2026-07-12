package server

import (
	"errors"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestSessionHandlers(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }

	workdir := t.TempDir()
	resolvedWorkdir, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		t.Fatalf("EvalSymlinks(workdir): %v", err)
	}
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir:         workdir,
		ProviderId:      "openai-responses",
		ModelId:         "gpt-5.6",
		ReasoningEffort: "max",
		FileAccess:      v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE,
		ShellAccess:     v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Session.Id != "session-1" || created.Session.Workdir != resolvedWorkdir ||
		created.Session.ProviderId != "openai-responses" || created.Session.ModelId != "gpt-5.6" ||
		created.Session.ReasoningEffort != "max" ||
		created.Session.FileAccess != v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE ||
		created.Session.ShellAccess != v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED ||
		created.Session.CreatedAt == 0 || created.Session.UpdatedAt == 0 {
		t.Fatalf("CreateSession() = %+v", created.Session)
	}

	got, err := client.GetSession(t.Context(), &v1.GetSessionRequest{SessionId: "session-1"})
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.Session.Id != created.Session.Id || got.Session.EventCount != 0 {
		t.Fatalf("GetSession() = %+v", got.Session)
	}

	title := "DeepSeek session"
	updatedWorkdir := t.TempDir()
	resolvedUpdatedWorkdir, err := filepath.EvalSymlinks(updatedWorkdir)
	if err != nil {
		t.Fatalf("EvalSymlinks(updated workdir): %v", err)
	}
	providerID := "deepseek"
	modelID := "deepseek-v4-pro"
	reasoningEffort := "max"
	fileAccess := v1.FileAccess_FILE_ACCESS_UNRESTRICTED
	shellAccess := v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED
	updated, err := client.UpdateSession(t.Context(), &v1.UpdateSessionRequest{
		SessionId:       "session-1",
		Title:           &title,
		Workdir:         &updatedWorkdir,
		ProviderId:      &providerID,
		ModelId:         &modelID,
		ReasoningEffort: &reasoningEffort,
		FileAccess:      &fileAccess,
		ShellAccess:     &shellAccess,
	})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if updated.Session.Title != title || updated.Session.Workdir != resolvedUpdatedWorkdir ||
		updated.Session.ProviderId != providerID || updated.Session.ModelId != modelID ||
		updated.Session.ReasoningEffort != reasoningEffort ||
		updated.Session.FileAccess != fileAccess || updated.Session.ShellAccess != shellAccess {
		t.Fatalf("UpdateSession() = %+v", updated.Session)
	}

	listed, err := client.ListSessions(t.Context(), &v1.ListSessionsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if listed.Total != 1 || len(listed.Sessions) != 1 || listed.Sessions[0].Id != "session-1" {
		t.Fatalf("ListSessions() = %+v", listed)
	}

	if _, err := client.DeleteSession(t.Context(), &v1.DeleteSessionRequest{SessionId: "session-1"}); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := client.GetSession(t.Context(), &v1.GetSessionRequest{SessionId: "session-1"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetSession(deleted) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestSessionHandlersValidateConfiguration(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	workdir := t.TempDir()

	if _, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: workdir, ProviderId: "missing", ModelId: "model",
	}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("CreateSession(missing provider) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: workdir, ProviderId: "openai-responses", ModelId: "missing-model",
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(missing model) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: workdir, ProviderId: "openai-responses", ModelId: "gpt-5.6", ReasoningEffort: "ultra",
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(unsupported effort) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: "missing-directory", ProviderId: "openai-responses", ModelId: "gpt-5.6",
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(missing workdir) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: workdir, ProviderId: "openai-responses", ModelId: "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("CreateSession(valid) error = %v", err)
	}
	providerID := "deepseek"
	if _, err := client.UpdateSession(t.Context(), &v1.UpdateSessionRequest{
		SessionId:  created.Session.Id,
		ProviderId: &providerID,
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateSession(incompatible model) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	got, err := client.GetSession(t.Context(), &v1.GetSessionRequest{SessionId: created.Session.Id})
	if err != nil {
		t.Fatalf("GetSession(after failed update) error = %v", err)
	}
	if got.Session.ProviderId != "openai-responses" || got.Session.ModelId != "gpt-5.6" {
		t.Fatalf("failed update changed session: %+v", got.Session)
	}
	if _, err := client.ListSessions(t.Context(), &v1.ListSessionsRequest{Limit: -1}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListSessions(negative limit) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.DeleteSession(t.Context(), &v1.DeleteSessionRequest{SessionId: "missing"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("DeleteSession(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestCreateSessionMapsIDGeneratorFailure(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "", errors.New("entropy unavailable") }

	if _, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "openai-responses", ModelId: "gpt-5.6",
	}); connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("CreateSession(ID failure) code = %v, want internal; error = %v", connect.CodeOf(err), err)
	}
}

func TestCreateSessionDefaultsPermissions(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "openai-responses", ModelId: "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Session.FileAccess != v1.FileAccess_FILE_ACCESS_WORKSPACE_READ ||
		created.Session.ShellAccess != v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED {
		t.Fatalf("CreateSession() permissions = %v, %v", created.Session.FileAccess, created.Session.ShellAccess)
	}
}

func TestSessionRemainsEditableAfterProviderDelete(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	if _, err := client.SaveProvider(t.Context(), &v1.SaveProviderRequest{
		Id:      "custom",
		Name:    "Custom",
		Type:    v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES,
		BaseUrl: "https://models.example/v1",
		ModelOverrides: []*v1.Model{{
			Id: "private-model",
		}},
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "custom", ModelId: "private-model",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := client.DeleteProvider(t.Context(), &v1.DeleteProviderRequest{ProviderId: "custom"}); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	title := "Needs reconfiguration"
	updated, err := client.UpdateSession(t.Context(), &v1.UpdateSessionRequest{
		SessionId: created.Session.Id,
		Title:     &title,
	})
	if err != nil {
		t.Fatalf("UpdateSession(title only) error = %v", err)
	}
	if updated.Session.Title != title || updated.Session.ProviderId != "custom" || updated.Session.ModelId != "private-model" {
		t.Fatalf("UpdateSession(title only) = %+v", updated.Session)
	}
}
