package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesStoredConfig(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	path := writeStoredConfig(t, home, StoredConfig{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		Providers: []ProviderConfig{
			{
				Name:    "openai",
				Format:  FormatOpenAI,
				APIKey:  "stored-openai-key",
				BaseURL: "https://stored.example",
			},
		},
	})

	t.Setenv("HOME", home)
	t.Chdir(workDir)

	cfg := Load()

	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, "openai")
	}
	if cfg.Model != "gpt-4o-mini" {
		t.Fatalf("Model = %q, want %q", cfg.Model, "gpt-4o-mini")
	}
	if cfg.APIKey != "stored-openai-key" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "stored-openai-key")
	}
	if cfg.BaseURL != "https://stored.example" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "https://stored.example")
	}
	if cfg.WorkDir != workDir {
		t.Fatalf("WorkDir = %q, want %q", cfg.WorkDir, workDir)
	}
	if cfg.Path != path {
		t.Fatalf("Path = %q, want %q", cfg.Path, path)
	}
}

func TestLoadPrefersEnvironmentOverrides(t *testing.T) {
	home := t.TempDir()
	workDir := t.TempDir()
	writeStoredConfig(t, home, StoredConfig{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		SafeMode: false,
		Providers: []ProviderConfig{
			{
				Name:    "openai",
				Format:  FormatOpenAI,
				APIKey:  "stored-openai-key",
				BaseURL: "https://stored.example",
			},
		},
	})

	t.Setenv("HOME", home)
	t.Setenv("KODA_PROVIDER", "gemini")
	t.Setenv("KODA_MODEL", "gemini-2.0-flash-exp")
	t.Setenv("KODA_BASE_URL", "https://env.example")
	t.Setenv("KODA_SAFE_MODE", "true")
	t.Setenv("GEMINI_API_KEY", "env-gemini-key")
	t.Chdir(workDir)

	cfg := Load()

	if cfg.Provider != "gemini" {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, "gemini")
	}
	if cfg.Model != "gemini-2.0-flash-exp" {
		t.Fatalf("Model = %q, want %q", cfg.Model, "gemini-2.0-flash-exp")
	}
	if cfg.APIKey != "env-gemini-key" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "env-gemini-key")
	}
	if cfg.BaseURL != "https://env.example" {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "https://env.example")
	}
	if !cfg.SafeMode {
		t.Fatal("SafeMode = false, want true")
	}
}

func TestLoadStoredConfigMigratesLegacyProviderMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	data := []byte(`{
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "providers": {
    "anthropic": { "api_key": "legacy-anthropic-key" },
    "custom-openai": { "api_key": "legacy-custom-key" }
  }
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stored, err := loadStoredConfig(path)
	if err != nil {
		t.Fatalf("loadStoredConfig() error = %v", err)
	}

	if stored.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want %q", stored.Provider, "anthropic")
	}
	if stored.Model != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want %q", stored.Model, "claude-sonnet-4-5")
	}
	if got := stored.FindProvider("anthropic"); got == nil || got.APIKey != "legacy-anthropic-key" || got.Format != FormatAnthropic {
		t.Fatalf("FindProvider(anthropic) = %+v, want API key and anthropic format", got)
	}
	if got := stored.FindProvider("custom-openai"); got == nil || got.APIKey != "legacy-custom-key" || got.Format != FormatOpenAI {
		t.Fatalf("FindProvider(custom-openai) = %+v, want API key and openai format", got)
	}
}

func TestSaveProviderSelectionEnsuresBuiltinsAndPreservesModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".koda", configFileName)
	cfg := &Config{
		Path: path,
		Stored: StoredConfig{
			Providers: []ProviderConfig{
				{
					Name:   "openai",
					Format: FormatOpenAI,
					APIKey: "old-openai-key",
					Models: []ModelConfig{{ID: "gpt-4o"}},
				},
			},
		},
	}

	if err := cfg.SaveProviderSelection("openai", "new-openai-key"); err != nil {
		t.Fatalf("SaveProviderSelection() error = %v", err)
	}

	stored := readStoredConfig(t, path)

	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want %q", cfg.Provider, "openai")
	}
	if cfg.Model != "" {
		t.Fatalf("Model = %q, want empty", cfg.Model)
	}
	if cfg.APIKey != "new-openai-key" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "new-openai-key")
	}
	if stored.Provider != "openai" {
		t.Fatalf("stored.Provider = %q, want %q", stored.Provider, "openai")
	}
	if got := stored.FindProvider("openai"); got == nil || got.APIKey != "new-openai-key" || len(got.Models) != 1 || got.Models[0].ID != "gpt-4o" {
		t.Fatalf("stored.FindProvider(openai) = %+v, want updated key and preserved models", got)
	}
	for _, name := range []string{"anthropic", "openai", "gemini"} {
		if stored.FindProvider(name) == nil {
			t.Fatalf("stored.FindProvider(%q) = nil, want builtin provider entry", name)
		}
	}
}

func TestSaveModelSelectionWritesStoredModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".koda", configFileName)
	cfg := &Config{
		Path:   path,
		Stored: StoredConfig{Provider: "anthropic"},
	}

	if err := cfg.SaveModelSelection("claude-sonnet-4-5"); err != nil {
		t.Fatalf("SaveModelSelection() error = %v", err)
	}

	stored := readStoredConfig(t, path)
	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want %q", cfg.Model, "claude-sonnet-4-5")
	}
	if stored.Model != "claude-sonnet-4-5" {
		t.Fatalf("stored.Model = %q, want %q", stored.Model, "claude-sonnet-4-5")
	}

	if err := cfg.SaveModelSelection(""); err == nil {
		t.Fatal("SaveModelSelection(\"\") error = nil, want error")
	}
}

func writeStoredConfig(t *testing.T, home string, stored StoredConfig) string {
	t.Helper()

	path := filepath.Join(home, ".koda", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func readStoredConfig(t *testing.T, path string) StoredConfig {
	t.Helper()

	stored, err := loadStoredConfig(path)
	if err != nil {
		t.Fatalf("loadStoredConfig() error = %v", err)
	}
	return stored
}
