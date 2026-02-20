package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"flowk/internal/actions/db/common"
	"flowk/internal/config"
	"flowk/internal/flow"
)

const (
	// ActionName identifies the MySQL action in the flow file.
	ActionName = "DB_MYSQL_OPERATION"
)

// Logger is a minimal interface implemented by the standard library logger.
type Logger interface {
	Printf(format string, v ...interface{})
}

func describePlatformDatabase(platformName, databaseName string) string {
	trimmed := strings.TrimSpace(platformName)
	if trimmed == "" {
		return fmt.Sprintf("[No platform found] database %s", databaseName)
	}
	return fmt.Sprintf("platform %s, database %s", trimmed, databaseName)
}

// Execute performs the requested MySQL operation for all databases configured in the
// provided platform configuration and returns the outcome of the operation along with its
// data type. The returned value can be consumed by subsequent tasks.
func Execute(ctx context.Context, cfg config.MySQLConfig, op string, skipTables []string, databaseName, command, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
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
		return nil, "", fmt.Errorf("unsupported MySQL operation %q", op)
	}
}

func checkConnection(ctx context.Context, cfg config.MySQLConfig, logger Logger) error {
	for _, database := range cfg.Databases {
		if !database.Valid() {
			continue
		}

		scope := describePlatformDatabase(cfg.Name, database.Name)
		logger.Printf("MySQL: checking connection to %s host: %s", scope, cfg.Host)

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
		}

		ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := db.PingContext(ctxWithTimeout); err != nil {
			db.Close()
			return fmt.Errorf("checking connection to MySQL database %s: %w", database.Name, err)
		}

		logger.Printf("MySQL: connection to %s successful", scope)
		db.Close()
	}

	return nil
}

func executeSQL(ctx context.Context, cfg config.MySQLConfig, databaseName, command string, logger Logger) (any, flow.ResultType, error) {
	trimmedDatabase := strings.TrimSpace(databaseName)
	if trimmedDatabase == "" {
		return nil, "", fmt.Errorf("mysql SQL operation: database is required")
	}

	trimmedCommand := strings.TrimSpace(command)
	if trimmedCommand == "" {
		return nil, "", fmt.Errorf("mysql SQL operation: command is required")
	}

	database, found := findDatabase(cfg, trimmedDatabase)
	if !found {
		return nil, "", fmt.Errorf("mysql SQL operation: database %q not found in platform configuration", trimmedDatabase)
	}

	db, err := openConnection(cfg, database)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
	}
	defer db.Close()

	logger.Printf("MySQL: executing SQL command in %s", describePlatformDatabase(cfg.Name, database.Name))

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
		logger.Printf("MySQL: SQL command returned %d row(s)", len(results))
		return results, flow.ResultTypeJSON, nil
	}

	logger.Printf("MySQL: SQL command executed successfully")
	return map[string]any{"success": true}, flow.ResultTypeJSON, nil
}

func loadCSV(ctx context.Context, cfg config.MySQLConfig, databaseName, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
	trimmedDatabase := strings.TrimSpace(databaseName)
	if trimmedDatabase == "" {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: database is required")
	}
	trimmedTable := strings.TrimSpace(tableName)
	if trimmedTable == "" {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: table is required")
	}

	database, found := findDatabase(cfg, trimmedDatabase)
	if !found {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: database %q not found in platform configuration", trimmedDatabase)
	}

	includeHeader := true
	if hasHeader != nil {
		includeHeader = *hasHeader
	}
	csvData, err := common.LoadCSVFile(common.CSVLoadOptions{FilePath: filePath, Columns: columns, Delimiter: delimiter, HeaderInFirstRow: includeHeader})
	if err != nil {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: %w", err)
	}

	db, err := openConnection(cfg, database)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
	}
	defer db.Close()

	logger.Printf("MySQL: loading %d CSV row(s) into %s.%s", len(csvData.Rows), database.Name, trimmedTable)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s)", quoteIdentifier(database.Name), quoteIdentifier(trimmedTable), joinQuotedIdentifiers(csvData.Columns), strings.TrimRight(strings.Repeat("?,", len(csvData.Columns)), ","))
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for i, row := range csvData.Rows {
		values := make([]any, len(row))
		for index := range row {
			values[index] = row[index]
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return nil, "", fmt.Errorf("mysql LOAD_CSV operation: inserting row %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("mysql LOAD_CSV operation: commit transaction: %w", err)
	}

	return map[string]any{"success": true, "rows_loaded": len(csvData.Rows), "database": database.Name, "table": trimmedTable}, flow.ResultTypeJSON, nil
}

func joinQuotedIdentifiers(identifiers []string) string {
	quoted := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		quoted = append(quoted, quoteIdentifier(identifier))
	}
	return strings.Join(quoted, ",")
}

func dropAllObjects(ctx context.Context, cfg config.MySQLConfig, databaseName string, logger Logger) error {
	databases, err := databasesForOperation(cfg, databaseName, common.OperationDropAllObjects)
	if err != nil {
		return err
	}

	for _, database := range databases {
		logger.Printf("MySQL: dropping all objects in %s", describePlatformDatabase(cfg.Name, database.Name))

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
		}

		if err := dropViews(ctx, db, database.Name, logger); err != nil {
			db.Close()
			return err
		}
		if err := dropTables(ctx, db, database.Name, logger); err != nil {
			db.Close()
			return err
		}
		db.Close()
	}

	return nil
}

