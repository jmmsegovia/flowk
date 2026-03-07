# SECRET_PROVIDER_VAULT action

The **SECRET_PROVIDER_VAULT** action provides native Vault HTTP operations (without external CLI commands) to manage KV v2 data that is later consumed via `${secret:vault:...}` placeholders.

## Supported operations

- `HEALTH`: checks Vault availability with `/v1/sys/health`.
- `KV_PUT`: creates or updates a KV v2 secret using `/v1/<kv_mount>/data/<path>`.
- `KV_GET`: reads a KV v2 secret from `/v1/<kv_mount>/data/<path>`.
- `KV_LIST`: lists child keys under a prefix with `/v1/<kv_mount>/metadata/<path>?list=true`.
- `KV_DELETE`: permanently deletes all versions and metadata with `/v1/<kv_mount>/metadata/<path>`.

## Main fields

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `operation` | String | Yes | `HEALTH`, `KV_PUT`, `KV_GET`, `KV_LIST`, or `KV_DELETE`. |
| `address` | String | Yes | Vault base URL, for example `http://127.0.0.1:8200`. |
| `token` | String | Yes | Token sent in `X-Vault-Token`. |
| `kv_mount` | String | No | KV v2 mount. Defaults to `secret`. |
| `path` | String | `KV_*` only | Secret path/prefix inside the mount. |
| `data` | Object | `KV_PUT` only | Key-value map to write. |

## Result notes

- `KV_PUT` returns only `written_keys` metadata.
- `KV_GET` returns `secret_data`.
- `KV_LIST` returns `secret_keys`.
- `KV_DELETE` returns `deleted: true` when the request succeeds.

## `HEALTH` example

```json
{
  "id": "vault.health",
  "name": "vault.health",
  "action": "SECRET_PROVIDER_VAULT",
  "operation": "HEALTH",
  "address": "http://127.0.0.1:8200",
  "token": "root"
}
```

## `KV_PUT` example

```json
{
  "id": "vault.seed.secret",
  "name": "vault.seed.secret",
  "action": "SECRET_PROVIDER_VAULT",
  "operation": "KV_PUT",
  "address": "http://127.0.0.1:8200",
  "token": "root",
  "kv_mount": "secret",
  "path": "apps/demo",
  "data": {
    "api_token": "demo-token"
  }
}
```

## `KV_GET` example

```json
{
  "id": "vault.read.secret",
  "name": "vault.read.secret",
  "action": "SECRET_PROVIDER_VAULT",
  "operation": "KV_GET",
  "address": "http://127.0.0.1:8200",
  "token": "root",
  "kv_mount": "secret",
  "path": "apps/demo"
}
```

## Security notes

- Keep `token` and secret payloads outside source control whenever possible.
- `KV_GET` includes secret values in the action result; avoid logging task results to insecure sinks.
- Use short-lived tokens with least privilege policies for automation.
