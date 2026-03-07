# vault_docker_provider_demo

End-to-end demos of native Vault integration using only FlowK actions.

All examples:

1. Start a Vault dev instance with the native `DOCKER` action.
2. Execute Vault operations with `SECRET_PROVIDER_VAULT`.
3. Stop and remove the Vault container with `DOCKER` actions.

## Available flows

- `vault_docker_provider_demo.json`: full lifecycle (`HEALTH`, `KV_PUT`, `KV_GET`, `KV_LIST`, placeholder resolution, `KV_DELETE`).
- `vault_docker_seed_and_read.json`: minimal seed + read scenario (`KV_PUT`, `KV_GET`).
- `vault_docker_list_and_delete.json`: list and delete scenario (`KV_LIST`, `KV_DELETE`).

## Requirements

- Local Docker available.
- FlowK configuration with the Vault provider enabled for placeholders.

## Example config (`config.vault.dev.yaml`)

```yaml
secrets:
  provider: vault
  vault:
    address: http://127.0.0.1:8200
    token: root
    kv_mount: secret
```

## Schema validation (without executing tasks)

```bash
flowk run -flow flows/test/variables/vault_docker_provider_demo/vault_docker_provider_demo.json -config flows/test/variables/vault_docker_provider_demo/config.vault.dev.yaml -validate-only
flowk run -flow flows/test/variables/vault_docker_provider_demo/vault_docker_seed_and_read.json -config flows/test/variables/vault_docker_provider_demo/config.vault.dev.yaml -validate-only
flowk run -flow flows/test/variables/vault_docker_provider_demo/vault_docker_list_and_delete.json -config flows/test/variables/vault_docker_provider_demo/config.vault.dev.yaml -validate-only
```

## Full execution

```bash
flowk run -flow flows/test/variables/vault_docker_provider_demo/vault_docker_provider_demo.json -config flows/test/variables/vault_docker_provider_demo/config.vault.dev.yaml
```
