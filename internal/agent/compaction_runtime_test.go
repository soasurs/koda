package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/soasurs/adk/model"
	adksession "github.com/soasurs/adk/session"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
)

func TestSelectCompactionPreservesCompleteTurns(t *testing.T) {
	events := []model.Event{
		testCompactionEvent(1, "turn-1", model.RoleUser, "first question"),
		testCompactionEvent(2, "turn-1", model.RoleAssistant, "first answer"),
		testCompactionEvent(3, "turn-2", model.RoleUser, "second question"),
		{ID: 4, TurnID: "turn-2", Content: model.Content{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.go"}`)}}}},
		{ID: 5, TurnID: "turn-2", Content: model.Content{Role: model.RoleTool, ToolResponse: &model.ToolResponse{ToolCallID: "call-1", Name: "read_file", Outcome: &tool.Result{Content: "package a"}}}},
		testCompactionEvent(6, "turn-2", model.RoleAssistant, "second answer"),
		testCompactionEvent(7, "turn-3", model.RoleUser, "third question"),
		testCompactionEvent(8, "turn-3", model.RoleAssistant, "third answer"),
		testCompactionEvent(9, "turn-4", model.RoleUser, strings.Repeat("x", 200)),
		testCompactionEvent(10, "turn-4", model.RoleAssistant, "fourth answer"),
	}

	selection, err := SelectCompaction(events, CompactionSelectorConfig{RetainTurns: 2, RetainTokens: 10_000})
	if err != nil {
		t.Fatalf("SelectCompaction() error = %v", err)
	}
	if selection.StartEventID != 1 || selection.BoundaryEventID != 7 || len(selection.Events) != 6 || len(selection.RetainedEvents) != 4 {
		t.Fatalf("selection = %+v", selection)
	}
	if selection.CompactedTurnCount != 2 || selection.RetainedTurnCount != 2 || selection.SourceTokens <= 0 || selection.RetainedTokens <= 0 {
		t.Fatalf("selection counts = %+v", selection)
	}
	selection.Events[0].Content.Content = "mutated"
	if events[0].Content.Content != "first question" {
		t.Fatal("SelectCompaction() returned aliased events")
	}

	tight, err := SelectCompaction(events, CompactionSelectorConfig{RetainTurns: 3, RetainTokens: 16})
	if err != nil {
		t.Fatalf("SelectCompaction(tight) error = %v", err)
	}
	if tight.BoundaryEventID != 9 || tight.RetainedTurnCount != 1 {
		t.Fatalf("tight selection = %+v, want newest turn retained intact", tight)
	}
}

func TestSelectCompactionRejectsInvalidHistory(t *testing.T) {
	tests := []struct {
		name   string
		events []model.Event
	}{
		{name: "one turn", events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "only")}},
		{name: "partial", events: []model.Event{{ID: 1, TurnID: "turn-1", Partial: true}, testCompactionEvent(2, "turn-2", model.RoleUser, "next")}},
		{name: "missing turn", events: []model.Event{{ID: 1}, testCompactionEvent(2, "turn-2", model.RoleUser, "next")}},
		{name: "noncontiguous", events: []model.Event{
			testCompactionEvent(1, "turn-1", model.RoleUser, "one"),
			testCompactionEvent(2, "turn-2", model.RoleUser, "two"),
			testCompactionEvent(3, "turn-1", model.RoleAssistant, "one again"),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := SelectCompaction(test.events, CompactionSelectorConfig{RetainTurns: 1, RetainTokens: 100}); err == nil {
				t.Fatal("SelectCompaction() error = nil")
			}
		})
	}
}

func TestPrepareCompactionEventsBoundsToolAndImagePayloads(t *testing.T) {
	large := "begin-" + strings.Repeat("中", 300) + "-end"
	events := []model.Event{
		{ID: 1, TurnID: "turn-1", Content: model.Content{Role: model.RoleUser, Parts: []model.ContentPart{{Type: model.ContentPartTypeImageBase64, MIMEType: "image/png", ImageBase64: strings.Repeat("a", 500)}}}},
		{ID: 2, TurnID: "turn-1", Content: model.Content{Role: model.RoleTool, ToolResponse: &model.ToolResponse{ToolCallID: "call-1", Name: "search", Outcome: &tool.Result{StructuredContent: json.RawMessage(`{"value":"` + large + `"}`)}}}},
	}
	prepared, err := PrepareCompactionEvents(events, 300)
	if err != nil {
		t.Fatalf("PrepareCompactionEvents() error = %v", err)
	}
	imageText := prepared[0].Content.Parts[0].Text
	if prepared[0].Content.Parts[0].Type != model.ContentPartTypeText || !strings.Contains(imageText, "image/png") || !strings.Contains(imageText, "sha256=") {
		t.Fatalf("prepared image = %+v", prepared[0].Content.Parts[0])
	}
	toolText := prepared[1].Content.ToolResponseValue().Text()
	if len(toolText) > 300 || !strings.Contains(toolText, "begin-") || !strings.Contains(toolText, "-end") || !strings.Contains(toolText, "compaction truncated") || !strings.Contains(toolText, "sha256=") {
		t.Fatalf("prepared tool output (%d bytes) = %q", len(toolText), toolText)
	}
	if events[0].Content.Parts[0].Type != model.ContentPartTypeImageBase64 || events[1].Content.ToolResponseValue().Text() != `{"value":"`+large+`"}` {
		t.Fatal("PrepareCompactionEvents() mutated source events")
	}
}

