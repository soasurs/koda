package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	adktrace "github.com/soasurs/adk/trace"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/permission"
	"github.com/soasurs/koda/internal/tools"
)

// runInteractions builds the context-scoped interaction adapters used by a
// live Run. publish must serialize calls to the Connect stream because ADK
// executes same-round tool calls concurrently.
func (h *Handler) runInteractions(publish func(*v1.RunResponse) error) agent.RunInteractions {
	return agent.RunInteractions{
		Authorizer: brokerAuthorizer{
			handler: h,
			broker:  h.approvals,
			publish: publish,
			newID:   newInteractionID,
		},
		Questioner: brokerQuestioner{
			handler: h,
			broker:  h.questions,
			publish: publish,
			newID:   newInteractionID,
		},
	}
}

type brokerAuthorizer struct {
	handler *Handler
	broker  *ApprovalBroker
	publish func(*v1.RunResponse) error
	newID   func() (string, error)
}

func (a brokerAuthorizer) Authorize(ctx context.Context, approval tools.Approval) error {
	if a.broker == nil {
		return errors.New("server: approval broker is not configured")
	}
	if a.publish == nil {
		return errors.New("server: run publisher is not configured")
	}
	if a.newID == nil {
		return errors.New("server: interaction ID generator is not configured")
	}
	info, err := runInfo(ctx)
	if err != nil {
		return err
	}
	id, err := a.newID()
	if err != nil {
		return fmt.Errorf("server: generate approval ID: %w", err)
	}
	request := v1.ToolApproval_builder{
		Id:            new(id),
		SessionId:     new(info.SessionID),
		TurnId:        new(info.TurnID),
		ToolCallId:    new(approval.ToolCallID),
		ToolName:      new(approval.ToolName),
		Summary:       new(approval.Summary),
		ArgumentsJson: new(string(approval.Arguments)),
		FileChanges:   fileChangesToProto(approval.FileChanges),
		Kind:          approvalKindToProto(approval.Kind).Enum(),
		Scope:         approvalScopeToProto(approval.Scope).Enum(),
		TargetPaths:   append([]string(nil), approval.TargetPaths...),
	}.Build()
	startedAt := time.Now()
	if a.handler != nil {
		a.handler.log(ctx, slog.LevelInfo, "tool approval requested",
			slog.String("approval_id", id),
			slog.String("session_id", info.SessionID),
			slog.String("turn_id", info.TurnID),
			slog.String("tool_call_id", approval.ToolCallID),
			slog.String("tool", approval.ToolName),
			slog.String("kind", string(approval.Kind)),
			slog.String("scope", string(approval.Scope)),
		)
	}
	accepted, err := a.broker.Await(ctx, request, func(v *v1.ToolApproval) error {
		resp := new(v1.RunResponse)
		resp.SetApproval(v)
		return a.publish(resp)
	})
	if err != nil {
		if a.handler != nil && errors.Is(err, context.Canceled) {
			a.handler.log(ctx, slog.LevelInfo, "tool approval canceled",
				slog.String("approval_id", id),
				slog.String("session_id", info.SessionID),
				slog.String("turn_id", info.TurnID),
				slog.Duration("duration", time.Since(startedAt)),
			)
		}
		return fmt.Errorf("server: await tool approval: %w", err)
	}
	if a.handler != nil {
		a.handler.log(ctx, slog.LevelInfo, "tool approval resolved",
			slog.String("approval_id", id),
			slog.String("session_id", info.SessionID),
			slog.String("turn_id", info.TurnID),
			slog.Bool("approved", accepted),
			slog.Duration("duration", time.Since(startedAt)),
		)
	}
	if !accepted {
		return tools.ErrApprovalRejected
	}
	return nil
}

type brokerQuestioner struct {
	handler *Handler
	broker  *QuestionBroker
	publish func(*v1.RunResponse) error
	newID   func() (string, error)
}

