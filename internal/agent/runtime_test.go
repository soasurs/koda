package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/soasurs/koda/internal/tools"
)

func TestRuntimeInteractionsRequireActiveRunContext(t *testing.T) {
	if err := (runtimeAuthorizer{}).Authorize(t.Context(), tools.Approval{}); !errors.Is(err, tools.ErrApprovalRequired) {
		t.Fatalf("Authorize() error = %v, want ErrApprovalRequired", err)
	}
	if _, err := (runtimeQuestioner{}).Ask(t.Context(), tools.QuestionRequest{}); !errors.Is(err, tools.ErrQuestionInteractionUnavailable) {
		t.Fatalf("Ask() error = %v, want ErrQuestionInteractionUnavailable", err)
	}
}

func TestRuntimeInteractionsDelegateToRunContext(t *testing.T) {
	wantApproval := tools.Approval{ToolCallID: "call-1", ToolName: "write_file"}
	wantQuestion := tools.QuestionRequest{ToolCallID: "call-2"}
	contextWithInteractions := WithRunInteractions(t.Context(), RunInteractions{
		Authorizer: authorizerFunc(func(_ context.Context, approval tools.Approval) error {
			if approval.ToolCallID != wantApproval.ToolCallID || approval.ToolName != wantApproval.ToolName {
				t.Fatalf("approval = %+v, want %+v", approval, wantApproval)
			}
			return nil
		}),
		Questioner: questionerFunc(func(_ context.Context, request tools.QuestionRequest) (tools.QuestionResolution, error) {
			if request.ToolCallID != wantQuestion.ToolCallID {
				t.Fatalf("request = %+v, want %+v", request, wantQuestion)
			}
			return tools.QuestionResolution{Canceled: true}, nil
		}),
	})
	if err := (runtimeAuthorizer{}).Authorize(contextWithInteractions, wantApproval); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	resolution, err := (runtimeQuestioner{}).Ask(contextWithInteractions, wantQuestion)
	if err != nil || !resolution.Canceled {
		t.Fatalf("Ask() = %+v, %v; want canceled resolution", resolution, err)
	}
}

type authorizerFunc func(context.Context, tools.Approval) error

func (f authorizerFunc) Authorize(ctx context.Context, approval tools.Approval) error {
	return f(ctx, approval)
}

type questionerFunc func(context.Context, tools.QuestionRequest) (tools.QuestionResolution, error)

func (f questionerFunc) Ask(ctx context.Context, request tools.QuestionRequest) (tools.QuestionResolution, error) {
	return f(ctx, request)
}
