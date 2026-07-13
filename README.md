# koda

`koda` is a local coding-agent service built with Go, Protocol Buffers,
Connect RPC, and [`github.com/soasurs/adk`](https://github.com/soasurs/adk).
It provides the runtime and API for clients that need streamed agent turns,
workspace tools, approvals, questions, provider configuration, and durable
conversation history.

[中文说明](README_zh-CN.md)

Koda includes an embedded local web interface whose source lives under
[`studio/`](studio/).

## Run Koda Studio

Requirements:

- Go 1.26, or the version declared by `go.mod`;
- a configured API key for at least one provider.

Start the embedded UI and open it in the default browser:

```bash
go run ./cmd/koda studio
```

Studio and the Connect API share the same loopback-only HTTP origin. Koda first
tries `localhost:8080`, falls back to an available loopback port when needed,
and prints the actual URL. Use `--addr` to select a port explicitly:

```bash
go run ./cmd/koda studio --addr 127.0.0.1:8787
```

## Run the headless service

Start the Connect API server:

```bash
go run ./cmd/koda serve
```

Koda first tries `localhost:8080`. If that port is occupied, it selects another
loopback port and prints the actual address. To select a port explicitly:

```bash
go run ./cmd/koda serve --addr 127.0.0.1:8787
```

The server accepts loopback addresses only and never opens a browser. Koda also
reads process-level settings from `~/.koda/koda.yaml`; command-line options take
precedence over the file. The file is optional and can configure the server
address and diagnostic log level:

```yaml
version: 1
server:
  address: 127.0.0.1:8080
log:
  level: info
```

`--addr` overrides `server.address`. When neither is set, Koda tries the default
address and falls back to an available loopback port if it is occupied. Log
levels are `debug`, `info`, `warn`, and `error`; the default is `info`. Logs at
every level are diagnostic output written to stderr, while the listening URL
remains on stdout. Debug logging includes safe ADK runtime metadata such as
operation durations and tool names, but not prompts, tool arguments, command
output, file contents, or credentials.

## Providers and local data

The following providers are built in:

| ID | API | Environment variable |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

Environment credentials take precedence over stored credentials and are not
copied into Koda's configuration file. Custom endpoints and model overrides can
be managed through `koda.v1.KodaService`.

Koda keeps its state under `~/.koda`:

```text
~/.koda/koda.yaml       optional process-level configuration
~/.koda/providers.json   provider definitions and credentials
~/.koda/koda.db          sessions and ADK conversation history
~/.koda/skills/          Agent Skills loaded when Koda starts
```

Provider definitions remain separate because Koda updates them through its API,
while `koda.yaml` is startup-only user configuration. Provider/model selection,
reasoning effort, workspace, and permissions remain session-scoped in the
database. The provider file is private to the current user. Model listing is
local-only; network discovery occurs only when a client explicitly calls
`RefreshModels`. Changing a provider connection invalidates its previously
discovered snapshot.

Each direct child of `~/.koda/skills` may contain one Agent Skill whose
`SKILL.md` name matches the directory name. Koda loads the catalog once at
process startup, exposes matching skills through `load_skill`, and lets agents
read listed UTF-8 resources through `read_skill_resource`. Restart Koda after
adding, removing, or changing a skill. A missing skills directory is treated as
an empty catalog. Invalid skills are logged and skipped without blocking
startup; if the skills directory itself cannot be loaded, Koda logs the error
and continues with an empty catalog. Clients can inspect the fixed startup
snapshot through `ListSkills` and `GetSkill`; Studio exposes the same list and
complete definitions under Settings > Skills.

## Directory browsing

Local clients can call `ListDirectories` before creating a session to choose a
working directory. An empty path starts at the current user's home directory;
each response contains only the selected directory's canonical path, parent
path, and immediate child directory names and paths. The RPC does not list
files, read file contents, or modify the filesystem. It remains protected by
the server's loopback Host and Origin checks.

## Agent runs

`Run` executes one multimodal user turn as a server stream. Inputs may contain
ordered text parts, HTTPS image URLs, or inline image bytes with a MIME type.
The stream emits four frame kinds:

- `Event` for model deltas and complete conversation events;
- `ToolApproval` when an operation needs user consent;
- `QuestionPrompt` when the agent asks for structured user input;
- `RunCompleted` after the turn has been committed successfully, including the
  latest durable `Session` snapshot.

Sessions select their own provider, model, reasoning effort, workspace, and
permission policy. Runs for the same session are serialized. If a turn cannot
be acknowledged with `RunCompleted`, its committed history is rolled back.
When a session still has an empty title, its first Run concurrently asks the
selected model for a concise title from the initial user input. The title is
stored and returned in `RunCompleted.session`; title-generation failure does
not fail the agent turn. Clients may display a local excerpt of the first input
as a temporary title while the Run is active.

## Tools and permissions

Plan agents can read files, list directories, search text, find files, ask
questions, and use `run_shell` for one allowlisted read-only Git command.
Other commands and mutating Git operations are rejected in Plan mode.

Build agents additionally receive whole-file creation and writing, Hashline
editing, and unrestricted command syntax through `run_shell`. Build shell
execution still follows the session's Shell approval policy.

Filesystem access is configured per session:

| Level | Workspace read | Workspace write | Outside workspace |
|---|---:|---:|---:|
| `WORKSPACE_READ` | allowed | approval | approval |
| `WORKSPACE_WRITE` | allowed | allowed | approval |
| `UNRESTRICTED` | allowed | allowed | allowed |

Paths are resolved through symlinks before their scope is classified. Shell
permission is independent because an unrestricted process can effectively
access the whole filesystem.

`read_file` and `search_text` return a content revision and `LINE:HASH`
anchors. `edit_file` validates them immediately before applying an atomic edit.
Predictable writes include a structured diff in both approval and result
frames.

## Development

The API source of truth is [`proto/koda/v1/service.proto`](proto/koda/v1/service.proto).
Generated files under `gen/` and `studio/src/gen/` are committed and must not
be edited manually.

Studio assets under `internal/studio/dist` are generated and ignored by Git.
Build them from the monorepo source with Node.js 24 and pnpm 10 after changing
Studio, or from a fresh checkout:

```bash
./build/studio.sh
```

Then run the Go checks:

```bash
gofmt -w .
go build ./...
go vet ./...
go test ./...
go test -cover ./...
go test -race ./...
git diff --check
```

After changing the Protocol Buffer contract, run `buf format -w`, `buf lint`,
`buf build`, and `buf generate` before the Go checks above.

## Release

Pushing a `v*` tag runs the release workflow. It builds Studio from the tagged
monorepo source, tests and packages native macOS amd64 and arm64 binaries,
generates SHA-256 checksums, and publishes the completed draft as a GitHub
Release.

Contributor-specific repository rules are in [AGENTS.md](AGENTS.md).

## Agent instructions

Koda assembles each coding agent's system instruction in layers. A stable,
embedded common prompt and Build or Plan mode prompt come first. Each Run then
adds the normalized working directory, effective session permissions, and
hierarchical `AGENTS.md` files from the filesystem root to the workspace. The
workspace instruction snapshot is reused across tool-call iterations in that
Run and refreshed on the next Run.

Runtime and workspace instructions are request-scoped context. They are sent to
the model for each iteration but are not added to conversation events or stored
in session history.

## License

Apache License 2.0. See [LICENSE](LICENSE).
