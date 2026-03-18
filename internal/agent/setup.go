package agent

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/soasurs/adk/model"
	"github.com/soasurs/adk/model/anthropic"
	"github.com/soasurs/adk/model/gemini"
	"github.com/soasurs/adk/model/openai"
	"github.com/soasurs/adk/session"
	"github.com/soasurs/adk/session/database"
	"github.com/soasurs/adk/session/memory"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/tools"
)

//go:embed prompt.md
var systemPrompt string

//go:embed prompt_plan.md
var planPrompt string

// newLLM creates the appropriate LLM client based on the provider in cfg.
// For custom providers, the API format is looked up from the stored config.
func newLLM(ctx context.Context, cfg *config.Config) (model.LLM, string, error) {
	provider := cfg.Provider
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = providerAPIKey(provider)
	}

	format := providerFormat(cfg)

	switch format {
	case config.FormatOpenAI:
		m := defaultModel(provider, cfg.Model)
		return openai.New(apiKey, cfg.BaseURL, m), m, nil

	case config.FormatGemini:
		m := defaultModel(provider, cfg.Model)
		llm, err := gemini.New(ctx, apiKey, m)
		if err != nil {
			return nil, "", fmt.Errorf("gemini: %w", err)
		}
		return llm, m, nil

	default: // FormatAnthropic
		m := defaultModel(provider, cfg.Model)
		return anthropic.New(apiKey, m), m, nil
	}
}

// providerFormat resolves the API format for the active provider.
// It looks up the provider's entry in the stored config; if not found it
// falls back to the built-in format for the three canonical names.
func providerFormat(cfg *config.Config) config.ProviderFormat {
	if pc := cfg.Stored.FindProvider(cfg.Provider); pc != nil && pc.Format != "" {
		return pc.Format
	}
	return config.BuiltinFormat(cfg.Provider)
}

func defaultModel(provider, configured string) string {
	if configured != "" {
		return configured
	}
	switch provider {
	case "openai":
		return "gpt-4o"
	case "gemini":
		return "gemini-2.0-flash"
	case "anthropic":
		return "claude-sonnet-4-5"
	default:
		return ""
	}
}

func providerAPIKey(provider string) string {
	switch provider {
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	default:
		return os.Getenv("ANTHROPIC_API_KEY")
	}
}

// buildTools instantiates all coding tools.
func buildTools() ([]tool.Tool, error) {
	constructors := []func() (tool.Tool, error){
		tools.NewReadFileTool,
		tools.NewWriteFileTool,
		tools.NewCreateFileTool,
		tools.NewListDirectoryTool,
		tools.NewGrepSearchTool,
		tools.NewFindFilesTool,
		tools.NewRunShellTool,
		tools.NewGitStatusTool,
		tools.NewGitDiffTool,
	}

	result := make([]tool.Tool, 0, len(constructors))
	for _, fn := range constructors {
		t, err := fn()
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}

// newSessionService creates a SQLite-backed (or in-memory) session service.
func newSessionService(cfg *config.Config) (session.SessionService, error) {
	if cfg.NoSession {
		return memory.NewMemorySessionService(), nil
	}

	dir := filepath.Join(os.Getenv("HOME"), ".koda")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create ~/.koda dir: %w", err)
	}

	dbPath := filepath.Join(dir, "sessions.db")
	db, err := sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sessions.db: %w", err)
	}

	if err := migrateSchema(db); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return database.NewDatabaseSessionService(db)
}

// migrateSchema creates the sessions and messages tables if they don't exist.
func migrateSchema(db *sqlx.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id INTEGER PRIMARY KEY,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			message_id        INTEGER PRIMARY KEY,
			session_id        INTEGER NOT NULL,
			role              TEXT    NOT NULL DEFAULT '',
			name              TEXT    NOT NULL DEFAULT '',
			content           TEXT    NOT NULL DEFAULT '',
			reasoning_content TEXT    NOT NULL DEFAULT '',
			tool_calls        TEXT    NOT NULL DEFAULT '[]',
			tool_call_id      TEXT    NOT NULL DEFAULT '',
			prompt_tokens     INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens      INTEGER NOT NULL DEFAULT 0,
			created_at        INTEGER NOT NULL,
			updated_at        INTEGER NOT NULL,
			compacted_at      INTEGER NOT NULL DEFAULT 0,
			deleted_at        INTEGER NOT NULL
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil && err != sql.ErrNoRows {
			return err
		}
	}

	if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN session_id INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	return nil
}

// loadAgentsMD reads AGENTS.md from the given directory.
// It returns the file's content, or an empty string if the file does not exist.
func loadAgentsMD(workDir string) string {
	path := filepath.Join(workDir, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
