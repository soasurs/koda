# AGENTS.md — koda repository guide

This file is the repository-local memory for coding agents and contributors.
Keep it aligned with the live implementation whenever architecture, workflows,
or public contracts change.

## Project status

`koda` is being rebuilt as a local, single-process coding-agent service in Go
1.26. The public API is defined with Protocol Buffers and Connect RPC.

Implemented today:

- `koda.v1.KodaService` protocol and generated Go/Connect bindings.
- Multimodal `Run` input, streamed events, tool approvals, session CRUD, event
  history, undo, provider management, and model-listing contracts.
- A concurrency-safe Provider Registry with built-in providers, custom
  providers, credential persistence, and connection revision tracking.
- Bundled model catalogs, provider-native HTTP discovery, user overrides, and
  durable last-known-good model snapshots.
- A SQLite-backed Session Store with Koda session metadata, ADK history schema
  initialization, lazy ADK ledger creation, and per-session run locking.
- Provider and model Connect handlers with protocol-level tests and explicit
  error-code mapping.
- Session CRUD Connect handlers with local provider/model validation and
  per-session update serialization.
- Workspace-aware coding tools: Hashline file editing, directory listing,
  ripgrep-backed search and filename discovery, read-only Git, and Shell.
- A typed `ask_questions` tool shared by Plan and Build agents, plus a
  cancellation-safe in-process Question Broker and answer RPC.
- Session-scoped filesystem and Shell permission types, structured file-diff
  protocol types, and a cancellation-safe in-process Approval Broker.
- Proto/ADK multimodal input and event conversion, including structured tool
  outcomes and file diffs, with a fake TurnRunner test seam.
- Event-history and undo Connect handlers with per-session serialization.
- A cached ADK agent factory that maps every registered provider type, resolves
  model-default reasoning effort, builds Plan/Build tool sets, and reloads
  workspace `AGENTS.md` instructions when they change.
- Context-scoped Approval/Question adapters that preserve provider tool-call
  metadata and translate the existing brokers into transient Run frames.
- A live streamed `Run` handler that initializes ADK sessions, reuses cached
  runners, serializes event and interaction frames, touches sessions after
  successful turns, validates terminal assistant responses before ADK commits,
  and relies on ADK for cancellation rollback.

Not implemented yet:

- HTTP server and `cmd/koda` entry point.
- A runnable CLI or UI.

Do not document or present roadmap items as working features.

## Current repository layout

```text
koda/
├── proto/koda/v1/service.proto              # source API contract
├── gen/koda/v1/                             # generated protobuf code
│   ├── service.pb.go
│   └── kodav1connect/service.connect.go
├── internal/provider/                       # provider registry + model catalog
├── internal/server/                          # Connect handlers
├── internal/store/                          # SQLite lifecycle + session catalog
├── internal/tools/                          # workspace-aware coding tools
├── internal/permission/                     # session permission policy
├── internal/agent/                          # cached ADK agents, prompts, runtime context
├── buf.yaml
├── buf.gen.yaml
├── go.mod
├── README.md
└── README_zh-CN.md
```

Expected future packages:

```text
internal/agent/   # LLM factory, prompts, runtime, agent cache
internal/server/  # Proto conversion, event/runtime handlers, approval broker
cmd/koda/         # process lifecycle and HTTP server
```

Keep generated Proto types at the server boundary. Core packages should prefer
their own domain types or ADK types rather than depending on `gen/koda/v1`.

## Settled design decisions

### API and turn lifecycle

- Connect uses the generated `simple` handler signatures.
- `Run` is server-streaming and emits exactly four frame kinds: `Event`,
  `ToolApproval`, `QuestionPrompt`, and `RunCompleted`.
- One turn starts with one user input and includes every assistant tool-call
  message, tool result, and subsequent model invocation until the final
  assistant response has a finish reason other than `tool_calls`.
- `stop` is the normal terminal reason, but `length` and `content_filter` also
  terminate a successful agent sequence.
- `RunCompleted` is emitted only after the turn finishes successfully.
- Failed, canceled, or prematurely abandoned turns must not remain in durable
  ADK history.
