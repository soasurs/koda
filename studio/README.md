# Koda Studio

Koda Studio is the local web interface included in the Koda monorepo.

The first local-first UI includes provider configuration, workspace browsing,
session management, persisted conversation history, streamed Build and Plan
runs, project-grouped sessions, Markdown responses, completed turns that fold
earlier agent activity behind the final response, localized message timestamps,
last-turn editing and retry, tool approvals, structured question prompts, and
system-aware light and dark themes. Sessions can be renamed or archived from
their sidebar context menu and restored from Settings > Sessions.

## Development

Requirements:

- Node.js 24
- pnpm 10
- Buf CLI when regenerating the Connect client

Install dependencies and start Vite:

```bash
pnpm install
pnpm dev
```

The Vite development server proxies Connect requests to Koda at
`http://localhost:8080`, keeping browser requests same-origin. Override the
Connect base URL with `VITE_KODA_API_URL` when necessary.

## Protocol generation

The source schema is `../proto/koda/v1/service.proto`. Generate both the Go and
TypeScript bindings from the repository root with:

```bash
pnpm generate:proto
```

Generated files under `../gen` and `src/gen` should be committed with the Proto
source that produced them.

## Checks

```bash
pnpm typecheck
pnpm lint
pnpm test
pnpm build
pnpm format:check
```
