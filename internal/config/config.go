package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const configFileName = "config.json"

// ProviderFormat identifies which API wire format a provider uses.
type ProviderFormat string

const (
	FormatAnthropic ProviderFormat = "anthropic"
	FormatOpenAI    ProviderFormat = "openai"
	FormatGemini    ProviderFormat = "gemini"
)

// ModelConfig describes a single model entry inside a provider definition.
type ModelConfig struct {
	// ID is the model identifier sent to the API (e.g. "gpt-4o").
	ID string `json:"id"`
	// Name is a human-readable display name (e.g. "GPT-4o"). Optional.
	Name string `json:"name,omitempty"`
	// InputPrice is the cost per 1M input tokens in USD. Optional.
	InputPrice float64 `json:"input_price,omitempty"`
	// OutputPrice is the cost per 1M output tokens in USD. Optional.
	OutputPrice float64 `json:"output_price,omitempty"`
	// ContextLength is the maximum context window in tokens. Optional.
	ContextLength int `json:"context_length,omitempty"`
}

// ProviderConfig holds the full configuration for one provider entry.
type ProviderConfig struct {
	// Name is the unique identifier for this provider (e.g. "anthropic", "my-openai").
	Name string `json:"name"`
	// Format specifies the API wire format: "anthropic", "openai", or "gemini".
	Format ProviderFormat `json:"format"`
	// APIKey is the authentication key for this provider.
	APIKey string `json:"api_key,omitempty"`
	// BaseURL overrides the default API endpoint. Required for OpenAI-compatible
	// third-party providers.
	BaseURL string `json:"base_url,omitempty"`
	// Models is an optional list of models to expose for this provider.
	// When non-empty, live model listing is skipped and this list is used instead.
	Models []ModelConfig `json:"models,omitempty"`
}

// StoredConfig is the on-disk representation of ~/.koda/config.json.
type StoredConfig struct {
	Provider  string           `json:"provider,omitempty"`
	Model     string           `json:"model,omitempty"`
	SafeMode  bool             `json:"safe_mode,omitempty"`
	Providers []ProviderConfig `json:"providers,omitempty"`
}

// Config is the runtime configuration resolved from flags, env vars, and the stored config.
type Config struct {
	Provider  string
	Model     string
	APIKey    string
	BaseURL   string
	NoSession bool
	SafeMode  bool
	WorkDir   string
	Stored    StoredConfig
	Path      string
}

func Load() *Config {
	cwd, _ := os.Getwd()
	path := defaultConfigPath()
	stored, _ := loadStoredConfig(path)

	provider := getenv("KODA_PROVIDER", stored.Provider)
	if provider == "" {
		provider = "anthropic"
	}

	model := getenv("KODA_MODEL", stored.Model)
	safeMode := getenvBool("KODA_SAFE_MODE", stored.SafeMode)

	pc := stored.FindProvider(provider)

	// BaseURL: env var > provider config > (empty)
	baseURL := getenv("KODA_BASE_URL", "")
	if baseURL == "" && pc != nil {
		baseURL = pc.BaseURL
	}

	// APIKey: env var for built-in providers > provider config > (empty)
	apiKey := builtinEnvAPIKey(provider)
	if apiKey == "" && pc != nil {
		apiKey = strings.TrimSpace(pc.APIKey)
	}

	return &Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		SafeMode: safeMode,
		WorkDir:  cwd,
		Stored:   stored,
		Path:     path,
	}
}

// FindProvider returns the ProviderConfig for the given name, or nil if not found.
func (s *StoredConfig) FindProvider(name string) *ProviderConfig {
	for i := range s.Providers {
		if s.Providers[i].Name == name {
			return &s.Providers[i]
		}
	}
	return nil
}

// upsertProvider updates an existing provider entry in-place or appends a new one.
func (s *StoredConfig) upsertProvider(pc ProviderConfig) {
	for i := range s.Providers {
		if s.Providers[i].Name == pc.Name {
			// Preserve existing models list if the update doesn't supply one.
			if len(pc.Models) == 0 {
				pc.Models = s.Providers[i].Models
			}
			s.Providers[i] = pc
			return
		}
	}
	s.Providers = append(s.Providers, pc)
}

