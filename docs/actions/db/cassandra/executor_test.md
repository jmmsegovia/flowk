# Functional Overview

`executor_test.go` focuses on the helpers that prepare Cassandra maintenance operations: the table-skipping logic used by truncation tasks and the keyspace selection routine that now powers optional per-operation filtering.

# Technical Implementation Details

* **`TestTableSkipperShouldSkip`:** Builds a `tableSkipper` with a mix of valid and invalid entries and verifies that lookups behave as expected across different keyspaces, case variations, and missing entries.
* **`TestTableSkipperIgnoresInvalidTables`:** Ensures malformed input (missing keyspace separator) causes the helper constructor to return `nil`, signalling no valid skip rules.
* **`TestTableSkipperNil`:** Demonstrates that a nil pointer behaves gracefully and never attempts to skip tables, preventing panics in calling code.
* **`TestKeyspacesForOperationAllKeyspaces`:** Confirms that omitting the `keyspace` parameter returns every valid keyspace from the platform configuration while discarding invalid credentials.
* **`TestKeyspacesForOperationSpecificKeyspace`:** Verifies that explicitly naming a keyspace (case-insensitively) restricts the operation to that single entry.
* **`TestKeyspacesForOperationMissingKeyspace`:** Exercises the error path when the requested keyspace is absent and checks that the message references both the operation and the missing name.
* **Testing strategy:** These tests rely solely on the Go `testing` package and avoid touching real Cassandra instances, keeping the feedback loop fast while safeguarding string normalisation logic.
