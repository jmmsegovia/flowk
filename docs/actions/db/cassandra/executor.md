# Functional Overview

`executor.go` provides the **DB_CASSANDRA_OPERATION** action. It connects to configured Cassandra keyspaces and performs maintenance tasks such as truncating tables, dropping objects, listing schema components, verifying connectivity, or executing ad-hoc CQL statements. Operations can optionally target a specific keyspace when a name is supplied; otherwise they fan out across every keyspace defined in the platform. The action returns structured data (for list/CQL operations) or booleans (for destructive operations) that downstream tasks can use to make decisions.

# Technical Implementation Details

* **Operation routing:** `Execute` normalises the requested operation string (uppercase and underscores) and dispatches to helper functions for each supported verb: dropping objects, truncating tables, listing tables/objects, checking connectivity, or running arbitrary CQL commands. Unsupported operations return an error.
* **Configuration inputs:** The action consumes `config.CassandraConfig`, which contains host addresses, port, and per-keyspace credentials sourced from the environment definition. Optional `skipTables` parameters are converted into a `tableSkipper` to skip truncation of specific tables. An optional `keyspace` parameter limits destructive or listing operations to a single keyspace.
* **Keyspace selection:** `keyspacesForOperation` resolves the set of keyspaces to process. When a target is provided it validates the name (case-insensitively) and returns that single entry, otherwise it filters out invalid credentials and processes every configured keyspace. Human-readable operation names are produced via `describeOperation` for use in error messages.
* **Logging:** A lightweight `Logger` interface expects `Printf`. Each helper logs progress, such as the keyspace currently processed, individual table names, or skipped objects.
* **Session management:** `openSession` creates `gocql` sessions with password authentication, 10-second connect/read timeouts, and QUORUM consistency. Sessions are closed promptly after use to avoid leaking connections.
* **Drop/truncate helpers:**
  * `dropAllObjects` orchestrates removal of materialized views, indexes, tables, and types for each keyspace.
  * `truncateAllTables` truncates all tables and materialized views, respecting the `tableSkipper`.
  * Dedicated helper functions (`truncateTables`, `truncateMaterializedViews`, `dropTables`, `dropMaterializedViews`, `dropIndexes`, `dropTypes`) build CQL statements with quoted identifiers and execute them under the provided context.
* **Schema discovery:** List operations use Cassandra system tables (`system_schema.*` or legacy alternatives) to retrieve names of tables, views, indexes, and user-defined types. When a system table is absent (depending on Cassandra version), fallback queries and the helper `isMissingSystemSchemaTable` detect and handle the situation gracefully.
* **Utility functions:**
  * `normalizeTableName` and `normalizeQualifiedTableName` trim and lowercase identifiers, splitting "keyspace.table" strings into components for reliable comparisons.
  * `tableSkipper.shouldSkip` checks whether a particular table should be excluded from truncation operations.
  * `quoteIdentifier` escapes double quotes so table names with special characters remain valid.
  * `logObjects` sorts and logs discovered schema elements, or indicates when none exist.
* **Return values:** Boolean operations return `true` plus `flow.ResultTypeBool` upon success. List operations return maps keyed by keyspace name with slices (or nested maps) describing schema objects, paired with `flow.ResultTypeJSON` so the workflow can inspect the structures or feed subsequent VARIABLES declarations that need Cassandra metadata. CQL commands return JSON: row arrays for statements that yield result sets, or `{ "success": true }` when no rows are produced.
* **CQL execution:** The new `executeCQL` helper locates the requested keyspace, opens a session, executes the provided statement, and inspects the resulting column metadata. When columns are present the rows are materialised via `Iter.MapScan` and returned verbatim; otherwise a success marker is produced. Errors reference the keyspace to aid troubleshooting.
