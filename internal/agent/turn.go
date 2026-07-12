package agent

import (
	"context"
	"errors"
	"iter"

	adkagent "github.com/soasurs/adk/agent"
	"github.com/soasurs/adk/model"
)

var errTurnIncomplete = errors.New("agent: turn ended without a terminal assistant event")

// turnCompletionAgent turns a natural agent exit without a terminal assistant
// response into an error before runner.Runner commits the turn. The Runner then
// rolls back every durable event created for the incomplete turn.
type turnCompletionAgent struct {
	delegate adkagent.Agent
}

func (a turnCompletionAgent) Name() string {
	return a.delegate.Name()
}

func (a turnCompletionAgent) Description() string {
	return a.delegate.Description()
}

func (a turnCompletionAgent) Run(ctx context.Context, events []model.Event) iter.Seq2[*model.Event, error] {
	return func(yield func(*model.Event, error) bool) {
		terminal := false
		for event, err := range a.delegate.Run(ctx, events) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				return
			}
			if event == nil {
				if !yield(nil, errors.New("agent: yielded nil event")) {
					return
				}
				return
			}
			if !event.Partial && event.Content.Role == model.RoleAssistant {
				terminal = terminalAssistantEvent(*event)
			}
			if !yield(event, nil) {
				return
			}
		}
		if !terminal {
			if !yield(nil, errTurnIncomplete) {
				return
			}
		}
	}
}

func terminalAssistantEvent(event model.Event) bool {
	switch event.FinishReason {
	case model.FinishReasonStop, model.FinishReasonLength, model.FinishReasonContentFilter:
		return true
	default:
		return false
	}
}

var _ adkagent.Agent = turnCompletionAgent{}
