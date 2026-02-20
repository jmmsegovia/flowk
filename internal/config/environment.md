# Functional Overview

`environment.go` reads platform environment definitions from JSON and builds in-memory structures describing Cassandra, Postgres, and MySQL connectivity for each platform. The resulting configuration is used by actions that interact with Cassandra clusters and SQL databases.

# Technical Implementation Details

* **Data structures:**
  * `Environment` holds a map of platform names to `Platform` entries.
  * `Platform` currently wraps a `CassandraConfig`, making room for other subsystems in the future.
  * `CassandraConfig` lists cluster hosts, port, and the keyspaces to target. `Keyspace` contains the authentication credentials.
  * `PostgresConfig` lists a host, port, SSL mode, and the databases to target. `PostgresDatabase` contains authentication credentials plus an optional schema name.
  * `MySQLConfig` lists a host, port, and the databases to target. `MySQLDatabase` contains authentication credentials.
* **Public API:** `LoadEnvironment` reads a JSON file, unmarshals it into a `rawEnvironment`, and constructs a validated `Environment`.
* **Validation pipeline:**
  * `buildCassandraConfig` converts string representations into strongly typed fields. It parses the port (`strconv.Atoi`), splits the `cluster` string into hostnames with `parseHosts`, and builds a slice of valid `Keyspace` entries.
  * `buildPostgresConfig` parses the host, port, and optional SSL mode while building a slice of valid `PostgresDatabase` entries.
  * `buildMySQLConfig` parses the host and port while building a slice of valid `MySQLDatabase` entries.
  * `Keyspace.Valid` ensures required fields (`Name`, `Username`, `Password`) are non-empty.
  * `PostgresDatabase.Valid` ensures required fields (`Name`, `Username`, `Password`) are non-empty.
  * `MySQLDatabase.Valid` ensures required fields (`Name`, `Username`, `Password`) are non-empty.
  * Missing hosts, databases, or keyspaces (or invalid port values) produce descriptive errors referencing the platform name.
* **Host parsing:** `parseHosts` treats commas, semicolons, whitespace, and newlines as separators, trimming each host before adding it to the result slice.
* **Raw types:** Nested `raw*` structs mirror the JSON layout, making the decoding step explicit and allowing for data cleansing before constructing the strongly typed configuration.
* **Error reporting:** All user-facing errors include context (`platform <name>`) so misconfigurations can be traced back to the source definition quickly.
