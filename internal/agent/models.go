package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	anthropicapi "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaiapi "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"

	"github.com/soasurs/koda/internal/config"
)

func ListProviderModels(ctx context.Context, cfg config.Config) ([]string, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)

	// Resolve the API format for the active provider.
	format := config.BuiltinFormat(cfg.Provider)
	if pc := cfg.Stored.FindProvider(cfg.Provider); pc != nil && pc.Format != "" {
		format = pc.Format
	}

	switch format {
	case config.FormatOpenAI:
		if apiKey == "" {
			return nil, fmt.Errorf("agent models: OpenAI API key is required")
		}
		client := openaiapi.NewClient(openaiClientOptions(cfg, apiKey)...)
		page, err := client.Models.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("agent models: list OpenAI models: %w", err)
		}
		models := make([]string, 0, len(page.Data))
		for _, model := range page.Data {
			models = append(models, model.ID)
		}
		return uniqueSortedModels(models), nil

	case config.FormatGemini:
		if apiKey == "" {
			return nil, fmt.Errorf("agent models: Google API key is required")
		}
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  apiKey,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			return nil, fmt.Errorf("agent models: create Google client: %w", err)
		}
		models := []string{}
		for model, iterErr := range client.Models.All(ctx) {
			if iterErr != nil {
				return nil, fmt.Errorf("agent models: list Google models: %w", iterErr)
			}
			name := strings.TrimPrefix(model.Name, "models/")
			if name != "" {
				models = append(models, name)
			}
		}
		return uniqueSortedModels(models), nil

	default: // FormatAnthropic
		if apiKey == "" {
			return nil, fmt.Errorf("agent models: Anthropic API key is required")
		}
		client := anthropicapi.NewClient(anthropicClientOptions(cfg, apiKey)...)
		page, err := client.Models.List(ctx, anthropicapi.ModelListParams{})
		if err != nil {
			return nil, fmt.Errorf("agent models: list Anthropic models: %w", err)
		}
		models := make([]string, 0, len(page.Data))
		for _, model := range page.Data {
			models = append(models, model.ID)
		}
		return uniqueSortedModels(models), nil
	}
}

func openaiClientOptions(cfg config.Config, apiKey string) []openaioption.RequestOption {
	options := []openaioption.RequestOption{openaioption.WithAPIKey(apiKey)}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		options = append(options, openaioption.WithBaseURL(strings.TrimSpace(cfg.BaseURL)))
	}
	return options
}

func anthropicClientOptions(cfg config.Config, apiKey string) []anthropicoption.RequestOption {
	options := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(apiKey)}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		options = append(options, anthropicoption.WithBaseURL(strings.TrimSpace(cfg.BaseURL)))
	}
	return options
}

func uniqueSortedModels(models []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	if len(result) == 0 {
		return result
	}
	slices.Sort(result)
	return result
}
