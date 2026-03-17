package agent

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/bwmarrin/snowflake"

	"github.com/soasurs/adk/agent/llmagent"
	amodel "github.com/soasurs/adk/model"
	"github.com/soasurs/adk/runner"
	"github.com/soasurs/adk/session"
	"github.com/soasurs/adk/session/compaction"
	"github.com/soasurs/adk/session/message"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/config"
)

const keepRecentRoundsOnCompact = 2

type AgentMode string

const (
	ModeBuild AgentMode = "build"
	ModePlan  AgentMode = "plan"
)

type ProviderOption struct {
	Value string
	Label string
}

type CompactResult struct {
	BeforeMessages   int
	AfterMessages    int
	ArchivedMessages int
	KeptMessages     int
	Summary          string
}

type Runtime struct {
	cfg        config.Config
	sessionSvc session.SessionService
	catalog    sessionCatalog
	tools      []tool.Tool
	runner     *runner.Runner
	llm        amodel.LLM
	modelName  string
	idNode     *snowflake.Node
	models     []string
	mode       AgentMode
}

func NewRuntime(ctx context.Context, cfg *config.Config) (*Runtime, error) {
	agentTools, err := buildTools()
	if err != nil {
		return nil, fmt.Errorf("agent runtime: build tools: %w", err)
	}

	svc, err := newSessionService(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: create session service: %w", err)
	}

	catalog, err := newSessionCatalog(cfg)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: create session catalog: %w", err)
	}

	idNode, err := snowflake.NewNode(1)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: init id generator: %w", err)
	}

	rt := &Runtime{
		cfg:        *cfg,
		sessionSvc: svc,
		catalog:    catalog,
		tools:      agentTools,
		idNode:     idNode,
		mode:       ModeBuild,
	}

	if err := rt.rebuildRunner(ctx); err != nil {
		return nil, err
	}

	return rt, nil
}

func (r *Runtime) Mode() AgentMode {
	return r.mode
}

func (r *Runtime) SetMode(ctx context.Context, mode AgentMode) error {
	if r.mode == mode {
		return nil
	}
	r.mode = mode
	return r.rebuildRunner(ctx)
}

func (r *Runtime) Run(ctx context.Context, sessionID int64, userInput string) iter.Seq2[*amodel.Event, error] {
	return r.runner.Run(ctx, sessionID, userInput)
}

func (r *Runtime) Provider() string {
	return r.cfg.Provider
}

func (r *Runtime) ProviderLabel() string {
	return providerLabel(r.cfg.Provider)
}

func (r *Runtime) ModelName() string {
	return r.modelName
}

func (r *Runtime) NewSession(ctx context.Context) (int64, error) {
	sessionID := r.idNode.Generate().Int64()
	if _, err := r.sessionSvc.CreateSession(ctx, sessionID); err != nil {
		return 0, fmt.Errorf("agent runtime: create session: %w", err)
	}
	if err := r.catalog.CreateSession(ctx, SessionMeta{
		SessionID: sessionID,
		Title:     "New Session",
		WorkDir:   r.cfg.WorkDir,
	}); err != nil {
		return 0, fmt.Errorf("agent runtime: create session metadata: %w", err)
	}
	return sessionID, nil
}

func (r *Runtime) TouchSession(ctx context.Context, sessionID int64, title string) error {
	if err := r.catalog.TouchSession(ctx, sessionID, title); err != nil {
		return fmt.Errorf("agent runtime: touch session metadata: %w", err)
	}
	return nil
}

func (r *Runtime) SetSessionTitle(ctx context.Context, sessionID int64, title string) error {
	if err := r.catalog.SetTitle(ctx, sessionID, title); err != nil {
		return fmt.Errorf("agent runtime: set session title: %w", err)
	}
	return nil
}

func (r *Runtime) GenerateTitle(ctx context.Context, userInput string) (string, error) {
	req := &amodel.LLMRequest{
		Model: r.modelName,
		Messages: []amodel.Message{
			{
				Role:    amodel.RoleSystem,
				Content: "Generate a concise 4-8 word title for this coding session based on the user's request. Return ONLY the title text—no quotes, no trailing punctuation, no explanation.",
			},
			{
				Role:    amodel.RoleUser,
				Content: userInput,
			},
		},
	}

	for resp, err := range r.llm.GenerateContent(ctx, req, nil, false) {
		if err != nil {
			return "", fmt.Errorf("agent runtime: generate title: %w", err)
		}
		if resp == nil || resp.Partial {
			continue
		}
		title := strings.TrimSpace(resp.Message.Content)
		if title == "" {
			break
		}
		return title, nil
	}

	return "", fmt.Errorf("agent runtime: generate title: empty response")
}

