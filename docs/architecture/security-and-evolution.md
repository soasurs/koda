# Security and evolution

[中文](security-and-evolution_zh-CN.md) · [Architecture index](../architecture.md)

Koda intentionally exposes powerful local capabilities. Its security model
depends on a small network boundary, explicit session permissions, careful path
classification, and strict handling of credentials and diagnostics.

## Network boundary

`koda serve` and `koda studio` bind loopback only. The HTTP layer rejects
non-loopback Host values and non-local browser Origin values. Loopback binding
alone is not sufficient because DNS rebinding or a hostile web origin could
otherwise reach local coding capabilities through the user's browser.

The current architecture has no authentication or multi-user isolation. Making
Koda remotely accessible would require an explicit identity, authentication,
authorization, transport security, and tenancy design; changing the listener
check alone is not a supported remote mode.

## Filesystem and process trust

Filesystem tools classify symlink-resolved targets against the Session
workspace. Their approval model protects operations implemented by those tools;
it is not an operating-system sandbox.

General Shell with unrestricted access can read and modify the full filesystem,
start processes, access the network, and inspect the environment. Its effective
authority is wider than a narrow File access setting. A stdio MCP server is
also a trusted local process running as the Koda user. Approving later MCP tool
calls does not sandbox server startup or implementation behavior.

An MCP server may be marked read-only only when every tool it exposes is
side-effect-free. Koda trusts this declaration because generic MCP schemas do
not provide a sufficient effect system for inference.

## Credential handling

Provider credentials are accepted from environment variables or the private
provider registry. Environment credentials override stored values and are not
persisted. Provider Base URLs reject user-info credentials. Discovery and model
requests put credentials in headers, not URLs, so URLs remain safe for common
diagnostic paths.

Provider registry and database files are restricted to the current user.
Public provider and MCP APIs return metadata but not resolved credentials,
headers, or stdio environment values.

## Logging and errors

Logs contain operational metadata such as request, Session, turn, tool, model,
duration, token count, capability kind, and scope. They must not contain:

- prompts or model content;
- tool arguments or output;
- command output;
- file contents or proposed content;
- API keys, authorization headers, or other credentials.

The server maps expected cancellation, deadline, configuration, and capacity
conditions to Connect codes at the transport boundary. Detailed internal causes
may be wrapped for diagnostics, but credential-bearing request material must
not become an error string. Tool approval rejection is a model-visible handled
result rather than an internal server failure.

## Test seams

Narrow boundaries keep high-risk behavior testable without a live model:

- `TurnRunner` supplies deterministic event sequences to Run tests;
- provider model factories can be replaced in agent tests;
- MCP connections and transports can be faked;
- clock, ID, title, and compactor functions have focused injection points;
- the server exercises Proto conversion and streaming against real Store
  behavior in integration tests.

Changes to the following invariants need focused regression coverage:

- per-session serialization and cancellation while waiting;
- durable failed/interrupted Turn recovery after runtime or transport failure;
- concurrent frame publication from same-round tools;
- stale provider revision and compaction generation rejection;
- symlink and future-path scope classification;
- Host and Origin rejection;
- secret exclusion from APIs, logs, URLs, and errors.

The full Go pipeline includes build, vet, ordinary tests, coverage, and race
tests because concurrency is part of the storage and streaming contract.

## Extension rules

- When adding an RPC, change the Proto source first, regenerate bindings,
  convert at the server boundary, and keep generated types out of core
  packages.
- When adding a Session setting, update the domain type, migration, Proto
  conversion, validation, and every agent cache key or runtime decision it
  affects. Decide how an in-flight Run is serialized against its update.
- When adding a provider, define a distinct adapter type when wire semantics
  differ, add bundled model metadata, implement explicit discovery if
  supported, and preserve revision and credential rules.
- When adding a tool, select its modes, identify capability kind and scope,
  resolve all targets, bound model-visible output, describe approval precisely,
  and revalidate state immediately before mutation.
- When adding a Run frame, define whether it is transient or durable, serialize
  publication with every other frame, specify cancellation behavior, and avoid
  duplicating an Event as another source of conversation truth.
- When adding persistent history transformation, design display history, model
  projection, undo, durable status, concurrency, migration, and failure recovery
  together. A summary field alone is not a complete durable design.

## Possible evolution

The following are design questions, not implemented features:

- provider-specific context window budgets instead of one process-wide value;
- durable recovery of approvals or questions across a client reconnect;
- explicit capability metadata for individual MCP tools;
- version negotiation between independently released clients and servers;
- export and migration formats for Sessions and compaction generations;
- a remote or multi-user deployment model with authentication and isolation.

Any such change must preserve or deliberately replace the current ownership,
acknowledgment, and security invariants rather than bypass them locally.
