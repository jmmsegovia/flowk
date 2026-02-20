# Actions Reference

FlowK comes with a comprehensive standard library of actions. This reference groups them by category.

## Core & Control Flow
Actions for managing variables, loops, debugging, and flow structure.

- **[PRINT](./core.md#print)**: Log messages to stdout/UI console.
- **[VARIABLES](./core.md#variables)**: Set, update, or transform variables.
- **[SLEEP](./core.md#sleep)**: Pause execution for a set duration.
- **[PARALLEL](./core.md#parallel)**: Run specific tasks concurrently.
- **[FOR](./core.md#for)**: Iterate over lists or numbers.
- **[EVALUATE](./core.md#evaluate)**: Branch or stop execution based on conditions.


## Authentication
OAuth and identity workflows.

- **[GMAIL](./auth.md#gmail)**: Send Gmail messages using OAuth2 access tokens.
- **[OAUTH2](./auth.md#oauth2)**: Build OAuth authorize URLs and exchange/refresh/revoke/introspect tokens.

## Network & Connectivity
Interacting with web services and remote machines.

- **[HTTP_REQUEST](./network.md#http_request)**: Make REST/HTTP requests (GET, POST, etc.) with validation.
- **[SSH](./network.md#ssh)**: Execute commands on remote servers via SSH.
- **[TELNET](./network.md#telnet)**: Interact with TCP services using send/expect steps.

## Database
Native database integrations for querying and assertions.

- **[DB_CASSANDRA_OPERATION](./db.md#db_cassandra_operation)**: Run queries against Cassandra.
- **[DB_MYSQL_OPERATION](./db.md#db_mysql_operation)**: Run queries against MySQL/MariaDB.
- **[DB_POSTGRES_OPERATION](./db.md#db_postgres_operation)**: Run queries against PostgreSQL.

## System & Infrastructure
OS-level operations and container management.

- **[SHELL](./system.md#shell)**: Run local shell commands.
- **[BASE64](./system.md#base64)**: Encode/decode text or files using Go's `encoding/base64`.
- **[DOCKER](./infra.md#docker)**: Manage Docker containers (run, stop, inspect).
- **[KUBERNETES](./infra.md#kubernetes)**: Apply manifests or check pod status.
- **[HELM](./infra.md#helm)**: Manage Helm repos, releases, charts, and templates.

## Storage
File system and cloud storage operations.

- **[GCLOUD_STORAGE](./storage.md#gcloud-storage)**: Interact with Google Cloud Storage buckets.

## Security
Cryptography and secrets management.

- **[PGP](./security.md#pgp)**: Encrypt/Decrypt files or strings.

---

*Note: Actions are dynamically registered. Use `flowk help action` to list actions or check the source code in `internal/actions` for the very latest updates.*
