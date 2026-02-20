package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"flowk/internal/actions/db/common"
	"flowk/internal/config"
	"flowk/internal/flow"
)

const (
	// ActionName identifies the Postgres action in the flow file.
	ActionName = "DB_POSTGRES_OPERATION"
)

// Logger is a minimal interface implemented by the standard library logger.
type Logger interface {
	Printf(format string, v ...interface{})
}

func describePlatformDatabase(platformName, databaseName, schemaName string) string {
	trimmed := strings.TrimSpace(platformName)
	if trimmed == "" {
		return fmt.Sprintf("[No platform found] database %s schema %s", databaseName, schemaName)
	}
	return fmt.Sprintf("platform %s, database %s schema %s", trimmed, databaseName, schemaName)
}

// Execute performs the requested Postgres operation for all databases configured in the
// provided platform configuration and returns the outcome of the operation along with its
// data type. The returned value can be consumed by subsequent tasks.
func Execute(ctx context.Context, cfg config.PostgresConfig, op string, skipTables []string, databaseName, command, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
	normalized := common.NormalizeOperation(op)
	switch normalized {
	case common.OperationDropAllObjects:
		if err := dropAllObjects(ctx, cfg, databaseName, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case common.OperationTruncateAllTables:
		if err := truncateAllTables(ctx, cfg, skipTables, databaseName, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case common.OperationListAllTables:
		tablesByDatabase, err := listAllTables(ctx, cfg, databaseName, logger)
		if err != nil {
			return nil, "", err
		}
		return tablesByDatabase, flow.ResultTypeJSON, nil
	case common.OperationListAllObjects:
		objectsByDatabase, err := listAllObjects(ctx, cfg, databaseName, logger)
		if err != nil {
			return nil, "", err
		}
		return objectsByDatabase, flow.ResultTypeJSON, nil
	case common.OperationCheckConnection:
		if err := checkConnection(ctx, cfg, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case common.OperationSQL:
		return executeSQL(ctx, cfg, databaseName, command, logger)
	case common.OperationLoadCSV:
		return loadCSV(ctx, cfg, databaseName, tableName, filePath, columns, delimiter, hasHeader, logger)
	default:
		return nil, "", fmt.Errorf("unsupported Postgres operation %q", op)
	}
}

func checkConnection(ctx context.Context, cfg config.PostgresConfig, logger Logger) error {
	for _, database := range cfg.Databases {
		if !database.Valid() {
			continue
		}

		schemaName := database.SchemaOrDefault()
		scope := describePlatformDatabase(cfg.Name, database.Name, schemaName)
		logger.Printf("Postgres: checking connection to %s host: %s", scope, cfg.Host)

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
		}

		ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := db.PingContext(ctxWithTimeout); err != nil {
			db.Close()
			return fmt.Errorf("checking connection to Postgres database %s: %w", database.Name, err)
		}

		logger.Printf("Postgres: connection to %s successful", scope)
		db.Close()
	}

	return nil
}

func executeSQL(ctx context.Context, cfg config.PostgresConfig, databaseName, command string, logger Logger) (any, flow.ResultType, error) {
	trimmedDatabase := strings.TrimSpace(databaseName)
	if trimmedDatabase == "" {
		return nil, "", fmt.Errorf("postgres SQL operation: database is required")
	}

	trimmedCommand := strings.TrimSpace(command)
	if trimmedCommand == "" {
		return nil, "", fmt.Errorf("postgres SQL operation: command is required")
	}

	database, found := findDatabase(cfg, trimmedDatabase)
	if !found {
		return nil, "", fmt.Errorf("postgres SQL operation: database %q not found in platform configuration", trimmedDatabase)
	}

	db, err := openConnection(cfg, database)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
	}
	defer db.Close()

	schemaName := database.SchemaOrDefault()
	logger.Printf("Postgres: executing SQL command in %s", describePlatformDatabase(cfg.Name, database.Name, schemaName))

	rows, err := db.QueryContext(ctx, trimmedCommand)
	if err != nil {
		return nil, "", fmt.Errorf("executing SQL command in database %s: %w", database.Name, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, "", fmt.Errorf("reading SQL command columns in database %s: %w", database.Name, err)
	}

	var results []map[string]any
	if len(columns) > 0 {
		results = make([]map[string]any, 0)
		for rows.Next() {
			row, err := common.ScanRow(columns, rows)
			if err != nil {
				return nil, "", fmt.Errorf("reading SQL row in database %s: %w", database.Name, err)
			}
			results = append(results, row)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("executing SQL command in database %s: %w", database.Name, err)
	}

	if len(columns) > 0 {
		if results == nil {
			results = make([]map[string]any, 0)
		}
		logger.Printf("Postgres: SQL command returned %d row(s)", len(results))
		return results, flow.ResultTypeJSON, nil
	}

	logger.Printf("Postgres: SQL command executed successfully")
	return map[string]any{"success": true}, flow.ResultTypeJSON, nil
}

func loadCSV(ctx context.Context, cfg config.PostgresConfig, databaseName, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
	trimmedDatabase := strings.TrimSpace(databaseName)
	if trimmedDatabase == "" {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: database is required")
	}
	trimmedTable := strings.TrimSpace(tableName)
	if trimmedTable == "" {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: table is required")
	}

	database, found := findDatabase(cfg, trimmedDatabase)
	if !found {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: database %q not found in platform configuration", trimmedDatabase)
	}

	includeHeader := true
	if hasHeader != nil {
		includeHeader = *hasHeader
	}
	csvData, err := common.LoadCSVFile(common.CSVLoadOptions{FilePath: filePath, Columns: columns, Delimiter: delimiter, HeaderInFirstRow: includeHeader})
	if err != nil {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: %w", err)
	}

	db, err := openConnection(cfg, database)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
	}
	defer db.Close()

	schemaName := database.SchemaOrDefault()
	logger.Printf("Postgres: loading %d CSV row(s) into %s.%s", len(csvData.Rows), schemaName, trimmedTable)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s)", quoteIdentifier(schemaName), quoteIdentifier(trimmedTable), joinQuotedIdentifiers(csvData.Columns), postgresPlaceholders(len(csvData.Columns)))
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for i, row := range csvData.Rows {
		values := make([]any, len(row))
		for index := range row {
			values[index] = row[index]
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return nil, "", fmt.Errorf("postgres LOAD_CSV operation: inserting row %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("postgres LOAD_CSV operation: commit transaction: %w", err)
	}

	return map[string]any{"success": true, "rows_loaded": len(csvData.Rows), "database": database.Name, "schema": schemaName, "table": trimmedTable}, flow.ResultTypeJSON, nil
}

func joinQuotedIdentifiers(identifiers []string) string {
	quoted := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		quoted = append(quoted, quoteIdentifier(identifier))
	}
	return strings.Join(quoted, ",")
}

func postgresPlaceholders(columns int) string {
	values := make([]string, 0, columns)
	for index := 1; index <= columns; index++ {
		values = append(values, fmt.Sprintf("$%d", index))
	}
	return strings.Join(values, ",")
}

func dropAllObjects(ctx context.Context, cfg config.PostgresConfig, databaseName string, logger Logger) error {
	databases, err := databasesForOperation(cfg, databaseName, common.OperationDropAllObjects)
	if err != nil {
		return err
	}

	for _, database := range databases {
		schemaName := database.SchemaOrDefault()
		logger.Printf("Postgres: dropping all objects in %s", describePlatformDatabase(cfg.Name, database.Name, schemaName))

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
		}

		if err := dropViews(ctx, db, schemaName, logger); err != nil {
			db.Close()
			return err
		}
		if err := dropIndexes(ctx, db, schemaName, logger); err != nil {
			db.Close()
			return err
		}
		if err := dropTables(ctx, db, schemaName, logger); err != nil {
			db.Close()
			return err
		}
		if err := dropTypes(ctx, db, schemaName, logger); err != nil {
			db.Close()
			return err
		}
		db.Close()
	}

	return nil
}

func truncateAllTables(ctx context.Context, cfg config.PostgresConfig, skipTables []string, databaseName string, logger Logger) error {
	skipper := common.NewTableSkipper(skipTables)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationTruncateAllTables)
	if err != nil {
		return err
	}

	for _, database := range databases {
		schemaName := database.SchemaOrDefault()
		logger.Printf("Postgres: truncating all tables in %s", describePlatformDatabase(cfg.Name, database.Name, schemaName))

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
		}

		if err := truncateTables(ctx, db, schemaName, skipper, logger); err != nil {
			db.Close()
			return err
		}

		db.Close()
	}

	return nil
}

