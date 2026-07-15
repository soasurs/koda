// Package config loads Koda's process-level configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	currentVersion = 1

	// DefaultContextWindowTokens is the process-wide context budget used when
	// koda.yaml does not override it.
	DefaultContextWindowTokens int64 = 256_000

	// DefaultCompactionTriggerPercent is the percentage of the context window
	// at which Koda prepares to compact a session.
	DefaultCompactionTriggerPercent = 80
	// DefaultCompactionReserveTokens keeps enough room for compaction and the
	// next model response.
	DefaultCompactionReserveTokens int64 = 32_768
	// DefaultCompactionSummaryMaxTokens bounds one compaction summary response.
	DefaultCompactionSummaryMaxTokens int64 = 8_192
	// DefaultCompactionRetainTurns is the maximum number of recent complete
	// turns retained verbatim after compaction.
	DefaultCompactionRetainTurns = 2
	// DefaultCompactionRetainTokens bounds the estimated recent-history tail.
	DefaultCompactionRetainTokens int64 = 12_000
	// DefaultCompactionRebaseInterval controls how many generations may be
	// accumulated before rebuilding a working snapshot.
	DefaultCompactionRebaseInterval = 5
)

// Config contains process-level settings loaded when Koda starts. Session and
// provider configuration have separate durable stores.
type Config struct {
	Version int           `yaml:"version"`
	Server  ServerConfig  `yaml:"server,omitempty"`
	Log     LogConfig     `yaml:"log,omitempty"`
	Context ContextConfig `yaml:"context,omitempty"`
	// Compaction configures process-wide durable context compaction.
	Compaction CompactionConfig `yaml:"compaction,omitempty"`
	MCP        MCPConfig        `yaml:"mcp,omitempty"`
}

// ServerConfig configures the local API server.
type ServerConfig struct {
	Address string `yaml:"address,omitempty"`
}

// LogConfig configures process diagnostic logging.
type LogConfig struct {
	Level  string `yaml:"level,omitempty"`
	Path   string `yaml:"path,omitempty"`
	Output string `yaml:"output,omitempty"`
}

// ContextConfig configures the process-wide model context budget.
type ContextConfig struct {
	WindowTokens int64 `yaml:"window_tokens,omitempty"`
}

// EffectiveWindowTokens returns the configured context budget or the built-in
// default when no override is present.
func (c ContextConfig) EffectiveWindowTokens() int64 {
	if c.WindowTokens == 0 {
		return DefaultContextWindowTokens
	}
	return c.WindowTokens
}

// CompactionConfig configures process-wide durable context compaction. Nil
// booleans and zero numeric values use the built-in defaults.
type CompactionConfig struct {
	Enabled          *bool `yaml:"enabled,omitempty"`
	TriggerPercent   int   `yaml:"trigger_percent,omitempty"`
	ReserveTokens    int64 `yaml:"reserve_tokens,omitempty"`
	SummaryMaxTokens int64 `yaml:"summary_max_tokens,omitempty"`
	RetainTurns      int   `yaml:"retain_turns,omitempty"`
	RetainTokens     int64 `yaml:"retain_tokens,omitempty"`
	Verify           *bool `yaml:"verify,omitempty"`
	RebaseInterval   int   `yaml:"rebase_interval,omitempty"`
}

// EffectiveEnabled reports whether automatic compaction is enabled.
func (c CompactionConfig) EffectiveEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// EffectiveTriggerPercent returns the configured trigger percentage.
func (c CompactionConfig) EffectiveTriggerPercent() int {
	if c.TriggerPercent == 0 {
		return DefaultCompactionTriggerPercent
	}
	return c.TriggerPercent
}

// EffectiveReserveTokens returns the configured compaction reserve.
func (c CompactionConfig) EffectiveReserveTokens() int64 {
	if c.ReserveTokens == 0 {
		return DefaultCompactionReserveTokens
	}
	return c.ReserveTokens
}

// EffectiveSummaryMaxTokens returns the configured summary output limit.
func (c CompactionConfig) EffectiveSummaryMaxTokens() int64 {
	if c.SummaryMaxTokens == 0 {
		return DefaultCompactionSummaryMaxTokens
	}
	return c.SummaryMaxTokens
}

