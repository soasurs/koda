package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

func TestIntegrationRunApprovalPersistsAndRestoresHistory(t *testing.T) {
	workspace := t.TempDir()
	upstream := newOpenAIStub(t, []openAIReply{
		{toolName: "create_file", toolArguments: `{"path":"created.txt","content":"created\n"}`},
		{text: "done"},
		{text: "second"},
	})
	defer upstream.Close()

	directory := t.TempDir()
	databasePath := filepath.Join(directory, "koda.db")
	registryPath := filepath.Join(directory, "providers.json")
	client, sessionStore, stop := startIntegrationService(t, registryPath, databasePath, upstream.URL)
	created := createIntegrationSession(t, client, workspace)
	before := created.GetUpdatedAt()

	stream, err := client.Run(t.Context(), runRequest(created.GetId(), "create the file", v1.AgentMode_AGENT_MODE_BUILD))
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var approval *v1.ToolApproval
	var completed *v1.RunCompleted
	for stream.Receive() {
		frame := stream.Msg()
		if value := frame.GetApproval(); value != nil {
			approval = value
			if _, err := client.ResolveToolApproval(t.Context(), v1.ResolveToolApprovalRequest_builder{
				ApprovalId: proto.String(value.GetId()), Approved: proto.Bool(true),
			}.Build()); err != nil {
				t.Fatalf("ResolveToolApproval() error = %v", err)
			}
		}
		if value := frame.GetCompleted(); value != nil {
			completed = value
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	if approval == nil || approval.GetToolCallId() != "call-1" || completed == nil {
		t.Fatalf("Run() approval = %+v, completed = %+v", approval, completed)
	}
	contents, err := os.ReadFile(filepath.Join(workspace, "created.txt"))
	if err != nil || string(contents) != "created\n" {
		t.Fatalf("created file = %q, %v", contents, err)
	}
	history := listIntegrationEvents(t, client, created.GetId())
	if len(history) != 4 || history[len(history)-1].GetMessage().GetText() != "done" {
		t.Fatalf("history = %+v, want user/tool-call/tool-result/assistant", history)
	}
	after, err := client.GetSession(t.Context(), v1.GetSessionRequest_builder{SessionId: proto.String(created.GetId())}.Build())
	if err != nil || after.GetSession().GetUpdatedAt() <= before {
		t.Fatalf("updated session = %+v, %v; before = %d", after, err, before)
	}

	stop()
	if err := sessionStore.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	client, reopenedStore, stop := startIntegrationService(t, registryPath, databasePath, upstream.URL)
	defer stop()
	defer reopenedStore.Close()
	if got := listIntegrationEvents(t, client, created.GetId()); len(got) != len(history) {
		t.Fatalf("reopened history length = %d, want %d", len(got), len(history))
	}
	runToCompletion(t, client, runRequest(created.GetId(), "continue", v1.AgentMode_AGENT_MODE_PLAN))
	requests := upstream.Requests()
	if len(requests) != 3 || !strings.Contains(requests[2], "done") {
		t.Fatalf("provider requests = %d; final request did not contain durable history: %s", len(requests), lastString(requests))
	}
}

func TestIntegrationQuestionAndMultimodalUndo(t *testing.T) {
	upstream := newOpenAIStub(t, []openAIReply{
		{toolName: "ask_questions", toolArguments: `{"questions":[{"id":"storage","header":"Storage","prompt":"Choose storage","options":[{"id":"sqlite","label":"SQLite"}]}]}`},
		{text: "SQLite selected"},
	})
	defer upstream.Close()
	client, sessionStore, stop := startIntegrationService(t, filepath.Join(t.TempDir(), "providers.json"), filepath.Join(t.TempDir(), "koda.db"), upstream.URL)
	defer stop()
	defer sessionStore.Close()
	session := createIntegrationSession(t, client, t.TempDir())
	request := v1.RunRequest_builder{
		SessionId: proto.String(session.GetId()), Mode: v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input: v1.Input_builder{Parts: []*v1.Part{
			v1.Part_builder{Text: proto.String("choose")}.Build(),
			v1.Part_builder{Image: v1.Image_builder{Data: []byte("image"), MimeType: proto.String("image/png")}.Build()}.Build(),
		}}.Build(),
	}.Build()
	stream, err := client.Run(t.Context(), request)
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	for stream.Receive() {
		if prompt := stream.Msg().GetQuestionPrompt(); prompt != nil {
			_, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
				PromptId: proto.String(prompt.GetId()), Answers: v1.QuestionAnswers_builder{Answers: []*v1.QuestionAnswer{
					v1.QuestionAnswer_builder{QuestionId: proto.String("storage"), SelectedOptionIds: []string{"sqlite"}}.Build(),
				}}.Build(),
			}.Build())
			if err != nil {
				t.Fatalf("SubmitQuestionAnswers() error = %v", err)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	undone, err := client.UndoLastMessage(t.Context(), v1.UndoLastMessageRequest_builder{SessionId: proto.String(session.GetId())}.Build())
	if err != nil {
		t.Fatalf("UndoLastMessage() error = %v", err)
	}
	if len(undone.GetInput().GetParts()) != 2 || string(undone.GetInput().GetParts()[1].GetImage().GetData()) != "image" || len(listIntegrationEvents(t, client, session.GetId())) != 0 {
		t.Fatalf("UndoLastMessage() = %+v; history was not cleared", undone)
	}
}

func TestIntegrationCanceledApprovalLeavesNoHistory(t *testing.T) {
	upstream := newOpenAIStub(t, []openAIReply{{toolName: "create_file", toolArguments: `{"path":"never.txt","content":"no"}`}})
	defer upstream.Close()
	client, sessionStore, stop := startIntegrationService(t, filepath.Join(t.TempDir(), "providers.json"), filepath.Join(t.TempDir(), "koda.db"), upstream.URL)
	defer stop()
	defer sessionStore.Close()
	session := createIntegrationSession(t, client, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := client.Run(ctx, runRequest(session.GetId(), "create", v1.AgentMode_AGENT_MODE_BUILD))
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	for stream.Receive() {
		if stream.Msg().GetApproval() != nil {
			cancel()
			break
		}
	}
	for stream.Receive() {
	}
	if code := connect.CodeOf(stream.Err()); code != connect.CodeCanceled {
		t.Fatalf("Run() code = %v, error = %v", code, stream.Err())
	}
	eventually(t, func() bool { return len(listIntegrationEvents(t, client, session.GetId())) == 0 })
}

func TestIntegrationProviderRefreshPersistsWithoutLeakingCredential(t *testing.T) {
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"data":[{"id":"discovered-model"}]}`)
	}))
	defer upstream.Close()
	directory := t.TempDir()
	registryPath := filepath.Join(directory, "providers.json")
	registry, err := provider.Open(registryPath)
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, nil)
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	sessionStore, err := store.Open(t.Context(), filepath.Join(directory, "koda.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer sessionStore.Close()
	handler, err := NewHandler(registry, catalog, sessionStore, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	_, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{Address: "127.0.0.1:0"})
	secret := "provider-secret"
	_, err = client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id: proto.String("custom"), Name: proto.String("Custom"), Type: v1.ProviderType_PROVIDER_TYPE_OPENAI_CHAT_COMPLETIONS.Enum(),
		BaseUrl: proto.String(upstream.URL), ApiKey: proto.String(secret),
	}.Build())
	if err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	refreshed, err := client.RefreshModels(t.Context(), v1.RefreshModelsRequest_builder{ProviderId: proto.String("custom")}.Build())
	if err != nil || len(refreshed.GetModels()) != 1 || refreshed.GetModels()[0].GetId() != "discovered-model" {
		t.Fatalf("RefreshModels() = %+v, %v", refreshed, err)
	}
	if authorization != "Bearer "+secret {
		t.Fatalf("Authorization header = %q", authorization)
	}
	providers, err := client.ListProviders(t.Context(), v1.ListProvidersRequest_builder{}.Build())
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if strings.Contains(providers.String(), secret) || strings.Contains(upstream.URL, secret) {
		t.Fatal("provider credential leaked through RPC or URL")
	}
	stop()
	reopened, err := provider.Open(registryPath)
	if err != nil {
		t.Fatalf("provider.Open(reopen) error = %v", err)
	}
	reopenedCatalog, err := provider.NewCatalog(reopened, nil)
	if err != nil {
		t.Fatalf("provider.NewCatalog(reopen) error = %v", err)
	}
	models, err := reopenedCatalog.List(t.Context(), "custom")
	if err != nil || len(models.Models) != 1 || models.Models[0].ID != "discovered-model" {
		t.Fatalf("persisted catalog = %+v, %v", models, err)
	}
}

func TestIntegrationSameSessionRunsSerializeAndDifferentSessionsRunConcurrently(t *testing.T) {
	handler := newHTTPTestHandler(t)
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	started := make(chan string, 2)
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, _ v1.AgentMode) (TurnRunner, error) {
		return gatedTurnRunner{sessionID: session.ID, active: &active, maximum: &maximum, started: started, release: release}, nil
	}
	_, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{Address: "127.0.0.1:0"})
	defer stop()
	handler.newSessionID = sequentialSessionIDs("session-1", "session-2")
	first := createBuiltinSession(t, client, t.TempDir())
	second := createBuiltinSession(t, client, t.TempDir())

	firstDone := startIntegrationRun(t, client, first.GetId())
	waitString(t, started)
	waitingCtx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	waitingDone := startIntegrationRunContext(client, waitingCtx, first.GetId())
	select {
	case value := <-started:
		t.Fatalf("second same-session Run started early: %q", value)
	case <-time.After(50 * time.Millisecond):
	}
	otherDone := startIntegrationRun(t, client, second.GetId())
	waitString(t, started)
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent Runs = %d, want 2 for different sessions", maximum.Load())
	}
	waitingErr := <-waitingDone
	if connect.CodeOf(waitingErr) != connect.CodeDeadlineExceeded && connect.CodeOf(waitingErr) != connect.CodeCanceled {
		t.Fatalf("waiting Run error = %v", waitingErr)
	}
	assertSessionMutationWaitsForRun(t, func(ctx context.Context) error {
		_, err := client.UpdateSession(ctx, v1.UpdateSessionRequest_builder{
			SessionId: proto.String(first.GetId()), Title: proto.String("blocked update"),
		}.Build())
		return err
	})
	assertSessionMutationWaitsForRun(t, func(ctx context.Context) error {
		_, err := client.UndoLastMessage(ctx, v1.UndoLastMessageRequest_builder{SessionId: proto.String(first.GetId())}.Build())
		return err
	})
	assertSessionMutationWaitsForRun(t, func(ctx context.Context) error {
		_, err := client.DeleteSession(ctx, v1.DeleteSessionRequest_builder{SessionId: proto.String(first.GetId())}.Build())
		return err
	})
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Run error = %v", err)
	}
	if err := <-otherDone; err != nil {
		t.Fatalf("other-session Run error = %v", err)
	}
}

type openAIReply struct {
	text          string
	toolName      string
	toolArguments string
}

type openAIStub struct {
	*httptest.Server
	mu       sync.Mutex
	replies  []openAIReply
	requests []string
}

func newOpenAIStub(t *testing.T, replies []openAIReply) *openAIStub {
	t.Helper()
	stub := &openAIStub{replies: replies}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("ReadAll(provider request) error = %v", err)
			return
		}
		stub.mu.Lock()
		index := len(stub.requests)
		stub.requests = append(stub.requests, string(body))
		stub.mu.Unlock()
		if request.URL.Path == "/models" {
			response.Header().Set("Content-Type", "application/json")
			fmt.Fprint(response, `{"data":[{"id":"integration-model"}]}`)
			return
		}
		if index >= len(replies) {
			http.Error(response, "unexpected provider request", http.StatusInternalServerError)
			return
		}
		response.Header().Set("Content-Type", "text/event-stream")
		writer := bufio.NewWriter(response)
		reply := replies[index]
		if reply.toolName != "" {
			writeSSE(writer, fmt.Sprintf(`{"id":"chat-1","object":"chat.completion.chunk","created":1,"model":"integration-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":%q,"arguments":%q}}]},"finish_reason":null}]}`, reply.toolName, reply.toolArguments))
			writeSSE(writer, `{"id":"chat-1","object":"chat.completion.chunk","created":1,"model":"integration-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		} else {
			encoded, _ := json.Marshal(reply.text)
			writeSSE(writer, fmt.Sprintf(`{"id":"chat-1","object":"chat.completion.chunk","created":1,"model":"integration-model","choices":[{"index":0,"delta":{"content":%s},"finish_reason":null}]}`, encoded))
			writeSSE(writer, `{"id":"chat-1","object":"chat.completion.chunk","created":1,"model":"integration-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		}
		fmt.Fprint(writer, "data: [DONE]\n\n")
		writer.Flush()
	}))
	return stub
}

func (s *openAIStub) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func writeSSE(writer io.Writer, value string) { fmt.Fprintf(writer, "data: %s\n\n", value) }

func startIntegrationService(t *testing.T, registryPath, databasePath, upstreamURL string) (kodav1connect.KodaServiceClient, *store.Store, func()) {
	t.Helper()
	registry, err := provider.Open(registryPath)
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	key := "integration-key"
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID: "integration", Name: "Integration", Type: provider.TypeOpenAIChatCompletions,
		BaseURL: upstreamURL, Enabled: true, ModelOverrides: []provider.Model{{ID: "integration-model"}},
	}, &key); err != nil {
		t.Fatalf("Registry.Save() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, nil)
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	sessionStore, err := store.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	handler, err := NewHandler(registry, catalog, sessionStore, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	handler.titleGenerator = func(context.Context, store.Session, model.Content) (string, error) {
		return "Integration title", nil
	}
	_, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{Address: "127.0.0.1:0"})
	return client, sessionStore, stop
}

func createIntegrationSession(t *testing.T, client kodav1connect.KodaServiceClient, workdir string) *v1.Session {
	t.Helper()
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(workdir), ProviderId: proto.String("integration"), ModelId: proto.String("integration-model"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return created.GetSession()
}

func createBuiltinSession(t *testing.T, client kodav1connect.KodaServiceClient, workdir string) *v1.Session {
	t.Helper()
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{Workdir: proto.String(workdir), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6")}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return created.GetSession()
}

func runRequest(sessionID, text string, mode v1.AgentMode) *v1.RunRequest {
	return v1.RunRequest_builder{SessionId: proto.String(sessionID), Mode: mode.Enum(), Input: v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: proto.String(text)}.Build()}}.Build()}.Build()
}

func runToCompletion(t *testing.T, client kodav1connect.KodaServiceClient, request *v1.RunRequest) {
	t.Helper()
	stream, err := client.Run(t.Context(), request)
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func listIntegrationEvents(t *testing.T, client kodav1connect.KodaServiceClient, sessionID string) []*v1.Event {
	t.Helper()
	result, err := client.ListEvents(t.Context(), v1.ListEventsRequest_builder{SessionId: proto.String(sessionID)}.Build())
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	return result.GetEvents()
}

type gatedTurnRunner struct {
	sessionID       string
	active, maximum *atomic.Int32
	started         chan<- string
	release         <-chan struct{}
}

func (r gatedTurnRunner) Run(ctx context.Context, _ string, _ model.Content) iter.Seq2[*model.Event, error] {
	return func(yield func(*model.Event, error) bool) {
		current := r.active.Add(1)
		defer r.active.Add(-1)
		for {
			maximum := r.maximum.Load()
			if current <= maximum || r.maximum.CompareAndSwap(maximum, current) {
				break
			}
		}
		select {
		case r.started <- r.sessionID:
		case <-ctx.Done():
			yield(nil, ctx.Err())
			return
		}
		select {
		case <-r.release:
		case <-ctx.Done():
			yield(nil, ctx.Err())
			return
		}
		yield(&model.Event{SessionID: r.sessionID, TurnID: "turn-" + r.sessionID, Author: "assistant", Content: model.Content{Role: model.RoleAssistant, Content: "done"}, FinishReason: model.FinishReasonStop}, nil)
	}
}

func sequentialSessionIDs(values ...string) func() (string, error) {
	var index atomic.Int32
	return func() (string, error) {
		position := int(index.Add(1) - 1)
		if position >= len(values) {
			return "", fmt.Errorf("no session ID available")
		}
		return values[position], nil
	}
}

func startIntegrationRun(t *testing.T, client kodav1connect.KodaServiceClient, sessionID string) <-chan error {
	t.Helper()
	return startIntegrationRunContext(client, t.Context(), sessionID)
}

func startIntegrationRunContext(client kodav1connect.KodaServiceClient, ctx context.Context, sessionID string) <-chan error {
	done := make(chan error, 1)
	go func() {
		stream, err := client.Run(ctx, runRequest(sessionID, "hello", v1.AgentMode_AGENT_MODE_PLAN))
		if err == nil {
			for stream.Receive() {
			}
			err = stream.Err()
		}
		done <- err
	}()
	return done
}

func waitString(t *testing.T, values <-chan string) string {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Run to start")
	}
	return ""
}

func eventually(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied")
}

func assertSessionMutationWaitsForRun(t *testing.T, mutate func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := mutate(ctx)
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded && connect.CodeOf(err) != connect.CodeCanceled {
		t.Fatalf("session mutation while Run holds lock error = %v", err)
	}
}

func lastString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
