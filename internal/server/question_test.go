package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestQuestionBrokerAnswersAndAllowsInvalidRetry(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	prompt := testQuestionPrompt("prompt-1")
	published := make(chan *v1.QuestionPrompt, 1)
	type outcome struct {
		answers  *v1.QuestionAnswers
		canceled bool
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		answers, canceled, err := handler.questions.Await(t.Context(), prompt, func(value *v1.QuestionPrompt) error {
			published <- value
			return nil
		})
		done <- outcome{answers: answers, canceled: canceled, err: err}
	}()
	select {
	case got := <-published:
		if got.GetId() != "prompt-1" || got.GetToolCallId() != "call-1" {
			t.Fatalf("published prompt = %+v", got)
		}
		got.GetQuestions()[0].SetPrompt("mutated by frontend")
	case <-time.After(time.Second):
		t.Fatal("Await() did not publish")
	}

	invalidAnswer := v1.QuestionAnswer_builder{
		QuestionId:        proto.String("database"),
		SelectedOptionIds: []string{"missing"},
	}.Build()
	invalid := v1.QuestionAnswers_builder{Answers: []*v1.QuestionAnswer{invalidAnswer}}.Build()
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: proto.String("prompt-1"),
		Answers:  invalid,
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(invalid) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	validAnswer := v1.QuestionAnswer_builder{
		QuestionId:        proto.String("database"),
		SelectedOptionIds: []string{"sqlite"},
	}.Build()
	valid := v1.QuestionAnswers_builder{Answers: []*v1.QuestionAnswer{validAnswer}}.Build()
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: proto.String("prompt-1"),
		Answers:  valid,
	}.Build()); err != nil {
		t.Fatalf("SubmitQuestionAnswers() error = %v", err)
	}
	select {
	case result := <-done:
		if result.err != nil || result.canceled || result.answers.GetAnswers()[0].GetSelectedOptionIds()[0] != "sqlite" {
			t.Fatalf("Await() result = %+v", result)
		}
		if prompt.GetQuestions()[0].GetPrompt() != "Which database?" {
			t.Fatal("published prompt mutated broker-owned input")
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not resolve")
	}
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: proto.String("prompt-1"),
		Answers:  valid,
	}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("SubmitQuestionAnswers(resolved) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestQuestionBrokerCancellationAndCleanup(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	done := make(chan bool, 1)
	go func() {
		_, canceled, err := handler.questions.Await(t.Context(), testQuestionPrompt("prompt-1"), func(*v1.QuestionPrompt) error { return nil })
		if err != nil {
			t.Errorf("Await() error = %v", err)
			return
		}
		done <- canceled
	}()
	waitForQuestionPrompt(t, handler.questions, "prompt-1")
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: proto.String("prompt-1"),
		Canceled: proto.Bool(true),
	}.Build()); err != nil {
		t.Fatalf("SubmitQuestionAnswers(canceled) error = %v", err)
	}
	select {
	case canceled := <-done:
		if !canceled {
			t.Fatal("Await() canceled = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not cancel")
	}

	ctx, cancel := context.WithCancel(t.Context())
	contextDone := make(chan error, 1)
	go func() {
		_, _, err := handler.questions.Await(ctx, testQuestionPrompt("prompt-2"), func(*v1.QuestionPrompt) error { return nil })
		contextDone <- err
	}()
	waitForQuestionPrompt(t, handler.questions, "prompt-2")
	cancel()
	if err := <-contextDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Await(context canceled) error = %v", err)
	}
	if err := handler.questions.ResolveCanceled("prompt-2"); !errors.Is(err, ErrQuestionPromptNotFound) {
		t.Fatalf("ResolveCanceled(cleaned) error = %v", err)
	}
}

func TestQuestionBrokerAndHandlerRejectInvalidRequests(t *testing.T) {
	broker := NewQuestionBroker()
	if _, _, err := broker.Await(nil, testQuestionPrompt("prompt-1"), func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(nil context) error = nil")
	}
	if _, _, err := broker.Await(t.Context(), nil, func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(nil prompt) error = nil")
	}
	if _, _, err := broker.Await(t.Context(), v1.QuestionPrompt_builder{}.Build(), func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(empty ID) error = nil")
	}
	if _, _, err := broker.Await(t.Context(), testQuestionPrompt("prompt-1"), nil); err == nil {
		t.Fatal("Await(nil publish) error = nil")
	}
	invalidPrompt := testQuestionPrompt("prompt-1")
	invalidPrompt.SetQuestions(nil)
	if _, _, err := broker.Await(t.Context(), invalidPrompt, func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(invalid questions) error = nil")
	}
	publishErr := errors.New("stream closed")
	if _, _, err := broker.Await(t.Context(), testQuestionPrompt("prompt-1"), func(*v1.QuestionPrompt) error { return publishErr }); !errors.Is(err, publishErr) {
		t.Fatalf("Await(publish error) = %v, want %v", err, publishErr)
	}
	if err := broker.ResolveAnswers("", v1.QuestionAnswers_builder{}.Build()); err == nil {
		t.Fatal("ResolveAnswers(empty ID) error = nil")
	}
	if err := broker.ResolveAnswers("prompt-1", nil); err == nil {
		t.Fatal("ResolveAnswers(nil) error = nil")
	}
	if err := broker.ResolveCanceled(""); err == nil {
		t.Fatal("ResolveCanceled(empty ID) error = nil")
	}

	client, _, _ := newTestService(t, staticDiscoverer{})
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(no resolution) code = %v; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.SubmitQuestionAnswers(t.Context(), v1.SubmitQuestionAnswersRequest_builder{
		PromptId: proto.String("prompt-1"),
		Canceled: proto.Bool(false),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(false canceled) code = %v; error = %v", connect.CodeOf(err), err)
	}
}

func testQuestionPrompt(id string) *v1.QuestionPrompt {
	return v1.QuestionPrompt_builder{
		Id:         proto.String(id),
		SessionId:  proto.String("session-1"),
		TurnId:     proto.String("turn-1"),
		ToolCallId: proto.String("call-1"),
		Questions: []*v1.Question{v1.Question_builder{
			Id:     proto.String("database"),
			Prompt: proto.String("Which database?"),
			Options: []*v1.QuestionOption{
				v1.QuestionOption_builder{Id: proto.String("sqlite"), Label: proto.String("SQLite")}.Build(),
				v1.QuestionOption_builder{Id: proto.String("postgres"), Label: proto.String("PostgreSQL")}.Build(),
			},
		}.Build()},
	}.Build()
}

func waitForQuestionPrompt(t *testing.T, broker *QuestionBroker, id string) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); ; {
		broker.mu.Lock()
		_, pending := broker.pending[id]
		broker.mu.Unlock()
		if pending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("question prompt %q was not registered", id)
		}
		time.Sleep(time.Millisecond)
	}
}
