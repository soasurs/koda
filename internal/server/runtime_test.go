package server

import (
	"context"
	"iter"
	"testing"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/store"
)

type fakeTurnRunner struct {
	gotSessionID string
	gotInput     model.Content
	events       []model.Event
}

func (r *fakeTurnRunner) Run(_ context.Context, sessionID string, input model.Content) iter.Seq2[*model.Event, error] {
	r.gotSessionID = sessionID
	r.gotInput = input
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
	created, err := client.CreateSession(t.Context(), &v1.CreateSessionRequest{
		Workdir: t.TempDir(), ProviderId: "openai-responses", ModelId: "gpt-5.6",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	fake := &fakeTurnRunner{events: []model.Event{
		{SessionID: created.Session.Id, TurnID: "turn-1", Partial: true, Content: model.Content{Role: model.RoleAssistant, Content: "hel"}},
		{ID: 7, SessionID: created.Session.Id, TurnID: "turn-1", Author: "assistant", Content: model.Content{Role: model.RoleAssistant, Content: "hello"}, FinishReason: model.FinishReasonStop, CreatedAt: 10, UpdatedAt: 11},
	}}
	var gotSession store.Session
	var gotMode v1.AgentMode
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, mode v1.AgentMode) (TurnRunner, error) {
		gotSession = session
		gotMode = mode
		return fake, nil
	}

	stream, err := client.Run(t.Context(), &v1.RunRequest{
		SessionId: created.Session.Id,
		Mode:      v1.AgentMode_AGENT_MODE_PLAN,
		Input:     &v1.Input{Parts: []*v1.Part{{Content: &v1.Part_Text{Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var events []*v1.Event
	for stream.Receive() {
		events = append(events, stream.Msg().GetEvent())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	if gotSession.ID != created.Session.Id || gotMode != v1.AgentMode_AGENT_MODE_PLAN || fake.gotSessionID != created.Session.Id ||
		len(fake.gotInput.Parts) != 1 || fake.gotInput.Parts[0].Text != "hello" {
		t.Fatalf("runtime factory inputs = session %+v, mode %v, runner input %+v", gotSession, gotMode, fake.gotInput)
	}
	if len(events) != 2 || !events[0].Partial || events[0].Id != "" || events[1].Id != "7" || events[1].FinishReason != v1.FinishReason_FINISH_REASON_STOP {
		t.Fatalf("Run() events = %+v", events)
	}
}

func TestRunUsesExpectedErrorCodesBeforeRuntimeExists(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	stream, err := client.Run(t.Context(), &v1.RunRequest{})
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("Run(without runtime) code = %v, want unimplemented; error = %v", connect.CodeOf(err), err)
	}

	handler.turnRunnerFactory = func(context.Context, store.Session, v1.AgentMode) (TurnRunner, error) {
		return &fakeTurnRunner{}, nil
	}
	stream, err = client.Run(t.Context(), &v1.RunRequest{SessionId: "session-1"})
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Run(invalid mode) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
}
