package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	adkrunner "github.com/soasurs/adk/runner"
	sessionevent "github.com/soasurs/adk/session/event"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

type fakeTurnRunner struct {
	gotSessionID     string
	gotInput         model.Content
	gotEnvironment   agent.RunEnvironment
	gotEnvironmentOK bool
	gotSnapshot      agent.CompactionSnapshot
	gotSnapshotOK    bool
	events           []model.Event
	err              error
}

func (r *fakeTurnRunner) Run(ctx context.Context, sessionID string, input model.Content) iter.Seq2[*model.Event, error] {
	r.gotSessionID = sessionID
	r.gotInput = input
	r.gotEnvironment, r.gotEnvironmentOK = agent.RunEnvironmentFromContext(ctx)
	r.gotSnapshot, r.gotSnapshotOK = agent.CompactionSnapshotFromContext(ctx)
	return func(yield func(*model.Event, error) bool) {
		for index := range r.events {
			event := r.events[index]
			if !yield(&event, nil) {
				return
			}
		}
		if r.err != nil {
			yield(nil, r.err)
		}
	}
}

type fakeSessionCompactor struct {
	request agent.CompactionRequest
	result  agent.CompactionResult
	err     error
	calls   int
}

func (c *fakeSessionCompactor) Compact(_ context.Context, request agent.CompactionRequest) (agent.CompactionResult, error) {
	c.calls++
	c.request = request
	return c.result, c.err
}

func TestRunLogsLifecycleWithoutMessageContent(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", "")
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	handler.logger = logger
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	handler.titleGenerator = nil
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, _ v1.AgentMode) (TurnRunner, error) {
		return &fakeTurnRunner{events: []model.Event{{
			ID: 7, SessionID: session.ID, TurnID: "turn-1", Author: "assistant",
			Content:      model.Content{Role: model.RoleAssistant, Content: "private assistant content"},
			FinishReason: model.FinishReasonStop,
		}}}, nil
	}
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: new(created.GetSession().GetId()),
		Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input:     v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("private user content")}.Build()}}.Build(),
	}.Build())
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{"msg=\"session created\"", "msg=\"run started\"", "msg=\"run completed\"", "session_id=session-1", "turn_id=turn-1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("logger output %q does not contain %q", got, want)
		}
	}
	for _, secret := range []string{"private user content", "private assistant content"} {
		if strings.Contains(got, secret) {
			t.Fatalf("logger output contains message content %q: %q", secret, got)
		}
	}
}

func TestTurnRunnerSeamAcceptsFakeRuntime(t *testing.T) {
	fake := &fakeTurnRunner{events: []model.Event{{Content: model.Content{Role: model.RoleAssistant, Content: "done"}}}}
	var runner TurnRunner = fake
	input := model.Content{Role: model.RoleUser, Content: "hello"}
	for event, err := range runner.Run(t.Context(), "session-1", input) {
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if event.Content.Content != "done" {
			t.Fatalf("event = %+v", event)
		}
	}
	if fake.gotSessionID != "session-1" || fake.gotInput.Content != "hello" {
		t.Fatalf("fake inputs = session %q, content %+v", fake.gotSessionID, fake.gotInput)
	}
}

