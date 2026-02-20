# MySQL action

`executor.go` provides the **DB_MYSQL_OPERATION** action. It connects to configured MySQL databases and performs maintenance tasks such as truncating tables, dropping objects, listing schema components, verifying connectivity, or executing ad-hoc SQL statements. Operations can optionally target a specific database when a name is supplied; otherwise they fan out across every database defined in the platform. The action returns structured data (for list/SQL operations) or booleans (for destructive operations) that downstream tasks can use to make decisions.

## Key behaviors

* **Configuration lookup:** The action expects a platform variable with a `mysql` section. Each database entry defines a database name plus credentials.
* **Operation flow:** `Execute` normalises the requested operation and routes it to the appropriate helper, ensuring MySQL operations align with the same operation names used by other database actions.
* **SQL execution:** `executeSQL` locates the requested database, opens a connection, and executes the provided statement. When columns are present the rows are materialised via `common.ScanRow` and returned verbatim; otherwise a success marker is produced. Errors reference the database to aid troubleshooting.
* **Schema operations:** Truncation, listing, and drop operations target tables/views/indexes from `information_schema` and log the objects encountered.