func truncateTables(ctx context.Context, db *sql.DB, schema string, skipper *common.TableSkipper, logger Logger) error {
	tables, err := listTables(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if skipper.ShouldSkip(schema, table) {
			logger.Printf("Postgres: skipping truncation of table %s.%s", schema, table)
			continue
		}

		query := fmt.Sprintf("TRUNCATE TABLE %s.%s", quoteIdentifier(schema), quoteIdentifier(table))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("truncating table %s.%s: %w", schema, table, err)
		}
		logger.Printf("Postgres: truncated table %s.%s", schema, table)
	}

	return nil
}

func dropTables(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	tables, err := listTables(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s.%s CASCADE", quoteIdentifier(schema), quoteIdentifier(table))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping table %s.%s: %w", schema, table, err)
		}
		logger.Printf("Postgres: dropped table %s.%s", schema, table)
	}

	return nil
}

func dropViews(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	views, err := listViews(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, view := range views {
		query := fmt.Sprintf("DROP VIEW IF EXISTS %s.%s CASCADE", quoteIdentifier(schema), quoteIdentifier(view))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping view %s.%s: %w", schema, view, err)
		}
		logger.Printf("Postgres: dropped view %s.%s", schema, view)
	}

	return nil
}

func dropIndexes(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	indexes, err := listIndexes(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, index := range indexes {
		query := fmt.Sprintf("DROP INDEX IF EXISTS %s.%s", quoteIdentifier(schema), quoteIdentifier(index))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping index %s.%s: %w", schema, index, err)
		}
		logger.Printf("Postgres: dropped index %s.%s", schema, index)
	}

	return nil
}

