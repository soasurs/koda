package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/store"
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
	id, err := sessionIDFromRequest(request.SessionId)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	mode, err := agentModeFromProto(request.Mode)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	input, err := inputFromProto(request.Input)
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
		if err := stream.Send(&v1.RunResponse{Payload: &v1.RunResponse_Event{Event: converted}}); err != nil {
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
	id, err := sessionIDFromRequest(request.SessionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if request.Limit < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event list limit must not be negative"))
	}
	if request.Offset < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event list offset must not be negative"))
	}
	events, total, err := h.store.ListEvents(ctx, id, store.ListEventsParams{
		Limit:  int(request.Limit),
		Offset: request.Offset,
	})
	if err != nil {
		return nil, sessionError(err)
	}
	converted, err := eventsToProto(events)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("convert stored events"))
	}
	return &v1.ListEventsResponse{Events: converted, Total: total}, nil
}

// UndoLastMessage deletes the most recent active user turn and returns its
// original multimodal input for editor restoration.
func (h *Handler) UndoLastMessage(ctx context.Context, request *v1.UndoLastMessageRequest) (*v1.UndoLastMessageResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("undo last message request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.SessionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.store.UndoLastMessage(ctx, id)
	if err != nil {
		return nil, sessionError(err)
	}
	response := &v1.UndoLastMessageResponse{
		TurnId:            result.TurnID,
		DeletedEventCount: result.DeletedEventCount,
	}
	if result.TurnID == "" {
		return response, nil
	}
	input, err := inputToProto(result.Input)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("convert removed user input"))
	}
	response.Input = input
	return response, nil
}
