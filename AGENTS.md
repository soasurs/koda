# AGENTS.md — koda coding-agent instructions

## Repository overview

**koda** is a terminal-based AI coding assistant (Claude Code / Aider style) written in Go 1.26.1.

```
koda/
├── cmd/koda/main.go          # CLI entry point, flag parsing
├── internal/
│   ├── agent/setup.go        # LLM + tools + session + runner wiring
│   ├── config/config.go      # Env-var / flag-based configuration
│   ├── tools/                # 9 tool implementations (file, shell, search, git)
│   └── tui/                  # Bubbletea TUI (app.go, messages.go, styles.go)
├── go.mod / go.sum
└── plan.md                   # Design document
```

The private framework `github.com/soasurs/adk` is replaced via a `go.mod` `replace`
directive pointing to `/Volumes/Code/go/soasurs/adk`. It must be present locally.

---

## Build & run commands

```bash
# Build the binary
go build ./cmd/koda

# Build all packages (confirms compilation)
go build ./...

# Run static analysis
go vet ./...

# Format all files (always run before committing)
gofmt -w .

# Run all tests
go test ./...

# Run tests in a single package
go test ./internal/tools/...

# Run a single named test
go test ./internal/tools/ -run TestReadFileTool -v

# Run with race detector
go test -race ./...

# Install the binary to $GOPATH/bin
go install ./cmd/koda

# Launch TUI (requires provider env var, e.g.)
ANTHROPIC_API_KEY=... go run ./cmd/koda

# One-shot mode
ANTHROPIC_API_KEY=... go run ./cmd/koda "list the files in this directory"
```

> **Note:** `go-sqlite3` is a CGo package. Ensure a C compiler (`gcc` / `clang`) is
> available. On macOS `xcode-select --install` is sufficient.

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

CLI flags (`--provider`, `--model`, `--no-session`, `--safe`) override env vars.

---

## Code style guidelines

### Formatting
- All code is formatted with `gofmt` (no tabs-vs-spaces debate — tabs only).
- Maximum line length is not enforced, but keep lines readable (~100 chars).
- Run `gofmt -w .` before every commit; CI should reject unformatted code.

### Import grouping
Imports are grouped in three blocks, separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "os"

    // 2. Third-party / external modules
    "github.com/charmbracelet/lipgloss"
    "github.com/jmoiron/sqlx"

    // 3. Internal packages (github.com/soasurs/koda/internal/...)
    "github.com/soasurs/koda/internal/config"
)
```

ADK packages (`github.com/soasurs/adk/...`) belong in the third-party block,
**not** in the internal block.

### Naming conventions
- Follow standard Go conventions: `CamelCase` for exported, `camelCase` for unexported.
- Acronyms stay uppercase: `LLM`, `TUI`, `API`, `URL`, `ID`.
- Interface types end in `-er` where idiomatic (`SessionService` is fine as-is).
- Unexported struct types implementing a single interface use the pattern
  `fooTool struct{ def tool.Definition }`.
- Input structs for tools are named `<toolName>Input` (e.g. `readFileInput`).
- Constructor functions are `New<Type>` (e.g. `NewReadFileTool`).

### Types and structs
- Prefer small, focused structs. Embed only when it adds clarity.
- Tool input structs carry both `json` and `jsonschema` struct tags on every
  field — the jsonschema tag text becomes the LLM-visible description:
  ```go
  Path string `json:"path" jsonschema:"Absolute or relative path to the file"`
  ```
- Optional fields use `omitempty` in their json tag.

### Error handling
- Always wrap errors with context using `fmt.Errorf("scope: %w", err)`.
  The scope prefix matches the tool/function name:
  ```go
  return "", fmt.Errorf("read_file: parse arguments: %w", err)
  ```
- Never silently discard errors with `_ = ...` unless the error is genuinely
  irrelevant (e.g. `grep` exiting 1 when no matches found). Add a comment
  explaining why in that case.
- Return errors up the call stack rather than logging and continuing.
- Use `context.Background()` when no parent context is available; never pass
  `nil` as a `context.Context`.

### Concurrency
- The TUI bridges the blocking `runner.Run` iterator to bubbletea via a buffered
  channel (`chan runnerMsg`). This is the canonical pattern for goroutine→TUI
  communication; follow it for any new background work.
- Keep goroutines short-lived and always signal completion (`done: true`) even
  on error paths.

### Tool implementations
- Each tool lives in `internal/tools/` and exports a single `New<Name>Tool() (tool.Tool, error)` constructor.
- The constructor builds the JSON schema with `jsonschema.ForType(reflect.TypeFor[inputType](), ...)`.
- Tool `Run` methods must be safe to call concurrently — avoid shared mutable state.
- Truncate large outputs to avoid saturating the LLM context (see `maxOutputBytes`
  and `maxLines` constants in the existing tools for reference).

### TUI / bubbletea
- The root model is `tui.Model` (value type, not pointer). Pointer receivers are
  used for methods that mutate state (`applyEvent`, `refreshViewport`, etc.).
- All rendering goes through `renderMessages` → `m.viewport.SetContent(...)`.
  Never write directly to the terminal outside of bubbletea.
- New `tea.Msg` types go in `internal/tui/messages.go`.
- Style variables are package-level `var` in `internal/tui/styles.go`.
  Do **not** call `.Copy()` on `lipgloss.Style` values (deprecated); styles are
  value types and can be assigned directly.

### Configuration
- Config lives in `internal/config.Config`. Add new fields there, not as globals.
- Priority order: CLI flag > env var > default. This is enforced in `main.go`
  by passing the loaded config as the default for each `flag.*Var` call.

---

## Key dependencies and their roles

| Package | Role |
|---|---|
| `github.com/soasurs/adk` | Agent framework: `llmagent`, `runner`, `session`, `tool`, `model` |
| `github.com/charmbracelet/bubbletea` | TUI event loop |
| `github.com/charmbracelet/bubbles` | `textarea`, `viewport`, `spinner` components |
| `github.com/charmbracelet/lipgloss` | Terminal styling (value-type styles) |
| `github.com/google/jsonschema-go` | JSON schema generation for tool input types |
| `github.com/jmoiron/sqlx` | SQLite session persistence |
| `github.com/mattn/go-sqlite3` | CGo SQLite3 driver (blank-imported in `agent/setup.go`) |

---

## Common pitfalls

- **`go-sqlite3` requires CGo.** If you see linker errors, check that `CGO_ENABLED=1`
  and a C compiler are available.
- **`adk` is local.** The `replace` directive in `go.mod` means `go get` for `adk`
  is a no-op; edit `/Volumes/Code/go/soasurs/adk` directly and re-run `go build`.
- **Session IDs are CRC32(abs(workdir)).** Running from different directories creates
  different sessions. Use `--no-session` for ephemeral runs during development.
- **`spinner.Tick` is a `tea.Cmd` method value** (`m.spinner.Tick`), not a function
  call (`m.spinner.Tick()`). Calling it returns `tea.Msg`, which cannot be appended
  to `[]tea.Cmd`.
