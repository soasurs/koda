package agent

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/soasurs/adk/model"
	adksession "github.com/soasurs/adk/session"
	"github.com/soasurs/adk/session/memory"

	"github.com/soasurs/koda/internal/permission"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
	"github.com/soasurs/koda/internal/tools"
)

func TestFactoryCachesPerSessionConfigurationAndProviderRevision(t *testing.T) {
	factory, registry := newTestFactory(t)
	var modelCount atomic.Int32
	var efforts []string
	factory.newModel = func(_ context.Context, _ provider.Provider, modelID, effort string) (model.LLM, error) {
		efforts = append(efforts, effort)
		return fakeModel{name: fmt.Sprintf("%s-%d", modelID, modelCount.Add(1))}, nil
	}

	workspace := t.TempDir()
	session := testSession(workspace)
	first, err := factory.Runner(t.Context(), session, ModeBuild)
	if err != nil {
		t.Fatalf("Runner(first) error = %v", err)
	}
	second, err := factory.Runner(t.Context(), session, ModeBuild)
	if err != nil {
		t.Fatalf("Runner(second) error = %v", err)
	}
	if first != second || modelCount.Load() != 1 || len(efforts) != 1 || efforts[0] != "high" {
		t.Fatalf("cached runners = %p, %p; model count = %d, efforts = %v", first, second, modelCount.Load(), efforts)
	}

	plan, err := factory.Runner(t.Context(), session, ModePlan)
	if err != nil {
		t.Fatalf("Runner(plan) error = %v", err)
	}
	if plan == first || modelCount.Load() != 2 {
		t.Fatalf("plan runner = %p, build runner = %p, model count = %d", plan, first, modelCount.Load())
	}

	updatedWorkspace := t.TempDir()
	updated := session
	updated.Workdir = updatedWorkspace
	workspaceRunner, err := factory.Runner(t.Context(), updated, ModeBuild)
	if err != nil {
		t.Fatalf("Runner(updated workspace) error = %v", err)
	}
	if workspaceRunner == first || modelCount.Load() != 3 {
		t.Fatalf("workspace runner = %p, first = %p, model count = %d", workspaceRunner, first, modelCount.Load())
	}

	if err := os.WriteFile(filepath.Join(updatedWorkspace, "AGENTS.md"), []byte("child instruction"), 0o600); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	instructionRunner, err := factory.Runner(t.Context(), updated, ModeBuild)
	if err != nil {
		t.Fatalf("Runner(updated instructions) error = %v", err)
	}
	if instructionRunner == workspaceRunner || modelCount.Load() != 4 {
		t.Fatalf("instruction runner = %p, workspace runner = %p, model count = %d", instructionRunner, workspaceRunner, modelCount.Load())
	}

	key := "replacement-key"
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID:             "test",
		Name:           "Test provider",
		Type:           provider.TypeOpenAIChatCompletions,
		BaseURL:        "https://example.test/v1",
		ModelOverrides: []provider.Model{{ID: "test-model", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}},
	}, &key); err != nil {
		t.Fatalf("Registry.Save(revision update) error = %v", err)
	}
	revisedRunner, err := factory.Runner(t.Context(), updated, ModeBuild)
	if err != nil {
		t.Fatalf("Runner(revised provider) error = %v", err)
	}
	if revisedRunner == instructionRunner || modelCount.Load() != 5 {
		t.Fatalf("revised runner = %p, previous = %p, model count = %d", revisedRunner, instructionRunner, modelCount.Load())
	}
}

func TestFactoryValidatesSessionAndModelConfiguration(t *testing.T) {
	factory, _ := newTestFactory(t)
	session := testSession(t.TempDir())

	invalidMode := Mode("invalid")
	if _, err := factory.Runner(t.Context(), session, invalidMode); err == nil {
		t.Fatal("Runner(invalid mode) error = nil")
	}

	session.FileAccess = "invalid"
	if _, err := factory.Runner(t.Context(), session, ModeBuild); err == nil {
		t.Fatal("Runner(invalid access) error = nil")
	}

	session = testSession(t.TempDir())
	session.ReasoningEffort = "max"
	if _, err := factory.Runner(t.Context(), session, ModeBuild); err == nil {
		t.Fatal("Runner(unsupported effort) error = nil")
	}
}

