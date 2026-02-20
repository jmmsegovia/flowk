# System & Infrastructure Actions

Actions for OS-level execution and container management.

## SHELL

Executes a shell command on the local machine where `flowk` is running.

### Action: `SHELL`

| Property | Type | Description |
| :--- | :--- | :--- |
| `shell` | Object | Optional. Defines the shell program (e.g., `/bin/bash`). |
| `command` | Array | **Required**. List of command lines to execute. |
| `environment` | Array | Optional environment variables. |
| `workingDirectory` | String | Directory to execute in. |

### Example
```json
{
  "id": "local_build",
  "action": "SHELL",
  "shell": { "program": "/bin/bash", "args": ["-c"] },
  "command": [
    "echo 'Starting build...'",
    "go build -v ./..."
  ]
}
```

---


## BASE64

Encodes and decodes data using Go's `encoding/base64` implementation (no external binaries required).

### Action: `BASE64`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. `ENCODE` or `DECODE`. |
| `input` | String | Inline input value. Use this or `inputFile`. |
| `inputFile` | String | Input file path. Use this or `input`. |
| `outputFile` | String | Optional output file path where resulting data will be written. |
| `wrap` | Integer | Optional encoding wrap width. Use `0` to disable wrapping; omitted defaults to 76. |
| `ignoreGarbage` | Boolean | Optional decode flag. When true, non-base64 bytes are stripped before decoding. |
| `urlSafe` | Boolean | Optional encoding flag. When true, outputs base64url without padding. |

### Example
```json
{
  "id": "base64_encode_demo",
  "action": "BASE64",
  "operation": "ENCODE",
  "input": "FlowK",
  "wrap": 0
}
```

---

## DOCKER

Manages Docker containers and images.

### Action: `DOCKER`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. `CONTAINER_RUN`, `IMAGE_PULL`, etc. |
| `image` | String | Image name (Required for run/pull). |
| `container` | String | Container name/ID (Required for stop/remove/exec). |
| `command` | Array | Command args for run/exec. |
| `env` | Array | Environment variables (`Key=Value`). |
| `ports` | Array | Port mappings (`8080:80`). |

### Example (Run Container)
```json
{
  "id": "start_redis",
  "action": "DOCKER",
  "operation": "CONTAINER_RUN",
  "image": "redis:alpine",
  "name": "my-redis",
  "ports": ["6379:6379"],
  "detach": true
}
```

---

## KUBERNETES

Interacts with a Kubernetes cluster.

### Action: `KUBERNETES`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. `GET_PODS`, `GET_LOGS`, `SCALE`, etc. |
| `context` | String | Kubeconfig context name. |
| `namespace` | String | K8s namespace. |
| `deployments` | Array | List of deployment names (for scale, readiness). |

### Example (Scale Deployment)
```json
{
  "id": "scale_app",
  "action": "KUBERNETES",
  "operation": "SCALE",
  "namespace": "production",
  "deployments": ["frontend-app"],
  "replicas": 3
}
```

---

## HELM

Manages Helm repositories and release lifecycle operations.

### Action: `HELM`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. Helm operation (e.g. `REPO_ADD`, `INSTALL`, `UPGRADE_INSTALL`). |
| `release_name` | String | Required for release-focused operations (`INSTALL`, `STATUS`, `UNINSTALL`, etc.). |
| `chart` | String | Chart reference or path for install/upgrade/lint/template/show operations. |
| `repository_name` | String | Repository alias used in `REPO_ADD`. |
| `repository_url` | String | Repository URL used in `REPO_ADD`. |
| `values_files` | Array | Optional list of values files passed with `--values`. |
| `set_values` | Array | Optional list of `KEY=VALUE` overrides passed with `--set`. |

### Example (`UPGRADE_INSTALL`)
```json
{
  "id": "helm_upgrade",
  "action": "HELM",
  "operation": "UPGRADE_INSTALL",
  "release_name": "web",
  "chart": "bitnami/nginx",
  "namespace": "apps",
  "set_values": ["image.tag=2026.02"],
  "wait": true,
  "timeout_seconds": 120
}
```