func TestCompactorGeneratesAndVerifiesRebasedSnapshot(t *testing.T) {
	draftJSON := testCompactionResultJSON(t, "draft segment", "draft state")
	verifiedJSON := testCompactionResultJSON(t, "verified segment", "verified state")
	checkpoint := testCompactionResult("checkpoint segment", "checkpoint state").StateSnapshot
	olderSegment := testCompactionResult("older segment", "older state").SegmentSummary
	llm := &scriptedModel{responses: []*model.LLMResponse{
		{Content: model.Content{Role: model.RoleAssistant, Content: "```json\n" + draftJSON + "\n```"}, FinishReason: model.FinishReasonStop},
		{Content: model.Content{Role: model.RoleAssistant, Content: verifiedJSON}, FinishReason: model.FinishReasonStop},
	}}
	compactor, err := NewCompactor(llm, nil)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	result, err := compactor.Compact(t.Context(), CompactionRequest{
		ModelID: "test-model", Events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "fix it")},
		PreviousSnapshot: &checkpoint, PriorSegmentSummaries: []CompactionSegmentSummary{olderSegment}, Rebase: true, Verify: true, MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if result.SegmentSummary.Overview != "verified segment" || result.StateSnapshot.Objective != "verified state" {
		t.Fatalf("Compact() = %+v", result)
	}
	if len(llm.requests) != 2 || len(llm.configs) != 2 || llm.configs[0].MaxTokens != 4096 || llm.streams[0] || llm.streams[1] {
		t.Fatalf("compaction calls = requests %d, configs %+v, streams %v", len(llm.requests), llm.configs, llm.streams)
	}
	firstPrompt := llm.requests[0].Contents[1].Content
	if !strings.Contains(firstPrompt, "MODE: rebase") || !strings.Contains(firstPrompt, "older segment") ||
		!strings.Contains(firstPrompt, "checkpoint state") || strings.Contains(firstPrompt, "PREVIOUS WORKING-STATE") ||
		strings.Contains(firstPrompt, `\"objective\"`) {
		t.Fatalf("rebase prompt = %q", firstPrompt)
	}
	if !strings.Contains(llm.requests[1].Contents[1].Content, "draft state") ||
		!strings.Contains(llm.requests[1].Contents[1].Content, "fix it") ||
		!strings.Contains(llm.requests[1].Contents[1].Content, "older segment") {
		t.Fatalf("verification prompt = %q", llm.requests[1].Contents[1].Content)
	}
}

func TestCompactionDurableSchemaRoundTripsAndRejectsText(t *testing.T) {
	result := testCompactionResult("  new decisions  ", "  finish compaction  ")
	result.StateSnapshot.NextSteps = []string{"  run real model test  "}
	segmentJSON, snapshotJSON, err := EncodeCompactionResult(result)
	if err != nil {
		t.Fatalf("EncodeCompactionResult() error = %v", err)
	}
	segment, err := DecodeCompactionSegmentSummary(segmentJSON)
	if err != nil {
		t.Fatalf("DecodeCompactionSegmentSummary() error = %v", err)
	}
	snapshot, err := DecodeCompactionStateSnapshot(snapshotJSON)
	if err != nil {
		t.Fatalf("DecodeCompactionStateSnapshot() error = %v", err)
	}
	if segment.Overview != "new decisions" || snapshot.Objective != "finish compaction" ||
		len(snapshot.NextSteps) != 1 || snapshot.NextSteps[0] != "run real model test" {
		t.Fatalf("decoded durable compaction = segment %+v, snapshot %+v", segment, snapshot)
	}
	if _, err := DecodeCompactionStateSnapshot("legacy text snapshot"); err == nil {
		t.Fatal("DecodeCompactionStateSnapshot(legacy text) error = nil")
	}

	var missing map[string]any
	if err := json.Unmarshal([]byte(snapshotJSON), &missing); err != nil {
		t.Fatal(err)
	}
	delete(missing, "next_steps")
	missingJSON, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCompactionStateSnapshot(string(missingJSON)); err == nil {
		t.Fatal("DecodeCompactionStateSnapshot(missing next_steps) error = nil")
	}
}

