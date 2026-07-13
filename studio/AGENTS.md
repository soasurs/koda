# Repository Guidelines

## Project Structure & Module Organization

Studio is a React 19 and Vite application written in TypeScript. Application bootstrapping, routing, query setup, and theme state live in `src/app/`. Reusable UI is under `src/components/`, route-level screens are in `src/pages/`, and API/query helpers are in `src/lib/`. Global styles are defined in `src/styles.css`.

Tests are colocated with the code they cover and use the `*.test.ts` or `*.test.tsx` suffix. `src/gen/` contains generated Connect/Protobuf code; regenerate it instead of editing it directly. The Koda protocol source is `../proto/koda/v1/service.proto`.

## Build, Test, and Development Commands

- `pnpm install`: install the locked dependencies. Use Node.js 24+ and pnpm 10.
- `pnpm dev`: start Vite; API requests are proxied to Koda at `http://localhost:8080`.
- `pnpm typecheck`: run the TypeScript project checks.
- `pnpm lint`: run ESLint, including React Hooks rules.
- `pnpm test`: run the Vitest suite once; use `pnpm test:watch` while developing.
- `pnpm build`: type-check and create the production bundle in `dist/`.
- `pnpm format:check`: verify Prettier formatting; `pnpm format` applies it.
- `pnpm generate:proto`: regenerate the Go and TypeScript bindings from the root Proto source using Buf.

## Coding Style & Naming Conventions

Prettier is authoritative: two-space indentation, single quotes, no semicolons, and trailing commas. Use `PascalCase` for React components and exported types, `camelCase` for functions and variables, and kebab-case filenames such as `provider-settings-page.tsx`. Prefer focused components and keep route-specific behavior in its page module.

## Testing Guidelines

Use Vitest with Testing Library and `jest-dom`. Test visible behavior and state transitions rather than implementation details. Add regression tests for bug fixes, especially streaming, cache reconciliation, and turn grouping. No coverage threshold is currently enforced.

## Commit & Pull Request Guidelines

Use scoped Conventional Commits, for example `feat(session): add optimistic user messages` or `fix(settings): keep navigation fixed`. Keep commits focused. Pull requests should explain behavior changes, list verification commands, link relevant issues, and include screenshots for visible UI changes. Call out protocol updates explicitly and commit regenerated stubs with their Proto source.

## Configuration & Security

Override the API origin with `VITE_KODA_API_URL` when needed. Never commit provider API keys, `.env` files, credentials, or local workspace data.
