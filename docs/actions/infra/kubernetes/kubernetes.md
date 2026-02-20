# Functional Overview

`kubernetes.go` implements the **KUBERNETES** action. It connects to a Kubernetes cluster using the configured context (and optional kubeconfig path) and executes the requested operation.

# Supported operations

- `GET_PODS`: list pods in a namespace.
- `GET_DEPLOYMENTS`: list deployments in a namespace.
- `GET_LOGS`: fetch logs for a set of pods or deployments.
- `SCALE`: scale deployments to a target replica count.
- `WAIT_FOR_POD_READINESS`: wait until deployments report ready pods.
- `PORT_FORWARD`: open a port-forward tunnel to a service.
- `STOP_PORT_FORWARD`: stop a previously opened port-forward.

# Payload notes

- `context` is required for all operations except `STOP_PORT_FORWARD`.
- `namespace` is optional; when omitted, the kubeconfig default (or `default`) is used.
- `GET_LOGS` requires either `pod` or `deployments`, but not both. Optional `container`, `since_time` (RFC3339), and `since_pod_start` can narrow logs.
- `SCALE` requires `namespace`, `deployments`, and `replicas`.
- `WAIT_FOR_POD_READINESS` requires `namespace`, `deployments`, `max_wait_seconds`, and `poll_interval_seconds`.
- `PORT_FORWARD` requires `service`, `local_port`, and `service_port`.
- `STOP_PORT_FORWARD` requires `local_port`.

# Result payloads

All operations return `flow.ResultTypeJSON`.

- `GET_PODS`: array of pod summaries (name, namespace, status, ready, restarts, age, IP, node, images, etc.).
- `GET_DEPLOYMENTS`: array of deployment summaries (name, namespace, desired/ready/available replicas, age).
- `GET_LOGS`: array of log file descriptors (`namespace`, `pod`, `container`, `file`).
- `SCALE`: array of scale results (`deployment`, `previousReplicas`, `desiredReplicas`, `changed`).
- `WAIT_FOR_POD_READINESS`: object with deployment readiness status, elapsed time, and success flag.
- `PORT_FORWARD`: object with namespace, service, pod, local/service ports, and target port.
- `STOP_PORT_FORWARD`: object with local port and stop status.

# Example (GET_PODS)

```json
{
  "id": "list_pods",
  "action": "KUBERNETES",
  "operation": "GET_PODS",
  "context": "DEV_CLUSTER",
  "namespace": "default"
}
```