func TestRunStreamsInjectedTurnRunner(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sessionID := created.GetSession().GetId()
	fake := &fakeTurnRunner{events: []model.Event{
		{SessionID: sessionID, TurnID: "turn-1", Partial: true, Content: model.Content{Role: model.RoleAssistant, Content: "hel"}},
		{ID: 7, SessionID: sessionID, TurnID: "turn-1", Author: "assistant", Content: model.Content{Role: model.RoleAssistant, Content: "hello"}, FinishReason: model.FinishReasonStop, CreatedAt: 10, UpdatedAt: 11},
	}}
	var gotSession store.Session
	var gotMode v1.AgentMode
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, mode v1.AgentMode) (TurnRunner, error) {
		gotSession = session
		gotMode = mode
		return fake, nil
	}
	handler.titleGenerator = func(_ context.Context, session store.Session, input model.Content) (string, error) {
		if session.ID != sessionID || input.Parts[0].Text != "hello" {
			return "", context.Canceled
		}
		return "Generated title", nil
	}

	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("hello")}.Build()}}.Build()
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: new(sessionID),
		Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input:     input,
	}.Build())
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var events []*v1.Event
	var completed []*v1.RunCompleted
	for stream.Receive() {
		if event := stream.Msg().GetEvent(); event != nil {
			events = append(events, event)
		}
		if value := stream.Msg().GetCompleted(); value != nil {
			completed = append(completed, value)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() stream error = %v", err)
	}
	if gotSession.ID != sessionID || gotMode != v1.AgentMode_AGENT_MODE_PLAN || fake.gotSessionID != sessionID ||
		len(fake.gotInput.Parts) != 1 || fake.gotInput.Parts[0].Text != "hello" || !fake.gotEnvironmentOK ||
		fake.gotEnvironment.Workdir != gotSession.Workdir || fake.gotEnvironment.FileAccess != gotSession.FileAccess ||
		fake.gotEnvironment.ShellAccess != gotSession.ShellAccess {
		t.Fatalf("runtime factory inputs = session %+v, mode %v, runner input %+v", gotSession, gotMode, fake.gotInput)
	}
	if len(events) != 2 || !events[0].GetPartial() || events[0].GetId() != "" || events[1].GetId() != "7" || events[1].GetFinishReason() != v1.FinishReason_FINISH_REASON_STOP || len(completed) != 1 || completed[0].GetTurnId() != "turn-1" || completed[0].GetSession().GetTitle() != "Generated title" {
		t.Fatalf("Run() events = %+v, completed = %+v", events, completed)
	}
	persisted, err := handler.store.GetSession(t.Context(), sessionID)
	if err != nil || persisted.Title != "Generated title" {
		t.Fatalf("persisted session = %+v, %v", persisted, err)
	}
}

