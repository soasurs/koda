package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/runner"

	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/store"
)

var errModelContextWindowIncompatible = errors.New("model context window is incompatible with compaction configuration")

type compactionPolicy struct {
	enabled          bool
	triggerTokens    int64
	hardLimitTokens  int64
	summaryMaxTokens int64
	retainTurns      int
	retainTokens     int64
	verify           bool
	rebaseInterval   int64
}

type sessionCompactor interface {
	Compact(context.Context, agent.CompactionRequest) (agent.CompactionResult, error)
}

type sessionCompactorFactory func(context.Context, store.Session) (sessionCompactor, error)

func resolveCompactionPolicy(windowTokens int64, value config.CompactionConfig) (compactionPolicy, error) {
	if windowTokens <= 0 {
		return compactionPolicy{}, errors.New("server: context window tokens must be positive")
	}
	policy := compactionPolicy{
		enabled:          value.EffectiveEnabled(),
		summaryMaxTokens: value.EffectiveSummaryMaxTokens(),
		retainTurns:      value.EffectiveRetainTurns(),
		retainTokens:     value.EffectiveRetainTokens(),
		verify:           value.EffectiveVerify(),
		rebaseInterval:   int64(value.EffectiveRebaseInterval()),
	}
	triggerPercent := value.EffectiveTriggerPercent()
	reserveTokens := value.EffectiveReserveTokens()
	if !policy.enabled {
		return policy, nil
	}
	if triggerPercent <= 0 || triggerPercent > 100 {
		return compactionPolicy{}, errors.New("server: compaction trigger percent must be between 1 and 100")
	}
	if reserveTokens <= 0 || reserveTokens >= windowTokens {
		return compactionPolicy{}, errors.New("server: compaction reserve tokens must be positive and smaller than the context window")
	}
	if policy.summaryMaxTokens <= 0 || policy.summaryMaxTokens > reserveTokens {
		return compactionPolicy{}, errors.New("server: compaction summary max tokens must be positive and no larger than the reserve")
	}
	if policy.retainTurns <= 0 || policy.retainTokens <= 0 {
		return compactionPolicy{}, errors.New("server: compaction retained turns and tokens must be positive")
	}
	if policy.rebaseInterval <= 0 {
		return compactionPolicy{}, errors.New("server: compaction rebase interval must be positive")
	}
	policy.hardLimitTokens = windowTokens - reserveTokens
	policy.triggerTokens = percentageTokens(windowTokens, triggerPercent)
	if policy.triggerTokens > policy.hardLimitTokens {
		policy.triggerTokens = policy.hardLimitTokens
	}
	return policy, nil
}

func percentageTokens(tokens int64, percent int) int64 {
	percentage := int64(percent)
	return tokens/100*percentage + tokens%100*percentage/100
}

func (p compactionPolicy) shouldAttempt(session store.Session) bool {
	if !p.enabled || !session.ContextMeasured || session.ContextTokens < p.triggerTokens {
		return false
	}
	if session.ContextTokens >= p.hardLimitTokens {
		return true
	}
	return session.ConsecutiveCompactionFailures == 0 || session.ContextTokens > session.LastCompactionAttemptUsage
}

func (p compactionPolicy) shouldRebase(nextGeneration int64) bool {
	return nextGeneration > 0 && nextGeneration%p.rebaseInterval == 0
}

func (h *Handler) compactionPolicyForSession(ctx context.Context, session store.Session) (compactionPolicy, error) {
	if err := ctx.Err(); err != nil {
		return compactionPolicy{}, err
	}
	policy, err := resolveCompactionPolicy(h.sessionContextWindowTokens(ctx, session), h.compactionConfig)
	if err != nil {
		return compactionPolicy{}, fmt.Errorf("%w: %s/%s: %v", errModelContextWindowIncompatible, session.ProviderID, session.ModelID, err)
	}
	return policy, nil
}

