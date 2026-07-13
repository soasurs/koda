package server

import (
	"context"
	"iter"
	"testing"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

type fakeTurnRunner struct {
	gotSessionID     string
	gotInput         model.Content
	gotEnvironment   agent.RunEnvironment
	gotEnvironmentOK bool
	events           []model.Event
}

func (r *fakeTurnRunner) Run(ctx context.Context, sessionID string, input model.Content) iter.Seq2[*model.Event, error] {
	r.gotSessionID = sessionID
	r.gotInput = input
	r.gotEnvironment, r.gotEnvironmentOK = agent.RunEnvironmentFromContext(ctx)
	return func(yield func(*model.Event, error) bool) {
		for index := range r.events {
			event := r.events[index]
			if !yield(&event, nil) {
				return
			}
		}
	}
}

func TestTurnRunnerSeamAcceptsFakeRuntime(t *testing.T) {
	fake := &fakeTurnRunner{events: []model.Event{{Content: model.Content{Role: model.RoleAssistant, Content: "done"}}}}
	var runner TurnRunner = fake
	input := model.Content{Role: model.RoleUser, Content: "hello"}
	for event, err := range runner.Run(t.Context(), "session-1", input) {
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if event.Content.Content != "done" {
			t.Fatalf("event = %+v", event)
		}
	}
	if fake.gotSessionID != "session-1" || fake.gotInput.Content != "hello" {
		t.Fatalf("fake inputs = session %q, content %+v", fake.gotSessionID, fake.gotInput)
	}
}

func TestRunStreamsInjectedTurnRunner(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("openai-responses"), ModelId: proto.String("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sessionID := created.GetSession().GetId()
	fake := &fakeTurnRunner{events: []model.Event{
		{SessionID: sessionID, TurnID: "turn-1", Partial: true, Content: model.Content{Role: model.RoleAssistant, Content: "hel"}},
		{ID: 7, SessionID: sessionID, TurnID: "turn-1", Author: "assistant", Content: model.Content{Role: model.RoleAssistant, Content: "hello"}, FinishReason: model.FinishReasonStop, CreatedAt: 10, UpdatedAt: 11},
	}}
	var gotSession store.Session
	var gotMode v1.AgentMode
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, mode v1.AgentMode) (TurnRunner, error) {
		gotSession = session
		gotMode = mode
		return fake, nil
	}

	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: proto.String("hello")}.Build()}}.Build()
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: proto.String(sessionID),
		Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input:     input,
	}.Build())
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var events []*v1.Event
	var completed []*v1.RunCompleted
	for stream.Receive() {
		if event := stream.Msg().GetEvent(); event != nil {
			events = append(events, event)
		}
		if value := stream.Msg().GetCompleted(); value != nil {
			completed = append(completed, value)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	if gotSession.ID != sessionID || gotMode != v1.AgentMode_AGENT_MODE_PLAN || fake.gotSessionID != sessionID ||
		len(fake.gotInput.Parts) != 1 || fake.gotInput.Parts[0].Text != "hello" || !fake.gotEnvironmentOK ||
		fake.gotEnvironment.Workdir != gotSession.Workdir || fake.gotEnvironment.FileAccess != gotSession.FileAccess ||
		fake.gotEnvironment.ShellAccess != gotSession.ShellAccess {
		t.Fatalf("runtime factory inputs = session %+v, mode %v, runner input %+v", gotSession, gotMode, fake.gotInput)
	}
	if len(events) != 2 || !events[0].GetPartial() || events[0].GetId() != "" || events[1].GetId() != "7" || events[1].GetFinishReason() != v1.FinishReason_FINISH_REASON_STOP || len(completed) != 1 || completed[0].GetTurnId() != "turn-1" {
		t.Fatalf("Run() events = %+v, completed = %+v", events, completed)
	}
}

func TestRunUsesExpectedErrorCodes(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{}.Build())
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Run(without session ID) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	handler.turnRunnerFactory = func(context.Context, store.Session, v1.AgentMode) (TurnRunner, error) {
		return &fakeTurnRunner{}, nil
	}
	stream, err = client.Run(t.Context(), v1.RunRequest_builder{SessionId: proto.String("session-1")}.Build())
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Run(invalid mode) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
}

func TestRunInitializesADKSessionBeforeResolvingProvider(t *testing.T) {
	client, registry, handler := newTestService(t, staticDiscoverer{})
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID: "unconfigured", Name: "Unconfigured", Type: provider.TypeOpenAIChatCompletions,
		ModelOverrides: []provider.Model{{ID: "model"}},
	}, nil); err != nil {
		t.Fatalf("Registry.Save() error = %v", err)
	}
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: proto.String(t.TempDir()), ProviderId: proto.String("unconfigured"), ModelId: proto.String("model"),
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
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("Run(unconfigured provider) code = %v, want failed_precondition; error = %v", connect.CodeOf(err), err)
	}
	adkSession, err := handler.store.ADKSessionService().GetSession(t.Context(), created.GetSession().GetId())
	if err != nil || adkSession == nil {
		t.Fatalf("ADK session after Run() = %v, %v; want initialized session", adkSession, err)
	}
}

func TestTerminalEvent(t *testing.T) {
	for _, test := range []struct {
		name  string
		event model.Event
		want  bool
	}{
		{name: "stop", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonStop}, want: true},
		{name: "length", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonLength}, want: true},
		{name: "content filter", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonContentFilter}, want: true},
		{name: "tool calls", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonToolCalls}, want: false},
		{name: "partial", event: model.Event{Partial: true, Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonStop}, want: false},
		{name: "tool", event: model.Event{Content: model.Content{Role: model.RoleTool}, FinishReason: model.FinishReasonStop}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := terminalEvent(test.event); got != test.want {
				t.Fatalf("terminalEvent() = %v, want %v", got, test.want)
			}
		})
	}
}
