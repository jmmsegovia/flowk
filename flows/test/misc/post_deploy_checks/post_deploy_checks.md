# post_deploy_checks

Detailed Description
Post deploy checks subflow. This subflow simulates running post-deployment checks, like an HTTP health check. Imports 1 subflow(s): docker_http_mock_health. Primary actions: Docker container management, HTTP/HTTPS requests, console output, controlled waits.

Requirements
- Docker installed and daemon running.
- HTTP connectivity to the configured endpoint(s).
- Local HTTP services available on 127.0.0.1/localhost as required by the flow.
