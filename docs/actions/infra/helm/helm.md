# Functional Overview

`helm.go` implements the **HELM** action. It executes Helm workflows using the official Helm Go SDK (`helm.sh/helm/v3`) and returns structured execution metadata for each operation. It does not shell out to the `helm` binary.

# Supported operations

- `REPO_ADD` → `helm repo add`
- `REPO_UPDATE` → `helm repo update`
- `REPO_LIST` → `helm repo list -o json`
- `SEARCH_REPO` → `helm search repo`
- `INSTALL` → `helm install`
- `UPGRADE` → `helm upgrade`
- `UPGRADE_INSTALL` → `helm upgrade --install`
- `LIST` → `helm list -o json`
- `STATUS` → `helm status -o json`
- `GET_VALUES` → `helm get values`
- `GET_ALL` → `helm get all`
- `HISTORY` → `helm history -o json`
- `ROLLBACK` → `helm rollback`
- `UNINSTALL` → `helm uninstall`
- `SHOW_VALUES` → `helm show values`
- `SHOW_CHART` → `helm show chart`
- `TEMPLATE` → `helm template`
- `LINT` → `helm lint`
- `CREATE` → `helm create`

# Payload notes

- `operation` is required for every HELM task.
- `repository_name` and `repository_url` are required for `REPO_ADD`.
- `query` is required for `SEARCH_REPO`.
- `release_name` + `chart` are required for `INSTALL`, `UPGRADE`, `UPGRADE_INSTALL`, and `TEMPLATE`.
- `release_name` is required for `STATUS`, `GET_VALUES`, `GET_ALL`, `HISTORY`, and `UNINSTALL`.
- `release_name` + `revision` are required for `ROLLBACK`.
- `chart` is required for `SHOW_VALUES`, `SHOW_CHART`, `LINT`, and `CREATE`.

Common optional flags:

- `namespace`, `kube_context`
- `values_files` (`--values` repeated)
- `set_values` (`--set` repeated; command output masks values)
- `version`, `create_namespace`, `wait`, `timeout_seconds`, `dry_run`
- `all_namespaces` for `LIST`
- `include_all` for `GET_VALUES`

# Result payload

All operations return `flow.ResultTypeJSON` with:

- `operation`: operation name.
- `command`: sanitized Helm-equivalent command array (`helm` + args) for traceability.
- `exitCode`: process exit code (0 on success).
- `stdout`, `stderr`: captured command output.
- `durationSeconds`: execution duration.

# Example (`UPGRADE_INSTALL`)

```json
{
  "id": "upgrade_api",
  "action": "HELM",
  "operation": "UPGRADE_INSTALL",
  "release_name": "api",
  "chart": "bitnami/nginx",
  "namespace": "apps",
  "create_namespace": true,
  "wait": true,
  "timeout_seconds": 180,
  "set_values": ["image.tag=1.2.3"]
}
```
