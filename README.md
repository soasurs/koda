# koda

`koda` is a local coding-agent service built with Go, Protocol Buffers,
Connect RPC, and [`github.com/soasurs/adk`](https://github.com/soasurs/adk).
It provides a durable, permission-aware runtime for coding agents and includes
an embedded local web interface.

[中文说明](README_zh-CN.md)

![Koda Studio Screenshot](docs/images/screenshot.png)

## What Koda provides

- reconnectable, server-owned multimodal agent turns with complete tool-call
  rounds;
- Build and Plan modes with workspace-aware tools;
- per-session provider, model, reasoning, workspace, and permission settings;
- explicit approvals and structured questions during a Run;
- durable SQLite conversation history with failed/interrupted Turn status,
  undo, replay, and model-aware context compaction;
- built-in Anthropic, OpenAI, Gemini, and DeepSeek adapters;
- process-level Agent Skills and MCP servers;
- an embedded React Studio and a Protobuf/Connect API for other local clients.

Koda is a local, single-process service. It listens on loopback only and keeps
its state under `~/.koda`.

Each Run has a server-assigned ID and a sequenced in-memory event journal.
Closing or refreshing Studio only detaches its subscription: the Run continues
in the local Koda process, and reopening the session restores its frames and
pending approvals or questions. Stop explicitly cancels the Run. Run journals
are process-local, so stopping Koda still interrupts active Runs.

Bundled model metadata supplies model-specific context windows for usage
reporting and automatic compaction. Custom model overrides can declare the same
capacity; models without one use the process fallback configured by
`context.window_tokens`.

## Quick start

Requirements:

- Go 1.26, or the version declared by `go.mod`;
- an API key for at least one supported provider;
- Node.js 24 and pnpm 10 when building Studio from a fresh source checkout.

Set a provider credential, for example:

```bash
export ANTHROPIC_API_KEY=...
```

From a fresh checkout, build the embedded Studio assets once:

```bash
./build/studio.sh
```

Start Koda Studio:

```bash
go run ./cmd/koda studio
```

Koda prints the selected loopback URL and opens it in the default browser. To
run only the Connect API server:

```bash
go run ./cmd/koda serve
```

Both commands try `localhost:8080` and fall back to an available loopback port.
Pass `--addr 127.0.0.1:8787` to select one explicitly.

## Documentation

- [Configuration](docs/configuration.md) describes `koda.yaml`, providers,
  local state, Agent Skills, MCP servers, and compaction settings.
- [Architecture](docs/architecture.md) explains the system boundaries, Run
  protocol, agent construction, tools, storage, compaction, and security model.
- [Public API](proto/koda/v1/service.proto) is the source of truth for the
  Protobuf and Connect contract.
- [Studio](studio/README.md) documents the frontend workspace.
- [Contributor guide](AGENTS.md) contains repository rules and verification
  commands.

## Development

Generated bindings under `gen/` and `studio/src/gen/` are committed and must
not be edited manually. Build ignored Studio assets before the first Go build
and after changing Studio:

```bash
./build/studio.sh
```

Then run the repository checks described in [AGENTS.md](AGENTS.md). After
changing `proto/koda/v1/service.proto`, format, lint, build, and generate it
with Buf before running the Go checks.

Pushing a `v*` tag runs the release workflow, which builds Studio, tests and
packages native macOS amd64 and arm64 binaries, generates checksums, and creates
a draft GitHub Release with the annotated tag body prepended to the automatically
generated release notes.

## License

Apache License 2.0. See [LICENSE](LICENSE).
