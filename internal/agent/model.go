package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/model/anthropic"
	"github.com/soasurs/adk/model/deepseek"
	"github.com/soasurs/adk/model/gemini"
	"github.com/soasurs/adk/model/openai"

	"github.com/soasurs/koda/internal/provider"
)

var (
	// ErrProviderNotConfigured indicates that the selected provider has no
	// available credential.
	ErrProviderNotConfigured = errors.New("provider is not configured")
)

type providerModelFactory func(context.Context, provider.Provider, string, string) (model.LLM, error)

func newProviderModel(ctx context.Context, value provider.Provider, modelID, reasoningEffort string) (model.LLM, error) {
	if !value.Configured() {
		return nil, fmt.Errorf("agent: provider %q: %w", value.ID, ErrProviderNotConfigured)
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, errors.New("agent: model ID must not be empty")
	}

	switch value.Type {
	case provider.TypeAnthropic:
		return newAnthropicModel(value, modelID, reasoningEffort)
	case provider.TypeOpenAIChatCompletions:
		return newOpenAIChatModel(value, modelID, reasoningEffort), nil
	case provider.TypeOpenAIResponses:
		return newOpenAIResponsesModel(value, modelID, reasoningEffort), nil
	case provider.TypeGemini:
		return newGeminiModel(ctx, value, modelID, reasoningEffort)
	case provider.TypeDeepSeek:
		return newDeepSeekModel(value, modelID, reasoningEffort), nil
	default:
		return nil, fmt.Errorf("agent: provider %q has unsupported type %q", value.ID, value.Type)
	}
}

func newAnthropicModel(value provider.Provider, modelID, effort string) (model.LLM, error) {
	opts := make([]anthropic.Option, 0, 2)
	if value.BaseURL != "" {
		opts = append(opts, anthropic.WithBaseURL(value.BaseURL))
	}
	budget, err := anthropicThinkingBudget(effort)
	if err != nil {
		return nil, err
	}
	if budget > 0 {
		opts = append(opts, anthropic.WithThinkingBudget(budget))
	}
	return anthropic.NewWithOptions(value.APIKey(), modelID, opts...), nil
}

func anthropicThinkingBudget(effort string) (int64, error) {
	switch effort {
	case "":
		return 0, nil
	case "low":
		return 1024, nil
	case "medium":
		return 2048, nil
	case "high":
		return 4096, nil
	case "xhigh":
		return 8192, nil
	case "max":
		return 16384, nil
	case "ultra":
		return 32768, nil
	default:
		return 0, fmt.Errorf("agent: Anthropic does not support reasoning effort %q", effort)
	}
}

func newOpenAIChatModel(value provider.Provider, modelID, effort string) model.LLM {
	opts := make([]openai.Option, 0, 1)
	if effort != "" {
		opts = append(opts, openai.WithReasoningEffort(openai.ReasoningEffort(effort)))
	}
	return openai.NewWithOptions(value.APIKey(), value.BaseURL, modelID, opts...)
}

func newOpenAIResponsesModel(value provider.Provider, modelID, effort string) model.LLM {
	opts := make([]openai.ResponsesOption, 0, 1)
	if effort != "" {
		opts = append(opts, openai.WithResponsesReasoningEffort(openai.ReasoningEffort(effort)))
	}
	return openai.NewResponsesWithOptions(value.APIKey(), value.BaseURL, modelID, opts...)
}

func newGeminiModel(ctx context.Context, value provider.Provider, modelID, effort string) (model.LLM, error) {
	opts := make([]gemini.Option, 0, 2)
	if value.BaseURL != "" {
		opts = append(opts, gemini.WithBaseURL(value.BaseURL))
	}
	if effort != "" {
		level, err := geminiThinkingLevel(effort)
		if err != nil {
			return nil, err
		}
		opts = append(opts, gemini.WithThinkingLevel(level))
	}
	result, err := gemini.NewWithOptions(ctx, value.APIKey(), modelID, opts...)
	if err != nil {
		return nil, fmt.Errorf("agent: create Gemini model: %w", err)
	}
	return result, nil
}

func geminiThinkingLevel(effort string) (gemini.ThinkingLevel, error) {
	switch effort {
	case "minimal":
		return gemini.ThinkingLevelMinimal, nil
	case "low":
		return gemini.ThinkingLevelLow, nil
	case "medium":
		return gemini.ThinkingLevelMedium, nil
	case "high":
		return gemini.ThinkingLevelHigh, nil
	default:
		return "", fmt.Errorf("agent: Gemini does not support reasoning effort %q", effort)
	}
}

func newDeepSeekModel(value provider.Provider, modelID, effort string) model.LLM {
	opts := make([]deepseek.Option, 0, 1)
	if effort != "" {
		opts = append(opts, deepseek.WithReasoningEffort(deepseek.ReasoningEffort(effort)))
	}
	return deepseek.NewWithBaseURLOptions(value.APIKey(), deepSeekBaseURL(value.BaseURL), modelID, opts...)
}

func deepSeekBaseURL(baseURL string) string {
	if baseURL == "" {
		return deepseek.BaseURL
	}
	return baseURL
}
