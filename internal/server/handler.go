// Package server implements Koda's Connect RPC boundary.
package server

import (
	"fmt"

	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

// Handler implements the Koda Connect service methods that do not require an
// LLM runtime. Unimplemented methods retain Connect's CodeUnimplemented
// behavior until their corresponding runtime layer is available.
type Handler struct {
	kodav1connect.UnimplementedKodaServiceHandler

	registry  *provider.Registry
	catalog   *provider.Catalog
	store     *store.Store
	approvals *ApprovalBroker
	questions *QuestionBroker

	newSessionID func() (string, error)
}

// NewHandler constructs a Handler backed by registry, catalog, and store.
func NewHandler(registry *provider.Registry, catalog *provider.Catalog, sessionStore *store.Store) (*Handler, error) {
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
	return &Handler{
		registry:     registry,
		catalog:      catalog,
		store:        sessionStore,
		approvals:    NewApprovalBroker(),
		questions:    NewQuestionBroker(),
		newSessionID: newSessionID,
	}, nil
}