func (q brokerQuestioner) Ask(ctx context.Context, request tools.QuestionRequest) (tools.QuestionResolution, error) {
	if q.broker == nil {
		return tools.QuestionResolution{}, errors.New("server: question broker is not configured")
	}
	if q.publish == nil {
		return tools.QuestionResolution{}, errors.New("server: run publisher is not configured")
	}
	if q.newID == nil {
		return tools.QuestionResolution{}, errors.New("server: interaction ID generator is not configured")
	}
	info, err := runInfo(ctx)
	if err != nil {
		return tools.QuestionResolution{}, err
	}
	id, err := q.newID()
	if err != nil {
		return tools.QuestionResolution{}, fmt.Errorf("server: generate question prompt ID: %w", err)
	}
	prompt := v1.QuestionPrompt_builder{
		Id:         new(id),
		SessionId:  new(info.SessionID),
		TurnId:     new(info.TurnID),
		ToolCallId: new(request.ToolCallID),
		Questions:  questionsToProto(request.Questions),
	}.Build()
	startedAt := time.Now()
	if q.handler != nil {
		q.handler.log(ctx, slog.LevelInfo, "question prompt requested",
			slog.String("prompt_id", id),
			slog.String("session_id", info.SessionID),
			slog.String("turn_id", info.TurnID),
			slog.String("tool_call_id", request.ToolCallID),
			slog.Int("question_count", len(request.Questions)),
		)
	}
	answers, canceled, err := q.broker.Await(ctx, prompt, func(value *v1.QuestionPrompt) error {
		resp := new(v1.RunResponse)
		resp.SetQuestionPrompt(value)
		return q.publish(resp)
	})
	if err != nil {
		if q.handler != nil && errors.Is(err, context.Canceled) {
			q.handler.log(ctx, slog.LevelInfo, "question prompt canceled",
				slog.String("prompt_id", id),
				slog.String("session_id", info.SessionID),
				slog.String("turn_id", info.TurnID),
				slog.Duration("duration", time.Since(startedAt)),
			)
		}
		return tools.QuestionResolution{}, fmt.Errorf("server: await question answers: %w", err)
	}
	if q.handler != nil {
		q.handler.log(ctx, slog.LevelInfo, "question prompt resolved",
			slog.String("prompt_id", id),
			slog.String("session_id", info.SessionID),
			slog.String("turn_id", info.TurnID),
			slog.Bool("canceled", canceled),
			slog.Duration("duration", time.Since(startedAt)),
		)
	}
	if canceled {
		return tools.QuestionResolution{Canceled: true}, nil
	}
	return tools.QuestionResolution{Answers: answersFromProto(answers.GetAnswers())}, nil
}

func runInfo(ctx context.Context) (adktrace.RunInfo, error) {
	info, ok := adktrace.RunInfoFromContext(ctx)
	if !ok || info.SessionID == "" || info.TurnID == "" {
		return adktrace.RunInfo{}, errors.New("server: active run metadata is unavailable")
	}
	return info, nil
}

func questionsToProto(questions []tools.Question) []*v1.Question {
	result := make([]*v1.Question, len(questions))
	for index, question := range questions {
		options := make([]*v1.QuestionOption, len(question.Options))
		for optionIndex, option := range question.Options {
			options[optionIndex] = v1.QuestionOption_builder{
				Id:          new(option.ID),
				Label:       new(option.Label),
				Description: new(option.Description),
			}.Build()
		}
		result[index] = v1.Question_builder{
			Id:            new(question.ID),
			Header:        new(question.Header),
			Prompt:        new(question.Prompt),
			Options:       options,
			Multiple:      new(question.Multiple),
			AllowFreeform: new(question.AllowFreeform),
		}.Build()
	}
	return result
}

func approvalKindToProto(kind permission.Kind) v1.ToolApprovalKind {
	switch kind {
	case permission.KindFileRead:
		return v1.ToolApprovalKind_TOOL_APPROVAL_KIND_FILE_READ
	case permission.KindFileWrite:
		return v1.ToolApprovalKind_TOOL_APPROVAL_KIND_FILE_WRITE
	case permission.KindShell:
		return v1.ToolApprovalKind_TOOL_APPROVAL_KIND_SHELL
	default:
		return v1.ToolApprovalKind_TOOL_APPROVAL_KIND_UNSPECIFIED
	}
}

func approvalScopeToProto(scope permission.Scope) v1.ToolApprovalScope {
	switch scope {
	case permission.ScopeWorkspace:
		return v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_WORKSPACE
	case permission.ScopeOutsideWorkspace:
		return v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_OUTSIDE_WORKSPACE
	case permission.ScopeGlobal:
		return v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_GLOBAL
	default:
		return v1.ToolApprovalScope_TOOL_APPROVAL_SCOPE_UNSPECIFIED
	}
}

var (
	_ tools.Authorizer = brokerAuthorizer{}
	_ tools.Questioner = brokerQuestioner{}
)
