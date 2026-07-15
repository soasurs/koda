package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/soasurs/adk/model"

	"github.com/soasurs/koda/internal/logging"
)

// Compactor prepares cumulative working-state snapshots with an LLM. It does
// not mutate session history or durable compaction state.
type Compactor struct {
	model  model.LLM
	logger *slog.Logger
}

// CompactionRequest contains the selected history and prior durable state.
// Rebase reconstructs the snapshot from a bounded checkpoint snapshot plus
// the segment summaries accumulated since that checkpoint.
type CompactionRequest struct {
	ModelID               string
	Events                []model.Event
	PreviousSnapshot      *CompactionStateSnapshot
	PriorSegmentSummaries []CompactionSegmentSummary
	Rebase                bool
	Verify                bool
	MaxTokens             int64
}

// CompactionSchemaVersion is the durable JSON schema emitted by the
// compaction model and stored in each segment summary and state snapshot.
const CompactionSchemaVersion = 1

// CompactionSegmentSummary describes only facts introduced by the newly
// compacted history segment. It is later used to rebase cumulative state.
type CompactionSegmentSummary struct {
	SchemaVersion  int      `json:"schema_version"`
	Overview       string   `json:"overview"`
	NewInformation []string `json:"new_information"`
	Decisions      []string `json:"decisions"`
	CompletedWork  []string `json:"completed_work"`
}

// CompactionStateSnapshot is the complete standalone working state supplied
// to subsequent model requests.
type CompactionStateSnapshot struct {
	SchemaVersion      int      `json:"schema_version"`
	Objective          string   `json:"objective"`
	UserRequirements   []string `json:"user_requirements"`
	Constraints        []string `json:"constraints"`
	Decisions          []string `json:"decisions"`
	ConfirmedFacts     []string `json:"confirmed_facts"`
	Hypotheses         []string `json:"hypotheses"`
	CompletedWork      []string `json:"completed_work"`
	CurrentProgress    []string `json:"current_progress"`
	PendingWork        []string `json:"pending_work"`
	RelevantFiles      []string `json:"relevant_files"`
	RelevantSymbols    []string `json:"relevant_symbols"`
	CommandsAndResults []string `json:"commands_and_results"`
	ErrorsAndFailures  []string `json:"errors_and_failures"`
	OpenQuestions      []string `json:"open_questions"`
	NextSteps          []string `json:"next_steps"`
}

// CompactionResult is the validated model output ready for an atomic store
// commit.
type CompactionResult struct {
	SegmentSummary CompactionSegmentSummary `json:"segment_summary"`
	StateSnapshot  CompactionStateSnapshot  `json:"state_snapshot"`
}

// EncodeCompactionResult returns the versioned JSON values stored in the
// segment_summary and state_snapshot columns.
func EncodeCompactionResult(result CompactionResult) (string, string, error) {
	result, err := normalizeCompactionResult(result)
	if err != nil {
		return "", "", err
	}
	segment, err := json.Marshal(result.SegmentSummary)
	if err != nil {
		return "", "", fmt.Errorf("agent: encode compaction segment summary: %w", err)
	}
	snapshot, err := json.Marshal(result.StateSnapshot)
	if err != nil {
		return "", "", fmt.Errorf("agent: encode compaction state snapshot: %w", err)
	}
	return string(segment), string(snapshot), nil
}

