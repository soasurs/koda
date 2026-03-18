package agent

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/soasurs/adk/agent/llmagent"
	amodel "github.com/soasurs/adk/model"
	"github.com/soasurs/adk/session/memory"
	"github.com/soasurs/adk/session/message"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/config"
)

func TestNormalizeSessionTitle(t *testing.T) {
	t.Parallel()

	long := "this title is intentionally much longer than seventy two characters so it should truncate cleanly"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank", input: "   ", want: "New Session"},
		{name: "trim and collapse whitespace", input: "  fix   config   loading  ", want: "fix config loading"},
		{name: "truncate", input: long, want: long[:69] + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSessionTitle(tt.input); got != tt.want {
				t.Fatalf("normalizeSessionTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMemorySessionCatalogMaintainsTitlesAndOrdering(t *testing.T) {
	ctx := context.Background()
	catalog := newMemorySessionCatalog()

	if err := catalog.CreateSession(ctx, SessionMeta{SessionID: 1, Title: "", WorkDir: "/tmp/a", CreatedAt: 100, UpdatedAt: 100}); err != nil {
		t.Fatalf("CreateSession(1) error = %v", err)
	}
	if err := catalog.CreateSession(ctx, SessionMeta{SessionID: 2, Title: "Second", WorkDir: "/tmp/b", CreatedAt: 200, UpdatedAt: 200}); err != nil {
		t.Fatalf("CreateSession(2) error = %v", err)
	}
	if err := catalog.TouchSession(ctx, 1, "  first   session  "); err != nil {
		t.Fatalf("TouchSession() error = %v", err)
	}

	sessions, err := catalog.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(ListSessions()) = %d, want 2", len(sessions))
	}
	if sessions[0].SessionID != 1 {
		t.Fatalf("first session ID = %d, want 1 after touch ordering", sessions[0].SessionID)
	}
	if sessions[0].Title != "first session" {
		t.Fatalf("first session title = %q, want %q", sessions[0].Title, "first session")
	}
	if sessions[1].Title != "Second" {
		t.Fatalf("second session title = %q, want %q", sessions[1].Title, "Second")
	}
}

func TestUndoLastUserMessageDeletesTail(t *testing.T) {
	ctx := context.Background()
	sessionSvc := memory.NewMemorySessionService()
	sess, err := sessionSvc.CreateSession(ctx, 42)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	for _, msg := range []*message.Message{
		{MessageID: 1, Role: string(amodel.RoleUser), Content: "first"},
		{MessageID: 2, Role: string(amodel.RoleAssistant), Content: "reply"},
		{MessageID: 3, Role: string(amodel.RoleUser), Content: "second"},
		{MessageID: 4, Role: string(amodel.RoleAssistant), Content: "follow-up"},
	} {
		if err := sess.CreateMessage(ctx, msg); err != nil {
			t.Fatalf("CreateMessage(%d) error = %v", msg.MessageID, err)
		}
	}

	catalog := newMemorySessionCatalog()
	if err := catalog.CreateSession(ctx, SessionMeta{SessionID: 42, Title: "Undo session", CreatedAt: 100, UpdatedAt: 100}); err != nil {
		t.Fatalf("CreateSession(catalog) error = %v", err)
	}

	rt := &Runtime{sessionSvc: sessionSvc, catalog: catalog}
	deleted, content, err := rt.UndoLastUserMessage(ctx, 42)
	if err != nil {
		t.Fatalf("UndoLastUserMessage() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if content != "second" {
		t.Fatalf("content = %q, want %q", content, "second")
	}

	msgs, err := sess.ListMessages(ctx)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(ListMessages()) = %d, want 2", len(msgs))
	}
	if msgs[0].MessageID != 1 || msgs[1].MessageID != 2 {
		t.Fatalf("remaining message IDs = [%d %d], want [1 2]", msgs[0].MessageID, msgs[1].MessageID)
	}

	sessions, err := catalog.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].UpdatedAt <= 100 {
		t.Fatalf("session metadata = %#v, want updated timestamp after undo", sessions)
	}
}

func TestUndoLastUserMessageNoUserInput(t *testing.T) {
	ctx := context.Background()
	sessionSvc := memory.NewMemorySessionService()
	sess, err := sessionSvc.CreateSession(ctx, 7)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := sess.CreateMessage(ctx, &message.Message{MessageID: 1, Role: string(amodel.RoleAssistant), Content: "reply"}); err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}

	rt := &Runtime{sessionSvc: sessionSvc}
	deleted, content, err := rt.UndoLastUserMessage(ctx, 7)
	if err != nil {
		t.Fatalf("UndoLastUserMessage() error = %v", err)
	}
	if deleted != 0 || content != "" {
		t.Fatalf("UndoLastUserMessage() = (%d, %q), want (0, \"\")", deleted, content)
	}
}

