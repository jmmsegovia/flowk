# Database Actions

Actions for interacting with relational and NoSQL databases.

## DB_MYSQL_OPERATION

Executes operations against a MySQL or MariaDB database.

### Action: `DB_MYSQL_OPERATION`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. Operation type (see below). |
| `database` | String | Target database name (Required for SQL). |
| `command` | String | SQL query string (Required for `SQL` operation). |

#### Operations
- `SQL`: Run arbitrary SQL query.
- `CHECK_CONNECTION`: Ping the database.
- `TRUNCATE_ALL_TABLES`: Dangerous. Clears all data.
- `LOAD_CSV`: Load data from file.

### Example
```json
{
  "id": "query_users",
  "action": "DB_MYSQL_OPERATION",
  "operation": "SQL",
  "database": "app_db",
  "command": "SELECT count(*) FROM users WHERE active = 1"
}
```

---

## DB_POSTGRES_OPERATION

Executes operations against a PostgreSQL database.

### Action: `DB_POSTGRES_OPERATION`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. Same supported operations as MySQL. |
| `database` | String | Target database name. |
| `command` | String | SQL query string. |

### Example
```json
{
  "id": "create_schema",
  "action": "DB_POSTGRES_OPERATION",
  "operation": "SQL",
  "database": "analytics",
  "command": "CREATE SCHEMA IF NOT EXISTS warehouse;"
}
```

---

## DB_CASSANDRA_OPERATION

Executes operations against a Cassandra / ScyllaDB cluster.

### Action: `DB_CASSANDRA_OPERATION`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. `CQL`, `CHECK_CONNECTION` etc. |
| `keyspace` | String | Target keyspace. |
| `command` | String | CQL query string (Required for `CQL` operation). |

### Example
```json
{
  "id": "query_sensor_data",
  "action": "DB_CASSANDRA_OPERATION",
  "operation": "CQL",
  "keyspace": "iot_data",
  "command": "SELECT * FROM sensors WHERE id = 'sensor_1';"
}
```
