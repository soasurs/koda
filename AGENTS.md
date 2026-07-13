# AGENTS.md — koda contributor guide

This file contains repository-specific instructions for coding agents and
contributors. Product usage belongs in `README.md`; temporary implementation
status and roadmaps do not belong here.

## Scope and architecture

Koda is a local, single-process coding-agent service written in Go. Its public
API uses Protocol Buffers and Connect RPC, and its agent runtime is built on
`github.com/soasurs/adk`.

Package responsibilities:

```text
proto/koda/v1/       public API source
gen/koda/v1/         generated Protobuf and Connect bindings
internal/server/     transport handlers and Proto/domain conversion
internal/agent/      ADK agent construction, caching, prompts, Run context
internal/provider/   provider registry, model catalogs, discovery
internal/store/      SQLite session metadata and ADK history integration
internal/tools/      workspace-aware coding tools
internal/permission/ session permission types and policy
cmd/koda/            command-line service entry point
```

Keep generated Proto types at the server boundary. Core packages should use
their own domain types or ADK types.

## Public contracts

- `proto/koda/v1/service.proto` is the API source of truth. Never edit `gen/`
  directly; regenerate it with Buf.
- `Run` is server-streaming and emits only `Event`, `ToolApproval`,
  `QuestionPrompt`, and `RunCompleted` frames.
- A successful turn includes every tool-call round through the final assistant
  response. `RunCompleted` is sent only after durable history and session
  metadata are consistent.
- Failed, canceled, abandoned, or unacknowledged turns must not remain in active
  ADK history.
- Partial events are transient. Complete user events must preserve multimodal
  parts so history and undo can round-trip the original input.
- Event IDs cross the API as decimal strings; timestamps use Unix milliseconds.
- Provider/model selection and permissions are session-scoped. Do not add a
  global active-provider setting.
- Reasoning effort is a provider/model-specific string.

When a public contract, command, or user-visible behavior changes, update both
READMEs in the same change.

## Runtime and storage invariants

- ADK session history is the conversation source of truth.
- Serialize Run, history mutation, session update, and deletion per session.
  Preserve context cancellation while waiting for a lock.
- Hold the Run serialization boundary until the completion frame is accepted;
  roll back committed events and metadata if acknowledgment fails.
- Creating Koda session metadata may lazily create its ADK ledger before the
  first Run. Deleting a session must remove its active metadata and history as
  one logical operation.
- Agent cache keys must cover provider ID and connection revision, model,
  resolved reasoning effort, mode, tool permissions, workdir, and workspace
  instruction fingerprint. Do not rebuild agents merely to inject per-Run
  metadata.
- Keep the embedded common and mode prompts as the stable system prefix. Add
  normalized runtime environment, effective permissions, and the Run-captured
  hierarchical `AGENTS.md` snapshot through `InstructionProvider`; dynamic
  instructions are ephemeral and must not enter conversation history.
- Approval and question handlers are resolved from Run context. Preserve the
  provider tool-call ID and publish transient interaction frames through the
  Run's concurrency-safe publisher.
- Do not add compaction or summary persistence without a complete durable
  design.

## Tools and permissions

- Resolve paths relative to the session workdir. Symlink-resolve existing paths
  and the closest existing ancestor of new paths before classifying scope.
- `WORKSPACE_READ` permits workspace reads only. Workspace writes and all
  outside-workspace access require approval.
- `WORKSPACE_WRITE` also permits workspace writes. Outside-workspace access
  still requires approval.
- `UNRESTRICTED` permits all filesystem reads and writes.
- Shell approval is independent. Unrestricted Shell implies effective access
  to the full filesystem.
- Plan agents receive read-only file/search tools, `ask_questions`, and
  `run_shell` restricted to one allowlisted read-only Git command. Reject other
  commands, mutating Git subcommands, unsafe Git options, environment overrides,
  and external Git helpers. Include repository/worktree metadata in scope
  classification.
- Build agents additionally receive file creation/writing, Hashline editing,
  and general `run_shell`.
- Disable user ripgrep configuration so tool behavior is determined by Koda's
  arguments.
- Cancel the full process group for timed-out Build shell commands.
- File approvals must describe the exact target and proposed content revision.
  Re-resolve and re-plan after a blocking approval; request approval again if
  either changes.
- `read_file` and `search_text` expose a content revision and `LINE:HASH`
  anchors. `edit_file` validates both immediately before an atomic write.

## Provider rules

- Built-in environment credentials override stored credentials and are never
  persisted or returned.
- Registry reads return deep copies of mutable data.
- Built-in providers cannot be deleted or changed to another adapter type.
- Connection revisions change only when adapter type, Base URL, or stored
  credentials change. A connection change clears its discovery snapshot.
- `Catalog.List` is local-only. `Catalog.Refresh` performs explicit
  provider-native discovery and commits a snapshot only if the connection
  revision is still current.
- Failed refreshes preserve the previous snapshot. Custom endpoints without a
  snapshot list only explicit overrides.
- Discovery credentials belong in headers, never URLs or errors.
- OpenAI Chat Completions and OpenAI Responses remain distinct provider types.

## Server security

- `koda serve` listens on loopback only and does not open a browser.
- Reject non-loopback HTTP Host values and non-local browser Origin values to
  prevent DNS-rebinding access to local capabilities.
- Never log, return, or commit API keys or other credentials.
- Provider Base URLs must not contain user-info credentials.
- Preserve `0700` registry directories, `0600` provider files, and atomic
  replacement.

## Go conventions

- Follow the Go version in `go.mod`; do not change it unless requested.
- Preserve standard initialisms: `ID`, `URL`, `API`, `HTTP`, `JSON`, and `LLM`.
- Exported symbols need doc comments beginning with the symbol name.
- Error messages start lowercase, have no trailing punctuation, and wrap causes
  with `%w` when useful.
- Never pass a nil `context.Context`. Propagate cancellation through storage,
  tools, hooks, streams, and provider calls.
- Keep mutable state concurrency-safe and return clones of owned slices/maps.
- Prefer the standard library in tests and use `t.Context()` for test-scoped
  operations.
- Group imports as standard library, third-party packages, then local module
  packages. Use the repository's import formatter when available.
- Do not add dependencies unless the requested behavior requires them.

## Verification

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

Run focused package tests first when useful. Use the ordinary Go cache unless a
real environment error requires an override.

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

Generation tools are pinned in `go.mod`; commit generated bindings with their
Proto source.

## Change discipline

- Make the smallest complete change and preserve unrelated user work.
- Do not edit generated or vendored files directly.
- Do not claim checks passed unless they were run.
- Do not commit, stage, push, amend, rebase, or create a pull request unless the
  user explicitly requests it.
- When requested, commit only related changes with a scoped Conventional Commit
  message: `<type>(<scope>): <subject>`.
