# Configuration

[中文](configuration_zh-CN.md)

Koda reads optional process-level configuration from `~/.koda/koda.yaml`.
Command-line options take precedence over the file. Provider selection,
workspace, model, reasoning effort, and permissions are session-scoped and are
therefore not configured in this file.

## Example

```yaml
version: 1
server:
  address: 127.0.0.1:8080
log:
  level: info
  output: console
  # path: ~/.koda/koda.log
context:
  window_tokens: 256000
compaction:
  enabled: true
  trigger_percent: 80
  reserve_tokens: 32768
  summary_max_tokens: 8192
  retain_turns: 2
  retain_tokens: 12000
  verify: true
  rebase_interval: 5
mcp:
  servers:
    - id: exa
      name: Exa
      transport: http
      url: https://mcp.exa.ai/mcp
      read_only: true
      headers:
        x-api-key: ${EXA_API_KEY}
    - id: local-search
      name: Local search
      transport: stdio
      command: npx
      args: [-y, example-mcp-server]
      env:
        SEARCH_TOKEN: ${SEARCH_TOKEN}
```

## Server and logging

`server.address` must be a loopback address. The `--addr` command-line option
overrides it. When neither is set, Koda tries `localhost:8080` and falls back to
an available loopback port when necessary.

The supported log levels are `debug`, `info`, `warn`, and `error`; the default
is `info`. `log.output` may be `console`, `file`, or `all`; an omitted value is
equivalent to `console`. Console diagnostics go to stderr, while the selected
listening URL goes to stdout. File output uses `log.path`, or
`~/.koda/koda.log` when no path is set. Debug logging includes safe runtime
metadata such as operation durations and tool names. Prompts, tool arguments,
command output, file contents, and credentials are not logged.

## Context accounting and compaction

`context.window_tokens` is the process-wide context budget reported for every
model and defaults to 256,000. Until a provider reports usage, a session's
usage remains unavailable. Studio displays the sum of the latest reported
prompt and completion usage.

Durable compaction is enabled by default. Before a new Run, Koda attempts it
when the preceding acknowledged turn reaches `trigger_percent`, or earlier
when necessary to preserve `reserve_tokens`. It keeps up to `retain_turns`
complete recent turns within `retain_tokens`, summarizes the older active
prefix, and injects the resulting working-state snapshot into later model
requests. The snapshot is not an ordinary conversation event.

Compaction produces versioned structured JSON. An invalid draft or verification
result shares one repair attempt. With `verify: true`, compaction therefore
makes at most three model calls. Every `rebase_interval` generations, Koda
rebuilds the state from a bounded checkpoint and the subsequent immutable
segment summaries to limit recursive summarization drift.

Below the reserve boundary, a failed attempt is recorded and the Run may
continue. Koda retries after measured usage increases. At the reserve boundary,
a failure returns `RESOURCE_EXHAUSTED`. Setting `enabled: false` disables new
compactions; an existing durable snapshot continues to be supplied to the
model.

See [Storage and context compaction](architecture/storage-and-compaction.md)
for the internal design.

## MCP servers

Koda connects every configured MCP server once during startup and treats the
discovered catalog as fixed for the process lifetime. Restart Koda after an MCP
configuration change.

HTTP entries use MCP Streamable HTTP. Remote endpoints must use HTTPS;
plaintext HTTP is accepted only for loopback endpoints. Stdio entries launch
`command` directly, without a shell, and may specify `args`, `env`, and
`workdir`.

Header, argument, and environment values support `$NAME` and `${NAME}`
expansion. Startup fails if a referenced variable is missing, a configured
server cannot connect, or the returned tool catalog is invalid or conflicts
with another catalog. Koda does not silently remove an expected capability.

Tools are exposed to models as `mcp__<server-id>__<tool-name>`. Results are
bounded before they enter model context. `ListMCPServers` and `GetMCPServer`,
and Studio's Settings > MCP page, expose the startup catalog without returning
HTTP headers or stdio environment values.

A server marked `read_only: true` is available in both Plan and Build mode and
runs without per-call approval. Other MCP tools are Build-only and require
approval for every call. Only use `read_only: true` when every exposed tool is
side-effect-free. A stdio server is a trusted local process running with the
Koda user's permissions; approval does not sandbox that process.

## Providers

Koda includes these providers:

| ID | API | Environment variable |
|---|---|---|
| `anthropic` | Anthropic Messages | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI Chat Completions | `OPENAI_API_KEY` |
| `openai-responses` | OpenAI Responses | `OPENAI_API_KEY` |
| `gemini` | Gemini GenerateContent | `GEMINI_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |

Environment credentials take precedence over stored credentials and are never
copied into Koda's provider file. Clients manage stored credentials, custom
endpoints, model overrides, and discovery through `koda.v1.KodaService`.
Model listing is local-only. Network discovery occurs only after an explicit
`RefreshModels`, and a connection change invalidates the previous discovery
snapshot.

## Local state

Koda stores local state under `~/.koda`:

```text
~/.koda/koda.yaml        optional process-level configuration
~/.koda/providers.json   provider definitions and stored credentials
~/.koda/koda.db          sessions and ADK conversation history
~/.koda/skills/          Agent Skills loaded at startup
```

The provider registry is separate from `koda.yaml` because clients update it
through the API, while `koda.yaml` is read only during process startup.
Provider files are restricted to the current user.

## Agent Skills

Each direct child of `~/.koda/skills` may contain one Agent Skill. Its
`SKILL.md` name must match the directory name. Koda loads the catalog once at
startup. Agents use `load_skill` to load a selected definition and
`read_skill_resource` to read its listed UTF-8 resources.

Restart Koda after adding, removing, or changing a skill. A missing directory
is treated as an empty catalog. Invalid skills are logged and skipped. If the
directory itself cannot be loaded, Koda logs the error and continues with an
empty catalog. `ListSkills`, `GetSkill`, and Studio's Settings > Skills page
expose the fixed startup snapshot.
