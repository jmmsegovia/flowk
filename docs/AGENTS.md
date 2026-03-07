# AGENTS.md

## Scope
This file applies to everything under `docs/`.

## Documentation language
1. Write all new documentation in English.
2. When editing an existing Spanish section, translate it to English as part of the same change when practical.
3. Keep technical identifiers unchanged (`action` names, JSON fields, CLI flags, env vars, file paths).

## Content rules
1. Keep examples runnable and aligned with current behavior.
2. Do not describe implicit behavior as guaranteed behavior; call out defaults and requirements explicitly.
3. Use consistent requirement wording in tables:
- `Required`: `Yes` / `No` / `<CONDITION> only`.
4. For action docs, include at least:
- Short purpose statement.
- Supported operations.
- Main fields table.
- At least one JSON example.
- Security notes when secrets/tokens/credentials are involved.

## Cross-file consistency
1. If an action contract changes, update both:
- The action-specific page in `docs/actions/**`.
- The category/index reference (`docs/actions/*.md`) if it summarizes that action.
2. Keep links relative within `docs/` and verify they resolve.

## Quality checks
1. Proofread for concise technical English.
2. Validate JSON/YAML snippets for syntax correctness.
3. Avoid mixing languages in the same section unless quoting external text.
