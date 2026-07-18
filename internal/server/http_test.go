package server

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soasurs/adk/model"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

func TestHTTPServerServesOptionalWebHandler(t *testing.T) {
	handler := newHTTPTestHandler(t)
	web := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	server, err := NewHTTPServer(handler, HTTPServerConfig{WebHandler: web})
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	response := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("GET / status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestHTTPServerServesConnectRunAndShutsDown(t *testing.T) {
	handler := newHTTPTestHandler(t)
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, _ v1.AgentMode) (TurnRunner, error) {
		return &fakeTurnRunner{events: []model.Event{{
			ID: 7, SessionID: session.ID, TurnID: "turn-1", Author: "assistant",
			Content: model.Content{Role: model.RoleAssistant, Content: "done"}, FinishReason: model.FinishReasonStop,
		}}}, nil
	}
	server, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{})
	defer stop()
	if server.Address() != DefaultAddress {
		t.Fatalf("default address = %q, want %q", server.Address(), DefaultAddress)
	}

	browseRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(browseRoot, "workspace"), 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	directories, err := client.ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(browseRoot),
	}.Build())
	if err != nil || len(directories.GetDirectories()) != 1 || directories.GetDirectories()[0].GetName() != "workspace" {
		t.Fatalf("ListDirectories() = %+v, %v", directories, err)
	}

	providers, err := client.ListProviders(t.Context(), v1.ListProvidersRequest_builder{}.Build())
	if err != nil || len(providers.GetProviders()) != 5 {
		t.Fatalf("ListProviders() = %+v, %v", providers, err)
	}
	skills, err := client.ListSkills(t.Context(), v1.ListSkillsRequest_builder{}.Build())
	if err != nil || len(skills.GetSkills()) != 0 {
		t.Fatalf("ListSkills() = %+v, %v", skills, err)
	}
	mcpServers, err := client.ListMCPServers(t.Context(), v1.ListMCPServersRequest_builder{}.Build())
	if err != nil || len(mcpServers.GetServers()) != 0 {
		t.Fatalf("ListMCPServers() = %+v, %v", mcpServers, err)
	}
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("hello")}.Build()}}.Build()
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: new(created.GetSession().GetId()),
		Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input:     input,
	}.Build())
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var event *v1.Event
	var completed *v1.RunCompleted
	for stream.Receive() {
		if value := stream.Msg().GetEvent(); value != nil {
			event = value
		}
		if value := stream.Msg().GetCompleted(); value != nil {
			completed = value
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	if event == nil || event.GetMessage().GetText() != "done" || completed == nil || completed.GetTurnId() != "turn-1" {
		t.Fatalf("Run() event = %+v, completed = %+v", event, completed)
	}
}

func TestHTTPServerRejectsNonLocalHostAndOrigin(t *testing.T) {
	handler := newHTTPTestHandler(t)
	var output bytes.Buffer
	logger, _, err := logging.New(&output, "warn", "")
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	server, err := NewHTTPServer(handler, HTTPServerConfig{Logger: logger})
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	for _, test := range []struct {
		name   string
		host   string
		origin string
		want   int
	}{
		{name: "loopback", host: "127.0.0.1:8080", origin: "http://localhost:3000", want: http.StatusNotFound},
		{name: "host rebinding", host: "attacker.example", want: http.StatusForbidden},
		{name: "remote origin", host: "localhost:8080", origin: "https://attacker.example", want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost/not-a-connect-route", nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error = %v", err)
			}
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			response := &statusRecorder{header: make(http.Header)}
			server.server.Handler.ServeHTTP(response, request)
			if response.status != test.want {
				t.Fatalf("status = %d, want %d", response.status, test.want)
			}
		})
	}
	got := output.String()
	if !strings.Contains(got, "msg=\"rejected non-local HTTP request\"") || !strings.Contains(got, "origin_host=attacker.example") {
		t.Fatalf("security log output = %q", got)
	}
}

type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header       { return r.header }
func (r *statusRecorder) Write([]byte) (int, error) { return 0, nil }
func (r *statusRecorder) WriteHeader(status int)    { r.status = status }