- Tool approval may block synchronously in an ADK hook. This is acceptable for
  the intended single-machine service. Pending approvals must be scoped to the
  active run, observe context cancellation, and always be cleaned up.

### Input and event model

- Run input is an ordered list of text or image parts.
- Inline image data is raw protobuf `bytes`; Connect JSON handles base64.
- Image MIME type is required for inline bytes. URL images must use HTTPS.
- Event IDs are exposed as decimal strings to avoid JavaScript integer loss.
- Timestamps are Unix milliseconds.
- Partial events contain text/reasoning deltas, are transient, and are never
  persisted.
- Complete user events must preserve their multimodal parts so `ListEvents` and
  `UndoLastMessage` can round-trip the original input.

### Provider and session scope

- There is no global active provider/model configuration.
- The global Provider Registry stores connection definitions, credentials,
  model overrides, and the last successful discovery snapshots.
- Sessions store `provider_id`, `model_id`, `reasoning_effort`, `file_access`,
  `shell_access`, and `workdir`.
- `RunRequest` carries only `session_id`, multimodal input, and build/plan mode.
- Reasoning effort is a provider/model-specific string, not a shared enum.
- Agent instances are cached by provider ID/revision, model ID, resolved
  reasoning effort, mode, Session tool configuration, and the workspace
  instruction fingerprint. Do not rebuild an agent on every turn merely to
  inject Run metadata.

Tool permission invariants:

- `FileAccessWorkspaceRead` automatically permits only reads inside `workdir`.
  Workspace writes and every outside-workspace access require approval.
- `FileAccessWorkspaceWrite` automatically permits workspace reads and writes;
  every outside-workspace access still requires approval.
- `FileAccessUnrestricted` automatically permits filesystem reads and writes
  everywhere.
- Shell approval is independent. `ShellAccessUnrestricted` intentionally grants
  arbitrary process execution and effective unrestricted filesystem access.
- All file paths are resolved relative to `workdir`; existing paths and the
  closest existing ancestor of new paths are symlink-resolved before scope is
  classified. An internal symlink to an external target is external.
- Plan agents receive only `read_file`, `list_directory`, `search_text`,
  `find_files`, read-only allowlisted `git`, and `ask_questions`. Build agents
  additionally receive `write_file`, `create_file`, Hashline `edit_file`, and
  `run_shell`.
- `read_file` and `search_text` expose a content revision plus `LINE:HASH`
  anchors. `edit_file` requires the revision, verifies all anchors, rejects
  stale input, applies a non-overlapping batch atomically, and returns fresh
  anchors plus structured file changes.
- ToolApproval may cover external reads as well as writes. It carries the
  capability kind, widest scope, resolved targets, and a proposed `FileChange`
  diff where the operation is predictable. `ToolResult` carries the actual
  applied changes.
- `ask_questions` blocks inside its custom `tool.Tool` implementation so it can
  retain the provider tool-call ID. `QuestionPrompt` is transient; validated
  frontend answers are returned and persisted as the normal ToolResult. An
  explicit frontend cancellation is a model-visible handled failure.
- Cached tools resolve approval/question handlers from the Run context. The
  live Run handler must provide Session/Turn metadata and a concurrency-safe
  outbound frame publisher; it must not rebuild agents or tools per turn to
  inject this metadata.

Built-in Provider Registry entries:

| ID | Type | Environment key |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

Provider Registry invariants:

- Default path: `~/.koda/providers.json`.
- Registry directory mode: `0700`; file mode: `0600`.
- Built-in environment keys override stored keys and are never persisted.
- A nil key passed to `Registry.Save` preserves the stored key; an explicitly
  empty key clears it.
- Provider JSON returned to ordinary callers never serializes the API key.
- Built-in providers cannot be deleted or changed to a different adapter type.
- Provider connection revisions change only when adapter type, Base URL, or
  stored credentials change. Names, model overrides, and model snapshots do
  not invalidate cached LLM clients.
