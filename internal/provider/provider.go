// Package provider manages model provider definitions and credentials.
package provider

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Type identifies the adapter used to communicate with a provider.
type Type string

const (
	// TypeAnthropic selects the Anthropic Messages adapter.
	TypeAnthropic Type = "anthropic"
	// TypeOpenAIChatCompletions selects the OpenAI-compatible Chat Completions adapter.
	TypeOpenAIChatCompletions Type = "openai_chat_completions"
	// TypeOpenAIResponses selects the OpenAI Responses adapter.
	TypeOpenAIResponses Type = "openai_responses"
	// TypeGemini selects the Gemini GenerateContent adapter.
	TypeGemini Type = "gemini"
	// TypeDeepSeek selects the DeepSeek adapter.
	TypeDeepSeek Type = "deepseek"
)

var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Model describes one model exposed by a provider.
type Model struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	ReasoningEfforts       []string `json:"reasoning_efforts,omitempty"`
	DefaultReasoningEffort string   `json:"default_reasoning_effort,omitempty"`
}

// Provider describes one built-in or user-defined model provider.
// Credentials and internal registry metadata are deliberately excluded from
// JSON serialization.
type Provider struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Type    Type    `json:"type"`
	BaseURL string  `json:"base_url,omitempty"`
	Models  []Model `json:"models,omitempty"`

	apiKey   string
	builtin  bool
	revision uint64
}

// APIKey returns the credential resolved for p. Callers must not log, expose,
// or serialize the returned value.
func (p Provider) APIKey() string {
	return p.apiKey
}

// Configured reports whether p has a non-empty API key.
func (p Provider) Configured() bool {
	return strings.TrimSpace(p.apiKey) != ""
}

// Builtin reports whether p is one of koda's built-in providers.
func (p Provider) Builtin() bool {
	return p.builtin
}

// Revision changes whenever p's stored definition changes. Runtime caches can
// include it in their keys to avoid reusing stale clients.
func (p Provider) Revision() uint64 {
	return p.revision
}

var builtinProviders = []Provider{
	{ID: "anthropic", Name: "Anthropic", Type: TypeAnthropic, builtin: true},
	{ID: "openai", Name: "OpenAI Chat Completions", Type: TypeOpenAIChatCompletions, builtin: true},
	{ID: "openai-responses", Name: "OpenAI Responses", Type: TypeOpenAIResponses, builtin: true},
	{ID: "gemini", Name: "Google", Type: TypeGemini, builtin: true},
	{ID: "deepseek", Name: "DeepSeek", Type: TypeDeepSeek, builtin: true},
}

func builtinProvider(id string) (Provider, bool) {
	for _, p := range builtinProviders {
		if p.ID == id {
			return cloneProvider(p), true
		}
	}
	return Provider{}, false
}

func normalizeProvider(p Provider) (Provider, error) {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.apiKey = ""
	p.builtin = false
	p.revision = 0

	if !providerIDPattern.MatchString(p.ID) {
		return Provider{}, fmt.Errorf("provider: invalid id %q", p.ID)
	}
	if p.Name == "" {
		return Provider{}, fmt.Errorf("provider: name must not be empty")
	}
	if !validType(p.Type) {
		return Provider{}, fmt.Errorf("provider: unsupported type %q", p.Type)
	}
	if p.BaseURL != "" {
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return Provider{}, fmt.Errorf("provider: invalid base URL %q", p.BaseURL)
		}
		if u.User != nil {
			return Provider{}, fmt.Errorf("provider: base URL must not contain credentials")
		}
	}

	models, err := normalizeModels(p.Models)
	if err != nil {
		return Provider{}, err
	}
	p.Models = models
	return p, nil
}

func normalizeModels(models []Model) ([]Model, error) {
	result := make([]Model, len(models))
	seenModels := make(map[string]struct{}, len(models))
	for i, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Name = strings.TrimSpace(model.Name)
		model.DefaultReasoningEffort = strings.TrimSpace(model.DefaultReasoningEffort)
		if model.ID == "" {
			return nil, fmt.Errorf("provider: model %d: id must not be empty", i)
		}
		if _, ok := seenModels[model.ID]; ok {
			return nil, fmt.Errorf("provider: duplicate model id %q", model.ID)
		}
		seenModels[model.ID] = struct{}{}
		if model.Name == "" {
			model.Name = model.ID
		}

		efforts := make([]string, 0, len(model.ReasoningEfforts))
		seenEfforts := make(map[string]struct{}, len(model.ReasoningEfforts))
		for _, effort := range model.ReasoningEfforts {
			effort = strings.TrimSpace(effort)
			if effort == "" {
				return nil, fmt.Errorf("provider: model %q: reasoning effort must not be empty", model.ID)
			}
			if _, ok := seenEfforts[effort]; ok {
				return nil, fmt.Errorf("provider: model %q: duplicate reasoning effort %q", model.ID, effort)
			}
			seenEfforts[effort] = struct{}{}
			efforts = append(efforts, effort)
		}
		model.ReasoningEfforts = efforts
		if model.DefaultReasoningEffort != "" && !slices.Contains(efforts, model.DefaultReasoningEffort) {
			return nil, fmt.Errorf(
				"provider: model %q: default reasoning effort %q is not supported",
				model.ID,
				model.DefaultReasoningEffort,
			)
		}
		result[i] = model
	}
	return result, nil
}

func validType(providerType Type) bool {
	switch providerType {
	case TypeAnthropic, TypeOpenAIChatCompletions, TypeOpenAIResponses, TypeGemini, TypeDeepSeek:
		return true
	default:
		return false
	}
}

func cloneProvider(p Provider) Provider {
	p.Models = cloneModels(p.Models)
	return p
}

func cloneModels(models []Model) []Model {
	if models == nil {
		return nil
	}
	result := make([]Model, len(models))
	for i, model := range models {
		model.ReasoningEfforts = slices.Clone(model.ReasoningEfforts)
		result[i] = model
	}
	return result
}
