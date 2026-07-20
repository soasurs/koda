package agent

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/soasurs/adk/agent/llmagent"
	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/runner"
	adksession "github.com/soasurs/adk/session"
	adkskill "github.com/soasurs/adk/skill"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
	"github.com/soasurs/koda/internal/tools"
)

// maxCachedRunners bounds the number of cached runner instances. Each runner
// holds its own model client with an HTTP connection pool, so the cache must
// not grow without bound. Normal usage stays well below this limit; the cap
// exists only as safety against pathological session creation.
const maxCachedRunners = 32

// Mode identifies the coding capabilities exposed to an agent.
type Mode string

const (
	// ModeBuild permits both read-only and mutating coding tools.
	ModeBuild Mode = "build"
	// ModePlan permits only read-only coding tools and questions.
	ModePlan Mode = "plan"
)

// Config configures a Factory.
type Config struct {
	// Registry resolves providers and their non-serialized credentials.
	Registry *provider.Registry
	// Catalog validates a selected model and resolves its default reasoning
	// effort without performing network discovery.
	Catalog *provider.Catalog
	// Sessions is the ADK session service shared by cached runners.
	Sessions adksession.SessionService
	// Logger receives diagnostic records for ADK runtime operations.
	Logger *slog.Logger
	// Skills contains process-level Agent Skills available to every agent.
	Skills *adkskill.Catalog
	// MCP contains process-wide MCP tools and their mode/approval policy.
	MCP MCPToolCatalog
	// Projector converts durable Turn facts into safe model context.
	Projector runner.Projector
}

// MCPToolCatalog constructs mode-appropriate MCP tool slices.
type MCPToolCatalog interface {
	BuildTools(tools.Authorizer) []tool.Tool
	PlanTools() []tool.Tool
}

// Factory creates ADK runners and caches immutable model, prompt, and tool
// configuration across Runs with equivalent session settings.
type Factory struct {
	registry *provider.Registry
	catalog  *provider.Catalog
	sessions adksession.SessionService
	logger   *slog.Logger

	skillInstruction string
	skillTools       []tool.Tool
	mcpTools         []tool.Tool
	mcpPlanTools     []tool.Tool
	mu               sync.Mutex
	cache            map[cacheKey]*list.Element
	lru              *list.List

	newModel  providerModelFactory
	projector runner.Projector
}

// cacheEntry wraps a cached runner keyed by its cache key.
type cacheEntry struct {
	key    cacheKey
	runner *runner.Runner
}

type cacheKey struct {
	sessionID        string
	workdir          string
	fileAccess       string
	shellAccess      string
	providerID       string
	providerRevision uint64
	modelID          string
	reasoningEffort  string
	mode             Mode
	instructionHash  [sha256.Size]byte
}

// New constructs an empty agent Factory.
func New(config Config) (*Factory, error) {
	if config.Registry == nil {
		return nil, errors.New("agent: provider registry must not be nil")
	}
	if config.Catalog == nil {
		return nil, errors.New("agent: provider catalog must not be nil")
	}
	if config.Catalog.Registry() != config.Registry {
		return nil, errors.New("agent: provider catalog belongs to a different registry")
	}
	if config.Sessions == nil {
		return nil, errors.New("agent: session service must not be nil")
	}
	var mcpTools, mcpPlanTools []tool.Tool
	if config.MCP != nil {
		mcpTools = config.MCP.BuildTools(runtimeAuthorizer{})
		mcpPlanTools = config.MCP.PlanTools()
	}
	for index, value := range mcpTools {
		if value == nil {
			return nil, fmt.Errorf("agent: MCP tool %d must not be nil", index)
		}
	}
	for index, value := range mcpPlanTools {
		if value == nil {
			return nil, fmt.Errorf("agent: MCP Plan tool %d must not be nil", index)
		}
	}
	var skillInstruction string
	var skillTools []tool.Tool
	if config.Skills != nil && len(config.Skills.Skills()) > 0 {
		var err error
		skillInstruction, err = config.Skills.Instruction(
			adkskill.WithUsageInstruction("Call `load_skill` only when the task genuinely falls within the skill's described domain. If no skill matches, proceed without loading any skill."),
		)
		if err != nil {
			return nil, fmt.Errorf("agent: render skill catalog: %w", err)
		}
		loadTool, err := adkskill.NewLoadTool(config.Skills)
		if err != nil {
			return nil, fmt.Errorf("agent: construct load skill tool: %w", err)
		}
		readResourceTool, err := adkskill.NewReadResourceTool(config.Skills)
		if err != nil {
			return nil, fmt.Errorf("agent: construct read skill resource tool: %w", err)
		}
		skillTools = []tool.Tool{loadTool, readResourceTool}
	}
	return &Factory{
		registry:         config.Registry,
		catalog:          config.Catalog,
		sessions:         config.Sessions,
		logger:           logging.OrDiscard(config.Logger),
		skillInstruction: skillInstruction,
		skillTools:       skillTools,
		mcpTools:         append([]tool.Tool(nil), mcpTools...),
		mcpPlanTools:     append([]tool.Tool(nil), mcpPlanTools...),
		cache:            make(map[cacheKey]*list.Element),
		lru:              list.New(),
		newModel:         newProviderModel,
		projector:        config.Projector,
	}, nil
}

