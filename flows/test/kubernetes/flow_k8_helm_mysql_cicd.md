# flow_k8_helm_mysql_cicd

Detailed Description
CI/CD-style flow that installs MySQL with a local Helm chart, installs an app release with Helm, scales the app deployment down to zero and back, waits for pod readiness, and runs sample MySQL queries over a Kubernetes port-forward. Primary actions: Helm operations, Kubernetes operations, MySQL database operations.
Imports 6 subflow(s): k8_cicd_vars, k8_helm_repos, k8_mysql_install, k8_app_rollout, k8_mysql_queries, k8_cleanup.

Requirements
- Valid kubeconfig with access to a Kubernetes cluster (tested with Docker Desktop context `docker-desktop`).
- Network access to the Bitnami Helm repo.
- Local port `3307` available for the MySQL port-forward tunnel.
- The local chart `flows/test/kubernetes/charts/mysql-lite` is used to deploy MySQL (image `mysql:8.0`).
- If you customize Helm name overrides, update `app_deployment` and `mysql_service` variables accordingly.
