package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
)

var (
	// ErrApprovalNotFound indicates that a pending approval has expired, was
	// canceled, or was already resolved.
	ErrApprovalNotFound = errors.New("tool approval not found")
)

// ApprovalBroker coordinates in-process, run-scoped approval waits. It does
// not persist a decision: cancellation and process exit always discard pending
// requests.
type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]*pendingApproval
}

type pendingApproval struct {
	decision chan bool
}

// NewApprovalBroker constructs an empty ApprovalBroker.
func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{pending: make(map[string]*pendingApproval)}
}

// Await publishes approval and blocks until a matching Resolve call, context
// cancellation, or a publish error. approval.ID must be unique for the active
// process. The caller owns generation of its stable ID and run metadata.
func (b *ApprovalBroker) Await(ctx context.Context, approval *v1.ToolApproval, publish func(*v1.ToolApproval) error) (bool, error) {
	if ctx == nil {
		return false, errors.New("approval broker: context must not be nil")
	}
	if approval == nil {
		return false, errors.New("approval broker: approval must not be nil")
	}
	id := strings.TrimSpace(approval.GetId())
	if id == "" {
		return false, errors.New("approval broker: approval ID must not be empty")
	}
	if publish == nil {
		return false, errors.New("approval broker: publish function must not be nil")
	}
	pending := &pendingApproval{decision: make(chan bool, 1)}
	b.mu.Lock()
	if _, exists := b.pending[id]; exists {
		b.mu.Unlock()
		return false, fmt.Errorf("approval broker: duplicate approval ID %q", id)
	}
	b.pending[id] = pending
	b.mu.Unlock()

	if err := publish(approval); err != nil {
		b.remove(id, pending)
		return false, err
	}
	select {
	case approved := <-pending.decision:
		return approved, nil
	case <-ctx.Done():
		b.remove(id, pending)
		return false, ctx.Err()
	}
}

// Resolve records one decision for a pending approval. An approval can be
// resolved exactly once.
func (b *ApprovalBroker) Resolve(id string, approved bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("approval ID must not be empty")
	}
	b.mu.Lock()
	pending, exists := b.pending[id]
	if exists {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !exists {
		return fmt.Errorf("resolve approval %q: %w", id, ErrApprovalNotFound)
	}
	pending.decision <- approved
	return nil
}

func (b *ApprovalBroker) remove(id string, pending *pendingApproval) {
	b.mu.Lock()
	if b.pending[id] == pending {
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

// ResolveToolApproval resolves one blocked tool call. The waiting runtime turns
// a rejection into a model-visible handled tool error rather than aborting its
// entire Run.
func (h *Handler) ResolveToolApproval(ctx context.Context, request *v1.ResolveToolApprovalRequest) (*v1.ResolveToolApprovalResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("resolve tool approval request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, connect.NewError(connect.CodeCanceled, err)
	}
	if err := h.approvals.Resolve(request.GetApprovalId(), request.GetApproved()); err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return v1.ResolveToolApprovalResponse_builder{}.Build(), nil
}
