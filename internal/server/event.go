package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/store"
)

// Run validates transport input, executes one cached agent turn, and streams
// events and transient frontend interactions. The handler holds the session
// run lock through the final completion frame so acknowledged history and
// session metadata remain consistent.
func (h *Handler) Run(ctx context.Context, request *v1.RunRequest, stream *connect.ServerStream[v1.RunResponse]) error {
	if h.turnRunnerFactory == nil && h.agentFactory == nil {
		return connect.NewError(connect.CodeUnimplemented, errors.New("agent runtime is not configured"))
	}
	if request == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("run request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	mode, err := agentModeFromProto(request.GetMode())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	input, err := inputFromProto(request.GetInput())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	lockedCtx, unlock, err := h.store.LockRunContext(ctx, id)
	if err != nil {
		return sessionError(err)
	}
	defer unlock()

	session, err := h.store.GetSession(lockedCtx, id)
	if err != nil {
		return sessionError(err)
	}
	var runner TurnRunner
	if h.turnRunnerFactory != nil {
		runner, err = h.turnRunnerFactory(lockedCtx, session, mode)
	} else {
		_, err = h.store.EnsureADKSession(lockedCtx, id)
	}
	if h.turnRunnerFactory == nil {
		if err != nil {
			return sessionError(err)
		}
		runtimeRunner, factoryErr := h.agentFactory.Runner(lockedCtx, session, agentModeToRuntime(mode))
		if factoryErr != nil {
			return runtimeError(factoryErr)
		}
		runner = runtimeRunner
	}
	if err != nil {
		return runtimeError(err)
	}
	if runner == nil {
		return connect.NewError(connect.CodeInternal, errors.New("agent runtime returned nil runner"))
	}
	runCtx, cancel := context.WithCancel(lockedCtx)
	defer cancel()
	publisher := &runPublisher{stream: stream, cancel: cancel}
	runCtx = agent.WithRunInteractions(runCtx, h.runInteractions(publisher.Publish))
	var (
		turnID   string
		terminal bool
		runErr   error
	)
	for event, err := range runner.Run(runCtx, id, input) {
		if err != nil {
			runErr = runtimeError(err)
			break
		}
		if event == nil {
			runErr = connect.NewError(connect.CodeInternal, errors.New("agent runtime yielded nil event"))
			break
		}
		if event.TurnID != "" {
			turnID = event.TurnID
		}
		converted, err := eventToProto(*event)
		if err != nil {
			runErr = connect.NewError(connect.CodeInternal, errors.New("convert agent event"))
			break
		}
		resp := new(v1.RunResponse)
		resp.SetEvent(converted)
		if err := publisher.Publish(resp); err != nil {
			runErr = runtimeError(err)
			break
		}
		terminal = terminalEvent(*event)
	}
	if runErr != nil {
		return runErr
	}
	if !terminal {
		runErr = connect.NewError(connect.CodeInternal, errors.New("agent runtime ended without a terminal assistant event"))
		if turnID != "" {
			return h.rollbackCommittedTurn(runCtx, session, turnID, runErr)
		}
		return runErr
	}
	if turnID == "" {
		return connect.NewError(connect.CodeInternal, errors.New("agent runtime ended without a turn ID"))
	}
	if err := h.store.TouchSession(runCtx, id); err != nil {
		return h.rollbackCommittedTurn(runCtx, session, turnID, sessionError(err))
	}
	completed := v1.RunResponse{}
	completed.SetCompleted(v1.RunCompleted_builder{TurnId: proto.String(turnID)}.Build())
	if err := publisher.Publish(&completed); err != nil {
		return h.rollbackCommittedTurn(runCtx, session, turnID, runtimeError(err))
	}
	return nil
}

func (h *Handler) rollbackCommittedTurn(ctx context.Context, session store.Session, turnID string, runErr error) error {
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.store.RollbackTurn(cleanupCtx, session.ID, turnID, session.UpdatedAt); err != nil {
		return connect.NewError(connect.CodeInternal, errors.Join(errors.New("rollback unacknowledged turn"), runErr, err))
	}
	return runErr
}

func terminalEvent(event model.Event) bool {
	if event.Partial || event.Content.Role != model.RoleAssistant {
		return false
	}
	switch event.FinishReason {
	case model.FinishReasonStop, model.FinishReasonLength, model.FinishReasonContentFilter:
		return true
	default:
		return false
	}
}

// ListEvents returns one page of active, complete events in conversation order.
func (h *Handler) ListEvents(ctx context.Context, request *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list events request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if request.GetLimit() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event list limit must not be negative"))
	}
	if request.GetOffset() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event list offset must not be negative"))
	}
	events, total, err := h.store.ListEvents(ctx, id, store.ListEventsParams{
		Limit:  int(request.GetLimit()),
		Offset: request.GetOffset(),
	})
	if err != nil {
		return nil, sessionError(err)
	}
	converted, err := eventsToProto(events)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("convert stored events"))
	}
	return v1.ListEventsResponse_builder{
		Events: converted,
		Total:  proto.Int64(total),
	}.Build(), nil
}

// UndoLastMessage deletes the most recent active user turn and returns its
// original multimodal input for editor restoration.
func (h *Handler) UndoLastMessage(ctx context.Context, request *v1.UndoLastMessageRequest) (*v1.UndoLastMessageResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("undo last message request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.store.UndoLastMessage(ctx, id)
	if err != nil {
		return nil, sessionError(err)
	}
	response := new(v1.UndoLastMessageResponse)
	response.SetTurnId(result.TurnID)
	response.SetDeletedEventCount(result.DeletedEventCount)
	if result.TurnID == "" {
		return response, nil
	}
	input, err := inputToProto(result.Input)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("convert removed user input"))
	}
	response.SetInput(input)
	return response, nil
}
