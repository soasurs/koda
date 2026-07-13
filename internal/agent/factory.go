package agent

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/soasurs/adk/agent/llmagent"
	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/runner"
	adksession "github.com/soasurs/adk/session"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
	"github.com/soasurs/koda/internal/tools"
)

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
}

// Factory creates ADK runners and caches immutable model, prompt, and tool
// configuration across Runs with equivalent session settings.
type Factory struct {
	registry *provider.Registry
	catalog  *provider.Catalog
	sessions adksession.SessionService

	mu    sync.Mutex
	cache map[cacheKey]*runner.Runner

	newModel providerModelFactory
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
	return &Factory{
		registry: config.Registry,
		catalog:  config.Catalog,
		sessions: config.Sessions,
		cache:    make(map[cacheKey]*runner.Runner),
		newModel: newProviderModel,
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
	instruction, instructionProvider, instructionHash, err := instructionConfiguration(mode, session.Workdir)
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
	if cached := f.cache[key]; cached != nil {
		f.mu.Unlock()
		return cached, nil
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
	}
	values, err := toolsForMode(mode, toolConfig)
	if err != nil {
		return nil, fmt.Errorf("agent: construct tools: %w", err)
	}
	llmAgent, err := llmagent.NewWithError(llmagent.Config{
		Name:                string(mode),
		Description:         modeDescription(mode),
		Model:               llm,
		Tools:               values,
		Instruction:         instruction,
		InstructionProvider: instructionProvider,
		GenerateConfig:      generateConfigFor(value.Type, reasoningEffort),
		Stream:              true,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: construct %s agent: %w", mode, err)
	}
	result, err := runner.New(turnCompletionAgent{delegate: llmAgent}, f.sessions)
	if err != nil {
		return nil, fmt.Errorf("agent: construct runner: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if cached := f.cache[key]; cached != nil {
		return cached, nil
	}
	f.evictSupersededLocked(key)
	f.cache[key] = result
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
	for candidate := range f.cache {
		if candidate.providerID == key.providerID && candidate.providerRevision != key.providerRevision {
			delete(f.cache, candidate)
			continue
		}
		if candidate.sessionID == key.sessionID && candidate.mode == key.mode && candidate != key {
			delete(f.cache, candidate)
		}
	}
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

func toolsForMode(mode Mode, config tools.Config) ([]tool.Tool, error) {
	switch mode {
	case ModeBuild:
		return tools.NewBuild(config)
	case ModePlan:
		return tools.NewReadOnly(config)
	default:
		return nil, fmt.Errorf("agent: invalid mode %q", mode)
	}
}

func modeDescription(mode Mode) string {
	if mode == ModePlan {
		return "Read-only coding-planning agent"
	}
	return "Coding agent with workspace-aware tools"
}
