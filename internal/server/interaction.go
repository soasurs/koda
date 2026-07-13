package server

import (
	"context"
	"errors"
	"fmt"

	adktrace "github.com/soasurs/adk/trace"
	"google.golang.org/protobuf/proto"

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
			broker:  h.approvals,
			publish: publish,
			newID:   newInteractionID,
		},
		Questioner: brokerQuestioner{
			broker:  h.questions,
			publish: publish,
			newID:   newInteractionID,
		},
	}
}

type brokerAuthorizer struct {
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
		Id:            proto.String(id),
		SessionId:     proto.String(info.SessionID),
		TurnId:        proto.String(info.TurnID),
		ToolCallId:    proto.String(approval.ToolCallID),
		ToolName:      proto.String(approval.ToolName),
		Summary:       proto.String(approval.Summary),
		ArgumentsJson: proto.String(string(approval.Arguments)),
		FileChanges:   fileChangesToProto(approval.FileChanges),
		Kind:          approvalKindToProto(approval.Kind).Enum(),
		Scope:         approvalScopeToProto(approval.Scope).Enum(),
		TargetPaths:   append([]string(nil), approval.TargetPaths...),
	}.Build()
	accepted, err := a.broker.Await(ctx, request, func(v *v1.ToolApproval) error {
		resp := new(v1.RunResponse)
		resp.SetApproval(v)
		return a.publish(resp)
	})
	if err != nil {
		return fmt.Errorf("server: await tool approval: %w", err)
	}
	if !accepted {
		return tools.ErrApprovalRejected
	}
	return nil
}

type brokerQuestioner struct {
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
		Id:         proto.String(id),
		SessionId:  proto.String(info.SessionID),
		TurnId:     proto.String(info.TurnID),
		ToolCallId: proto.String(request.ToolCallID),
		Questions:  questionsToProto(request.Questions),
	}.Build()
	answers, canceled, err := q.broker.Await(ctx, prompt, func(value *v1.QuestionPrompt) error {
		resp := new(v1.RunResponse)
		resp.SetQuestionPrompt(value)
		return q.publish(resp)
	})
	if err != nil {
		return tools.QuestionResolution{}, fmt.Errorf("server: await question answers: %w", err)
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
				Id:          proto.String(option.ID),
				Label:       proto.String(option.Label),
				Description: proto.String(option.Description),
			}.Build()
		}
		result[index] = v1.Question_builder{
			Id:            proto.String(question.ID),
			Header:        proto.String(question.Header),
			Prompt:        proto.String(question.Prompt),
			Options:       options,
			Multiple:      proto.Bool(question.Multiple),
			AllowFreeform: proto.Bool(question.AllowFreeform),
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