- Registry reads return deep copies; callers must not gain mutable access to
  registry-owned slices.

Model Catalog invariants:

- Bundled catalogs under `internal/provider/catalog/` are embedded into the
  binary, validated at startup, and used only for default built-in endpoints.
- `Catalog.List` is local-only. It never performs implicit network I/O.
- `Catalog.Refresh` uses provider-native discovery and persists only a
  successful snapshot. Errors leave the previous snapshot unchanged.
- A refresh snapshot is committed only if the provider connection revision is
  unchanged; results from an old credential or Base URL are rejected.
- A discovered model is enriched with bundled metadata by ID. User overrides
  are applied last and may also add private model IDs.
- A custom endpoint without a successful snapshot lists only explicit user
  overrides; it does not assume that official provider models exist there.
- Anthropic and Gemini discovery handle pagination. Gemini discovery filters
  out models that do not advertise `generateContent`.
- Discovery credentials are sent in headers, never URL query strings or error
  messages.

### OpenAI APIs

OpenAI Chat Completions and OpenAI Responses are distinct provider types and
distinct built-in registry entries. They share `OPENAI_API_KEY`, but keep model
catalogs and revisions separate. The live runtime uses ADK's Chat Completions
adapter for `openai` and the Responses adapter for `openai-responses`. ADK
session history remains the source of truth.

### Deferred scope

- Compaction is intentionally absent from the first API contract. Do not add a
  placeholder RPC without a complete summary-persistence design.
- Project instructions come from the session workdir and its `AGENTS.md`
  hierarchy. They are not global server configuration.

## Recommended implementation order

1. Add `cmd/koda`, graceful shutdown, and end-to-end Connect tests.

Review this order after each completed slice; do not build all layers at once.

## Proto workflow

`proto/koda/v1/service.proto` is the source of truth. Never edit files under
`gen/` directly.

After changing the contract, run:

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

Generation uses Go tool dependencies pinned in `go.mod`:

- `protoc-gen-go`
- `protoc-gen-connect-go`

Commit generated bindings with the Proto source.

## Go workflow

Follow the Go version declared in `go.mod`. Run ordinary Go commands without a
custom cache path unless a real environment error requires one.

After Go edits:

```bash
gofmt -w .
go build ./...
go vet ./...
go test ./...
go test -cover ./...
go test -race ./...
git diff --check
```

Focused tests are encouraged before the full pipeline:

```bash
go test ./internal/provider/...
go test -cover ./internal/provider/...
go test -race ./internal/provider/...
```

Documentation-only changes do not require the full Go pipeline unless they
change examples, generated output, or documented commands that need checking.

## Go conventions

- Use standard Go naming and preserve initialisms: `ID`, `URL`, `API`, `HTTP`,
  `JSON`, `LLM`.
- Exported symbols require doc comments that begin with the symbol name.
- Error messages start lowercase and have no trailing punctuation.
- Wrap errors with operation context and `%w`.
- Never pass a nil `context.Context`; propagate cancellation through storage,
  tools, hooks, streams, and provider calls.
- Prefer the standard library in tests. Use `t.Context()` for operation
  lifetimes tied to a test.
- Keep mutable state concurrency-safe and return clones of owned slices/maps.
- Add dependencies only when the requested implementation needs them.

## Security rules

- Never log, return, or include API keys in errors.
- Never serialize real credentials into Proto responses, tests, examples, or
  documentation.
- Use fake placeholders in examples.
- Provider Base URLs must not contain user-info credentials.
- Preserve secure permissions and atomic replacement for credential files.
- The future HTTP server must listen on `127.0.0.1` by default. Binding to other
  interfaces must require an explicit option because koda can execute shell and
  file mutations.

## Git and documentation

- Do not commit, push, stage, amend, rebase, or create pull requests unless the
  user explicitly requests it.
- Preserve unrelated user changes.
- Commit messages use Conventional Commits with a required scope:
  `<type>(<scope>): <subject>`.
- Keep `README.md`, `README_zh-CN.md`, and this file synchronized when public
  APIs, architecture, commands, or implementation status change.