// Runner returns the cached runner matching session and mode. It resolves
// model defaults locally and creates a distinct entry when session-scoped tool
// settings or workspace instructions change.
func (f *Factory) Runner(ctx context.Context, session store.Session, mode Mode) (*runner.Runner, error) {
	if err := validateSession(session); err != nil {
		return nil, err
	}
	if !mode.valid() {
		return nil, fmt.Errorf("agent: invalid mode %q", mode)
	}

	value, reasoningEffort, err := f.resolveProviderAndModel(ctx, session)
	if err != nil {
		return nil, err
	}
	instruction, instructionProvider, instructionHash, err := instructionConfiguration(mode, session.Workdir, f.skillInstruction)
	if err != nil {
		return nil, err
	}
	key := cacheKey{
		sessionID:        session.ID,
		workdir:          session.Workdir,
		fileAccess:       string(session.FileAccess),
		shellAccess:      string(session.ShellAccess),
		providerID:       value.ID,
		providerRevision: value.Revision(),
		modelID:          session.ModelID,
		reasoningEffort:  reasoningEffort,
		mode:             mode,
		instructionHash:  instructionHash,
	}

	f.mu.Lock()
	if elem := f.cache[key]; elem != nil {
		f.lru.MoveToFront(elem)
		f.mu.Unlock()
		return elem.Value.(*cacheEntry).runner, nil
	}
	f.mu.Unlock()

	llm, err := f.newModel(ctx, value, session.ModelID, reasoningEffort)
	if err != nil {
		return nil, err
	}
	toolConfig := tools.Config{
		Workdir:     session.Workdir,
		FileAccess:  session.FileAccess,
		ShellAccess: session.ShellAccess,
		Authorizer:  runtimeAuthorizer{},
		Questioner:  runtimeQuestioner{},
		Logger:      f.logger,
	}
	mcpTools := f.mcpTools
	if mode == ModePlan {
		mcpTools = f.mcpPlanTools
	}
	additional := make([]tool.Tool, 0, len(f.skillTools)+len(mcpTools))
	additional = append(additional, f.skillTools...)
	additional = append(additional, mcpTools...)
	values, err := toolsForMode(mode, toolConfig, additional)
	if err != nil {
		return nil, fmt.Errorf("agent: construct tools: %w", err)
	}
	llmAgent, err := llmagent.NewWithError(llmagent.Config{
		Name:                string(mode),
		Description:         modeDescription(mode),
		Model:               llm,
		Tools:               values,
		BeforeLLMCalls:      []llmagent.BeforeLLMCall{compactionHistoryHook},
		Instruction:         instruction,
		InstructionProvider: instructionProvider,
		GenerateConfig:      generateConfigFor(value.Type, reasoningEffort),
		Stream:              true,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: construct %s agent: %w", mode, err)
	}
	result, err := runner.New(
		turnCompletionAgent{delegate: llmAgent},
		f.sessions,
		runner.WithTracer(logging.NewADKTracer(f.logger)),
		runner.WithProjector(f.projector),
	)
	if err != nil {
		return nil, fmt.Errorf("agent: construct runner: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if elem := f.cache[key]; elem != nil {
		f.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry).runner, nil
	}
	f.evictSupersededLocked(key)
	entry := &cacheEntry{key: key, runner: result}
	f.cache[key] = f.lru.PushFront(entry)
	if f.lru.Len() > maxCachedRunners {
		f.evictLRULocked()
	}
	return result, nil
}

// Compactor returns a compactor using the session's selected provider and
// model. Compactors are short-lived and are not added to the interactive
// Runner cache.
func (f *Factory) Compactor(ctx context.Context, session store.Session) (*Compactor, error) {
	if err := validateSession(session); err != nil {
		return nil, err
	}
	value, _, err := f.resolveProviderAndModel(ctx, session)
	if err != nil {
		return nil, err
	}
	llm, err := f.newModel(ctx, value, session.ModelID, "")
	if err != nil {
		return nil, err
	}
	result, err := NewCompactor(llm, f.logger)
	if err != nil {
		return nil, fmt.Errorf("agent: construct compactor: %w", err)
	}
	return result, nil
}

func generateConfigFor(providerType provider.Type, reasoningEffort string) *model.GenerateConfig {
	if providerType != provider.TypeAnthropic {
		return nil
	}
	budget, err := anthropicThinkingBudget(reasoningEffort)
	if err != nil {
		return nil
	}
	if budget == 0 {
		return nil
	}
	return &model.GenerateConfig{MaxTokens: budget + 2048}
}

func (f *Factory) resolveProviderAndModel(ctx context.Context, session store.Session) (provider.Provider, string, error) {
	value, err := f.registry.Get(ctx, session.ProviderID)
	if err != nil {
		return provider.Provider{}, "", fmt.Errorf("agent: resolve provider: %w", err)
	}
	if !value.Enabled {
		return provider.Provider{}, "", fmt.Errorf("agent: provider %q is disabled", session.ProviderID)
	}
	catalog, err := f.catalog.List(ctx, session.ProviderID)
	if err != nil {
		return provider.Provider{}, "", fmt.Errorf("agent: resolve model catalog: %w", err)
	}
	for _, candidate := range catalog.Models {
		if candidate.ID != session.ModelID {
			continue
		}
		effort := strings.TrimSpace(session.ReasoningEffort)
		if effort == "" {
			effort = candidate.DefaultReasoningEffort
		}
		if effort != "" && !slices.Contains(candidate.ReasoningEfforts, effort) {
			return provider.Provider{}, "", fmt.Errorf(
				"agent: model %q does not support reasoning effort %q",
				session.ModelID,
				effort,
			)
		}
		return value, effort, nil
	}
	return provider.Provider{}, "", fmt.Errorf(
		"agent: provider %q does not expose model %q",
		session.ProviderID,
		session.ModelID,
	)
}

func (f *Factory) evictSupersededLocked(key cacheKey) {
	for candidate, elem := range f.cache {
		if candidate.providerID == key.providerID && candidate.providerRevision != key.providerRevision {
			f.lru.Remove(elem)
			delete(f.cache, candidate)
			continue
		}
		if candidate.sessionID == key.sessionID && candidate.mode == key.mode && candidate != key {
			f.lru.Remove(elem)
			delete(f.cache, candidate)
		}
	}
}

// evictLRULocked removes the least-recently-used cached runner. It must be
// called while f.mu is held and only when the cache exceeds maxCachedRunners.
func (f *Factory) evictLRULocked() {
	elem := f.lru.Back()
	if elem == nil {
		return
	}
	entry := elem.Value.(*cacheEntry)
	f.lru.Remove(elem)
	delete(f.cache, entry.key)
}

func validateSession(session store.Session) error {
	if strings.TrimSpace(session.ID) == "" {
		return errors.New("agent: session ID must not be empty")
	}
	if strings.TrimSpace(session.Workdir) == "" {
		return errors.New("agent: session workdir must not be empty")
	}
	if strings.TrimSpace(session.ProviderID) == "" {
		return errors.New("agent: session provider ID must not be empty")
	}
	if strings.TrimSpace(session.ModelID) == "" {
		return errors.New("agent: session model ID must not be empty")
	}
	if !session.FileAccess.Valid() {
		return fmt.Errorf("agent: invalid file access %q", session.FileAccess)
	}
	if !session.ShellAccess.Valid() {
		return fmt.Errorf("agent: invalid shell access %q", session.ShellAccess)
	}
	return nil
}

func (m Mode) valid() bool {
	return m == ModeBuild || m == ModePlan
}

func toolsForMode(mode Mode, config tools.Config, additional []tool.Tool) ([]tool.Tool, error) {
	var values []tool.Tool
	var err error
	switch mode {
	case ModeBuild:
		values, err = tools.NewBuild(config)
	case ModePlan:
		values, err = tools.NewReadOnly(config)
	default:
		return nil, fmt.Errorf("agent: invalid mode %q", mode)
	}
	if err != nil {
		return nil, err
	}
	return append(values, additional...), nil
}

func modeDescription(mode Mode) string {
	if mode == ModePlan {
		return "Read-only coding-planning agent"
	}
	return "Coding agent with workspace-aware tools"
}