// EffectiveRetainTurns returns the configured recent-turn limit.
func (c CompactionConfig) EffectiveRetainTurns() int {
	if c.RetainTurns == 0 {
		return DefaultCompactionRetainTurns
	}
	return c.RetainTurns
}

// EffectiveRetainTokens returns the configured recent-history token budget.
func (c CompactionConfig) EffectiveRetainTokens() int64 {
	if c.RetainTokens == 0 {
		return DefaultCompactionRetainTokens
	}
	return c.RetainTokens
}

// EffectiveVerify reports whether model verification is enabled.
func (c CompactionConfig) EffectiveVerify() bool {
	return c.Verify == nil || *c.Verify
}

// EffectiveRebaseInterval returns the configured snapshot rebase interval.
func (c CompactionConfig) EffectiveRebaseInterval() int {
	if c.RebaseInterval == 0 {
		return DefaultCompactionRebaseInterval
	}
	return c.RebaseInterval
}

// MCPConfig configures process-wide MCP servers loaded when Koda starts.
type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers,omitempty"`
}

// MCPServerConfig configures one streamable HTTP or stdio MCP server.
type MCPServerConfig struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name,omitempty"`
	Transport string            `yaml:"transport"`
	URL       string            `yaml:"url,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty"`
	Command   string            `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	Workdir   string            `yaml:"workdir,omitempty"`
	ReadOnly  bool              `yaml:"read_only,omitempty"`
}

// DefaultPath returns the default Koda configuration path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: find home directory: %w", err)
	}
	return filepath.Join(home, ".koda", "koda.yaml"), nil
}

// LoadDefault loads Config from DefaultPath. A missing file returns an empty
// Config so callers can apply their built-in defaults.
func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return Load(path)
}

// Load reads a strict YAML configuration from path. A missing file returns an
// empty Config; an existing file must declare the current format version.
func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, errors.New("config: path must not be empty")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer file.Close() //nolint:errcheck // Read errors are reported by the decoder.

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var result Config
	if err := decoder.Decode(&result); err != nil {
		return Config{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, fmt.Errorf("config: decode %s: multiple YAML documents are not supported", path)
		}
		return Config{}, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if result.Version != currentVersion {
		return Config{}, fmt.Errorf("config: unsupported version %d", result.Version)
	}
	result.Server.Address = strings.TrimSpace(result.Server.Address)
	result.Log.Level = strings.ToLower(strings.TrimSpace(result.Log.Level))
	result.Log.Path = strings.TrimSpace(result.Log.Path)
	result.Log.Output = strings.ToLower(strings.TrimSpace(result.Log.Output))
	for index := range result.MCP.Servers {
		server := &result.MCP.Servers[index]
		server.ID = strings.TrimSpace(server.ID)
		server.Name = strings.TrimSpace(server.Name)
		server.Transport = strings.ToLower(strings.TrimSpace(server.Transport))
		server.URL = strings.TrimSpace(server.URL)
		server.Command = strings.TrimSpace(server.Command)
		server.Workdir = strings.TrimSpace(server.Workdir)
	}
	switch result.Log.Level {
	case "", "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("config: unsupported log level %q", result.Log.Level)
	}
	switch result.Log.Output {
	case "", "all", "console", "file":
	default:
		return Config{}, fmt.Errorf("config: unsupported log output %q", result.Log.Output)
	}
	if result.Context.WindowTokens < 0 {
		return Config{}, errors.New("config: context window tokens must not be negative")
	}
	if result.Compaction.TriggerPercent < 0 || result.Compaction.TriggerPercent > 100 {
		return Config{}, errors.New("config: compaction trigger percent must be between 0 and 100; 0 uses the default")
	}
	if result.Compaction.ReserveTokens < 0 {
		return Config{}, errors.New("config: compaction reserve tokens must not be negative")
	}
	if result.Compaction.SummaryMaxTokens < 0 {
		return Config{}, errors.New("config: compaction summary max tokens must not be negative")
	}
	if result.Compaction.RetainTurns < 0 {
		return Config{}, errors.New("config: compaction retain turns must not be negative")
	}
	if result.Compaction.RetainTokens < 0 {
		return Config{}, errors.New("config: compaction retain tokens must not be negative")
	}
	if result.Compaction.RebaseInterval < 0 {
		return Config{}, errors.New("config: compaction rebase interval must not be negative")
	}
	return result, nil
}
