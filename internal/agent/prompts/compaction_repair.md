The previous compaction output was rejected.

Generate one complete replacement that exactly matches the version 1 schema
above.

- Correct only formatting, schema compliance, omissions, and unsupported claims.
- Preserve facts supported by the original compaction input.
- Do not add facts that are absent from the original input.
- Include every required field and use `[]` for empty categories.
- Keep the replacement concise enough to fit within the output limit.
- Return only the replacement JSON object, without Markdown fences or commentary.