// DecodeCompactionSegmentSummary decodes and validates one durable versioned
// segment summary.
func DecodeCompactionSegmentSummary(value string) (CompactionSegmentSummary, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var result CompactionSegmentSummary
	if err := decoder.Decode(&result); err != nil {
		return CompactionSegmentSummary{}, fmt.Errorf("agent: decode durable compaction segment summary: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return CompactionSegmentSummary{}, err
	}
	normalized, err := normalizeCompactionSegmentSummary(result)
	if err != nil {
		return CompactionSegmentSummary{}, err
	}
	return normalized, nil
}

// DecodeCompactionStateSnapshot decodes and validates one durable versioned
// state snapshot.
func DecodeCompactionStateSnapshot(value string) (CompactionStateSnapshot, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var result CompactionStateSnapshot
	if err := decoder.Decode(&result); err != nil {
		return CompactionStateSnapshot{}, fmt.Errorf("agent: decode durable compaction state snapshot: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return CompactionStateSnapshot{}, err
	}
	normalized, err := normalizeCompactionStateSnapshot(result)
	if err != nil {
		return CompactionStateSnapshot{}, err
	}
	return normalized, nil
}

// NewCompactor constructs a compactor using the selected session model.
func NewCompactor(llm model.LLM, logger *slog.Logger) (*Compactor, error) {
	if llm == nil {
		return nil, errors.New("agent: compactor model must not be nil")
	}
	return &Compactor{model: llm, logger: logging.OrDiscard(logger)}, nil
}

// Compact generates and optionally verifies one compaction generation.
func (c *Compactor) Compact(ctx context.Context, request CompactionRequest) (CompactionResult, error) {
	if err := ctx.Err(); err != nil {
		return CompactionResult{}, err
	}
	request.ModelID = strings.TrimSpace(request.ModelID)
	if request.ModelID == "" {
		return CompactionResult{}, errors.New("agent: compaction model ID must not be empty")
	}
	if len(request.Events) == 0 {
		return CompactionResult{}, errors.New("agent: compaction events must not be empty")
	}
	if request.MaxTokens <= 0 {
		return CompactionResult{}, errors.New("agent: compaction max tokens must be positive")
	}
	prepared, err := PrepareCompactionEvents(request.Events, DefaultCompactionToolOutputBytes)
	if err != nil {
		return CompactionResult{}, err
	}
	source, err := marshalCompactionSource(prepared)
	if err != nil {
		return CompactionResult{}, err
	}
	prompt, err := buildCompactionPrompt(request, source)
	if err != nil {
		return CompactionResult{}, err
	}
	systemPrompt, err := embeddedPrompt("prompts/compaction.md")
	if err != nil {
		return CompactionResult{}, err
	}
	repairsRemaining := 1
	draft, err := c.generateWithRepair(ctx, "draft", request.ModelID, systemPrompt, prompt, request.MaxTokens, &repairsRemaining)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: generate compaction: %w", err)
	}
	if !request.Verify {
		return draft, nil
	}

	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: encode compaction draft: %w", err)
	}
	verifyPrompt := "ORIGINAL COMPACTION INPUT:\n" + prompt + "\n\nDRAFT COMPACTION (JSON):\n" + string(draftJSON)
	verifySystemPrompt, err := embeddedPrompt("prompts/compaction_verify.md")
	if err != nil {
		return CompactionResult{}, err
	}
	verified, err := c.generateWithRepair(ctx, "verify", request.ModelID, verifySystemPrompt, verifyPrompt, request.MaxTokens, &repairsRemaining)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: verify compaction: %w", err)
	}
	return verified, nil
}

type retryableCompactionOutputError struct {
	cause    error
	response string
	reason   string
}

func (e *retryableCompactionOutputError) Error() string { return e.cause.Error() }
func (e *retryableCompactionOutputError) Unwrap() error { return e.cause }

func (c *Compactor) generateWithRepair(
	ctx context.Context,
	stage, modelID, systemPrompt, prompt string,
	maxTokens int64,
	repairsRemaining *int,
) (CompactionResult, error) {
	result, err := c.generate(ctx, modelID, systemPrompt, prompt, maxTokens)
	var outputErr *retryableCompactionOutputError
	if err == nil || !errors.As(err, &outputErr) || repairsRemaining == nil || *repairsRemaining == 0 {
		return result, err
	}
	(*repairsRemaining)--
	c.log(ctx, slog.LevelDebug, "compaction output rejected",
		slog.String("stage", stage),
		slog.Int("attempt", 1),
		slog.String("reason", outputErr.reason),
		slog.Int("response_bytes", len(outputErr.response)),
		slog.String("validation_error", outputErr.cause.Error()),
		slog.Bool("repair", true),
	)
	basePrompt, promptErr := embeddedPrompt("prompts/compaction.md")
	if promptErr != nil {
		return CompactionResult{}, promptErr
	}
	repairRules, promptErr := embeddedPrompt("prompts/compaction_repair.md")
	if promptErr != nil {
		return CompactionResult{}, promptErr
	}
	repairPrompt := buildCompactionRepairPrompt(stage, prompt, outputErr)
	repaired, repairErr := c.generate(ctx, modelID, basePrompt+"\n\n"+repairRules, repairPrompt, maxTokens)
	if repairErr != nil {
		c.log(ctx, slog.LevelDebug, "compaction output repair failed",
			slog.String("stage", stage),
			slog.Int("attempt", 2),
			slog.Any("error", repairErr),
		)
		return CompactionResult{}, fmt.Errorf("repair invalid compaction output after %v: %w", err, repairErr)
	}
	c.log(ctx, slog.LevelDebug, "compaction output repaired",
		slog.String("stage", stage),
		slog.Int("attempts", 2),
	)
	return repaired, nil
}

func (c *Compactor) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	if requestID := logging.RequestID(ctx); requestID != "" {
		attrs = append([]slog.Attr{slog.String("request_id", requestID)}, attrs...)
	}
	c.logger.LogAttrs(ctx, level, message, attrs...)
}

