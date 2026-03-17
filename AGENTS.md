# AGENTS.md — koda coding-agent instructions
## Repository overview
**koda** is a terminal-based AI coding assistant written in Go 1.26.
```
koda/
├── cmd/koda/main.go            # CLI entry point, flag parsing
├── internal/
│   ├── agent/
│   │   ├── setup.go            # LLM + tools + session + schema migration
│   │   ├── runtime.go          # Runtime struct, build/plan modes, compaction
│   │   ├── models.go           # Live model listing per provider
│   │   ├── session_catalog.go  # Session metadata persistence
│   │   ├── prompt.md           # System prompt (go:embed)
│   │   └── prompt_plan.md      # Plan-mode system prompt (go:embed)
│   ├── config/config.go        # Env-var / flag / file-based configuration
│   ├── tools/                  # 9 tools across 4 files (file, search, shell, git)
│   └── tui/                    # Bubbletea TUI (app.go, messages.go, styles.go, commands.go)
├── go.mod / go.sum
└── README.md
```
---
## Build & run commands
```bash
go build ./cmd/koda              # Build the binary
go build ./...                   # Build all packages (confirms compilation)
go vet ./...                     # Static analysis
gofmt -w .                      # Format all files (always run before committing)
go test ./...                    # Run all tests
go test ./internal/tools/...     # Run tests in a single package
go test ./internal/tools/ -run TestFoo -v   # Run a single named test
go test -race ./...              # Run with race detector
go install ./cmd/koda            # Install binary to $GOPATH/bin
```
> **Note:** `go-sqlite3` is a CGo package. Ensure `CGO_ENABLED=1` and a C compiler
> (`gcc` / `clang`) are available. On macOS: `xcode-select --install`.
---
## Environment variables
| Variable | Purpose |
|---|---|
| `KODA_PROVIDER` | `anthropic` (default), `openai`, or `gemini` |
| `KODA_MODEL` | Model name override (e.g. `claude-sonnet-4-5`, `gpt-4o`) |
| `ANTHROPIC_API_KEY` | Required for Anthropic |
| `OPENAI_API_KEY` | Required for OpenAI |
| `GEMINI_API_KEY` | Required for Gemini |
| `KODA_BASE_URL` | Optional base URL override for OpenAI-compatible endpoints |
Priority order: CLI flag > env var > stored config (`~/.koda/config.json`) > default.
---
## Code style guidelines
### Formatting
- All code is formatted with `gofmt` — tabs only, no line-length limit enforced.
- Run `gofmt -w .` before every commit.
### Import grouping
Imports use three or four blocks separated by blank lines. ADK packages may
appear in the third-party block or in their own block after internal imports:
```go
import (
    // 1. Standard library
    "context"
    "fmt"
    // 2. Third-party (charm.land/*, google, etc.)
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    // 3. Internal packages
    "github.com/soasurs/koda/internal/config"
    // 4. ADK packages (sometimes merged with block 2)
    "github.com/soasurs/adk/tool"
)
```
> **Important:** Charm libraries use `charm.land/` module paths (v2), not
> `github.com/charmbracelet/`.
### Naming conventions
- Standard Go: `CamelCase` exported, `camelCase` unexported.
- Acronyms stay uppercase: `LLM`, `TUI`, `API`, `URL`, `ID`.
- Tool struct pattern: `fooTool struct{ def tool.Definition }`.
- Tool input structs: `<toolName>Input` (e.g. `readFileInput`, `listDirInput`).
- Constructors: `New<Type>Tool() (tool.Tool, error)`.
### Types and structs
- Tool input structs carry both `json` and `jsonschema` struct tags:
  ```go
  Path string `json:"path" jsonschema:"Absolute or relative path to the file"`
  ```
- Optional fields use `omitempty` in the json tag.
### Error handling
- Wrap errors with context: `fmt.Errorf("<scope>: <detail>: %w", err)`.
- In tools the scope is the tool name (`read_file:`, `grep_search:`).
- In other packages the scope is the package or component name (`config:`, `agent runtime:`).
  ```go
  return "", fmt.Errorf("read_file: parse arguments: %w", err)
  ```
- Never silently discard errors unless genuinely irrelevant; add a comment:
  ```go
  _ = cmd.Run() // grep exits 1 when no matches; treat as empty result
  ```
- Return errors up the call stack. Never pass `nil` as a `context.Context`.
### Concurrency
- TUI ↔ goroutine communication uses a buffered channel (`chan runnerMsg`, cap 32).
- Always signal completion (`done: true`) even on error paths, via `defer`.
- Keep goroutines short-lived.
### Tool implementations
Tools live in `internal/tools/` grouped by category (4 files, 9 tools):
- `file.go` — `read_file`, `write_file`, `create_file`, `list_directory`
- `search.go` — `grep_search`, `find_files`
- `shell.go` — `run_shell`
- `git.go` — `git_status`, `git_diff`
Each file exports `New<Name>Tool() (tool.Tool, error)` constructors.
The constructor builds the JSON schema with `jsonschema.ForType(reflect.TypeFor[inputType](), ...)`.
Tool `Run` methods must be safe to call concurrently. Truncate large outputs
(see `maxOutputBytes`, `maxLines`, `maxMatches` constants).
### TUI / bubbletea
- Root model `tui.Model` is a value type. Pointer receivers for mutation
  (`applyEvent`, `refreshViewport`), value receivers for rendering.
- New `tea.Msg` types go in `messages.go`. Styles go in `styles.go`.
- Do **not** call `.Copy()` on lipgloss styles (deprecated); assign directly.
- `spinner.Tick` is a method value (`m.spinner.Tick`), not a call.
### Configuration
- Config lives in `internal/config.Config`. Add new fields there, not as globals.
- Supports persistent storage via `~/.koda/config.json` (provider, model, API keys).
---
## Key dependencies
| Package | Role |
|---|---|
| `github.com/soasurs/adk` | Agent framework: `llmagent`, `runner`, `session`, `tool`, `model` |
| `charm.land/bubbletea/v2` | TUI event loop |
| `charm.land/bubbles/v2` | `textarea`, `viewport`, `spinner` components |
| `charm.land/lipgloss/v2` | Terminal styling |
| `github.com/anthropics/anthropic-sdk-go` | Anthropic API client |
| `github.com/openai/openai-go/v3` | OpenAI API client |
| `google.golang.org/genai` | Gemini API client |
| `github.com/google/jsonschema-go` | JSON schema generation for tool inputs |
| `github.com/jmoiron/sqlx` | SQLite session persistence |
| `github.com/mattn/go-sqlite3` | CGo SQLite3 driver (blank-imported in `setup.go`) |
| `github.com/bwmarrin/snowflake` | Unique session ID generation |
---
## Common pitfalls
- **`go-sqlite3` requires CGo.** Linker errors → check `CGO_ENABLED=1` and C compiler.
- **Session IDs are snowflake-generated.** Different sessions are independent;
  use `--no-session` for ephemeral runs during development.
- **`charm.land/` not `github.com/charmbracelet/`.** The Charm libraries migrated
  to `charm.land` module paths in v2. Use the correct import path.