func dropTypes(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	types, err := listTypes(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, typ := range types {
		query := fmt.Sprintf("DROP TYPE IF EXISTS %s.%s CASCADE", quoteIdentifier(schema), quoteIdentifier(typ))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping type %s.%s: %w", schema, typ, err)
		}
		logger.Printf("Postgres: dropped type %s.%s", schema, typ)
	}

	return nil
}

func listTables(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE'`, schema)
	if err != nil {
		return nil, fmt.Errorf("listing tables for schema %s: %w", schema, err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listing tables for schema %s: %w", schema, err)
		}
		tables = append(tables, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing tables for schema %s: %w", schema, err)
	}

	return tables, nil
}

func listViews(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.views WHERE table_schema = $1`, schema)
	if err != nil {
		return nil, fmt.Errorf("listing views for schema %s: %w", schema, err)
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listing views for schema %s: %w", schema, err)
		}
		views = append(views, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing views for schema %s: %w", schema, err)
	}

	return views, nil
}

func listIndexes(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT indexname FROM pg_indexes WHERE schemaname = $1`, schema)
	if err != nil {
		return nil, fmt.Errorf("listing indexes for schema %s: %w", schema, err)
	}
	defer rows.Close()

	var indexes []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listing indexes for schema %s: %w", schema, err)
		}
		indexes = append(indexes, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing indexes for schema %s: %w", schema, err)
	}

	return indexes, nil
}

