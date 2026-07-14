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
	if approval.GetSessionId() != "session-1" || approval.GetTurnId() != "turn-1" || approval.GetToolCallId() != "call-1" ||
		approval.GetToolName() != "write_file" || approval.GetArgumentsJson() != `{"path":"notes.txt","content":"hello"}` ||
		approval.GetKind() != v1.ToolApprovalKind_TOOL_APPROVAL_KIND_FILE_WRITE ||
		approval.GetScope() != v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_WORKSPACE || len(approval.GetFileChanges()) != 1 {
		t.Fatalf("approval = %+v", approval)
	}
	if err := handler.approvals.Resolve(approval.GetId(), true); err != nil {
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

func TestApprovalKindToProtoIncludesMCP(t *testing.T) {
	if got := approvalKindToProto(permission.KindMCP); got != v1.ToolApprovalKind_TOOL_APPROVAL_KIND_MCP {
		t.Fatalf("approvalKindToProto(KindMCP) = %v", got)
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
	if err := handler.approvals.Resolve(approval.GetId(), false); err != nil {
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
	if prompt.GetSessionId() != "session-1" || prompt.GetTurnId() != "turn-1" || prompt.GetToolCallId() != "call-2" || len(prompt.GetQuestions()) != 1 {
		t.Fatalf("prompt = %+v", prompt)
	}
	resolvedAnswer := v1.QuestionAnswer_builder{
		QuestionId:        new("storage"),
		SelectedOptionIds: []string{"sqlite"},
	}.Build()
	if err := handler.questions.ResolveAnswers(prompt.GetId(), v1.QuestionAnswers_builder{Answers: []*v1.QuestionAnswer{resolvedAnswer}}.Build()); err != nil {
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