func TestAvailableProvidersAndModels(t *testing.T) {
	rt := &Runtime{
		cfg: config.Config{
			Provider: "custom-openai",
			Stored: config.StoredConfig{
				Providers: []config.ProviderConfig{
					{
						Name: "custom-openai",
						Models: []config.ModelConfig{
							{ID: "configured-a"},
							{ID: "configured-b"},
						},
					},
				},
			},
		},
		modelName: "active-model",
	}

	gotProviders := rt.AvailableProviders()
	wantProviders := []ProviderOption{
		{Value: "anthropic", Label: "Anthropic"},
		{Value: "openai", Label: "OpenAI"},
		{Value: "gemini", Label: "Google"},
		{Value: "custom-openai", Label: "custom-openai"},
	}
	if !reflect.DeepEqual(gotProviders, wantProviders) {
		t.Fatalf("AvailableProviders() = %#v, want %#v", gotProviders, wantProviders)
	}

	gotConfiguredModels := rt.AvailableModels()
	wantConfiguredModels := []string{"active-model", "configured-a", "configured-b"}
	if !reflect.DeepEqual(gotConfiguredModels, wantConfiguredModels) {
		t.Fatalf("AvailableModels() configured = %#v, want %#v", gotConfiguredModels, wantConfiguredModels)
	}

	rt.models = []string{"live-a", "live-b"}
	gotLiveModels := rt.AvailableModels()
	wantLiveModels := []string{"active-model", "live-a", "live-b"}
	if !reflect.DeepEqual(gotLiveModels, wantLiveModels) {
		t.Fatalf("AvailableModels() live = %#v, want %#v", gotLiveModels, wantLiveModels)
	}
}

func TestThinkingGenerateConfig(t *testing.T) {
	tests := []struct {
		name     string
		thinking ThinkingLevel
		enabled  bool
		effort   amodel.ReasoningEffort
	}{
		{name: "off", thinking: ThinkingOff, enabled: false, effort: amodel.ReasoningEffortNone},
		{name: "low", thinking: ThinkingLow, enabled: true, effort: amodel.ReasoningEffortLow},
		{name: "medium", thinking: ThinkingMedium, enabled: true, effort: amodel.ReasoningEffortMedium},
		{name: "high", thinking: ThinkingHigh, enabled: true, effort: amodel.ReasoningEffortHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := (&Runtime{thinking: tt.thinking}).thinkingGenerateConfig()
			if cfg.EnableThinking == nil || *cfg.EnableThinking != tt.enabled {
				t.Fatalf("EnableThinking = %v, want %v", cfg.EnableThinking, tt.enabled)
			}
			if cfg.ReasoningEffort != tt.effort {
				t.Fatalf("ReasoningEffort = %v, want %v", cfg.ReasoningEffort, tt.effort)
			}
		})
	}
}

func TestBeforeToolCallBlocksMutatingToolsInSafeMode(t *testing.T) {
	rt := &Runtime{cfg: config.Config{SafeMode: true}}
	ctx := WithToolConfirmation(context.Background(), func(ctx context.Context, request ToolConfirmationRequest) error {
		if request.ToolName != "write_file" {
			t.Fatalf("request.ToolName = %q, want %q", request.ToolName, "write_file")
		}
		if request.Summary != "write internal/config/config.go" {
			t.Fatalf("request.Summary = %q, want %q", request.Summary, "write internal/config/config.go")
		}
		return fmt.Errorf("rejected")
	})

	result, err := rt.beforeToolCall(ctx, &llmagent.ToolCall{
		Definition: tool.Definition{Name: "write_file"},
		Request: amodel.ToolCall{
			ID:        "tc-1",
			Name:      "write_file",
			Arguments: `{"path":"internal/config/config.go","content":"package config"}`,
		},
	})
	if err != nil {
		t.Fatalf("beforeToolCall() error = %v", err)
	}
	if result == nil {
		t.Fatal("beforeToolCall() result = nil, want blocked result")
	}
	if result.Message.Content != "blocked by safe mode: rejected" {
		t.Fatalf("result.Message.Content = %q, want blocked message", result.Message.Content)
	}
}
