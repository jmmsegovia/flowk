# Release Notes

## v0.1.2

- Enforce `name` as required in flow/task schemas, propagate it through runtime validation, and render `name`/`flowNames` consistently in UI/API/docs.
- Add UI layout persistence to disk (config directory under `ui/layouts`) with Save/Reset/Auto-save controls for node positions and viewport.
- Add native Vault secret resolution via config (`secrets.provider: vault`) using `${secret:vault:<path>#<field>}` placeholders in task payload expansion.
- Introduce `SECRET_PROVIDER_VAULT` system action for native Vault HTTP operations (`HEALTH`, `KV_PUT`, `KV_GET`, `KV_LIST`, `KV_DELETE`) without external Vault CLI calls.
- Add end-to-end Vault Docker demo flows and aligned docs/schemas for provider configuration, placeholders, and KV v2 operations.
- Extend `EVALUATE` to support native Vault secret placeholders in conditions.
- Expose raw task payload data in the UI inspector for better debugging and troubleshooting.
- Expose OpenAPI contract for `-serve-ui` backend at `/api/openapi.json` to support client generation/integration.
- Standardize backend UI/API error messages in English.

## v0.1.1

- Add required name support across schemas, UI, and docs
- Enforce `name` in flow/task schemas and update tests/fixtures
- Expose `name`/`flowNames` in UI API and render names in React Flow/inspector/sidebar
- Update documentation/examples to include required `name` fields

## v0.1.0

- Initial public release.
