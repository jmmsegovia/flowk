# dog_breed_insights_with_subflows

Detailed Description
Complex example that combines Dog API analytics with Kubernetes observability using reusable subflows. Imports 4 subflow(s): docker_http_mock_dog_api, dog_api_ingestion, dog_api_metrics, k8s_observability_checks. Primary actions: Docker container management, assertions and conditional logic, HTTP/HTTPS requests, console output, controlled waits, variable configuration.

Requirements
- Docker installed and daemon running.
- HTTP connectivity to the configured endpoint(s).