func TestRunCompactsAcknowledgedHistoryAndInjectsSnapshot(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	var logOutput bytes.Buffer
	logger, err := logging.New(&logOutput, "info", "")
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	handler.logger = logger
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	seedServerCompactionHistory(t, handler, created.GetSession().GetId(), 700)
	policy, err := resolveCompactionPolicy(1_000, config.CompactionConfig{
		TriggerPercent: 50, ReserveTokens: 200, SummaryMaxTokens: 100,
		RetainTurns: 1, RetainTokens: 500, RebaseInterval: 2,
	})
	if err != nil {
		t.Fatalf("resolveCompactionPolicy() error = %v", err)
	}
	handler.contextWindowTokens = 1_000
	handler.compaction = policy
	projectorCalls := 0
	handler.projector = adkrunner.ProjectorFunc(func(_ context.Context, input adkrunner.ProjectionInput) ([]model.Event, error) {
		projectorCalls++
		result := make([]model.Event, len(input.Events))
		for index, event := range input.Events {
			result[index] = event.ToModel()
		}
		return result, nil
	})
	compactor := &fakeSessionCompactor{result: testServerCompactionResult("first two turns", "working state")}
	expectedSegment, expectedSnapshot, err := agent.EncodeCompactionResult(compactor.result)
	if err != nil {
		t.Fatalf("EncodeCompactionResult() error = %v", err)
	}
	handler.compactorFactory = func(context.Context, store.Session) (sessionCompactor, error) { return compactor, nil }
	runner := &fakeTurnRunner{events: []model.Event{{
		ID: 99, SessionID: created.GetSession().GetId(), TurnID: "turn-4", Author: "assistant",
		Content: model.Content{Role: model.RoleAssistant, Content: "continued"}, FinishReason: model.FinishReasonStop,
	}}}
	var runnerSession store.Session
	handler.turnRunnerFactory = func(_ context.Context, session store.Session, _ v1.AgentMode) (TurnRunner, error) {
		runnerSession = session
		return runner, nil
	}
	handler.titleGenerator = nil

	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: new(created.GetSession().GetId()), Mode: v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input: v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("continue")}.Build()}}.Build(),
	}.Build())
	if err != nil {
		t.Fatalf("Run() setup error = %v", err)
	}
	var completed *v1.RunCompleted
	var progress []*v1.CompactionProgress
	for stream.Receive() {
		if value := stream.Msg().GetCompactionProgress(); value != nil {
			progress = append(progress, value)
		}
		if value := stream.Msg().GetCompleted(); value != nil {
			completed = value
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if projectorCalls != 1 || compactor.calls != 1 || len(compactor.request.Events) != 4 || compactor.request.Rebase ||
		compactor.request.ModelID != "gpt-5.6" || compactor.request.MaxTokens != 100 {
		t.Fatalf("compactor calls = %d, request = %+v", compactor.calls, compactor.request)
	}
	if runnerSession.CompactionGeneration != 1 || runnerSession.ContextMeasured || runnerSession.ContextTokens != 0 ||
		!runner.gotSnapshotOK || runner.gotSnapshot.Generation != 1 || runner.gotSnapshot.Content != expectedSnapshot {
		t.Fatalf("runner session = %+v, snapshot = %+v, present = %t", runnerSession, runner.gotSnapshot, runner.gotSnapshotOK)
	}
	if completed == nil || completed.GetSession().GetContextUsage().GetMeasured() {
		t.Fatalf("RunCompleted = %+v", completed)
	}
	if len(progress) != 2 ||
		progress[0].GetStage() != v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_STARTED ||
		progress[0].GetGeneration() != 1 || progress[0].GetContextTokens() != 700 ||
		progress[1].GetStage() != v1.CompactionProgressStage_COMPACTION_PROGRESS_STAGE_COMPLETED ||
		progress[1].GetGeneration() != 1 || progress[1].GetContextTokens() != 700 ||
		progress[1].GetSourceTokens() <= 0 || progress[1].GetEstimatedTokensAfter() <= 0 {
		t.Fatalf("compaction progress = %+v", progress)
	}
	active, err := handler.store.ListEvents(t.Context(), created.GetSession().GetId())
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(active) != 2 || active[0].TurnID != "turn-3" {
		t.Fatalf("active events = %+v", active)
	}
	history, err := client.ListEvents(t.Context(), v1.ListEventsRequest_builder{
		SessionId: new(created.GetSession().GetId()),
	}.Build())
	if err != nil {
		t.Fatalf("ListEvents(history) error = %v", err)
	}
	if len(history.GetEvents()) != 6 || history.GetCompaction().GetGeneration() != 1 ||
		history.GetCompaction().GetCompactedEventCount() != 4 || history.GetUndoableTurnId() != "turn-3" {
		t.Fatalf("ListEvents(history) = %+v", history)
	}
	current, err := handler.store.GetCurrentCompaction(t.Context(), created.GetSession().GetId())
	if err != nil || current == nil || current.Generation != 1 || current.SegmentSummary != expectedSegment || current.StateSnapshot != expectedSnapshot {
		t.Fatalf("current compaction = %+v, %v", current, err)
	}
	logs := logOutput.String()
	if !strings.Contains(logs, "msg=\"context compaction started\"") || !strings.Contains(logs, "msg=\"context compaction completed\"") {
		t.Fatalf("compaction logs = %q", logs)
	}
	for _, content := range []string{"first two turns", "working state", "event-1"} {
		if strings.Contains(logs, content) {
			t.Fatalf("compaction logs contain message content %q: %q", content, logs)
		}
	}
}

func TestRunCompactionFailurePolicy(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	seedServerCompactionHistory(t, handler, created.GetSession().GetId(), 700)
	policy, err := resolveCompactionPolicy(1_000, config.CompactionConfig{
		TriggerPercent: 50, ReserveTokens: 200, SummaryMaxTokens: 100,
		RetainTurns: 1, RetainTokens: 500, RebaseInterval: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler.compaction = policy
	compactor := &fakeSessionCompactor{err: errors.New("summary unavailable")}
	handler.compactorFactory = func(context.Context, store.Session) (sessionCompactor, error) { return compactor, nil }
	session, err := handler.store.GetSession(t.Context(), created.GetSession().GetId())
	if err != nil {
		t.Fatal(err)
	}
	soft, current, err := handler.prepareRunCompaction(t.Context(), session)
	if err != nil || current != nil || soft.ConsecutiveCompactionFailures != 1 || soft.LastCompactionAttemptUsage != 700 || compactor.calls != 1 {
		t.Fatalf("soft failure = session %+v, current %+v, calls %d, error %v", soft, current, compactor.calls, err)
	}
	deferred, _, err := handler.prepareRunCompaction(t.Context(), soft)
	if err != nil || deferred.ConsecutiveCompactionFailures != 1 || compactor.calls != 1 {
		t.Fatalf("deferred retry = session %+v, calls %d, error %v", deferred, compactor.calls, err)
	}

	handler.compaction.hardLimitTokens = 600
	_, _, err = handler.prepareRunCompaction(t.Context(), deferred)
	if connect.CodeOf(err) != connect.CodeResourceExhausted || compactor.calls != 2 {
		t.Fatalf("hard failure = calls %d, code %v, error %v", compactor.calls, connect.CodeOf(err), err)
	}
	failed, getErr := handler.store.GetSession(t.Context(), created.GetSession().GetId())
	if getErr != nil || failed.ConsecutiveCompactionFailures != 2 || failed.LastCompactionAttemptUsage != 700 {
		t.Fatalf("persisted failures = %+v, %v", failed, getErr)
	}
	compactor.err = nil
	compactor.result = testServerCompactionResult("recovered", "recovered state")
	recovered, current, err := handler.prepareRunCompaction(t.Context(), failed)
	if err != nil || current == nil || compactor.calls != 3 || recovered.CompactionGeneration != 1 ||
		recovered.ConsecutiveCompactionFailures != 0 || recovered.LastCompactionAttemptUsage != 0 || recovered.ContextMeasured {
		t.Fatalf("recovered compaction = session %+v, current %+v, calls %d, error %v", recovered, current, compactor.calls, err)
	}
}

func TestCompactionPolicySchedulesPeriodicRebase(t *testing.T) {
	policy, err := resolveCompactionPolicy(256_000, config.CompactionConfig{})
	if err != nil {
		t.Fatalf("resolveCompactionPolicy() error = %v", err)
	}
	if policy.triggerTokens != 204_800 || policy.hardLimitTokens != 223_232 || policy.shouldRebase(4) || !policy.shouldRebase(5) {
		t.Fatalf("policy = %+v", policy)
	}
	request := agent.CompactionRequest{Rebase: true}
	generations := make([]store.Compaction, 9)
	for index := range generations {
		generation := int64(index + 1)
		result := testServerCompactionResult(fmt.Sprintf("segment-%d", generation), fmt.Sprintf("snapshot-%d", generation))
		segment, snapshot, err := agent.EncodeCompactionResult(result)
		if err != nil {
			t.Fatalf("EncodeCompactionResult(%d) error = %v", generation, err)
		}
		generations[index] = store.Compaction{Generation: generation, SegmentSummary: segment, StateSnapshot: snapshot}
	}
	if err := configureRebaseRequest(&request, generations, 5); err != nil {
		t.Fatalf("configureRebaseRequest() error = %v", err)
	}
	if request.PreviousSnapshot == nil || request.PreviousSnapshot.Objective != "snapshot-5" || len(request.PriorSegmentSummaries) != 4 ||
		request.PriorSegmentSummaries[0].Overview != "segment-6" || request.PriorSegmentSummaries[3].Overview != "segment-9" {
		t.Fatalf("bounded rebase request = %+v", request)
	}
}

func TestRunUsesExpectedErrorCodes(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{}.Build())
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Run(without session ID) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}

	handler.turnRunnerFactory = func(context.Context, store.Session, v1.AgentMode) (TurnRunner, error) {
		return &fakeTurnRunner{}, nil
	}
	stream, err = client.Run(t.Context(), v1.RunRequest_builder{SessionId: new("session-1")}.Build())
	if err == nil {
		for stream.Receive() {
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Run(invalid mode) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
}

func TestRunInitializesADKSessionBeforeResolvingProvider(t *testing.T) {
	client, registry, handler := newTestService(t, staticDiscoverer{})
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID: "unconfigured", Name: "Unconfigured", Type: provider.TypeOpenAIChatCompletions,
		Enabled: true, ModelOverrides: []provider.Model{{ID: "model"}},
	}, nil); err != nil {
		t.Fatalf("Registry.Save() error = %v", err)
	}
	handler.newSessionID = func() (string, error) { return "session-1", nil }
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("unconfigured"), ModelId: new("model"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	input := v1.Input_builder{Parts: []*v1.Part{v1.Part_builder{Text: new("hello")}.Build()}}.Build()
	stream, err := client.Run(t.Context(), v1.RunRequest_builder{
		SessionId: new(created.GetSession().GetId()),
		Mode:      v1.AgentMode_AGENT_MODE_PLAN.Enum(),
		Input:     input,
	}.Build())
	var terminated *v1.RunTerminated
	if err == nil {
		for stream.Receive() {
			if value := stream.Msg().GetTerminated(); value != nil {
				terminated = value
			}
		}
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition || terminated == nil || terminated.GetState() != v1.RunState_RUN_STATE_FAILED || terminated.GetFailure().GetCode() != connect.CodeFailedPrecondition.String() {
		t.Fatalf("Run(unconfigured provider) terminated = %+v, error = %v", terminated, err)
	}
	adkSession, err := handler.store.ADKSessionService().GetSession(t.Context(), created.GetSession().GetId())
	if err != nil || adkSession == nil {
		t.Fatalf("ADK session after Run() = %v, %v; want initialized session", adkSession, err)
	}
}

func TestTerminalEvent(t *testing.T) {
	for _, test := range []struct {
		name  string
		event model.Event
		want  bool
	}{
		{name: "stop", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonStop}, want: true},
		{name: "length", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonLength}, want: true},
		{name: "content filter", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonContentFilter}, want: true},
		{name: "tool calls", event: model.Event{Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonToolCalls}, want: false},
		{name: "partial", event: model.Event{Partial: true, Content: model.Content{Role: model.RoleAssistant}, FinishReason: model.FinishReasonStop}, want: false},
		{name: "tool", event: model.Event{Content: model.Content{Role: model.RoleTool}, FinishReason: model.FinishReasonStop}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := terminalEvent(test.event); got != test.want {
				t.Fatalf("terminalEvent() = %v, want %v", got, test.want)
			}
		})
	}
}

