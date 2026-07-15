package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	adksession "github.com/soasurs/adk/session"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/store"
)

// Run validates transport input, executes one cached agent turn, and streams
// events and transient frontend interactions. Durable Turn state is owned by
// ADK and is not rolled back when transport delivery fails.
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
	runCtx, cancel := context.WithCancel(lockedCtx)
	defer cancel()
	publisher := &runPublisher{stream: stream, cancel: cancel}

	session, err := h.store.GetSession(runCtx, id)
	if err != nil {
		return h.sessionFailure(runCtx, "load run session", err, slog.String("session_id", id))
	}
	compactionAttempted := h.compaction.shouldAttempt(session)
	compactionGeneration := session.CompactionGeneration + 1
	compactionContextTokens := session.ContextTokens
	if compactionAttempted {
		if err := publishCompactionProgress(publisher, v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_STARTED, compactionGeneration, compactionContextTokens, nil); err != nil {
			return h.runtimeFailure(runCtx, "publish compaction start", err, slog.String("session_id", id))
		}
	}
	previousCompactionGeneration := session.CompactionGeneration
	session, currentCompaction, err := h.prepareRunCompaction(runCtx, session)
	if err != nil {
		if compactionAttempted {
			if publishErr := publishCompactionProgress(publisher, v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_FAILED, compactionGeneration, compactionContextTokens, nil); publishErr != nil {
				return h.runtimeFailure(runCtx, "publish compaction failure", publishErr, slog.String("session_id", id))
			}
		}
		return err
	}
	if compactionAttempted {
		stage := v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_FAILED
		if session.CompactionGeneration > previousCompactionGeneration {
			stage = v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_COMPLETED
		}
		if err := publishCompactionProgress(publisher, stage, compactionGeneration, compactionContextTokens, currentCompaction); err != nil {
			return h.runtimeFailure(runCtx, "publish compaction completion", err, slog.String("session_id", id))
		}
	}
	var runner TurnRunner
	if h.turnRunnerFactory != nil {
		runner, err = h.turnRunnerFactory(runCtx, session, mode)
	} else {
		_, err = h.store.EnsureADKSession(runCtx, id)
	}
	if h.turnRunnerFactory == nil {
		if err != nil {
			return h.sessionFailure(runCtx, "ensure ADK session", err, slog.String("session_id", id))
		}
		runtimeRunner, factoryErr := h.agentFactory.Runner(runCtx, session, agentModeToRuntime(mode))
		if factoryErr != nil {
			return h.runtimeFailure(runCtx, "construct runner", factoryErr,
				slog.String("session_id", id),
				slog.String("provider_id", session.ProviderID),
				slog.String("model_id", session.ModelID),
			)
		}
		runner = runtimeRunner
	}
	if err != nil {
		return h.runtimeFailure(runCtx, "construct injected runner", err, slog.String("session_id", id))
	}
	if runner == nil {
		err := errors.New("agent runtime returned nil runner")
		return h.internalFailure(runCtx, "construct runner", err, err, slog.String("session_id", id))
	}
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
			return h.internalFailure(runCtx, "prepare compacted history", errors.New("prepare compacted history"), err, slog.String("session_id", id))
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
		titleResult.cancel()
		return runErr
	}
	if !terminal {
		titleResult.cancel()
		runErr = connect.NewError(connect.CodeInternal, errors.New("agent runtime ended without a terminal assistant event"))
		h.log(runCtx, slog.LevelError, "run ended without terminal event",
			slog.String("session_id", id),
			slog.String("turn_id", turnID),
		)
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
		return mapped
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
		return mapped
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

func publishCompactionProgress(publisher *runPublisher, stage v1.CompactionProgressStage, generation, contextTokens int64, compaction *store.Compaction) error {
	progress := v1.CompactionProgress_builder{
		Stage:         stage.Enum(),
		Generation:    new(generation),
		ContextTokens: new(contextTokens),
	}
	if compaction != nil && stage == v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_COMPLETED {
		progress.SourceTokens = new(compaction.SourceTokens)
		progress.EstimatedTokensAfter = new(compaction.EstimatedTokensAfter)
	}
	response := new(v1.RunResponse)
	response.SetCompactionProgress(progress.Build())
	return publisher.Publish(response)
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

// ListEvents returns complete visible history and its compaction and undo
// boundaries in one snapshot.
func (h *Handler) ListEvents(ctx context.Context, request *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list events request must not be nil"))
	}
	id, err := sessionIDFromRequest(request.GetSessionId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	history, err := h.store.ListHistory(ctx, id)
	if err != nil {
		return nil, h.sessionFailure(ctx, "list events", err, slog.String("session_id", id))
	}
	converted, err := eventsToProto(history.Events)
	if err != nil {
		return nil, h.internalFailure(ctx, "convert stored events", errors.New("convert stored events"), err,
			slog.String("session_id", id),
		)
	}
	response := v1.ListEventsResponse_builder{
		Events:         converted,
		Turns:          turnsToProto(history.Turns),
		UndoableTurnId: new(history.UndoableTurnID),
	}.Build()
	if current := history.CurrentCompaction; current != nil {
		response.SetCompaction(v1.CompactionStatus_builder{
			Generation:           new(current.Generation),
			CompactedEventCount:  new(history.CompactedEventCount),
			SourceTokens:         new(current.SourceTokens),
			EstimatedTokensAfter: new(current.EstimatedTokensAfter),
			ModelId:              new(current.ModelID),
			CreatedAt:            new(current.CreatedAt.UnixMilli()),
		}.Build())
	}
	return response, nil
}

func turnsToProto(turns []*adksession.Turn) []*v1.Turn {
	result := make([]*v1.Turn, 0, len(turns))
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		converted := v1.Turn_builder{
			Id:         new(turn.ID),
			Status:     turnStatusToProto(turn.Status).Enum(),
			Reason:     turnReasonToProto(turn.Reason).Enum(),
			StartedAt:  new(turn.StartedAt),
			FinishedAt: new(turn.FinishedAt),
		}.Build()
		if failure := turn.Failure; failure != nil {
			converted.SetFailure(v1.TurnFailure_builder{
				Code:    new(failure.Code),
				Message: new(failure.Message),
				Stage:   turnFailureStageToProto(failure.Stage).Enum(),
			}.Build())
		}
		result = append(result, converted)
	}
	return result
}

func turnStatusToProto(status adksession.TurnStatus) v1.TurnStatus {
	switch status {
	case adksession.TurnRunning:
		return v1.TurnStatus_TURN_STATUS_RUNNING
	case adksession.TurnCompleted:
		return v1.TurnStatus_TURN_STATUS_COMPLETED
	case adksession.TurnInterrupted:
		return v1.TurnStatus_TURN_STATUS_INTERRUPTED
	case adksession.TurnFailed:
		return v1.TurnStatus_TURN_STATUS_FAILED
	default:
		return v1.TurnStatus_TURN_STATUS_UNSPECIFIED
	}
}

func turnReasonToProto(reason adksession.TurnReason) v1.TurnReason {
	switch reason {
	case adksession.TurnReasonCanceled:
		return v1.TurnReason_TURN_REASON_CANCELED
	case adksession.TurnReasonDeadline:
		return v1.TurnReason_TURN_REASON_DEADLINE_EXCEEDED
	case adksession.TurnReasonConsumerStopped:
		return v1.TurnReason_TURN_REASON_CONSUMER_STOPPED
	case adksession.TurnReasonAgentError:
		return v1.TurnReason_TURN_REASON_AGENT_ERROR
	case adksession.TurnReasonAbandoned:
		return v1.TurnReason_TURN_REASON_ABANDONED
	default:
		return v1.TurnReason_TURN_REASON_UNSPECIFIED
	}
}

func turnFailureStageToProto(stage adksession.TurnFailureStage) v1.TurnFailureStage {
	switch stage {
	case adksession.TurnFailureStageAgent:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_AGENT
	case adksession.TurnFailureStageProvider:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_PROVIDER
	case adksession.TurnFailureStageTool:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_TOOL
	case adksession.TurnFailureStagePersistence:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_PERSISTENCE
	case adksession.TurnFailureStageConsumer:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_CONSUMER
	default:
		return v1.TurnFailureStage_TURN_FAILURE_STAGE_UNSPECIFIED
	}
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
	result, err := h.store.UndoLastMessage(ctx, id, request.GetExpectedTurnId())
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