func (r *Runtime) ListSessions(ctx context.Context) ([]SessionMeta, error) {
	sessions, err := r.catalog.ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: list sessions: %w", err)
	}
	return sessions, nil
}

func (r *Runtime) DeleteSession(ctx context.Context, sessionID int64) error {
	if err := r.sessionSvc.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("agent runtime: delete session messages: %w", err)
	}
	if err := r.catalog.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("agent runtime: delete session metadata: %w", err)
	}
	return nil
}

func (r *Runtime) SetProvider(ctx context.Context, provider string) error {
	return r.SetProviderAPIKey(ctx, provider, "")
}

func (r *Runtime) SetProviderAPIKey(ctx context.Context, provider, apiKey string) error {
	canonical, err := canonicalProvider(provider)
	if err != nil {
		return err
	}
	r.cfg.Provider = canonical
	r.cfg.Model = ""
	if strings.TrimSpace(apiKey) != "" {
		r.cfg.APIKey = strings.TrimSpace(apiKey)
	} else {
		r.cfg.APIKey = providerAPIKey(canonical)
	}
	if err := r.rebuildRunner(ctx); err != nil {
		return err
	}
	if err := r.cfg.SaveProviderSelection(canonical, apiKey); err != nil {
		return fmt.Errorf("agent runtime: save provider selection: %w", err)
	}
	if err := r.refreshModels(ctx); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) SetModel(ctx context.Context, modelName string) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return fmt.Errorf("agent runtime: model name is required")
	}
	r.cfg.Model = modelName
	if r.cfg.APIKey == "" {
		r.cfg.APIKey = providerAPIKey(r.cfg.Provider)
	}
	if err := r.rebuildRunner(ctx); err != nil {
		return err
	}
	if err := r.cfg.SaveModelSelection(modelName); err != nil {
		return fmt.Errorf("agent runtime: save model selection: %w", err)
	}
	return nil
}

func (r *Runtime) SessionMessages(ctx context.Context, sessionID int64) ([]*message.Message, error) {
	sess, err := r.getSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	msgs, err := sess.ListMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: list messages: %w", err)
	}
	return msgs, nil
}

func (r *Runtime) CompactSession(ctx context.Context, sessionID int64) (*CompactResult, error) {
	sess, err := r.getSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	msgs, err := sess.ListMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: list messages: %w", err)
	}

	result := &CompactResult{BeforeMessages: len(msgs), AfterMessages: len(msgs)}
	if len(msgs) == 0 {
		return result, nil
	}

	compactor, err := compaction.NewSlidingWindowCompactor(
		compaction.Config{KeepRecentRounds: keepRecentRoundsOnCompact},
		r.summarizeMessages,
	)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: create compactor: %w", err)
	}

	splitID, summaryMsg, err := compactor(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: compact session: %w", err)
	}
	if summaryMsg == nil {
		result.KeptMessages = len(msgs)
		return result, nil
	}

	splitIdx := len(msgs)
	if splitID > 0 {
		for i, msg := range msgs {
			if msg.MessageID == splitID {
				splitIdx = i
				break
			}
		}
	}

	if err := sess.CompactMessages(ctx, splitID, summaryMsg); err != nil {
		return nil, fmt.Errorf("agent runtime: apply compaction: %w", err)
	}

	afterMsgs, err := sess.ListMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: list compacted session: %w", err)
	}

	result.AfterMessages = len(afterMsgs)
	result.ArchivedMessages = splitIdx
	result.KeptMessages = len(msgs) - splitIdx
	result.Summary = summaryMsg.Content
	return result, nil
}

func (r *Runtime) AvailableProviders() []ProviderOption {
	return []ProviderOption{
		{Value: "anthropic", Label: "Anthropic"},
		{Value: "openai", Label: "OpenAI"},
		{Value: "gemini", Label: "Google"},
	}
}

func (r *Runtime) AvailableModels() []string {
	base := append([]string(nil), r.models...)
	if len(base) == 0 {
		base = defaultAvailableModels(r.cfg.Provider)
	}
	if r.modelName != "" && !contains(base, r.modelName) {
		return append([]string{r.modelName}, base...)
	}
	return base
}

func (r *Runtime) RefreshModels(ctx context.Context) error {
	return r.refreshModels(ctx)
}

