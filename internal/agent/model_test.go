package agent

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/soasurs/koda/internal/provider"
)

func TestNewProviderModelConstructsEverySupportedAdapter(t *testing.T) {
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	key := "test-key"
	tests := []struct {
		name   string
		typeID provider.Type
		effort string
	}{
		{name: "anthropic", typeID: provider.TypeAnthropic, effort: "max"},
		{name: "openai chat", typeID: provider.TypeOpenAIChatCompletions, effort: "ultra"},
		{name: "openai responses", typeID: provider.TypeOpenAIResponses, effort: "max"},
		{name: "gemini", typeID: provider.TypeGemini, effort: "high"},
		{name: "deepseek", typeID: provider.TypeDeepSeek, effort: "max"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id := "test-" + string(test.typeID)
			if _, err := registry.Save(t.Context(), provider.Provider{
				ID:      id,
				Name:    test.name,
				Type:    test.typeID,
				BaseURL: "https://example.test",
			}, &key); err != nil {
				t.Fatalf("Registry.Save() error = %v", err)
			}
			value, err := registry.Get(t.Context(), id)
			if err != nil {
				t.Fatalf("Registry.Get() error = %v", err)
			}
			llm, err := newProviderModel(t.Context(), value, "model-1", test.effort)
			if err != nil {
				t.Fatalf("newProviderModel() error = %v", err)
			}
			if llm == nil || llm.Name() != "model-1" {
				t.Fatalf("model = %#v; want model named model-1", llm)
			}
		})
	}
}

func TestNewProviderModelRejectsMissingCredentialAndInvalidGeminiEffort(t *testing.T) {
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	value, err := registry.Get(t.Context(), "gemini")
	if err != nil {
		t.Fatalf("Registry.Get() error = %v", err)
	}
	if _, err := newProviderModel(t.Context(), value, "gemini-test", "high"); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("newProviderModel(unconfigured) error = %v, want ErrProviderNotConfigured", err)
	}

	key := "test-key"
	if _, err := registry.Save(t.Context(), provider.Provider{
		ID:      "configured-gemini",
		Name:    "Configured Gemini",
		Type:    provider.TypeGemini,
		BaseURL: "https://example.test",
	}, &key); err != nil {
		t.Fatalf("Registry.Save() error = %v", err)
	}
	value, err = registry.Get(t.Context(), "configured-gemini")
	if err != nil {
		t.Fatalf("Registry.Get(configured) error = %v", err)
	}
	if _, err := newProviderModel(t.Context(), value, "gemini-test", "ultra"); err == nil {
		t.Fatal("newProviderModel(invalid Gemini effort) error = nil")
	}
}

func TestAnthropicThinkingBudget(t *testing.T) {
	tests := map[string]int64{
		"":       0,
		"low":    1024,
		"medium": 2048,
		"high":   4096,
		"xhigh":  8192,
		"max":    16384,
		"ultra":  32768,
	}
	for effort, want := range tests {
		got, err := anthropicThinkingBudget(effort)
		if err != nil || got != want {
			t.Errorf("anthropicThinkingBudget(%q) = %d, %v; want %d, nil", effort, got, err, want)
		}
	}
	if _, err := anthropicThinkingBudget("unsupported"); err == nil {
		t.Fatal("anthropicThinkingBudget(unsupported) error = nil")
	}
}
