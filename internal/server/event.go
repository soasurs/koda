package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/store"
	"google.golang.org/protobuf/proto"
)

// Run validates transport input and forwards events from an injected
// session-specific TurnRunner. NewHandler intentionally leaves the factory
// unset, so production Run remains unimplemented until agent construction and
// complete turn semantics are added.
func (h *Handler) Run(ctx context.Context, request *v1.RunRequest, stream *connect.ServerStream[v1.RunResponse]) error {
	if h.turnRunnerFactory == nil {
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
	session, err := h.store.GetSession(ctx, id)
	if err != nil {
		return sessionError(err)
	}
	runner, err := h.turnRunnerFactory(ctx, session, mode)
	if err != nil {
		return runtimeError(err)
	}
	if runner == nil {
		return connect.NewError(connect.CodeInternal, errors.New("agent runtime returned nil runner"))
	}
	for event, err := range runner.Run(ctx, id, input) {
		if err != nil {
			return runtimeError(err)
		}
		if event == nil {
			return connect.NewError(connect.CodeInternal, errors.New("agent runtime yielded nil event"))
		}
		converted, err := eventToProto(*event)
		if err != nil {
			return connect.NewError(connect.CodeInternal, errors.New("convert agent event"))
		}
		resp := new(v1.RunResponse)
		resp.SetEvent(converted)
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
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
