package agent

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/soasurs/adk/model"
)

const estimatedImageTokens int64 = 1_024

// CompactionSelectorConfig controls how much recent history remains verbatim.
// Both limits apply to whole Turn event groups; the newest Turn is always
// retained even when it alone exceeds RetainTokens.
type CompactionSelectorConfig struct {
	RetainTurns  int
	RetainTokens int64
}

// CompactionSelection describes an active-history prefix that can be archived
// and the recent suffix that remains verbatim. BoundaryEventID is the first
// retained event.
type CompactionSelection struct {
	Events             []model.Event
	RetainedEvents     []model.Event
	StartEventID       int64
	BoundaryEventID    int64
	SourceTokens       int64
	RetainedTokens     int64
	CompactedTurnCount int
	RetainedTurnCount  int
}

type eventTurn struct {
	start  int
	tokens int64
}

// SelectCompaction selects the oldest whole Turns for compaction while
// preserving a bounded recent tail. It never splits a Turn, so durable facts
// keep their original boundary before projection.
func SelectCompaction(events []model.Event, config CompactionSelectorConfig) (CompactionSelection, error) {
	if config.RetainTurns <= 0 {
		return CompactionSelection{}, errors.New("agent: compaction retain turns must be positive")
	}
	if config.RetainTokens <= 0 {
		return CompactionSelection{}, errors.New("agent: compaction retain tokens must be positive")
	}
	turns, err := splitEventTurns(events)
	if err != nil {
		return CompactionSelection{}, err
	}
	if len(turns) < 2 {
		return CompactionSelection{}, errors.New("agent: compaction requires at least two complete turns")
	}

	retainedTurns := 0
	var retainedTokens int64
	boundaryTurn := len(turns)
	for index := len(turns) - 1; index >= 0 && retainedTurns < config.RetainTurns; index-- {
		candidate := turns[index]
		if retainedTurns > 0 && retainedTokens+candidate.tokens > config.RetainTokens {
			break
		}
		boundaryTurn = index
		retainedTurns++
		retainedTokens += candidate.tokens
	}
	if boundaryTurn == 0 {
		return CompactionSelection{}, errors.New("agent: compaction selection leaves no history to compact")
	}

	boundaryIndex := turns[boundaryTurn].start
	selected := cloneEvents(events[:boundaryIndex])
	retained := cloneEvents(events[boundaryIndex:])
	return CompactionSelection{
		Events:             selected,
		RetainedEvents:     retained,
		StartEventID:       selected[0].ID,
		BoundaryEventID:    retained[0].ID,
		SourceTokens:       EstimateEventsTokens(selected),
		RetainedTokens:     retainedTokens,
		CompactedTurnCount: boundaryTurn,
		RetainedTurnCount:  retainedTurns,
	}, nil
}

func splitEventTurns(events []model.Event) ([]eventTurn, error) {
	if len(events) == 0 {
		return nil, errors.New("agent: compaction history must not be empty")
	}
	seen := make(map[string]struct{})
	turns := make([]eventTurn, 0)
	for index, event := range events {
		if event.Partial {
			return nil, fmt.Errorf("agent: compaction history contains partial event %d", event.ID)
		}
		if event.ID <= 0 {
			return nil, fmt.Errorf("agent: compaction event at index %d has invalid ID %d", index, event.ID)
		}
		if event.TurnID == "" {
			return nil, fmt.Errorf("agent: compaction event %d has no turn ID", event.ID)
		}
		if len(turns) > 0 && events[turns[len(turns)-1].start].TurnID == event.TurnID {
			turns[len(turns)-1].tokens += EstimateContentTokens(event.Content)
			continue
		}
		if _, exists := seen[event.TurnID]; exists {
			return nil, fmt.Errorf("agent: compaction turn %q is not contiguous", event.TurnID)
		}
		seen[event.TurnID] = struct{}{}
		turns = append(turns, eventTurn{start: index, tokens: EstimateContentTokens(event.Content)})
	}
	return turns, nil
}

// EstimateEventsTokens returns a conservative provider-neutral estimate used
// only to choose compaction boundaries. Provider-reported usage remains the
// authoritative context-window measurement.
func EstimateEventsTokens(events []model.Event) int64 {
	var total int64
	for _, event := range events {
		total += EstimateContentTokens(event.Content)
	}
	return total
}

// EstimateContentTokens estimates the tokens needed to resend content.
func EstimateContentTokens(content model.Content) int64 {
	total := int64(8)
	if content.Role == model.RoleTool {
		response := content.ToolResponseValue()
		return total + 8 + estimateTextTokens(response.Name) + estimateTextTokens(response.Text())
	}
	total += estimateTextTokens(content.Content) + estimateTextTokens(content.ReasoningContent)
	for _, part := range content.Parts {
		switch part.Type {
		case model.ContentPartTypeText:
			total += estimateTextTokens(part.Text)
		case model.ContentPartTypeImageURL, model.ContentPartTypeImageBase64:
			total += estimatedImageTokens
		}
	}
	for _, call := range content.ToolCalls {
		total += 8 + estimateTextTokens(call.Name) + estimateTextTokens(string(call.Arguments))
	}
	return total
}

func estimateTextTokens(value string) int64 {
	var ascii, nonASCII int64
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		if r < utf8.RuneSelf {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII
}