func TestFactoryGenerateTitleUsesEmbeddedPromptWithoutReasoning(t *testing.T) {
	factory, _ := newTestFactory(t)
	scripted := &scriptedModel{responses: []*model.LLMResponse{{
		Content:      model.Content{Role: model.RoleAssistant, Content: "`Fix flaky tests`\nExplanation"},
		FinishReason: model.FinishReasonStop,
	}}}
	var effort string
	factory.newModel = func(_ context.Context, _ provider.Provider, _ string, gotEffort string) (model.LLM, error) {
		effort = gotEffort
		return scripted, nil
	}

	input := model.Content{Role: model.RoleUser, Content: "Please fix the flaky tests"}
	title, err := factory.GenerateTitle(t.Context(), testSession(t.TempDir()), input)
	if err != nil {
		t.Fatalf("GenerateTitle() error = %v", err)
	}
	if title != "Fix flaky tests" || effort != "" {
		t.Fatalf("GenerateTitle() = %q, effort = %q", title, effort)
	}
	if len(scripted.requests) != 1 || len(scripted.requests[0].Contents) != 2 ||
		scripted.requests[0].Contents[0].Role != model.RoleSystem ||
		!strings.Contains(scripted.requests[0].Contents[0].Content, "Return only the title") ||
		scripted.requests[0].Contents[1].Content != input.Content {
		t.Fatalf("title request = %+v", scripted.requests)
	}
	if len(scripted.configs) != 1 || scripted.configs[0].MaxTokens != 64 || len(scripted.streams) != 1 || scripted.streams[0] {
		t.Fatalf("title generation options = configs %+v, streams %v", scripted.configs, scripted.streams)
	}
}

func TestLoadWorkspaceInstructionsOrdersParentBeforeChild(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("parent rule"), 0o600); err != nil {
		t.Fatalf("WriteFile(parent AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("child rule"), 0o600); err != nil {
		t.Fatalf("WriteFile(child AGENTS.md) error = %v", err)
	}
	instructions, err := LoadWorkspaceInstructions(workspace)
	if err != nil {
		t.Fatalf("LoadWorkspaceInstructions() error = %v", err)
	}
	if instructions == "" || strings.Index(instructions, "parent rule") >= strings.Index(instructions, "child rule") {
		t.Fatalf("instructions = %q, want parent before child", instructions)
	}
}

func TestToolsForMode(t *testing.T) {
	config := toolsConfig(t.TempDir())
	plan, err := toolsForMode(ModePlan, config)
	if err != nil {
		t.Fatalf("toolsForMode(plan) error = %v", err)
	}
	build, err := toolsForMode(ModeBuild, config)
	if err != nil {
		t.Fatalf("toolsForMode(build) error = %v", err)
	}
	if len(plan) != 6 || len(build) != 9 {
		t.Fatalf("tool counts = %d, %d; want 6, 9", len(plan), len(build))
	}
}

func TestFactoryRunnerPassesRunInteractionsIntoCachedTools(t *testing.T) {
	factory, _ := newTestFactory(t)
	workspace := t.TempDir()
	session := testSession(workspace)
	if _, err := factory.sessions.CreateSession(t.Context(), adksession.CreateSessionRequest{
		SessionID: session.ID,
		AppID:     "koda",
		UserID:    "local",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	scripted := &scriptedModel{responses: []*model.LLMResponse{
		{
			Content: model.Content{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{
				ID:        "call-1",
				Name:      "create_file",
				Arguments: []byte(`{"path":"created.txt","content":"created\n"}`),
			}}},
			FinishReason: model.FinishReasonToolCalls,
		},
		{
			Content:      model.Content{Role: model.RoleAssistant, Content: "done"},
			FinishReason: model.FinishReasonStop,
		},
	}}
	factory.newModel = func(context.Context, provider.Provider, string, string) (model.LLM, error) {
		return scripted, nil
	}
	runner, err := factory.Runner(t.Context(), session, ModeBuild)
	if err != nil {
		t.Fatalf("Runner() error = %v", err)
	}
	var approvals []tools.Approval
	ctx := WithRunInteractions(t.Context(), RunInteractions{
		Authorizer: authorizerFunc(func(_ context.Context, approval tools.Approval) error {
			approvals = append(approvals, approval)
			return nil
		}),
	})
	ctx = WithRunEnvironment(ctx, RunEnvironment{
		Workdir: workspace, FileAccess: session.FileAccess, ShellAccess: session.ShellAccess,
	})
	for _, err := range runner.Run(ctx, session.ID, model.Content{Content: "create a file"}) {
		if err != nil {
			t.Fatalf("Runner.Run() error = %v", err)
		}
	}
	contents, err := os.ReadFile(filepath.Join(workspace, "created.txt"))
	if err != nil {
		t.Fatalf("ReadFile(created.txt) error = %v", err)
	}
	if string(contents) != "created\n" || len(approvals) != 1 || approvals[0].ToolCallID != "call-1" || approvals[0].ToolName != "create_file" {
		t.Fatalf("contents = %q, approvals = %+v", contents, approvals)
	}
	if len(scripted.requests) != 2 {
		t.Fatalf("LLM request count = %d, want 2", len(scripted.requests))
	}
	for index, request := range scripted.requests {
		if len(request.Contents) == 0 || request.Contents[0].Role != model.RoleSystem ||
			!strings.Contains(request.Contents[0].Content, "# Role and operating principles") ||
			!strings.Contains(request.Contents[0].Content, "# Runtime environment") {
			t.Fatalf("request %d system instruction = %+v", index, request.Contents)
		}
	}
	adkSession, err := factory.sessions.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := adkSession.ListEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Role == string(model.RoleSystem) || strings.Contains(event.Content, "# Runtime environment") {
			t.Fatalf("dynamic instruction persisted in event %+v", event)
		}
	}
}

