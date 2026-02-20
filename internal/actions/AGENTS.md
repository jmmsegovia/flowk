# AGENTS.md

## Scope
Applies to `internal/actions/**`.

## Implementation contract for new or modified actions
1. Follow existing package layout:
   - `action.go` for registry wiring and `Execute` bridge.
   - runtime logic in a focused file (`<action>.go` or similar).
   - `schema.json` + `schema.go` (embedded schema fragment).
   - tests covering both success and error/validation paths.
2. Register through `registry.Register(action{})` in `init()`.
3. Keep `Name()` stable and uppercase-compatible with flow payloads.
4. Return `registry.Result` with accurate `flow.ResultType`.
5. Validate payloads deterministically and fail with actionable errors.
6. Prefer pure helper functions for core logic to maximize testability.
7. Never leak secrets into logs/results; mask sensitive values when needed.

## Schema quality bar
- Every user-facing field should have a clear description.
- Encode constraints in schema (`enum`, `oneOf`, bounds, conditionals).
- Use `if/then` for operation-specific requirements.
- Keep schema and runtime validation consistent.
- Never add global `enum`/`pattern` constraints for shared fields (e.g. `operation`) at the top-level `properties`. Always scope them inside `if action == <ACTION>` + `then` to avoid restricting other actions when schemas are merged.

## Testing
- Add table-driven tests for validation and execution edge cases.
- Include at least one realistic payload per supported operation.
- Cover backward-compatibility behavior when legacy fields exist.