func TestCompactorRepairsInvalidSchemaOnce(t *testing.T) {
	var logs bytes.Buffer
	logger, _, err := logging.New(&logs, "debug", "")
	if err != nil {
		t.Fatal(err)
	}
	valid := testCompactionResultJSON(t, "repaired segment", "repaired state")
	llm := &scriptedModel{responses: []*model.LLMResponse{
		{Content: model.Content{Role: model.RoleAssistant, Content: `{"segment_summary":"bad","state_snapshot":"bad"}`}, FinishReason: model.FinishReasonStop},
		{Content: model.Content{Role: model.RoleAssistant, Content: valid}, FinishReason: model.FinishReasonStop},
	}}
	compactor, err := NewCompactor(llm, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithRequestID(t.Context(), "repair-request")
	result, err := compactor.Compact(ctx, CompactionRequest{
		ModelID: "test-model", Events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "fix it")}, MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if result.StateSnapshot.Objective != "repaired state" || len(llm.requests) != 2 {
		t.Fatalf("Compact() = %+v, requests = %d", result, len(llm.requests))
	}
	repairRequest := llm.requests[1]
	if !strings.Contains(repairRequest.Contents[0].Content, "previous compaction output was rejected") ||
		!strings.Contains(repairRequest.Contents[1].Content, "PREVIOUS INVALID OUTPUT") ||
		!strings.Contains(repairRequest.Contents[1].Content, `"segment_summary":"bad"`) {
		t.Fatalf("repair request = %+v", repairRequest.Contents)
	}
	if !strings.Contains(logs.String(), `msg="compaction output rejected"`) ||
		!strings.Contains(logs.String(), `msg="compaction output repaired"`) ||
		!strings.Contains(logs.String(), `request_id=repair-request`) ||
		strings.Contains(logs.String(), `"segment_summary":"bad"`) {
		t.Fatalf("repair logs = %q", logs.String())
	}
}

func TestCompactorUsesOneSharedRepairAcrossDraftAndVerify(t *testing.T) {
	repairedDraft := testCompactionResultJSON(t, "draft", "state")
	llm := &scriptedModel{responses: []*model.LLMResponse{
		{Content: model.Content{Role: model.RoleAssistant, Content: `{"segment_summary":"bad"}`}, FinishReason: model.FinishReasonStop},
		{Content: model.Content{Role: model.RoleAssistant, Content: repairedDraft}, FinishReason: model.FinishReasonStop},
		{Content: model.Content{Role: model.RoleAssistant, Content: `{"state_snapshot":"bad"}`}, FinishReason: model.FinishReasonStop},
	}}
	compactor, err := NewCompactor(llm, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = compactor.Compact(t.Context(), CompactionRequest{
		ModelID: "test-model", Events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "fix it")},
		Verify: true, MaxTokens: 4096,
	})
	if err == nil || len(llm.requests) != 3 {
		t.Fatalf("Compact() error = %v, requests = %d; want failed verify without a fourth call", err, len(llm.requests))
	}
}

