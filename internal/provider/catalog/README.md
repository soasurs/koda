# Bundled model catalogs

These files are the reviewed, offline fallback for built-in providers using
their default API endpoints. They are intentionally curated rather than
exhaustive. A successful provider API discovery determines availability;
matching bundled entries enrich the discovered IDs with display names and
reasoning-effort and context-window metadata.

Update model IDs, effort values, and context windows from provider
documentation:

- Anthropic: <https://platform.claude.com/docs/en/build-with-claude/effort>
- Anthropic model windows: <https://platform.claude.com/docs/en/about-claude/models/overview>
- OpenAI: <https://developers.openai.com/api/docs/models>
- Gemini: <https://ai.google.dev/gemini-api/docs/thinking>
- Gemini model windows: <https://ai.google.dev/gemini-api/docs/models>
- DeepSeek: <https://api-docs.deepseek.com/guides/thinking_mode>
- DeepSeek model windows: <https://api-docs.deepseek.com/quick_start/pricing>

Keep Chat Completions and Responses in separate files even when a model
supports both APIs. Do not add UI-only modes as API effort values; for example,
Claude Code's `ultracode` mode is not an Anthropic API effort level.
