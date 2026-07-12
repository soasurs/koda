package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
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
		if got.Id != "prompt-1" || got.ToolCallId != "call-1" {
			t.Fatalf("published prompt = %+v", got)
		}
		got.Questions[0].Prompt = "mutated by frontend"
	case <-time.After(time.Second):
		t.Fatal("Await() did not publish")
	}

	invalid := &v1.QuestionAnswers{Answers: []*v1.QuestionAnswer{{
		QuestionId:        "database",
		SelectedOptionIds: []string{"missing"},
	}}}
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{
		PromptId: "prompt-1",
		Resolution: &v1.SubmitQuestionAnswersRequest_Answers{
			Answers: invalid,
		},
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(invalid) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	valid := &v1.QuestionAnswers{Answers: []*v1.QuestionAnswer{{
		QuestionId:        "database",
		SelectedOptionIds: []string{"sqlite"},
	}}}
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{
		PromptId: "prompt-1",
		Resolution: &v1.SubmitQuestionAnswersRequest_Answers{
			Answers: valid,
		},
	}); err != nil {
		t.Fatalf("SubmitQuestionAnswers() error = %v", err)
	}
	select {
	case result := <-done:
		if result.err != nil || result.canceled || result.answers.Answers[0].SelectedOptionIds[0] != "sqlite" {
			t.Fatalf("Await() result = %+v", result)
		}
		if prompt.Questions[0].Prompt != "Which database?" {
			t.Fatal("published prompt mutated broker-owned input")
		}
	case <-time.After(time.Second):
		t.Fatal("Await() did not resolve")
	}
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{
		PromptId: "prompt-1",
		Resolution: &v1.SubmitQuestionAnswersRequest_Answers{
			Answers: valid,
		},
	}); connect.CodeOf(err) != connect.CodeNotFound {
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
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{
		PromptId:   "prompt-1",
		Resolution: &v1.SubmitQuestionAnswersRequest_Canceled{Canceled: true},
	}); err != nil {
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
	if _, _, err := broker.Await(t.Context(), &v1.QuestionPrompt{}, func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(empty ID) error = nil")
	}
	if _, _, err := broker.Await(t.Context(), testQuestionPrompt("prompt-1"), nil); err == nil {
		t.Fatal("Await(nil publish) error = nil")
	}
	invalidPrompt := testQuestionPrompt("prompt-1")
	invalidPrompt.Questions = nil
	if _, _, err := broker.Await(t.Context(), invalidPrompt, func(*v1.QuestionPrompt) error { return nil }); err == nil {
		t.Fatal("Await(invalid questions) error = nil")
	}
	publishErr := errors.New("stream closed")
	if _, _, err := broker.Await(t.Context(), testQuestionPrompt("prompt-1"), func(*v1.QuestionPrompt) error { return publishErr }); !errors.Is(err, publishErr) {
		t.Fatalf("Await(publish error) = %v, want %v", err, publishErr)
	}
	if err := broker.ResolveAnswers("", &v1.QuestionAnswers{}); err == nil {
		t.Fatal("ResolveAnswers(empty ID) error = nil")
	}
	if err := broker.ResolveAnswers("prompt-1", nil); err == nil {
		t.Fatal("ResolveAnswers(nil) error = nil")
	}
	if err := broker.ResolveCanceled(""); err == nil {
		t.Fatal("ResolveCanceled(empty ID) error = nil")
	}

	client, _, _ := newTestService(t, staticDiscoverer{})
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(no resolution) code = %v; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.SubmitQuestionAnswers(t.Context(), &v1.SubmitQuestionAnswersRequest{
		PromptId:   "prompt-1",
		Resolution: &v1.SubmitQuestionAnswersRequest_Canceled{Canceled: false},
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SubmitQuestionAnswers(false canceled) code = %v; error = %v", connect.CodeOf(err), err)
	}
}

func testQuestionPrompt(id string) *v1.QuestionPrompt {
	return &v1.QuestionPrompt{
		Id:         id,
		SessionId:  "session-1",
		TurnId:     "turn-1",
		ToolCallId: "call-1",
		Questions: []*v1.Question{{
			Id:     "database",
			Prompt: "Which database?",
			Options: []*v1.QuestionOption{
				{Id: "sqlite", Label: "SQLite"},
				{Id: "postgres", Label: "PostgreSQL"},
			},
		}},
	}
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
