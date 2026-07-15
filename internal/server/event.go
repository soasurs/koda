package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/logging"
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
	requestID, err := newInteractionID()
	if err != nil {
		return h.internalFailure(ctx, "generate run request ID", errors.New("generate run request ID"), err)
	}
	ctx = logging.WithRequestID(ctx, requestID)
	startedAt := time.Now()
	h.log(ctx, slog.LevelInfo, "run started",
		slog.String("session_id", id),
		slog.String("mode", mode.String()),
	)
	lockedCtx, unlock, err := h.store.LockRunContext(ctx, id)
	if err != nil {
		return h.sessionFailure(ctx, "lock run", err, slog.String("session_id", id))
	}
	defer unlock()

	session, err := h.store.GetSession(lockedCtx, id)
	if err != nil {
		return h.sessionFailure(lockedCtx, "load run session", err, slog.String("session_id", id))
	}
	session, currentCompaction, err := h.prepareRunCompaction(lockedCtx, session)
	if err != nil {
		return err
	}
	var runner TurnRunner
	if h.turnRunnerFactory != nil {
		runner, err = h.turnRunnerFactory(lockedCtx, session, mode)
	} else {
		_, err = h.store.EnsureADKSession(lockedCtx, id)
	}
	if h.turnRunnerFactory == nil {
		if err != nil {
			return h.sessionFailure(lockedCtx, "ensure ADK session", err, slog.String("session_id", id))
		}
		runtimeRunner, factoryErr := h.agentFactory.Runner(lockedCtx, session, agentModeToRuntime(mode))
		if factoryErr != nil {
			return h.runtimeFailure(lockedCtx, "construct runner", factoryErr,
				slog.String("session_id", id),
				slog.String("provider_id", session.ProviderID),
				slog.String("model_id", session.ModelID),
			)
		}
		runner = runtimeRunner
	}
	if err != nil {
		return h.runtimeFailure(lockedCtx, "construct injected runner", err, slog.String("session_id", id))
	}
	if runner == nil {
		err := errors.New("agent runtime returned nil runner")
		return h.internalFailure(lockedCtx, "construct runner", err, err, slog.String("session_id", id))
	}
	runCtx, cancel := context.WithCancel(lockedCtx)
	defer cancel()
	publisher := &runPublisher{stream: stream, cancel: cancel}
	runCtx = agent.WithRunInteractions(runCtx, h.runInteractions(publisher.Publish))
	runCtx = agent.WithRunEnvironment(runCtx, agent.RunEnvironment{
		Workdir:     session.Workdir,
		FileAccess:  session.FileAccess,
		ShellAccess: session.ShellAccess,
	})
	if currentCompaction != nil {
		runCtx, err = agent.WithCompactionSnapshot(runCtx, agent.CompactionSnapshot{
			Generation: currentCompaction.Generation,
			Content:    currentCompaction.StateSnapshot,
		})
		if err != nil {
			return h.internalFailure(lockedCtx, "prepare compacted history", errors.New("prepare compacted history"), err, slog.String("session_id", id))
		}
	}
	titleResult := h.startTitleGeneration(runCtx, session, input)
	var (
		turnID           string
		terminal         bool
		runErr           error
		eventCount       int
		toolCallCount    int
		promptTokens     int64
		completionTokens int64
		totalTokens      int64
	)
	for event, err := range runner.Run(runCtx, id, input) {
		if err != nil {
			runErr = h.runtimeFailure(runCtx, "execute run", err, slog.String("session_id", id))
			break
		}
		if event == nil {
			err := errors.New("agent runtime yielded nil event")
			runErr = h.internalFailure(runCtx, "receive agent event", err, err, slog.String("session_id", id))
			break
		}
		if event.TurnID != "" {
			turnID = event.TurnID
		}
		converted, err := eventToProto(*event)
		if err != nil {
			runErr = h.internalFailure(runCtx, "convert agent event", errors.New("convert agent event"), err,
				slog.String("session_id", id),
				slog.String("turn_id", turnID),
			)
			break
		}
		if !event.Partial {
			eventCount++
			toolCallCount += len(event.Content.ToolCalls)
			if event.Usage != nil {
				promptTokens += event.Usage.PromptTokens
				completionTokens += event.Usage.CompletionTokens
				totalTokens += event.Usage.TotalTokens
			}
		}
		resp := new(v1.RunResponse)
		resp.SetEvent(converted)
		if err := publisher.Publish(resp); err != nil {
			runErr = h.runtimeFailure(runCtx, "publish run event", err,
				slog.String("session_id", id),
				slog.String("turn_id", turnID),
			)
			break
		}
		terminal = terminalEvent(*event)
	}
	if runErr != nil {
		return runErr
	}
	if !terminal {
		titleResult.cancel()
		runErr = connect.NewError(connect.CodeInternal, errors.New("agent runtime ended without a terminal assistant event"))
		h.log(runCtx, slog.LevelError, "run ended without terminal event",
			slog.String("session_id", id),
			slog.String("turn_id", turnID),
		)
		if turnID != "" {
			return h.rollbackCommittedTurn(runCtx, session, turnID, runErr)
		}
		return runErr
	}
	if turnID == "" {
		titleResult.cancel()
		err := errors.New("agent runtime ended without a turn ID")
		return h.internalFailure(runCtx, "complete run", err, err, slog.String("session_id", id))
	}
	title, titleErr := titleResult.wait()
	if titleErr != nil && !errors.Is(titleErr, context.Canceled) {
		h.log(runCtx, slog.LevelWarn, "session title generation failed",
			slog.String("session_id", id),
			slog.Any("error", titleErr),
		)
	}
	committedSession, err := h.commitRunMetadata(runCtx, session, title)
	if err != nil {
		mapped := h.sessionFailure(runCtx, "commit run metadata", err,
			slog.String("session_id", id),
			slog.String("turn_id", turnID),
		)
		return h.rollbackCommittedTurn(runCtx, session, turnID, mapped)
	}
	completed := v1.RunResponse{}
	completed.SetCompleted(v1.RunCompleted_builder{
		TurnId:  new(turnID),
		Session: h.sessionToProto(committedSession),
	}.Build())
	if err := publisher.Publish(&completed); err != nil {
		mapped := h.runtimeFailure(runCtx, "publish run completion", err,
			slog.String("session_id", id),
			slog.String("turn_id", turnID),
		)
		return h.rollbackCommittedTurn(runCtx, session, turnID, mapped)
	}
	h.log(runCtx, slog.LevelInfo, "run completed",
		slog.String("session_id", id),
		slog.String("turn_id", turnID),
		slog.String("provider_id", session.ProviderID),
		slog.String("model_id", session.ModelID),
		slog.String("mode", mode.String()),
		slog.Int("event_count", eventCount),
		slog.Int("tool_call_count", toolCallCount),
		slog.Int64("prompt_tokens", promptTokens),
		slog.Int64("completion_tokens", completionTokens),
		slog.Int64("total_tokens", totalTokens),
		slog.Duration("duration", time.Since(startedAt)),
	)
	return nil
}

