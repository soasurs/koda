# Providers and integrations

[中文](providers-and-integrations_zh-CN.md) · [Architecture index](../architecture.md)

Provider connections, model catalogs, Agent Skills, and MCP servers have
different mutation and trust models. Koda keeps these boundaries explicit so a
Session can select capabilities without turning process-global state into an
implicit active configuration.

## Provider registry

The registry combines built-in definitions with user-defined providers and
persists mutable definitions in `providers.json`. Built-in providers may be
configured but cannot be deleted or changed to a different adapter type.

Credential resolution is deliberately asymmetric:

- a built-in environment credential overrides a stored credential;
- an environment credential is never written back to disk or returned by the
  public API;
- a stored credential is preserved when an update omits the credential field;
- credentials belong in request headers, never URLs or error strings.

Registry methods return deep copies of owned slices and maps. Callers cannot
mutate cached definitions or discovery snapshots without a registry operation.

## Connection revisions

Each provider has a connection revision. It changes only when adapter type,
base URL, or stored credential changes. Display metadata and model overrides do
not describe a different network connection and therefore do not advance it.

The revision participates in two consistency mechanisms:

- an agent cache entry cannot survive a connection change;
- a long-running model discovery result commits only when its captured revision
  is still current.

A connection change clears the previous discovery snapshot. This prevents a
response from an old endpoint or credential from becoming the catalog for a
new connection.

## Model catalog and discovery

`Catalog.List` composes bundled model metadata, explicit overrides, and the
last successful discovery snapshot without network access. Agent construction
therefore has deterministic local lookup behavior.

`Catalog.Refresh` is the explicit network operation. It uses the provider's
native discovery mechanism, normalizes the response, and atomically commits a
snapshot if the connection revision is still current. A failed refresh leaves
the previous snapshot intact. A custom endpoint without a snapshot exposes
only explicit overrides.

OpenAI Chat Completions and OpenAI Responses remain different provider types.
They have distinct request and response semantics even when they share an API
key and model naming.

## Session selection

Provider ID, model ID, and reasoning effort belong to a Session. There is no
process-global active provider. The agent factory validates the selected model
against the local catalog and resolves a missing reasoning effort to the
model's declared default.

This makes concurrent sessions independent and keeps Studio settings from
silently changing an already configured session.

## Agent Skills

Skills are loaded into a fixed process catalog at startup. Invalid individual
skills are skipped and logged; failure to load the skill directory falls back
to an empty catalog. The agent factory renders a catalog instruction and adds
ADK tools for loading a chosen skill and its declared UTF-8 resources.

The catalog instruction participates in the runner instruction fingerprint.
Because the catalog is fixed for the process lifetime, adding or changing a
skill requires restart rather than live cache invalidation.

## MCP lifecycle and exposure

The MCP manager opens every configured transport and discovers tools during
startup. It fails fast on connection errors, nil tools, invalid names, duplicate
server IDs, or exposed-name conflicts. The immutable display catalog is returned
as a deep copy.

Every model-visible name is namespaced as
`mcp__<server-id>__<tool-name>`. This makes ownership explicit and avoids
collisions with built-in coding tools. Results are bounded before they enter
model context.

An explicitly read-only server contributes tools to both Plan and Build mode
without per-call approval. Other servers contribute approval-wrapped tools only
to Build mode. Read-only is a trusted configuration assertion about the entire
server, not an inferred property.

HTTP transports use Streamable HTTP and require TLS outside loopback. Stdio
transports launch the configured executable directly, not through a shell.
Environment expansion happens before connection, and secret header or
environment values are excluded from the public catalog.

## Studio boundary

Studio consumes the same Connect contract available to another local client.
The server owns durable Sessions, Events, provider definitions, catalogs, and
interaction resolution. Studio owns presentation state such as:

- optimistic partial output;
- a temporary first-input title while title generation runs;
- expanded or folded tool activity;
- localized timestamps and current theme;
- transient compaction progress.

After refresh, Studio reconstructs durable state through Session and Event
RPCs. It must not persist partial frames as conversation truth. The
`RunCompleted.session` snapshot replaces optimistic Session state at the Run
commit boundary.

Directory browsing is a service-scoped pre-Session capability used by local
clients to choose a workdir. It returns canonical directory names and paths but
never file contents and never mutates the filesystem.