func listTypes(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = $1 AND t.typtype IN ('c', 'e', 'd')`, schema)
	if err != nil {
		return nil, fmt.Errorf("listing types for schema %s: %w", schema, err)
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listing types for schema %s: %w", schema, err)
		}
		types = append(types, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing types for schema %s: %w", schema, err)
	}

	return types, nil
}

func openConnection(cfg config.PostgresConfig, database config.PostgresDatabase) (*sql.DB, error) {
	schemaName := database.SchemaOrDefault()
	sslMode := strings.TrimSpace(cfg.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(database.Username, database.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + database.Name,
		RawQuery: url.Values{
			"sslmode": []string{sslMode},
		}.Encode(),
	}

	dsn := u.String()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database %s schema %s: %w", database.Name, schemaName, err)
	}

	return db, nil
}

func quoteIdentifier(id string) string {
	escaped := strings.ReplaceAll(id, "\"", "\"\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

func normalizeOperation(op string) string {
	op = strings.ToUpper(strings.TrimSpace(op))
	op = strings.ReplaceAll(op, " ", "_")
	return op
}

func listAllTables(ctx context.Context, cfg config.PostgresConfig, databaseName string, logger Logger) (map[string][]string, error) {
	result := make(map[string][]string)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationListAllTables)
	if err != nil {
		return nil, err
	}

	for _, database := range databases {
		schemaName := database.SchemaOrDefault()
		logger.Printf("Postgres: listing tables in %s", describePlatformDatabase(cfg.Name, database.Name, schemaName))

		db, err := openConnection(cfg, database)
		if err != nil {
			return nil, fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
		}

		tables, err := listTables(ctx, db, schemaName)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, schemaName, "table", "tables", tables)
		result[database.Name] = append([]string(nil), tables...)

		db.Close()
	}

	return result, nil
}

func listAllObjects(ctx context.Context, cfg config.PostgresConfig, databaseName string, logger Logger) (map[string]map[string][]string, error) {
	result := make(map[string]map[string][]string)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationListAllObjects)
	if err != nil {
		return nil, err
	}

	for _, database := range databases {
		schemaName := database.SchemaOrDefault()
		logger.Printf("Postgres: listing objects in %s", describePlatformDatabase(cfg.Name, database.Name, schemaName))

		db, err := openConnection(cfg, database)
		if err != nil {
			return nil, fmt.Errorf("connecting to Postgres database %s: %w", database.Name, err)
		}

		tables, err := listTables(ctx, db, schemaName)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, schemaName, "table", "tables", tables)

		views, err := listViews(ctx, db, schemaName)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, schemaName, "view", "views", views)

		indexes, err := listIndexes(ctx, db, schemaName)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, schemaName, "index", "indexes", indexes)

		types, err := listTypes(ctx, db, schemaName)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, schemaName, "type", "types", types)

		result[database.Name] = map[string][]string{
			"tables":  append([]string(nil), tables...),
			"views":   append([]string(nil), views...),
			"indexes": append([]string(nil), indexes...),
			"types":   append([]string(nil), types...),
		}

		db.Close()
	}

	return result, nil
}

func databasesForOperation(cfg config.PostgresConfig, databaseName, operation string) ([]config.PostgresDatabase, error) {
	trimmed := strings.TrimSpace(databaseName)
	if trimmed == "" {
		databases := make([]config.PostgresDatabase, 0, len(cfg.Databases))
		for _, database := range cfg.Databases {
			if database.Valid() {
				databases = append(databases, database)
			}
		}
		return databases, nil
	}

	database, found := findDatabase(cfg, trimmed)
	if !found {
		return nil, fmt.Errorf("postgres %s operation: database %q not found in platform configuration", common.DescribeOperation(operation), trimmed)
	}

	return []config.PostgresDatabase{database}, nil
}

func logObjects(logger Logger, platformName, database, schema, singular, plural string, names []string) {
	scope := describePlatformDatabase(platformName, database, schema)
	if len(names) == 0 {
		logger.Printf("Postgres: no %s found in %s", plural, scope)
		return
	}

	sort.Strings(names)
	for _, name := range names {
		logger.Printf("Postgres: %s %s.%s", singular, schema, name)
	}
}

func findDatabase(cfg config.PostgresConfig, name string) (config.PostgresDatabase, bool) {
	normalizedTarget := common.NormalizeTableName(name)
	if normalizedTarget == "" {
		return config.PostgresDatabase{}, false
	}

	for _, database := range cfg.Databases {
		if !database.Valid() {
			continue
		}
		if common.NormalizeTableName(database.Name) == normalizedTarget {
			return database, true
		}
	}

	return config.PostgresDatabase{}, false
}
