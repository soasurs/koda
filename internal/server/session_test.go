package server

import (
	"errors"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

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
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir:         new(workdir),
		ProviderId:      new("openai-responses"),
		ModelId:         new("gpt-5.6"),
		ReasoningEffort: new("max"),
		FileAccess:      v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE.Enum(),
		ShellAccess:     v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED.Enum(),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	session := created.GetSession()
	if session.GetId() != "session-1" || session.GetWorkdir() != resolvedWorkdir ||
		session.GetProviderId() != "openai-responses" || session.GetModelId() != "gpt-5.6" ||
		session.GetReasoningEffort() != "max" ||
		session.GetFileAccess() != v1.FileAccess_FILE_ACCESS_WORKSPACE_WRITE ||
		session.GetShellAccess() != v1.ShellAccess_SHELL_ACCESS_UNRESTRICTED ||
		session.GetCreatedAt() == 0 || session.GetUpdatedAt() == 0 {
		t.Fatalf("CreateSession() = %+v", session)
	}

	got, err := client.GetSession(t.Context(), v1.GetSessionRequest_builder{SessionId: new("session-1")}.Build())
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	gotSession := got.GetSession()
	if gotSession.GetId() != session.GetId() || gotSession.GetEventCount() != 0 {
		t.Fatalf("GetSession() = %+v", gotSession)
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
	updated, err := client.UpdateSession(t.Context(), v1.UpdateSessionRequest_builder{
		SessionId:       new("session-1"),
		Title:           new(title),
		Workdir:         new(updatedWorkdir),
		ProviderId:      new(providerID),
		ModelId:         new(modelID),
		ReasoningEffort: new(reasoningEffort),
		FileAccess:      v1.FileAccess_FILE_ACCESS_UNRESTRICTED.Enum(),
		ShellAccess:     v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED.Enum(),
	}.Build())
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	updatedSession := updated.GetSession()
	if updatedSession.GetTitle() != title || updatedSession.GetWorkdir() != resolvedUpdatedWorkdir ||
		updatedSession.GetProviderId() != providerID || updatedSession.GetModelId() != modelID ||
		updatedSession.GetReasoningEffort() != reasoningEffort ||
		updatedSession.GetFileAccess() != v1.FileAccess_FILE_ACCESS_UNRESTRICTED || updatedSession.GetShellAccess() != v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED {
		t.Fatalf("UpdateSession() = %+v", updatedSession)
	}

	listed, err := client.ListSessions(t.Context(), v1.ListSessionsRequest_builder{Limit: proto.Int32(1)}.Build())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if listed.GetTotal() != 1 || len(listed.GetSessions()) != 1 || listed.GetSessions()[0].GetId() != "session-1" {
		t.Fatalf("ListSessions() = %+v", listed)
	}

	if _, err := client.DeleteSession(t.Context(), v1.DeleteSessionRequest_builder{SessionId: new("session-1")}.Build()); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if _, err := client.GetSession(t.Context(), v1.GetSessionRequest_builder{SessionId: new("session-1")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetSession(deleted) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestSessionHandlersValidateConfiguration(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	workdir := t.TempDir()

	if _, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(workdir), ProviderId: new("missing"), ModelId: new("model"),
	}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("CreateSession(missing provider) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(workdir), ProviderId: new("openai-responses"), ModelId: new("missing-model"),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(missing model) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(workdir), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"), ReasoningEffort: new("ultra"),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(unsupported effort) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new("missing-directory"), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateSession(missing workdir) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(workdir), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession(valid) error = %v", err)
	}
	createdSessionID := created.GetSession().GetId()
	if _, err := client.UpdateSession(t.Context(), v1.UpdateSessionRequest_builder{
		SessionId:  new(createdSessionID),
		ProviderId: new("deepseek"),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("UpdateSession(incompatible model) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	got, err := client.GetSession(t.Context(), v1.GetSessionRequest_builder{SessionId: new(createdSessionID)}.Build())
	if err != nil {
		t.Fatalf("GetSession(after failed update) error = %v", err)
	}
	gotSession := got.GetSession()
	if gotSession.GetProviderId() != "openai-responses" || gotSession.GetModelId() != "gpt-5.6" {
		t.Fatalf("failed update changed session: %+v", gotSession)
	}
	if _, err := client.ListSessions(t.Context(), v1.ListSessionsRequest_builder{Limit: proto.Int32(-1)}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListSessions(negative limit) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.DeleteSession(t.Context(), v1.DeleteSessionRequest_builder{SessionId: new("missing")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("DeleteSession(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestCreateSessionMapsIDGeneratorFailure(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "", errors.New("entropy unavailable") }

	if _, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build()); connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("CreateSession(ID failure) code = %v, want internal; error = %v", connect.CodeOf(err), err)
	}
}

func TestCreateSessionDefaultsPermissions(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	session := created.GetSession()
	if session.GetFileAccess() != v1.FileAccess_FILE_ACCESS_WORKSPACE_READ ||
		session.GetShellAccess() != v1.ShellAccess_SHELL_ACCESS_APPROVAL_REQUIRED {
		t.Fatalf("CreateSession() permissions = %v, %v", session.GetFileAccess(), session.GetShellAccess())
	}
}

func TestSessionRemainsEditableAfterProviderDelete(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	if _, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id:      new("custom"),
		Name:    new("Custom"),
		Type:    v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES.Enum(),
		BaseUrl: new("https://models.example/v1"),
		ModelOverrides: []*v1.Model{v1.Model_builder{
			Id: new("private-model"),
		}.Build()},
	}.Build()); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("custom"), ModelId: new("private-model"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	createdSessionID := created.GetSession().GetId()
	if _, err := client.DeleteProvider(t.Context(), v1.DeleteProviderRequest_builder{ProviderId: new("custom")}.Build()); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	title := "Needs reconfiguration"
	updated, err := client.UpdateSession(t.Context(), v1.UpdateSessionRequest_builder{
		SessionId: new(createdSessionID),
		Title:     new(title),
	}.Build())
	if err != nil {
		t.Fatalf("UpdateSession(title only) error = %v", err)
	}
	updatedSession := updated.GetSession()
	if updatedSession.GetTitle() != title || updatedSession.GetProviderId() != "custom" || updatedSession.GetModelId() != "private-model" {
		t.Fatalf("UpdateSession(title only) = %+v", updatedSession)
	}
}
