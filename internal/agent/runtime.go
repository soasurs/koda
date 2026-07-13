// Package agent constructs and caches Koda's ADK-backed coding agents.
package agent

import (
	"context"

	"github.com/soasurs/koda/internal/permission"
	"github.com/soasurs/koda/internal/tools"
)

type runInteractionsContextKey struct{}
type runEnvironmentContextKey struct{}

// RunEnvironment describes the session-scoped workspace and effective access
// settings for one Run.
type RunEnvironment struct {
	Workdir     string
	FileAccess  permission.FileAccess
	ShellAccess permission.ShellAccess
}

// WithRunEnvironment returns a child context carrying environment settings for
// one active Run. The instruction provider validates the values before use.
func WithRunEnvironment(ctx context.Context, environment RunEnvironment) context.Context {
	return context.WithValue(ctx, runEnvironmentContextKey{}, environment)
}

// RunEnvironmentFromContext returns the environment attached to an active Run.
func RunEnvironmentFromContext(ctx context.Context) (RunEnvironment, bool) {
	environment, ok := ctx.Value(runEnvironmentContextKey{}).(RunEnvironment)
	return environment, ok
}

// RunInteractions supplies the frontend interactions for one active Run.
// Cached agents and tools resolve this value from context so session and turn
// metadata never needs to be captured when an agent is constructed.
type RunInteractions struct {
	// Authorizer waits for an approval decision when a tool operation requires
	// confirmation.
	Authorizer tools.Authorizer
	// Questioner publishes ask_questions prompts and waits for answers.
	Questioner tools.Questioner
}

// WithRunInteractions returns a child context carrying interactions for one
// active Run. ctx must not be nil.
func WithRunInteractions(ctx context.Context, interactions RunInteractions) context.Context {
	return context.WithValue(ctx, runInteractionsContextKey{}, interactions)
}

func runInteractionsFromContext(ctx context.Context) (RunInteractions, bool) {
	interactions, ok := ctx.Value(runInteractionsContextKey{}).(RunInteractions)
	return interactions, ok
}

type runtimeAuthorizer struct{}

func (runtimeAuthorizer) Authorize(ctx context.Context, approval tools.Approval) error {
	interactions, ok := runInteractionsFromContext(ctx)
	if !ok || interactions.Authorizer == nil {
		return tools.ErrApprovalRequired
	}
	return interactions.Authorizer.Authorize(ctx, approval)
}

type runtimeQuestioner struct{}

func (runtimeQuestioner) Ask(ctx context.Context, request tools.QuestionRequest) (tools.QuestionResolution, error) {
	interactions, ok := runInteractionsFromContext(ctx)
	if !ok || interactions.Questioner == nil {
		return tools.QuestionResolution{}, tools.ErrQuestionInteractionUnavailable
	}
	return interactions.Questioner.Ask(ctx, request)
}

var (
	_ tools.Authorizer = runtimeAuthorizer{}
	_ tools.Questioner = runtimeQuestioner{}
)