func (h *Handler) prepareRunCompaction(ctx context.Context, session store.Session, policy compactionPolicy) (store.Session, *store.Compaction, error) {
	if policy.shouldAttempt(session) {
		updated, err := h.compactSession(ctx, session, policy)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return store.Session{}, nil, runtimeError(err)
			}
			failed := session
			recorded, recordErr := h.store.RecordCompactionFailure(ctx, session.ID, session.CompactionGeneration, session.ContextTokens)
			if recordErr == nil {
				failed = recorded
			}
			h.log(ctx, slog.LevelWarn, "context compaction failed",
				slog.String("session_id", session.ID),
				slog.Int64("generation", session.CompactionGeneration+1),
				slog.Int64("context_tokens", session.ContextTokens),
				slog.Int("consecutive_failures", failed.ConsecutiveCompactionFailures),
				slog.Any("error", err),
				slog.Any("record_error", recordErr),
			)
			if session.ContextTokens >= policy.hardLimitTokens {
				return store.Session{}, nil, connect.NewError(connect.CodeResourceExhausted, errors.New("context compaction failed near the context limit; retry the run"))
			}
			session = failed
		} else {
			session = updated
		}
	}

	current, err := h.store.GetCurrentCompaction(ctx, session.ID)
	if err != nil {
		return store.Session{}, nil, h.sessionFailure(ctx, "load current compaction", err, slog.String("session_id", session.ID))
	}
	if session.CurrentCompactionID != 0 && (current == nil || current.ID != session.CurrentCompactionID) {
		err := fmt.Errorf("session current compaction %d is unavailable", session.CurrentCompactionID)
		return store.Session{}, nil, h.internalFailure(ctx, "load current compaction", errors.New("load current compaction"), err, slog.String("session_id", session.ID))
	}
	if current != nil {
		if _, err := agent.DecodeCompactionStateSnapshot(current.StateSnapshot); err != nil {
			return store.Session{}, nil, h.internalFailure(ctx, "decode current compaction", errors.New("decode current compaction"), err, slog.String("session_id", session.ID))
		}
	}
	return session, current, nil
}