func TestFactoryRunnerRollsBackIncompleteTurn(t *testing.T) {
	factory, _ := newTestFactory(t)
	session := testSession(t.TempDir())
	if _, err := factory.sessions.CreateSession(t.Context(), adksession.CreateSessionRequest{
		SessionID: session.ID,
		AppID:     "koda",
		UserID:    "local",
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	factory.newModel = func(context.Context, provider.Provider, string, string) (model.LLM, error) {
		return fakeModel{name: "empty"}, nil
	}
	runner, err := factory.Runner(t.Context(), session, ModePlan)
	if err != nil {
		t.Fatalf("Runner() error = %v", err)
	}
	var runErr error
	ctx := WithRunEnvironment(t.Context(), RunEnvironment{
		Workdir: session.Workdir, FileAccess: session.FileAccess, ShellAccess: session.ShellAccess,
	})
	for _, err := range runner.Run(ctx, session.ID, model.Content{Content: "hello"}) {
		runErr = err
	}
	if !errors.Is(runErr, errTurnIncomplete) {
		t.Fatalf("Runner.Run() error = %v, want errTurnIncomplete", runErr)
	}
	adkSession, err := factory.sessions.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	events, err := adkSession.ListEvents(t.Context())
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("persisted events = %+v, want rollback", events)
	}
}

func newTestFactory(t *testing.T) (*Factory, *provider.Registry) {
	t.Helper()
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	key := "test-key"
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID:             "test",
		Name:           "Test provider",
		Type:           provider.TypeOpenAIChatCompletions,
		ModelOverrides: []provider.Model{{ID: "test-model", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}},
	}, &key); err != nil {
		t.Fatalf("Registry.Save() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, nil)
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	factory, err := New(Config{Registry: registry, Catalog: catalog, Sessions: memory.NewMemorySessionService()})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	return factory, registry
}

func testSession(workdir string) store.Session {
	return store.Session{
		ID:          "session-1",
		Workdir:     workdir,
		ProviderID:  "test",
		ModelID:     "test-model",
		FileAccess:  permission.FileAccessWorkspaceRead,
		ShellAccess: permission.ShellAccessApprovalRequired,
	}
}

func toolsConfig(workdir string) tools.Config {
	return tools.Config{
		Workdir:     workdir,
		FileAccess:  permission.FileAccessWorkspaceRead,
		ShellAccess: permission.ShellAccessApprovalRequired,
	}
}

type fakeModel struct {
	name string
}

func (m fakeModel) Name() string {
	return m.name
}

func (m fakeModel) GenerateContent(context.Context, *model.LLMRequest, *model.GenerateConfig, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(func(*model.LLMResponse, error) bool) {}
}

type scriptedModel struct {
	responses []*model.LLMResponse
	requests  []*model.LLMRequest
	configs   []*model.GenerateConfig
	streams   []bool
	index     atomic.Int32
}

func (m *scriptedModel) Name() string {
	return "scripted"
}

func (m *scriptedModel) GenerateContent(_ context.Context, request *model.LLMRequest, config *model.GenerateConfig, stream bool) iter.Seq2[*model.LLMResponse, error] {
	m.requests = append(m.requests, request)
	m.configs = append(m.configs, config)
	m.streams = append(m.streams, stream)
	return func(yield func(*model.LLMResponse, error) bool) {
		index := int(m.index.Add(1) - 1)
		if index < len(m.responses) {
			yield(m.responses[index], nil)
		}
	}
}