func TestHTTPServerServesInteractionRPCs(t *testing.T) {
	handler := newHTTPTestHandler(t)
	_, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{Address: "127.0.0.1:0"})
	defer stop()

	approvalContext, cancelApproval := context.WithCancel(t.Context())
	defer cancelApproval()
	publishedApproval := make(chan struct{}, 1)
	approvalDone := make(chan struct {
		accepted bool
		err      error
	}, 1)
	go func() {
		accepted, err := handler.approvals.Await(approvalContext, v1.ToolApproval_builder{Id: new("approval-1")}.Build(), func(*v1.ToolApproval) error {
			publishedApproval <- struct{}{}
			return nil
		})
		approvalDone <- struct {
			accepted bool
			err      error
		}{accepted: accepted, err: err}
	}()
	awaitSignal(t, publishedApproval)
	if _, err := client.ResolveToolApproval(t.Context(), v1.ResolveToolApprovalRequest_builder{
		ApprovalId: new("approval-1"), Approved: new(true),
	}.Build()); err != nil {
		t.Fatalf("ResolveToolApproval() error = %v", err)
	}
	if result := awaitApproval(t, approvalDone); result.err != nil || !result.accepted {
		t.Fatalf("approval result = %+v", result)
	}

	questionContext, cancelQuestion := context.WithCancel(t.Context())
	defer cancelQuestion()
	publishedQuestion := make(chan struct{}, 1)
	questionDone := make(chan struct {
		canceled bool
		err      error
	}, 1)
	prompt := v1.QuestionPrompt_builder{
		Id: new("prompt-1"),
		Questions: []*v1.Question{v1.Question_builder{
			Id: new("choice"), Header: new("Choice"), Prompt: new("Choose one"),
			Options: []*v1.QuestionOption{
				v1.QuestionOption_builder{Id: new("one"), Label: new("One")}.Build(),
				v1.QuestionOption_builder{Id: new("two"), Label: new("Two")}.Build(),
			},
		}.Build()},
	}.Build()
	go func() {
		_, canceled, err := handler.questions.Await(questionContext, prompt, func(*v1.QuestionPrompt) error {
			publishedQuestion <- struct{}{}
			return nil
		})
		questionDone <- struct {
			canceled bool
			err      error
		}{canceled: canceled, err: err}
	}()
	awaitSignal(t, publishedQuestion)
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: new("prompt-1"), Canceled: new(true),
	}.Build()); err != nil {
		t.Fatalf("SubmitQuestionAnswers() error = %v", err)
	}
	if result := awaitQuestion(t, questionDone); result.err != nil || !result.canceled {
		t.Fatalf("question result = %+v", result)
	}
}

func TestHTTPServerShutdownCancelsBlockedRun(t *testing.T) {
	handler := newHTTPTestHandler(t)
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	blocked := &blockingTurnRunner{started: make(chan struct{}, 1)}
	handler.turnRunnerFactory = func(context.Context, store.Session, v1.AgentMode) (TurnRunner, error) {
		return blocked, nil
	}
	_, client, stop := startHTTPTestServer(t, handler, HTTPServerConfig{})
	defer stop()
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("hello")}.Build()}}.Build()
	runDone := make(chan error, 1)
	go func() {
		stream, err := client.Run(t.Context(), v1.RunRequest_builder{
			SessionId: new(created.GetSession().GetId()),
			Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
			Input:     input,
		}.Build())
		if err == nil {
			for stream.Receive() {
			}
			err = stream.Err()
		}
		runDone <- err
	}()
	awaitSignal(t, blocked.started)
	stop()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("blocked Run did not finish after shutdown")
	}
}

func newHTTPTestHandler(t *testing.T) *Handler {
	t.Helper()
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, staticDiscoverer{})
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	handler, err := NewHandler(registry, catalog, openTestStore(t), nil, nil, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func startHTTPTestServer(t *testing.T, handler *Handler, config HTTPServerConfig) (*HTTPServer, kodav1connect.KodaServiceClient, func()) {
	t.Helper()
	server, err := NewHTTPServer(handler, config)
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, listener)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("Serve() error = %v, want context canceled", err)
				}
			case <-time.After(time.Second):
				t.Error("Serve() did not shut down")
			}
		})
	}
	baseURL := "http://" + listener.Addr().String()
	return server, kodav1connect.NewKodaServiceClient(http.DefaultClient, baseURL), stop
}

type blockingTurnRunner struct {
	started chan struct{}
}

func (r blockingTurnRunner) Run(ctx context.Context, _ string, _ model.Content) iter.Seq2[*model.Event, error] {
	return func(yield func(*model.Event, error) bool) {
		select {
		case r.started <- struct{}{}:
		case <-ctx.Done():
		}
		<-ctx.Done()
		yield(nil, ctx.Err())
	}
}

func awaitSignal(t *testing.T, value <-chan struct{}) {
	t.Helper()
	select {
	case <-value:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interaction publication")
	}
}

func awaitApproval(t *testing.T, value <-chan struct {
	accepted bool
	err      error
}) struct {
	accepted bool
	err      error
} {
	t.Helper()
	select {
	case result := <-value:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for approval resolution")
	}
	return struct {
		accepted bool
		err      error
	}{}
}

func awaitQuestion(t *testing.T, value <-chan struct {
	canceled bool
	err      error
}) struct {
	canceled bool
	err      error
} {
	t.Helper()
	select {
	case result := <-value:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for question resolution")
	}
	return struct {
		canceled bool
		err      error
	}{}
}