func truncateAllTables(ctx context.Context, cfg config.MySQLConfig, skipTables []string, databaseName string, logger Logger) error {
	skipper := common.NewTableSkipper(skipTables)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationTruncateAllTables)
	if err != nil {
		return err
	}

	for _, database := range databases {
		logger.Printf("MySQL: truncating all tables in %s", describePlatformDatabase(cfg.Name, database.Name))

		db, err := openConnection(cfg, database)
		if err != nil {
			return fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
		}

		if err := truncateTables(ctx, db, database.Name, skipper, logger); err != nil {
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
			logger.Printf("MySQL: skipping truncation of table %s.%s", schema, table)
			continue
		}

		query := fmt.Sprintf("TRUNCATE TABLE %s.%s", quoteIdentifier(schema), quoteIdentifier(table))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("truncating table %s.%s: %w", schema, table, err)
		}
		logger.Printf("MySQL: truncated table %s.%s", schema, table)
	}

	return nil
}

func dropTables(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	tables, err := listTables(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", quoteIdentifier(schema), quoteIdentifier(table))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping table %s.%s: %w", schema, table, err)
		}
		logger.Printf("MySQL: dropped table %s.%s", schema, table)
	}

	return nil
}

func dropViews(ctx context.Context, db *sql.DB, schema string, logger Logger) error {
	views, err := listViews(ctx, db, schema)
	if err != nil {
		return err
	}

	for _, view := range views {
		query := fmt.Sprintf("DROP VIEW IF EXISTS %s.%s", quoteIdentifier(schema), quoteIdentifier(view))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("dropping view %s.%s: %w", schema, view, err)
		}
		logger.Printf("MySQL: dropped view %s.%s", schema, view)
	}

	return nil
}

func listTables(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE'`, schema)
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
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM information_schema.views WHERE table_schema = ?`, schema)
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
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT index_name FROM information_schema.statistics WHERE table_schema = ?`, schema)
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

func openConnection(cfg config.MySQLConfig, database config.MySQLDatabase) (*sql.DB, error) {
	driverCfg := mysqlDriver.NewConfig()
	driverCfg.User = database.Username
	driverCfg.Passwd = database.Password
	driverCfg.Net = "tcp"
	driverCfg.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	driverCfg.DBName = database.Name
	driverCfg.ParseTime = true

	db, err := sql.Open("mysql", driverCfg.FormatDSN())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database %s: %w", database.Name, err)
	}

	return db, nil
}

func quoteIdentifier(id string) string {
	escaped := strings.ReplaceAll(id, "`", "``")
	return fmt.Sprintf("`%s`", escaped)
}

func listAllTables(ctx context.Context, cfg config.MySQLConfig, databaseName string, logger Logger) (map[string][]string, error) {
	result := make(map[string][]string)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationListAllTables)
	if err != nil {
		return nil, err
	}

	for _, database := range databases {
		logger.Printf("MySQL: listing tables in %s", describePlatformDatabase(cfg.Name, database.Name))

		db, err := openConnection(cfg, database)
		if err != nil {
			return nil, fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
		}

		tables, err := listTables(ctx, db, database.Name)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, "table", "tables", tables)
		result[database.Name] = append([]string(nil), tables...)

		db.Close()
	}

	return result, nil
}

func listAllObjects(ctx context.Context, cfg config.MySQLConfig, databaseName string, logger Logger) (map[string]map[string][]string, error) {
	result := make(map[string]map[string][]string)

	databases, err := databasesForOperation(cfg, databaseName, common.OperationListAllObjects)
	if err != nil {
		return nil, err
	}

	for _, database := range databases {
		logger.Printf("MySQL: listing objects in %s", describePlatformDatabase(cfg.Name, database.Name))

		db, err := openConnection(cfg, database)
		if err != nil {
			return nil, fmt.Errorf("connecting to MySQL database %s: %w", database.Name, err)
		}

		tables, err := listTables(ctx, db, database.Name)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, "table", "tables", tables)

		views, err := listViews(ctx, db, database.Name)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, "view", "views", views)

		indexes, err := listIndexes(ctx, db, database.Name)
		if err != nil {
			db.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, database.Name, "index", "indexes", indexes)

		result[database.Name] = map[string][]string{
			"tables":  append([]string(nil), tables...),
			"views":   append([]string(nil), views...),
			"indexes": append([]string(nil), indexes...),
		}

		db.Close()
	}

	return result, nil
}

func databasesForOperation(cfg config.MySQLConfig, databaseName, operation string) ([]config.MySQLDatabase, error) {
	trimmed := strings.TrimSpace(databaseName)
	if trimmed == "" {
		databases := make([]config.MySQLDatabase, 0, len(cfg.Databases))
		for _, database := range cfg.Databases {
			if database.Valid() {
				databases = append(databases, database)
			}
		}
		return databases, nil
	}

	database, found := findDatabase(cfg, trimmed)
	if !found {
		return nil, fmt.Errorf("mysql %s operation: database %q not found in platform configuration", common.DescribeOperation(operation), trimmed)
	}

	return []config.MySQLDatabase{database}, nil
}

func logObjects(logger Logger, platformName, database, singular, plural string, names []string) {
	scope := describePlatformDatabase(platformName, database)
	if len(names) == 0 {
		logger.Printf("MySQL: no %s found in %s", plural, scope)
		return
	}

	sort.Strings(names)
	for _, name := range names {
		logger.Printf("MySQL: %s %s.%s", singular, database, name)
	}
}

func findDatabase(cfg config.MySQLConfig, name string) (config.MySQLDatabase, bool) {
	normalizedTarget := common.NormalizeTableName(name)
	if normalizedTarget == "" {
		return config.MySQLDatabase{}, false
	}

	for _, database := range cfg.Databases {
		if !database.Valid() {
			continue
		}
		if common.NormalizeTableName(database.Name) == normalizedTarget {
			return database, true
		}
	}

	return config.MySQLDatabase{}, false
}
