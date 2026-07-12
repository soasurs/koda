package server

import (
	"context"
	"errors"
	"iter"

	"connectrpc.com/connect"
	"github.com/soasurs/adk/model"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/store"
)

// TurnRunner is the narrow runtime boundary used by the future Run handler.
// ADK's runner.Runner satisfies this interface, while tests may supply a fake
// event sequence without constructing an LLM or contacting a provider.
type TurnRunner interface {
	Run(context.Context, string, model.Content) iter.Seq2[*model.Event, error]
}

// turnRunnerFactory selects a session and mode-specific runner. The concrete
// factory will later own provider construction and agent caching; keeping it
// separate lets handler tests exercise streaming with a fake TurnRunner.
type turnRunnerFactory func(context.Context, store.Session, v1.AgentMode) (TurnRunner, error)

func agentModeFromProto(mode v1.AgentMode) (v1.AgentMode, error) {
	switch mode {
	case v1.AgentMode_AGENT_MODE_BUILD, v1.AgentMode_AGENT_MODE_PLAN:
		return mode, nil
	default:
		return v1.AgentMode_AGENT_MODE_UNSPECIFIED, errors.New("agent mode must be BUILD or PLAN")
	}
}

func runtimeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	default:
		return connect.NewError(connect.CodeInternal, errors.New("agent runtime failed"))
	}
}
