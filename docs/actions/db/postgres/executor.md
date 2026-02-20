# Functional Overview

`executor.go` provides the **DB_POSTGRES_OPERATION** action. It connects to configured PostgreSQL databases and performs maintenance tasks such as truncating tables, dropping objects, listing schema components, verifying connectivity, or executing ad-hoc SQL statements. Operations can optionally target a specific database when a name is supplied; otherwise they fan out across every database defined in the platform. The action returns structured data (for list/SQL operations) or booleans (for destructive operations) that downstream tasks can use to make decisions.

# Technical Implementation Details

* **Operation routing:** `Execute` normalises the requested operation string (uppercase and underscores) and dispatches to helper functions for each supported verb: dropping objects, truncating tables, listing tables/objects, checking connectivity, or running arbitrary SQL commands. Unsupported operations return an error.
* **Configuration inputs:** The action consumes `config.PostgresConfig`, which contains host, port, SSL mode, and per-database credentials sourced from the platform definition. Optional `skipTables` parameters are converted into a `tableSkipper` to skip truncation of specific tables. An optional `database` parameter limits destructive or listing operations to a single database.
* **Database selection:** `databasesForOperation` resolves the set of databases to process. When a target is provided it validates the name (case-insensitively) and returns that single entry, otherwise it filters out invalid credentials and processes every configured database. Human-readable operation names are produced via `describeOperation` for use in error messages.
* **Logging:** A lightweight `Logger` interface expects `Printf`. Each helper logs progress, such as the database/schema currently processed, individual table or index names, or skipped objects.
* **Connection management:** `openConnection` builds a DSN using the `pgx` stdlib driver, applying host, port, credentials, database name, and SSL mode (defaulting to `disable`). It verifies connectivity with a 10-second ping timeout and closes connections promptly after use to avoid leaks.
* **Drop/truncate helpers:**
  * `dropAllObjects` orchestrates removal of views, indexes, tables, and types for each configured schema.
  * `truncateAllTables` truncates all tables in the schema, respecting the `tableSkipper`.
  * Dedicated helper functions (`truncateTables`, `dropTables`, `dropViews`, `dropIndexes`, `dropTypes`) build SQL statements with quoted identifiers and execute them under the provided context.
* **Schema discovery:** List operations query `information_schema` and `pg_catalog` sources to retrieve names of tables, views, indexes, and user-defined types (composite, enum, and domain). The schema name is taken from the database config (defaulting to `public`) to scope the queries.
* **Utility functions:**
  * `normalizeTableName` and `normalizeQualifiedTableName` trim and lowercase identifiers, splitting "schema.table" strings into components for reliable comparisons.
  * `tableSkipper.shouldSkip` checks whether a particular table should be excluded from truncation operations.
  * `quoteIdentifier` escapes double quotes so identifiers with special characters remain valid.
  * `logObjects` sorts and logs discovered schema elements, or indicates when none exist.
  * `common.ScanRow` converts SQL result sets into `map[string]any` rows, decoding byte slices to strings for JSON-friendly output.
* **Return values:** Boolean operations return `true` plus `flow.ResultTypeBool` upon success. List operations return maps keyed by database name with slices (or nested maps) describing schema objects, paired with `flow.ResultTypeJSON` so the workflow can inspect the structures or feed subsequent VARIABLES declarations that need Postgres metadata. SQL commands return JSON: row arrays for statements that yield result sets, or `{ "success": true }` when no rows are produced.
* **SQL execution:** `executeSQL` locates the requested database, opens a connection, and executes the provided statement. When columns are present the rows are materialised via `scanRow` and returned verbatim; otherwise a success marker is produced. Errors reference the database to aid troubleshooting.
