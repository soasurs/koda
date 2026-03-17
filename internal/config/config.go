package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const configFileName = "config.json"

type ProviderSettings struct {
	APIKey string `json:"api_key,omitempty"`
}

type StoredConfig struct {
	Provider  string                      `json:"provider,omitempty"`
	Model     string                      `json:"model,omitempty"`
	BaseURL   string                      `json:"base_url,omitempty"`
	Providers map[string]ProviderSettings `json:"providers,omitempty"`
}

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
	baseURL := getenv("KODA_BASE_URL", stored.BaseURL)
	apiKey := apiKey(provider)
	if apiKey == "" {
		apiKey = storedAPIKey(stored, provider)
	}

	return &Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		WorkDir:  cwd,
		Stored:   stored,
		Path:     path,
	}
}

func (c *Config) SaveProviderSelection(provider, apiKey string) error {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return fmt.Errorf("config: provider is required")
	}
	if c.Stored.Providers == nil {
		c.Stored.Providers = map[string]ProviderSettings{}
	}
	settings := c.Stored.Providers[provider]
	if strings.TrimSpace(apiKey) != "" {
		settings.APIKey = strings.TrimSpace(apiKey)
	}
	c.Stored.Providers[provider] = settings
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

func (c *Config) StoredAPIKey(provider string) string {
	return storedAPIKey(c.Stored, provider)
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

	var stored StoredConfig
	if err := json.Unmarshal(data, &stored); err != nil {
		return StoredConfig{}, fmt.Errorf("config: decode stored config: %w", err)
	}
	if stored.Providers == nil {
		stored.Providers = map[string]ProviderSettings{}
	}
	return stored, nil
}

func saveStoredConfig(path string, stored StoredConfig) error {
	if stored.Providers == nil {
		stored.Providers = map[string]ProviderSettings{}
	}
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

func storedAPIKey(stored StoredConfig, provider string) string {
	if stored.Providers == nil {
		return ""
	}
	return strings.TrimSpace(stored.Providers[provider].APIKey)
}

func apiKey(provider string) string {
	switch provider {
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	default:
		return os.Getenv("ANTHROPIC_API_KEY")
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
