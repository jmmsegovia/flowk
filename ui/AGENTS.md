# AGENTS.md

## Scope
Applies to `ui/**`.

## UI development rules
1. Keep components typed; avoid `any` unless unavoidable.
2. Reuse existing patterns in `src/components`, `src/pages`, and `src/state`.
3. When adding user-visible text, update both `src/i18n/en.json` and `src/i18n/es.json`.
4. Keep action guide category mappings consistent and explicit.
5. Prefer small presentational helpers over deeply nested JSX blocks.

## Validation
- Run `npm run build` for compile-level validation.
- If behavior is changed significantly, add/update targeted tests if present.
