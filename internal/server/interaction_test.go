package server

import (
	"errors"
	"testing"
	"time"

	adktrace "github.com/soasurs/adk/trace"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/permission"
	"github.com/soasurs/koda/internal/tools"
)

func TestRunInteractionsPublishAndResolveApproval(t *testing.T) {
	_, _, handler := newTestService(t, staticDiscoverer{})
	frames := make(chan *v1.RunResponse, 1)
	interactions := handler.runInteractions(func(frame *v1.RunResponse) error {
		frames <- frame
		return nil
	})
	ctx := adktrace.ContextWithRunInfo(t.Context(), adktrace.RunInfo{SessionID: "session-1", TurnID: "turn-1"})
	done := make(chan error, 1)
	go func() {
		done <- interactions.Authorizer.Authorize(ctx, tools.Approval{
			ToolCallID:  "call-1",
			ToolName:    "write_file",
			Arguments:   []byte(`{"path":"notes.txt","content":"hello"}`),
			Kind:        permission.KindFileWrite,
			Scope:       permission.ScopeWorkspace,
			TargetPaths: []string{"/workspace/notes.txt"},
			Summary:     "write notes.txt",
			FileChanges: []tools.FileChange{{Path: "notes.txt", Kind: tools.FileChangeCreate}},
		})
	}()

	approval := waitForApprovalFrame(t, frames)
	if approval.SessionId != "session-1" || approval.TurnId != "turn-1" || approval.ToolCallId != "call-1" ||
		approval.ToolName != "write_file" || approval.ArgumentsJson != `{"path":"notes.txt","content":"hello"}` ||
		approval.Kind != v1.ToolApprovalKind_TOOL_APPROVAL_KIND_FILE_WRITE ||
		approval.Scope != v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_WORKSPACE || len(approval.FileChanges) != 1 {
		t.Fatalf("approval = %+v", approval)
	}
	if err := handler.approvals.Resolve(approval.Id, true); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Authorize() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Authorize() did not resolve")
	}
}

func TestRunInteractionsTranslateRejectionAndQuestions(t *testing.T) {
	_, _, handler := newTestService(t, staticDiscoverer{})
	frames := make(chan *v1.RunResponse, 2)
	interactions := handler.runInteractions(func(frame *v1.RunResponse) error {
		frames <- frame
		return nil
	})
	ctx := adktrace.ContextWithRunInfo(t.Context(), adktrace.RunInfo{SessionID: "session-1", TurnID: "turn-1"})

	rejected := make(chan error, 1)
	go func() {
		rejected <- interactions.Authorizer.Authorize(ctx, tools.Approval{Kind: permission.KindShell, Scope: permission.ScopeGlobal})
	}()
	approval := waitForApprovalFrame(t, frames)
	if err := handler.approvals.Resolve(approval.Id, false); err != nil {
		t.Fatalf("Resolve(reject) error = %v", err)
	}
	select {
	case err := <-rejected:
		if !errors.Is(err, tools.ErrApprovalRejected) {
			t.Fatalf("Authorize(rejected) error = %v, want ErrApprovalRejected", err)
		}
	case <-time.After(time.Second):
		t.Fatal("rejected Authorize() did not resolve")
	}

	answers := make(chan tools.QuestionResolution, 1)
	questionErrors := make(chan error, 1)
	go func() {
		resolution, err := interactions.Questioner.Ask(ctx, tools.QuestionRequest{
			ToolCallID: "call-2",
			Questions: []tools.Question{{
				ID:     "storage",
				Header: "Storage",
				Prompt: "Which storage backend?",
				Options: []tools.QuestionOption{
					{ID: "sqlite", Label: "SQLite"},
					{ID: "postgres", Label: "PostgreSQL"},
				},
			}},
		})
		questionErrors <- err
		answers <- resolution
	}()
	prompt := waitForQuestionFrame(t, frames)
	if prompt.SessionId != "session-1" || prompt.TurnId != "turn-1" || prompt.ToolCallId != "call-2" || len(prompt.Questions) != 1 {
		t.Fatalf("prompt = %+v", prompt)
	}
	if err := handler.questions.ResolveAnswers(prompt.Id, &v1.QuestionAnswers{Answers: []*v1.QuestionAnswer{{
		QuestionId:        "storage",
		SelectedOptionIds: []string{"sqlite"},
	}}}); err != nil {
		t.Fatalf("ResolveAnswers() error = %v", err)
	}
	select {
	case err := <-questionErrors:
		if err != nil {
			t.Fatalf("Ask() error = %v", err)
		}
		resolution := <-answers
		if len(resolution.Answers) != 1 || resolution.Answers[0].QuestionID != "storage" || resolution.Answers[0].SelectedOptionIDs[0] != "sqlite" {
			t.Fatalf("resolution = %+v", resolution)
		}
	case <-time.After(time.Second):
		t.Fatal("Ask() did not resolve")
	}
}

func TestRunInteractionsRequireRunMetadata(t *testing.T) {
	_, _, handler := newTestService(t, staticDiscoverer{})
	interactions := handler.runInteractions(func(*v1.RunResponse) error { return nil })
	if err := interactions.Authorizer.Authorize(t.Context(), tools.Approval{}); err == nil {
		t.Fatal("Authorize(without run metadata) error = nil")
	}
	if _, err := interactions.Questioner.Ask(t.Context(), tools.QuestionRequest{}); err == nil {
		t.Fatal("Ask(without run metadata) error = nil")
	}
}

func waitForApprovalFrame(t *testing.T, frames <-chan *v1.RunResponse) *v1.ToolApproval {
	t.Helper()
	select {
	case frame := <-frames:
		if approval := frame.GetApproval(); approval != nil {
			return approval
		}
		t.Fatalf("frame = %+v, want approval", frame)
	case <-time.After(time.Second):
		t.Fatal("approval frame was not published")
	}
	return nil
}

func waitForQuestionFrame(t *testing.T, frames <-chan *v1.RunResponse) *v1.QuestionPrompt {
	t.Helper()
	select {
	case frame := <-frames:
		if prompt := frame.GetQuestionPrompt(); prompt != nil {
			return prompt
		}
		t.Fatalf("frame = %+v, want question prompt", frame)
	case <-time.After(time.Second):
		t.Fatal("question prompt frame was not published")
	}
	return nil
}