func seedServerCompactionHistory(t *testing.T, handler *Handler, sessionID string, contextTokens int64) {
	t.Helper()
	ledger, err := handler.store.EnsureADKSession(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("EnsureADKSession() error = %v", err)
	}
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for index := 1; index <= 6; index++ {
		role := model.RoleUser
		if index%2 == 0 {
			role = model.RoleAssistant
		}
		promptTokens, completionTokens := int64(0), int64(0)
		if index == 6 {
			promptTokens = contextTokens - 50
			completionTokens = 50
		}
		if err := ledger.CreateEvent(t.Context(), &sessionevent.Event{
			EventID: int64(index), TurnID: fmt.Sprintf("turn-%d", (index+1)/2),
			Role: string(role), Content: fmt.Sprintf("event-%d", index),
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			CreatedAt: createdAt.Add(time.Duration(index) * time.Millisecond).UnixMilli(),
			UpdatedAt: createdAt.Add(time.Duration(index) * time.Millisecond).UnixMilli(),
		}); err != nil {
			t.Fatalf("CreateEvent(%d) error = %v", index, err)
		}
	}
}

func testServerCompactionResult(overview, objective string) agent.CompactionResult {
	return agent.CompactionResult{
		SegmentSummary: agent.CompactionSegmentSummary{
			SchemaVersion: agent.CompactionSchemaVersion,
			Overview:      overview, NewInformation: []string{}, Decisions: []string{}, CompletedWork: []string{},
		},
		StateSnapshot: agent.CompactionStateSnapshot{
			SchemaVersion: agent.CompactionSchemaVersion,
			Objective:     objective, UserRequirements: []string{}, Constraints: []string{}, Decisions: []string{},
			ConfirmedFacts: []string{}, Hypotheses: []string{}, CompletedWork: []string{}, CurrentProgress: []string{},
			PendingWork: []string{}, RelevantFiles: []string{}, RelevantSymbols: []string{}, CommandsAndResults: []string{},
			ErrorsAndFailures: []string{}, OpenQuestions: []string{}, NextSteps: []string{},
		},
	}
}