func (h *Handler) rollbackCommittedTurn(ctx context.Context, session store.Session, turnID string, runErr error) error {
	h.log(ctx, slog.LevelWarn, "rolling back unacknowledged turn",
		slog.String("session_id", session.ID),
		slog.String("turn_id", turnID),
		slog.String("code", connect.CodeOf(runErr).String()),
	)
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.store.RollbackTurn(cleanupCtx, session.ID, turnID, session); err != nil {
		joined := errors.Join(errors.New("rollback unacknowledged turn"), runErr, err)
		return h.internalFailure(cleanupCtx, "rollback unacknowledged turn", errors.New("rollback unacknowledged turn"), joined,
			slog.String("session_id", session.ID),
			slog.String("turn_id", turnID),
		)
	}
	h.log(cleanupCtx, slog.LevelWarn, "unacknowledged turn rolled back",
		slog.String("session_id", session.ID),
		slog.String("turn_id", turnID),
	)
	return runErr
}

type generatedTitle struct {
	result <-chan titleGenerationResult
	cancel context.CancelFunc
}

type titleGenerationResult struct {
	title string
	err   error
}

func (h *Handler) startTitleGeneration(ctx context.Context, session store.Session, input model.Content) generatedTitle {
	if h.titleGenerator == nil || session.Title != "" || session.EventCount != 0 {
		return generatedTitle{cancel: func() {}}
	}
	titleCtx, cancel := context.WithCancel(ctx)
	result := make(chan titleGenerationResult, 1)
	go func() {
		title, err := h.titleGenerator(titleCtx, session, input)
		result <- titleGenerationResult{title: title, err: err}
	}()
	return generatedTitle{result: result, cancel: cancel}
}

func (r generatedTitle) wait() (string, error) {
	defer r.cancel()
	if r.result == nil {
		return "", nil
	}
	result := <-r.result
	return result.title, result.err
}

func (h *Handler) commitRunMetadata(ctx context.Context, previous store.Session, title string) (store.Session, error) {
	if title != "" {
		return h.store.UpdateSession(ctx, previous.ID, store.UpdateSessionParams{Title: &title})
	}
	if err := h.store.TouchSession(ctx, previous.ID); err != nil {
		return store.Session{}, err
	}
	return h.store.GetSession(ctx, previous.ID)
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

// ListEvents returns all active, complete events in conversation order.
func (h *Handler) ListEvents(ctx context.Context, request *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list events request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	events, err := h.store.ListEvents(ctx, id)
	if err != nil {
		return nil, h.sessionFailure(ctx, "list events", err, slog.String("session_id", id))
	}
	converted, err := eventsToProto(events)
	if err != nil {
		return nil, h.internalFailure(ctx, "convert stored events", errors.New("convert stored events"), err,
			slog.String("session_id", id),
		)
	}
	return v1.ListEventsResponse_builder{
		Events: converted,
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
		return nil, h.sessionFailure(ctx, "undo last message", err, slog.String("session_id", id))
	}
	response := new(v1.UndoLastMessageResponse)
	response.SetTurnId(result.TurnID)
	response.SetDeletedEventCount(result.DeletedEventCount)
	if result.TurnID == "" {
		h.log(ctx, slog.LevelDebug, "session undo found no active turn", slog.String("session_id", id))
		return response, nil
	}
	input, err := inputToProto(result.Input)
	if err != nil {
		return nil, h.internalFailure(ctx, "convert removed user input", errors.New("convert removed user input"), err,
			slog.String("session_id", id),
			slog.String("turn_id", result.TurnID),
		)
	}
	response.SetInput(input)
	h.log(ctx, slog.LevelInfo, "session turn undone",
		slog.String("session_id", id),
		slog.String("turn_id", result.TurnID),
		slog.Int64("deleted_event_count", result.DeletedEventCount),
	)
	return response, nil
}