func (r *Runtime) rebuildRunner(ctx context.Context) error {
	llm, modelName, err := newLLM(ctx, &r.cfg)
	if err != nil {
		return fmt.Errorf("agent runtime: create LLM: %w", err)
	}

	var agentName string
	var agentDesc string
	var promptTpl string
	var activeTools []tool.Tool

	if r.mode == ModePlan {
		agentName = "koda-plan"
		agentDesc = "An expert software architect and planner"
		promptTpl = planPrompt
		activeTools = filterReadTools(r.tools)
	} else {
		agentName = "koda-build"
		agentDesc = "An expert coding assistant"
		promptTpl = systemPrompt
		activeTools = r.tools
	}

	instruction := fmt.Sprintf(promptTpl, r.cfg.WorkDir)
	if agentsMD := loadAgentsMD(r.cfg.WorkDir); agentsMD != "" {
		instruction += "\n\n---\n\n## Project-Specific Instructions (from AGENTS.md)\n\n" + agentsMD
	}

	a := llmagent.New(llmagent.Config{
		Name:        agentName,
		Description: agentDesc,
		Model:       llm,
		GenerateConfig: &amodel.GenerateConfig{
			EnableThinking: new(true),
		},
		Tools:         activeTools,
		Instruction:   instruction,
		Stream:        true,
		MaxIterations: 50,
	})

	run, err := runner.New(a, r.sessionSvc)
	if err != nil {
		return fmt.Errorf("agent runtime: create runner: %w", err)
	}

	r.llm = llm
	r.modelName = modelName
	r.runner = run
	return nil
}

func (r *Runtime) getSession(ctx context.Context, sessionID int64) (session.Session, error) {
	sess, err := r.sessionSvc.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("agent runtime: get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("agent runtime: session %d not found", sessionID)
	}
	return sess, nil
}

func (r *Runtime) summarizeMessages(ctx context.Context, msgs []*message.Message) (string, error) {
	var transcript strings.Builder
	for _, msg := range msgs {
		transcript.WriteString("[")
		transcript.WriteString(msg.Role)
		transcript.WriteString("] ")
		transcript.WriteString(msg.Content)
		transcript.WriteString("\n\n")
	}

	req := &amodel.LLMRequest{
		Model: r.modelName,
		Messages: []amodel.Message{
			{
				Role:    amodel.RoleSystem,
				Content: "Summarize the archived conversation so a coding assistant can continue it later. Preserve user goals, technical constraints, key decisions, files or packages discussed, errors encountered, and any unfinished work.",
			},
			{
				Role:    amodel.RoleUser,
				Content: transcript.String(),
			},
		},
	}

	for resp, err := range r.llm.GenerateContent(ctx, req, nil, false) {
		if err != nil {
			return "", fmt.Errorf("agent runtime: summarize session: %w", err)
		}
		if resp == nil || resp.Partial {
			continue
		}
		if strings.TrimSpace(resp.Message.Content) == "" {
			break
		}
		return resp.Message.Content, nil
	}

	return "", fmt.Errorf("agent runtime: summarize session: empty response")
}

func (r *Runtime) refreshModels(ctx context.Context) error {
	models, err := ListProviderModels(ctx, r.cfg)
	if err != nil {
		return err
	}
	r.models = models
	return nil
}

func defaultAvailableModels(provider string) []string {
	switch provider {
	case "openai":
		return []string{"gpt-4o", "gpt-4.1", "gpt-4.1-mini", "o4-mini"}
	case "gemini":
		return []string{"gemini-2.0-flash", "gemini-2.5-flash", "gemini-2.5-pro"}
	default:
		return []string{"claude-sonnet-4-5", "claude-3-7-sonnet-latest", "claude-3-5-haiku-latest"}
	}
}

func canonicalProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "openai", nil
	case "anthropic":
		return "anthropic", nil
	case "google", "gemini":
		return "gemini", nil
	default:
		return "", fmt.Errorf("agent runtime: unsupported provider %q", provider)
	}
}

func providerLabel(provider string) string {
	switch provider {
	case "openai":
		return "OpenAI"
	case "gemini":
		return "Google"
	default:
		return "Anthropic"
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func filterReadTools(all []tool.Tool) []tool.Tool {
	var readOnly []tool.Tool
	for _, t := range all {
		name := t.Definition().Name
		if name == "read_file" || name == "list_directory" || name == "grep_search" || name == "find_files" {
			readOnly = append(readOnly, t)
		}
	}
	return readOnly
}
