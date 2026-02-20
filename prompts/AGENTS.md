# AGENTS.md

## Scope
Applies to `prompts/**`.

## Prompt authoring rules
1. Prompts in this folder must be written in English unless a file explicitly targets another language.
2. Use explicit, ordered execution steps so an LLM can follow a deterministic workflow.
3. Include concrete quality gates (implementation, schema, tests, docs, examples, validation).
4. Require consistency checks across runtime code, JSON schema, UI guide integration, and docs.
5. Prefer actionable wording ("do X") over ambiguous guidance.
