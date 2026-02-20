# AGENTS.md

## Scope
This file applies to the entire repository unless a deeper `AGENTS.md` overrides it.

## Project map (quick orientation)
- `cmd/flowk`: CLI entrypoint and wiring.
- `internal/app`: flow execution engine and runtime orchestration.
- `internal/flow`: flow model + schema-based validation.
- `internal/actions/*`: action implementations and action-level schemas.
- `internal/server/ui`: HTTP/SSE backend for the web UI.
- `ui/`: React + TypeScript frontend.
- `docs/`: end-user and developer documentation.
- `flows/test/`: runnable sample flows.
- `prompts/`: prompts/templates used to guide LLM-driven generation.

## Global rules
1. Keep runtime, schema, docs, and examples aligned whenever behavior changes.
2. Prefer minimal, focused diffs over broad refactors.
3. Preserve backwards compatibility for existing task payloads when practical.
4. Do not introduce silent behavior changes; document notable semantics in docs.
5. If a change touches execution behavior, add or update tests.

## Validation expectations
Run the narrowest useful checks first, then broader checks if needed:
- Go unit tests for touched packages.
- `go test ./...` when changes cross multiple subsystems.
- UI checks (`npm run build` and/or relevant tests) for frontend changes.
- Flow validation for new/changed examples.

## Done criteria
A change is considered done only when:
- Implementation compiles.
- Relevant tests/checks pass.
- Documentation/examples are updated (if user-facing behavior changed).
