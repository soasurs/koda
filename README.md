# koda

`koda` is a local coding-agent service written in Go. It is being rebuilt around
a versioned Protocol Buffer contract, Connect RPC, and the
[`github.com/soasurs/adk`](https://github.com/soasurs/adk) agent framework.

[中文说明](README_zh-CN.md)

## Status

The repository is in an active rewrite and does not currently contain a
runnable server or CLI.

Available today:

- The `koda.v1.KodaService` Protocol Buffer contract.
- Generated Go and Connect bindings.
- Multimodal Run input and streamed event/approval/completion types.
- Provider, model, session, history, and undo API contracts.
- A persistent, concurrency-safe Provider Registry.
- Bundled model catalogs plus live Anthropic, OpenAI-compatible, Gemini, and
  DeepSeek model discovery with durable last-known-good snapshots.
- A SQLite-backed Session Store for Koda metadata and ADK conversation history.
- Provider and model Connect handlers with protocol-level tests.
- Session CRUD Connect handlers with protocol-level tests.
- Workspace-aware file, search, Git, and Shell tool implementations, including
  Hashline editing and structured file diffs.
- An `ask_questions` tool for Plan and Build agents, with typed frontend
  prompts, validated answers, and cancellation semantics.
- Session-scoped filesystem and Shell permission contracts plus an in-process,
  cancellation-safe approval broker.

The next implementation slice is Proto/ADK input and event conversion with a
fake-runtime test seam, then agent construction that wires the tools and
approval/question brokers into streamed Run handling. See
[AGENTS.md](AGENTS.md) for the current architecture decisions and development
order.

## Architecture

```text
Provider Registry ─┐
Session Store ─────┼──> Agent Runtime ──> Connect Server ──> Local clients
Tools + Prompts ───┘
```

Current source layout:

```text
proto/koda/v1/service.proto              API source of truth
gen/koda/v1/                             generated Go/Connect bindings
internal/provider/                       Provider Registry and model catalog
internal/server/                          Connect handlers
internal/store/                          SQLite lifecycle and session catalog
internal/tools/                          Workspace-aware coding tools
internal/permission/                     Session permission policy
buf.yaml / buf.gen.yaml                  lint and generation configuration
```

Planned packages include `internal/agent` and `cmd/koda`.

## API model

### Run

`Run` executes one user turn as a server stream. A stream may contain:

- `Event`: partial text/reasoning deltas or complete durable events.
- `ToolApproval`: a file, Git, or Shell operation waiting for approval.
- `QuestionPrompt`: an `ask_questions` call waiting for frontend-authored input.
- `RunCompleted`: the successful terminal frame for the turn.

A turn starts with one multimodal user input and includes all model tool calls,
tool results, and subsequent model invocations until the model returns a final
response that does not request another tool call.

Run input supports ordered text and image parts. Images may use an HTTPS URL or
inline bytes with a MIME type. Connect JSON represents protobuf bytes as base64.

### Sessions

Provider and model selection is session-scoped rather than global. A session
stores:

- provider and model IDs;
- provider-specific reasoning effort;
- filesystem access level;
- Shell access level;
- working directory;
- title and timestamps.

`RunRequest` carries only the session ID, input, and build/plan mode.

Session workdirs are normalized to existing, symlink-resolved absolute
directories when a session is created or updated. Provider, model, and
reasoning-effort selections are validated from the local model catalog; the
validation never performs network discovery.

### Session Store

The default database is `~/.koda/koda.db`. Koda metadata lives in
`koda_sessions`; ADK owns its prefixed history tables in the same SQLite
database. Creating a Koda session does not create an empty ADK ledger: it is
created immediately before the first Run, which keeps metadata creation atomic
without duplicating ADK's storage writes. Complete runs share a context-aware,
per-session in-process lock.

### Tools and approval

File access is session-scoped and has three automatic permission levels:

| Level | Workspace read | Workspace write | Outside-workspace access |
|---|---:|---:|---:|
| `WORKSPACE_READ` | allowed | approval | approval |
| `WORKSPACE_WRITE` | allowed | allowed | approval |
| `UNRESTRICTED` | allowed | allowed | allowed |

Shell access is independent: it requires approval by default, while its
unrestricted level intentionally grants arbitrary process and effective
filesystem access.

Plan agents receive `read_file`, `list_directory`, `search_text`,
`find_files`, a read-only allowlisted `git` tool, and `ask_questions`. Build
agents additionally receive `write_file`, `create_file`, Hashline-based
`edit_file`, and `run_shell`. File tools resolve symlinks before classifying
their location.
`read_file` and `search_text` return a file revision and `LINE:HASH` anchors;
`edit_file` verifies both before applying a batch atomically.

An approval may synchronously pause a tool call. The future Run runtime emits a
`ToolApproval` frame with a proposed structured diff where it is predictable;
the client resolves it through `ResolveToolApproval`. Pending approvals are
in-process, run-scoped, cancellation-safe, and discarded after resolution.
`ask_questions` blocks inside the tool itself; submitted frontend answers become
the normal persisted ToolResult, while the QuestionPrompt frame is transient.

## Provider Registry

The registry stores provider definitions, credentials, user model overrides,
and discovered model snapshots at:

```text
~/.koda/providers.json
```

The directory is written with mode `0700` and the file with mode `0600`.
Writes use a temporary file and atomic rename.

Built-in providers:

| ID | API | Environment variable |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

Environment credentials take precedence over stored credentials and are never
copied into the registry file. Custom providers can select any supported
adapter type and may provide an HTTP(S) Base URL.

Model listing is layered:

1. Bundled, reviewed catalogs provide an offline baseline for default built-in
   providers.
2. `RefreshModels` discovers the models exposed by the configured provider API
   and persists the last successful snapshot.
3. Provider-specific `model_overrides` add private models or override metadata
   by model ID.

`ListModels` is local-only and never performs an implicit network request. A
failed refresh preserves the last successful snapshot. Custom endpoints without
a successful snapshot expose only their explicit overrides. Catalog refreshes
do not change the provider connection revision or invalidate cached LLM clients.

Reasoning effort is model-specific. Model catalogs may advertise values such as
`minimal`, `low`, `medium`, `high`, `xhigh`, `max`, or `ultra`; the future
runtime will validate the selected value against the session's model.

## Development

Requirements:

- Go 1.26 or the version declared by `go.mod`.
- Buf CLI.

Validate the current repository:

```bash
buf lint
buf build
go build ./...
go vet ./...
go test ./...
go test -cover ./...
go test -race ./...
```

After changing `proto/koda/v1/service.proto`:

```bash
buf format -w
buf lint
buf build
buf generate
go build ./...
go vet ./...
go test ./...
go test -cover ./...
```

The generators are pinned as Go tool dependencies in `go.mod`. Generated files
under `gen/` are committed but must not be edited manually.

## Roadmap

The current implementation order is:

1. Proto-to-ADK input/event conversion and runtime test seams.
2. Cached build/plan agents and approval/question interaction wiring.
3. Streamed Run handling, process lifecycle, and end-to-end tests.

## License

Apache License 2.0. See [LICENSE](LICENSE).