// ensureBuiltins guarantees that the three built-in providers always have entries.
// This means a fresh or legacy config will be populated on first save.
func (s *StoredConfig) ensureBuiltins() {
	builtins := []struct {
		name   string
		format ProviderFormat
	}{
		{"anthropic", FormatAnthropic},
		{"openai", FormatOpenAI},
		{"gemini", FormatGemini},
	}
	for _, b := range builtins {
		if s.FindProvider(b.name) == nil {
			s.Providers = append(s.Providers, ProviderConfig{
				Name:   b.name,
				Format: b.format,
			})
		}
	}
}

func (c *Config) SaveProviderSelection(provider, apiKey string) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("config: provider is required")
	}

	c.Stored.ensureBuiltins()

	pc := c.Stored.FindProvider(provider)
	if pc == nil {
		// Unknown custom provider — create a minimal entry. Caller is expected
		// to have set format via the chooser; we default to openai-compat here
		// as a safe fallback.
		c.Stored.upsertProvider(ProviderConfig{
			Name:   provider,
			Format: FormatOpenAI,
			APIKey: strings.TrimSpace(apiKey),
		})
	} else {
		updated := *pc
		if strings.TrimSpace(apiKey) != "" {
			updated.APIKey = strings.TrimSpace(apiKey)
		}
		c.Stored.upsertProvider(updated)
	}

	c.Stored.Provider = provider
	c.Stored.Model = ""
	c.Provider = provider
	if strings.TrimSpace(apiKey) != "" {
		c.APIKey = strings.TrimSpace(apiKey)
	}
	c.Model = ""
	return saveStoredConfig(c.Path, c.Stored)
}

func (c *Config) SaveModelSelection(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("config: model is required")
	}
	c.Stored.Model = model
	c.Model = model
	return saveStoredConfig(c.Path, c.Stored)
}

// StoredAPIKey returns the stored API key for provider (used by the TUI to
// pre-fill the key input when switching providers).
func (c *Config) StoredAPIKey(provider string) string {
	if pc := c.Stored.FindProvider(provider); pc != nil {
		return strings.TrimSpace(pc.APIKey)
	}
	return ""
}

func defaultConfigPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		return configFileName
	}
	return filepath.Join(home, ".koda", configFileName)
}

func loadStoredConfig(path string) (StoredConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return StoredConfig{}, nil
		}
		return StoredConfig{}, fmt.Errorf("config: read stored config: %w", err)
	}

	var raw struct {
		Provider  string          `json:"provider,omitempty"`
		Model     string          `json:"model,omitempty"`
		SafeMode  bool            `json:"safe_mode,omitempty"`
		Providers json.RawMessage `json:"providers,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return StoredConfig{}, fmt.Errorf("config: decode stored config: %w", err)
	}

	stored := StoredConfig{
		Provider: raw.Provider,
		Model:    raw.Model,
		SafeMode: raw.SafeMode,
	}

	providers := bytes.TrimSpace(raw.Providers)
	if len(providers) == 0 || bytes.Equal(providers, []byte("null")) {
		return stored, nil
	}

	if providers[0] == '[' {
		if err := json.Unmarshal(providers, &stored.Providers); err != nil {
			return StoredConfig{}, fmt.Errorf("config: decode providers array: %w", err)
		}
		return stored, nil
	}

	var legacy map[string]struct {
		APIKey string `json:"api_key,omitempty"`
	}
	if err := json.Unmarshal(providers, &legacy); err != nil {
		return StoredConfig{}, fmt.Errorf("config: decode legacy providers map: %w", err)
	}
	for name, ps := range legacy {
		stored.upsertProvider(ProviderConfig{
			Name:   name,
			Format: BuiltinFormat(name),
			APIKey: ps.APIKey,
		})
	}

	return stored, nil
}

func saveStoredConfig(path string, stored StoredConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: create config dir: %w", err)
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode stored config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("config: write stored config: %w", err)
	}
	return nil
}

// BuiltinFormat returns the default ProviderFormat for the three built-in
// provider names; everything else defaults to openai (OpenAI-compatible).
func BuiltinFormat(name string) ProviderFormat {
	switch name {
	case "anthropic":
		return FormatAnthropic
	case "gemini":
		return FormatGemini
	default:
		return FormatOpenAI
	}
}

// builtinEnvAPIKey returns the API key from the standard environment variable
// for a built-in provider name. Returns "" for unknown/custom providers.
func builtinEnvAPIKey(provider string) string {
	switch provider {
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	default:
		return ""
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
