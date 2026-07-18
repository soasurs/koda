// Package server implements Koda's Connect RPC boundary.
package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/runner"
	adkskill "github.com/soasurs/adk/skill"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/logging"
	kodamcp "github.com/soasurs/koda/internal/mcp"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

// Handler implements the Koda Connect service methods.
type Handler struct {
	kodav1connect.UnimplementedKodaServiceHandler

	registry            *provider.Registry
	catalog             *provider.Catalog
	store               *store.Store
	approvals           *ApprovalBroker
	questions           *QuestionBroker
	runs                *runManager
	agentFactory        *agent.Factory
	skills              *adkskill.Catalog
	mcp                 MCPCatalog
	logger              *slog.Logger
	contextWindowTokens int64
	compaction          compactionPolicy
	projector           runner.Projector

	newSessionID      func() (string, error)
	turnRunnerFactory turnRunnerFactory
	compactorFactory  sessionCompactorFactory
	titleGenerator    func(context.Context, store.Session, model.Content) (string, error)
}

type handlerOptions struct {
	contextWindowTokens int64
	compaction          config.CompactionConfig
}

// WithCompactionConfig configures automatic durable context compaction.
func WithCompactionConfig(value config.CompactionConfig) HandlerOption {
	return func(options *handlerOptions) error {
		options.compaction = value
		return nil
	}
}

// HandlerOption configures one Handler.
type HandlerOption func(*handlerOptions) error

// WithContextWindowTokens configures the process-wide context window budget
// reported for every session.
func WithContextWindowTokens(tokens int64) HandlerOption {
	return func(options *handlerOptions) error {
		if tokens <= 0 {
			return fmt.Errorf("server: context window tokens must be positive")
		}
		options.contextWindowTokens = tokens
		return nil
	}
}

// MCPCatalog exposes the process-wide MCP tools and display metadata needed by
// the service without coupling handler tests to live MCP connections.
type MCPCatalog interface {
	agent.MCPToolCatalog
	Servers() []kodamcp.Server
}

// NewHandler constructs a Handler backed by provider and skill catalogs and a
// session store.
func NewHandler(registry *provider.Registry, catalog *provider.Catalog, sessionStore *store.Store, skillCatalog *adkskill.Catalog, mcpCatalog MCPCatalog, logger *slog.Logger, optionValues ...HandlerOption) (*Handler, error) {
	if registry == nil {
		return nil, fmt.Errorf("server: provider registry must not be nil")
	}
	if catalog == nil {
		return nil, fmt.Errorf("server: provider catalog must not be nil")
	}
	if catalog.Registry() != registry {
		return nil, fmt.Errorf("server: provider catalog belongs to a different registry")
	}
	if sessionStore == nil {
		return nil, fmt.Errorf("server: session store must not be nil")
	}
	options := handlerOptions{contextWindowTokens: config.DefaultContextWindowTokens}
	for index, option := range optionValues {
		if option == nil {
			return nil, fmt.Errorf("server: handler option %d must not be nil", index)
		}
		if err := option(&options); err != nil {
			return nil, err
		}
	}
	compaction, err := resolveCompactionPolicy(options.contextWindowTokens, options.compaction)
	if err != nil {
		return nil, err
	}
	logger = logging.OrDiscard(logger)
	projector := runner.NewDefaultProjector()
	agentFactory, err := agent.New(agent.Config{
		Registry:  registry,
		Catalog:   catalog,
		Sessions:  sessionStore.ADKSessionService(),
		Logger:    logger,
		Skills:    skillCatalog,
		MCP:       mcpCatalog,
		Projector: projector,
	})
	if err != nil {
		return nil, fmt.Errorf("server: construct agent factory: %w", err)
	}
	runs := newRunManager(newInteractionID)
	approvals := NewApprovalBroker()
	questions := NewQuestionBroker()
	approvals.resolved = runs.resolveApproval
	questions.resolved = runs.resolveQuestion
	return &Handler{
		registry:            registry,
		catalog:             catalog,
		store:               sessionStore,
		approvals:           approvals,
		questions:           questions,
		runs:                runs,
		agentFactory:        agentFactory,
		skills:              skillCatalog,
		mcp:                 mcpCatalog,
		logger:              logger,
		contextWindowTokens: options.contextWindowTokens,
		compaction:          compaction,
		projector:           projector,
		newSessionID:        newSessionID,
		titleGenerator:      agentFactory.GenerateTitle,
	}, nil
}
