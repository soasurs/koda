# koda

> **⚠️ Early Development — Expect breaking changes, incomplete features, and rough edges.**

koda is a terminal-based AI coding assistant written in Go — similar in spirit to Claude Code and Aider. It provides a rich TUI for interactive conversations with LLMs, equipped with file editing, shell execution, code search, and git tools to help you code faster from the terminal.

## Features

- **Interactive TUI** — Bubbletea-based interface with scrollable message history, multi-line input, and streaming responses
- **One-shot mode** — Pipe a prompt as a CLI argument for quick, non-interactive use
- **Multi-provider LLM support** — Anthropic (Claude), OpenAI (GPT), and Google Gemini, switchable at runtime via `/connect` and `/model` commands
- **9 built-in tools** the agent can invoke:
  | Tool | Description |
  |------|-------------|
  | `read_file` | Read file contents (with optional line range) |
  | `write_file` | Overwrite a file |
  | `create_file` | Create a new file (refuses to overwrite) |
  | `list_directory` | List directory contents |
  | `grep_search` | Regex search across files |
  | `find_files` | Glob-based file finder |
  | `run_shell` | Execute shell commands (60 s timeout) |
  | `git_status` | Show working tree status |
  | `git_diff` | Show diffs (staged or unstaged) |
- **Build & Plan modes** — Toggle with `Tab` between full tool access (Build) and a read-only subset (Plan) for architecture analysis
- **Session persistence** — SQLite-backed conversation history at `~/.koda/sessions.db`; browse and resume with `/sessions`
- **Context compaction** — Automatic sliding-window compaction keeps context manageable; trigger manually with `/compact`
- **Collapsible tool output** — Long tool results are collapsed by default; press `x` to expand
- **Project-aware** — Reads workspace `AGENTS.md` to pick up project-specific instructions

## Requirements

- **Go 1.26.1+**
- **C compiler** (CGo required by `go-sqlite3`) — on macOS: `xcode-select --install`
- **Private dependency:** `github.com/soasurs/adk` must be cloned locally (see `go.mod` `replace` directive)

## Installation

```bash
# Clone the repository
git clone https://github.com/soasurs/koda.git
cd koda

# Ensure the adk dependency is available locally
# (adjust the replace path in go.mod if needed)

# Build
go build ./cmd/koda

# Or install to $GOPATH/bin
go install ./cmd/koda
```

## Usage

### Set up a provider

Export one of the API key environment variables:

```bash
export ANTHROPIC_API_KEY=sk-...    # Anthropic (default)
export OPENAI_API_KEY=sk-...       # OpenAI
export GEMINI_API_KEY=...          # Google Gemini
```

Or store keys persistently via the `/connect` slash command inside the TUI.

### Interactive mode

```bash
koda
```

### One-shot mode

```bash
koda "list the files in this directory"
```

### CLI flags

| Flag | Description |
|------|-------------|
| `--provider` | LLM provider: `anthropic`, `openai`, or `gemini` |
| `--model` | Model name override (e.g. `claude-sonnet-4-5`, `gpt-4o`) |
| `--no-session` | Disable SQLite session persistence (in-memory only) |
| `--safe` | Reserved: require confirmation for shell commands |

### Environment variables

| Variable | Purpose |
|----------|---------|
| `KODA_PROVIDER` | Default provider |
| `KODA_MODEL` | Default model name |
| `KODA_BASE_URL` | Custom base URL for OpenAI-compatible endpoints |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GEMINI_API_KEY` | Google Gemini API key |

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Tab` | Toggle Build ↔ Plan mode |
| `Esc` (×2) | Cancel running agent |
| `[` / `]` | Navigate between messages |
| `x` | Expand / collapse tool output |
| `PgUp` / `PgDn` | Scroll viewport |

### Slash commands

| Command | Action |
|---------|--------|
| `/connect` | Choose LLM provider and enter API key |
| `/model` | Select model from live provider list |
| `/sessions` | Browse and resume previous sessions |
| `/compact` | Compact current session context |

## Project Structure

```
koda/
├── cmd/koda/main.go              # CLI entry point
├── internal/
│   ├── agent/
│   │   ├── setup.go              # LLM + tools + session wiring
│   │   ├── runtime.go            # Build/Plan agents, session lifecycle
│   │   ├── models.go             # Live model listing per provider
│   │   ├── session_catalog.go    # Session metadata (SQLite + in-memory)
│   │   ├── prompt.md             # System prompt (embedded)
│   │   └── prompt_plan.md        # Plan-mode system prompt
│   ├── config/config.go          # Env / flag / file configuration
│   ├── tools/                    # Tool implementations
│   │   ├── file.go               # read, write, create, list
│   │   ├── shell.go              # run_shell
│   │   ├── search.go             # grep_search, find_files
│   │   └── git.go                # git_status, git_diff
│   └── tui/                      # Bubbletea TUI
│       ├── app.go                # Main model, streaming, rendering
│       ├── commands.go           # Slash command handling
│       ├── messages.go           # Message types and tea.Msg definitions
│       └── styles.go             # lipgloss styles
├── go.mod
└── AGENTS.md                     # Coding-agent instructions
```

## License

[Apache 2.0](LICENSE)