package server

import (
	"context"
	"errors"
	"iter"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/soasurs/adk/model"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
	"google.golang.org/protobuf/proto"
)

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

	providers, err := client.ListProviders(t.Context(), v1.ListProvidersRequest_builder{}.Build())
	if err != nil || len(providers.GetProviders()) != 5 {
		t.Fatalf("ListProviders() = %+v, %v", providers, err)
	}
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: proto.String("hello")}.Build()}}.Build()
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: proto.String(created.GetSession().GetId()),
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
		accepted, err := handler.approvals.Await(approvalContext, v1.ToolApproval_builder{Id: proto.String("approval-1")}.Build(), func(*v1.ToolApproval) error {
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
		ApprovalId: proto.String("approval-1"), Approved: proto.Bool(true),
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
		Id: proto.String("prompt-1"),
		Questions: []*v1.Question{v1.Question_builder{
			Id: proto.String("choice"), Header: proto.String("Choice"), Prompt: proto.String("Choose one"),
			Options: []*v1.QuestionOption{
				v1.QuestionOption_builder{Id: proto.String("one"), Label: proto.String("One")}.Build(),
				v1.QuestionOption_builder{Id: proto.String("two"), Label: proto.String("Two")}.Build(),
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
		PromptId: proto.String("prompt-1"), Canceled: proto.Bool(true),
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
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: proto.String("hello")}.Build()}}.Build()
	runDone := make(chan error, 1)
	go func() {
		stream, err := client.Run(t.Context(), v1.RunRequest_builder{
			SessionId: proto.String(created.GetSession().GetId()),
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
	handler, err := NewHandler(registry, catalog, openTestStore(t))
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