func TestCompactorRegeneratesLengthLimitedOutputWithoutEchoingIt(t *testing.T) {
	valid := testCompactionResultJSON(t, "short segment", "short state")
	llm := &scriptedModel{responses: []*model.LLMResponse{
		{Content: model.Content{Role: model.RoleAssistant, Content: "sensitive truncated output"}, FinishReason: model.FinishReasonLength},
		{Content: model.Content{Role: model.RoleAssistant, Content: valid}, FinishReason: model.FinishReasonStop},
	}}
	compactor, err := NewCompactor(llm, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = compactor.Compact(t.Context(), CompactionRequest{
		ModelID: "test-model", Events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "fix it")}, MaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	repairPrompt := llm.requests[1].Contents[1].Content
	if strings.Contains(repairPrompt, "sensitive truncated output") || !strings.Contains(repairPrompt, "reached the token limit") {
		t.Fatalf("length repair prompt = %q", repairPrompt)
	}
}

func TestCompactorRejectsInvalidModelOutput(t *testing.T) {
	llm := &scriptedModel{responses: []*model.LLMResponse{{
		Content:      model.Content{Role: model.RoleAssistant, Content: `{"segment_summary":{"schema_version":1,"overview":"ok","new_information":[],"decisions":[],"completed_work":[]},"state_snapshot":{"schema_version":1,"objective":"state","user_requirements":[],"constraints":[],"decisions":[],"confirmed_facts":[],"hypotheses":[],"completed_work":[],"current_progress":[],"pending_work":[],"relevant_files":[],"relevant_symbols":[],"commands_and_results":[],"errors_and_failures":[],"open_questions":[],"next_steps":[]},"extra":true}`},
		FinishReason: model.FinishReasonStop,
	}}}
	compactor, err := NewCompactor(llm, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = compactor.Compact(t.Context(), CompactionRequest{
		ModelID: "test-model", Events: []model.Event{testCompactionEvent(1, "turn-1", model.RoleUser, "hello")}, MaxTokens: 100,
	})
	if err == nil {
		t.Fatal("Compact() error = nil")
	}
}

func TestFactoryCompactorUsesSessionModelWithoutReasoning(t *testing.T) {
	factory, _ := newTestFactory(t)
	var modelID, effort string
	factory.newModel = func(_ context.Context, _ provider.Provider, gotModelID, gotEffort string) (model.LLM, error) {
		modelID, effort = gotModelID, gotEffort
		return fakeModel{name: gotModelID}, nil
	}
	compactor, err := factory.Compactor(t.Context(), testSession(t.TempDir()))
	if err != nil {
		t.Fatalf("Factory.Compactor() error = %v", err)
	}
	if compactor == nil || modelID != "test-model" || effort != "" {
		t.Fatalf("Factory.Compactor() = %v, model ID = %q, effort = %q", compactor, modelID, effort)
	}
}

func TestRunnerInjectsCompactionSnapshotOnlyIntoModelRequest(t *testing.T) {
	factory, _ := newTestFactory(t)
	session := testSession(t.TempDir())
	if _, err := factory.sessions.CreateSession(t.Context(), adksession.CreateSessionRequest{SessionID: session.ID, AppID: "koda", UserID: "local"}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	llm := &scriptedModel{responses: []*model.LLMResponse{{
		Content: model.Content{Role: model.RoleAssistant, Content: "done"}, FinishReason: model.FinishReasonStop,
	}}}
	factory.newModel = func(context.Context, provider.Provider, string, string) (model.LLM, error) { return llm, nil }
	runner, err := factory.Runner(t.Context(), session, ModePlan)
	if err != nil {
		t.Fatalf("Runner() error = %v", err)
	}
	ctx := WithRunEnvironment(t.Context(), RunEnvironment{Workdir: session.Workdir, FileAccess: session.FileAccess, ShellAccess: session.ShellAccess})
	ctx, err = WithCompactionSnapshot(ctx, CompactionSnapshot{Generation: 3, Content: "current task state"})
	if err != nil {
		t.Fatalf("WithCompactionSnapshot() error = %v", err)
	}
	for _, err := range runner.Run(ctx, session.ID, model.Content{Content: "continue"}) {
		if err != nil {
			t.Fatalf("Runner.Run() error = %v", err)
		}
	}
	if len(llm.requests) != 1 || len(llm.requests[0].Contents) < 3 || llm.requests[0].Contents[1].Role != model.RoleUser ||
		!strings.Contains(llm.requests[0].Contents[1].Content, "generation=3") || !strings.Contains(llm.requests[0].Contents[1].Content, "current task state") {
		t.Fatalf("model request = %+v", llm.requests)
	}
	ledger, err := factory.sessions.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := ledger.ListEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("persisted events = %+v", events)
	}
	for _, event := range events {
		if strings.Contains(event.Content, "current task state") || strings.Contains(event.Content, "koda_compaction") {
			t.Fatalf("synthetic compaction history persisted: %+v", event)
		}
	}
}

func testCompactionEvent(id int64, turnID string, role model.Role, text string) model.Event {
	return model.Event{ID: id, TurnID: turnID, Content: model.Content{Role: role, Content: text}}
}

func testCompactionResult(overview, objective string) CompactionResult {
	return CompactionResult{
		SegmentSummary: CompactionSegmentSummary{
			SchemaVersion: CompactionSchemaVersion,
			Overview:      overview, NewInformation: []string{}, Decisions: []string{}, CompletedWork: []string{},
		},
		StateSnapshot: CompactionStateSnapshot{
			SchemaVersion: CompactionSchemaVersion,
			Objective:     objective, UserRequirements: []string{}, Constraints: []string{}, Decisions: []string{},
			ConfirmedFacts: []string{}, Hypotheses: []string{}, CompletedWork: []string{}, CurrentProgress: []string{},
			PendingWork: []string{}, RelevantFiles: []string{}, RelevantSymbols: []string{}, CommandsAndResults: []string{},
			ErrorsAndFailures: []string{}, OpenQuestions: []string{}, NextSteps: []string{},
		},
	}
}

func testCompactionResultJSON(t *testing.T, overview, objective string) string {
	t.Helper()
	value, err := json.Marshal(testCompactionResult(overview, objective))
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}