func (h *Handler) compactSession(ctx context.Context, session store.Session, policy compactionPolicy) (store.Session, error) {
	facts, err := h.store.ListProjectionHistory(ctx, session.ID)
	if err != nil {
		return store.Session{}, fmt.Errorf("list compaction history: %w", err)
	}
	events := make([]model.Event, len(facts.Events))
	for index, event := range facts.Events {
		events[index] = event.ToModel()
	}
	selection, err := agent.SelectCompaction(events, agent.CompactionSelectorConfig{
		RetainTurns: policy.retainTurns, RetainTokens: policy.retainTokens,
	})
	if err != nil {
		return store.Session{}, err
	}
	projectedHistory, err := h.projector.Project(ctx, runner.ProjectionInput{
		Turns: facts.Turns, Events: facts.Events,
	})
	if err != nil {
		return store.Session{}, fmt.Errorf("project compaction history: %w", err)
	}
	selectedTurnIDs := make(map[string]struct{})
	knownTurnIDs := make(map[string]struct{})
	for _, event := range events {
		knownTurnIDs[event.TurnID] = struct{}{}
	}
	for _, event := range selection.Events {
		selectedTurnIDs[event.TurnID] = struct{}{}
	}
	projected := make([]model.Event, 0, len(projectedHistory))
	projectedRetained := make([]model.Event, 0, len(projectedHistory))
	for _, event := range projectedHistory {
		if _, ok := knownTurnIDs[event.TurnID]; !ok {
			return store.Session{}, fmt.Errorf("project compaction history: projected event has unknown turn ID %q", event.TurnID)
		}
		if _, selected := selectedTurnIDs[event.TurnID]; selected {
			projected = append(projected, event)
		} else {
			projectedRetained = append(projectedRetained, event)
		}
	}

	current, err := h.store.GetCurrentCompaction(ctx, session.ID)
	if err != nil {
		return store.Session{}, fmt.Errorf("load previous compaction: %w", err)
	}
	nextGeneration := session.CompactionGeneration + 1
	rebase := policy.shouldRebase(nextGeneration)
	request := agent.CompactionRequest{
		ModelID: session.ModelID, Events: projected, Rebase: rebase,
		Verify: policy.verify, MaxTokens: policy.summaryMaxTokens,
	}
	if current != nil && !rebase {
		previous, err := agent.DecodeCompactionStateSnapshot(current.StateSnapshot)
		if err != nil {
			return store.Session{}, fmt.Errorf("decode previous compaction snapshot: %w", err)
		}
		request.PreviousSnapshot = &previous
	}
	if rebase {
		generations, err := h.store.ListCompactions(ctx, session.ID)
		if err != nil {
			return store.Session{}, fmt.Errorf("list compaction generations: %w", err)
		}
		checkpointGeneration := nextGeneration - policy.rebaseInterval
		if err := configureRebaseRequest(&request, generations, checkpointGeneration); err != nil {
			return store.Session{}, err
		}
	}

	h.log(ctx, slog.LevelInfo, "context compaction started",
		slog.String("session_id", session.ID),
		slog.Int64("generation", nextGeneration),
		slog.Int64("context_tokens", session.ContextTokens),
		slog.Int("compacted_turns", selection.CompactedTurnCount),
		slog.Int("retained_turns", selection.RetainedTurnCount),
		slog.Bool("rebase", rebase),
	)
	compactor, err := h.newSessionCompactor(ctx, session)
	if err != nil {
		return store.Session{}, err
	}
	if compactor == nil {
		return store.Session{}, errors.New("server: compactor factory returned nil")
	}
	result, err := compactor.Compact(ctx, request)
	if err != nil {
		return store.Session{}, err
	}
	segmentSummary, stateSnapshot, err := agent.EncodeCompactionResult(result)
	if err != nil {
		return store.Session{}, err
	}
	sourceTokens := agent.EstimateEventsTokens(projected)
	retainedTokens := agent.EstimateEventsTokens(projectedRetained)
	estimatedTokensAfter := retainedTokens + agent.EstimateContentTokens(modelSnapshotContent(stateSnapshot))
	committed, err := h.store.CommitCompaction(ctx, session.ID, store.CommitCompactionParams{
		ExpectedGeneration: session.CompactionGeneration,
		StartEventID:       selection.StartEventID, BoundaryEventID: selection.BoundaryEventID,
		SegmentSummary: segmentSummary, StateSnapshot: stateSnapshot,
		SourceTokens: sourceTokens, EstimatedTokensAfter: estimatedTokensAfter,
		ModelID: session.ModelID,
	})
	if err != nil {
		return store.Session{}, err
	}
	h.log(ctx, slog.LevelInfo, "context compaction completed",
		slog.String("session_id", session.ID),
		slog.Int64("generation", committed.Generation),
		slog.Int64("source_tokens", committed.SourceTokens),
		slog.Int64("estimated_tokens_after", committed.EstimatedTokensAfter),
		slog.Int64("boundary_event_id", committed.BoundaryEventID),
		slog.Bool("rebase", rebase),
	)
	return h.store.GetSession(ctx, session.ID)
}

func configureRebaseRequest(request *agent.CompactionRequest, generations []store.Compaction, checkpointGeneration int64) error {
	if request == nil {
		return errors.New("server: rebase request must not be nil")
	}
	checkpointFound := checkpointGeneration == 0
	for _, generation := range generations {
		switch {
		case generation.Generation == checkpointGeneration:
			snapshot, err := agent.DecodeCompactionStateSnapshot(generation.StateSnapshot)
			if err != nil {
				return fmt.Errorf("decode compaction checkpoint generation %d: %w", generation.Generation, err)
			}
			request.PreviousSnapshot = &snapshot
			checkpointFound = true
		case generation.Generation > checkpointGeneration:
			segment, err := agent.DecodeCompactionSegmentSummary(generation.SegmentSummary)
			if err != nil {
				return fmt.Errorf("decode compaction segment generation %d: %w", generation.Generation, err)
			}
			request.PriorSegmentSummaries = append(request.PriorSegmentSummaries, segment)
		}
	}
	if !checkpointFound {
		return fmt.Errorf("compaction checkpoint generation %d is missing", checkpointGeneration)
	}
	return nil
}

func (h *Handler) newSessionCompactor(ctx context.Context, session store.Session) (sessionCompactor, error) {
	if h.compactorFactory != nil {
		return h.compactorFactory(ctx, session)
	}
	return h.agentFactory.Compactor(ctx, session)
}

func modelSnapshotContent(snapshot string) model.Content {
	return model.Content{Role: model.RoleUser, Content: snapshot}
}