func buildCompactionRepairPrompt(stage, originalPrompt string, outputErr *retryableCompactionOutputError) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "STAGE: %s\nVALIDATION ERROR:\n%s\n\nORIGINAL COMPACTION INPUT:\n%s", stage, outputErr.cause, originalPrompt)
	if outputErr.reason == "schema" {
		fmt.Fprintf(&builder, "\n\nPREVIOUS INVALID OUTPUT:\n%s", outputErr.response)
	} else {
		builder.WriteString("\n\nThe previous output reached the token limit. Regenerate the complete result more concisely; do not continue the truncated output.")
	}
	return builder.String()
}

func (c *Compactor) generate(ctx context.Context, modelID, systemPrompt, prompt string, maxTokens int64) (CompactionResult, error) {
	request := &model.LLMRequest{
		Model: modelID,
		Contents: []model.Content{
			{Role: model.RoleSystem, Content: systemPrompt},
			{Role: model.RoleUser, Content: prompt},
		},
	}
	var complete *model.LLMResponse
	for response, err := range c.model.GenerateContent(ctx, request, &model.GenerateConfig{MaxTokens: maxTokens}, false) {
		if err != nil {
			return CompactionResult{}, err
		}
		if response == nil || response.Partial {
			continue
		}
		complete = response
	}
	if complete == nil {
		return CompactionResult{}, errors.New("model returned no complete response")
	}
	if complete.FinishReason == model.FinishReasonLength {
		return CompactionResult{}, &retryableCompactionOutputError{
			cause: errors.New("model reached compaction output limit"), response: complete.Content.Content, reason: "length",
		}
	}
	if complete.FinishReason != model.FinishReasonStop {
		return CompactionResult{}, fmt.Errorf("model ended compaction with finish reason %q", complete.FinishReason)
	}
	result, err := decodeCompactionResult(complete.Content.Content)
	if err != nil {
		return CompactionResult{}, &retryableCompactionOutputError{
			cause: err, response: complete.Content.Content, reason: "schema",
		}
	}
	return result, nil
}

type compactedEvent struct {
	ID                int64               `json:"id"`
	TurnID            string              `json:"turn_id"`
	Role              model.Role          `json:"role"`
	Text              string              `json:"text,omitempty"`
	Parts             []model.ContentPart `json:"parts,omitempty"`
	ToolCalls         []compactedToolCall `json:"tool_calls,omitempty"`
	ToolResponseName  string              `json:"tool_response_name,omitempty"`
	ToolResponseText  string              `json:"tool_response_text,omitempty"`
	ToolResponseError bool                `json:"tool_response_error,omitempty"`
}

type compactedToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func marshalCompactionSource(events []model.Event) ([]byte, error) {
	values := make([]compactedEvent, len(events))
	for index, event := range events {
		value := compactedEvent{
			ID:     event.ID,
			TurnID: event.TurnID,
			Role:   event.Content.Role,
			Text:   event.Content.Content,
			Parts:  event.Content.Parts,
		}
		for _, call := range event.Content.ToolCalls {
			value.ToolCalls = append(value.ToolCalls, compactedToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		if event.Content.Role == model.RoleTool {
			response := event.Content.ToolResponseValue()
			value.ToolResponseName = response.Name
			value.ToolResponseText = response.Text()
			_, value.ToolResponseError = response.Outcome.(interface{ Error() string })
		}
		values[index] = value
	}
	result, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("agent: encode compaction source: %w", err)
	}
	return result, nil
}

func buildCompactionPrompt(request CompactionRequest, source []byte) (string, error) {
	var priorLabel string
	var prior any
	if request.Rebase {
		segmentSummaries := request.PriorSegmentSummaries
		if segmentSummaries == nil {
			segmentSummaries = []CompactionSegmentSummary{}
		}
		priorLabel = "REBASE CHECKPOINT AND SEGMENT SUMMARIES (JSON)"
		prior = struct {
			CheckpointSnapshot *CompactionStateSnapshot   `json:"checkpoint_snapshot"`
			SegmentSummaries   []CompactionSegmentSummary `json:"segment_summaries"`
		}{
			CheckpointSnapshot: request.PreviousSnapshot,
			SegmentSummaries:   segmentSummaries,
		}
	} else {
		priorLabel = "PREVIOUS WORKING-STATE SNAPSHOT (JSON VALUE)"
		prior = request.PreviousSnapshot
	}
	priorJSON, err := json.Marshal(prior)
	if err != nil {
		return "", fmt.Errorf("agent: encode prior compaction state: %w", err)
	}
	mode := "incremental"
	if request.Rebase {
		mode = "rebase"
	}
	return fmt.Sprintf("MODE: %s\n%s:\n%s\n\nNEW SOURCE EVENTS (JSON):\n%s", mode, priorLabel, priorJSON, source), nil
}

func decodeCompactionResult(value string) (CompactionResult, error) {
	value = strings.TrimSpace(value)
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start < 0 || end < start {
		return CompactionResult{}, errors.New("model returned invalid compaction JSON")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(value[start : end+1]))
	decoder.DisallowUnknownFields()
	var result CompactionResult
	if err := decoder.Decode(&result); err != nil {
		return CompactionResult{}, fmt.Errorf("decode compaction JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return CompactionResult{}, err
	}
	return normalizeCompactionResult(result)
}

func normalizeCompactionResult(result CompactionResult) (CompactionResult, error) {
	segment, err := normalizeCompactionSegmentSummary(result.SegmentSummary)
	if err != nil {
		return CompactionResult{}, err
	}
	snapshot, err := normalizeCompactionStateSnapshot(result.StateSnapshot)
	if err != nil {
		return CompactionResult{}, err
	}
	result.SegmentSummary = segment
	result.StateSnapshot = snapshot
	return result, nil
}

func normalizeCompactionSegmentSummary(segment CompactionSegmentSummary) (CompactionSegmentSummary, error) {
	if segment.SchemaVersion != CompactionSchemaVersion {
		return CompactionSegmentSummary{}, fmt.Errorf("model returned segment summary schema version %d, want %d", segment.SchemaVersion, CompactionSchemaVersion)
	}
	segment.Overview = strings.TrimSpace(segment.Overview)
	if segment.Overview == "" {
		return CompactionSegmentSummary{}, errors.New("model returned an empty segment summary overview")
	}
	fields := []struct {
		name  string
		value *[]string
	}{
		{"segment_summary.new_information", &segment.NewInformation},
		{"segment_summary.decisions", &segment.Decisions},
		{"segment_summary.completed_work", &segment.CompletedWork},
	}
	for _, field := range fields {
		if err := normalizeRequiredStringSlice(field.name, field.value); err != nil {
			return CompactionSegmentSummary{}, err
		}
	}
	return segment, nil
}

func normalizeCompactionStateSnapshot(snapshot CompactionStateSnapshot) (CompactionStateSnapshot, error) {
	if snapshot.SchemaVersion != CompactionSchemaVersion {
		return CompactionStateSnapshot{}, fmt.Errorf("model returned state snapshot schema version %d, want %d", snapshot.SchemaVersion, CompactionSchemaVersion)
	}
	snapshot.Objective = strings.TrimSpace(snapshot.Objective)
	if snapshot.Objective == "" {
		return CompactionStateSnapshot{}, errors.New("model returned an empty state snapshot objective")
	}
	fields := []struct {
		name  string
		value *[]string
	}{
		{"state_snapshot.user_requirements", &snapshot.UserRequirements},
		{"state_snapshot.constraints", &snapshot.Constraints},
		{"state_snapshot.decisions", &snapshot.Decisions},
		{"state_snapshot.confirmed_facts", &snapshot.ConfirmedFacts},
		{"state_snapshot.hypotheses", &snapshot.Hypotheses},
		{"state_snapshot.completed_work", &snapshot.CompletedWork},
		{"state_snapshot.current_progress", &snapshot.CurrentProgress},
		{"state_snapshot.pending_work", &snapshot.PendingWork},
		{"state_snapshot.relevant_files", &snapshot.RelevantFiles},
		{"state_snapshot.relevant_symbols", &snapshot.RelevantSymbols},
		{"state_snapshot.commands_and_results", &snapshot.CommandsAndResults},
		{"state_snapshot.errors_and_failures", &snapshot.ErrorsAndFailures},
		{"state_snapshot.open_questions", &snapshot.OpenQuestions},
		{"state_snapshot.next_steps", &snapshot.NextSteps},
	}
	for _, field := range fields {
		if err := normalizeRequiredStringSlice(field.name, field.value); err != nil {
			return CompactionStateSnapshot{}, err
		}
	}
	return snapshot, nil
}

func normalizeRequiredStringSlice(name string, values *[]string) error {
	if *values == nil {
		return fmt.Errorf("model omitted required compaction field %s", name)
	}
	for index := range *values {
		(*values)[index] = strings.TrimSpace((*values)[index])
		if (*values)[index] == "" {
			return fmt.Errorf("model returned an empty item in compaction field %s", name)
		}
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode compaction JSON: trailing value")
		}
		return fmt.Errorf("decode compaction JSON: %w", err)
	}
	return nil
}
