// Package server implements Koda's Connect RPC boundary.
package server

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/soasurs/adk/model"
	adkskill "github.com/soasurs/adk/skill"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/agent"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

// Handler implements the Koda Connect service methods.
type Handler struct {
	kodav1connect.UnimplementedKodaServiceHandler

	registry     *provider.Registry
	catalog      *provider.Catalog
	store        *store.Store
	approvals    *ApprovalBroker
	questions    *QuestionBroker
	agentFactory *agent.Factory
	skills       *adkskill.Catalog
	logger       *slog.Logger

	newSessionID      func() (string, error)
	turnRunnerFactory turnRunnerFactory
	titleGenerator    func(context.Context, store.Session, model.Content) (string, error)
}

// NewHandler constructs a Handler backed by provider and skill catalogs and a
// session store.
func NewHandler(registry *provider.Registry, catalog *provider.Catalog, sessionStore *store.Store, skillCatalog *adkskill.Catalog, logger *slog.Logger) (*Handler, error) {
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
	logger = logging.OrDiscard(logger)
	agentFactory, err := agent.New(agent.Config{
		Registry: registry,
		Catalog:  catalog,
		Sessions: sessionStore.ADKSessionService(),
		Logger:   logger,
		Skills:   skillCatalog,
	})
	if err != nil {
		return nil, fmt.Errorf("server: construct agent factory: %w", err)
	}
	return &Handler{
		registry:       registry,
		catalog:        catalog,
		store:          sessionStore,
		approvals:      NewApprovalBroker(),
		questions:      NewQuestionBroker(),
		agentFactory:   agentFactory,
		skills:         skillCatalog,
		logger:         logger,
		newSessionID:   newSessionID,
		titleGenerator: agentFactory.GenerateTitle,
	}, nil
}
