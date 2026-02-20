package cassandra

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gocql/gocql"

	"flowk/internal/actions/db/common"
	"flowk/internal/config"
	"flowk/internal/flow"
)

const (
	// ActionName identifies the Cassandra action in the flow file.
	ActionName = "DB_CASSANDRA_OPERATION"

	operationDropAllObjects    = "DROP_ALL_OBJECTS"
	operationTruncateAllTables = "TRUNCATE_ALL_TABLES"
	operationListAllTables     = "LIST_ALL_TABLES"
	operationListAllObjects    = "LIST_ALL_OBJECTS"
	operationCheckConnection   = "CHECK_CONNECTION"
	operationCQL               = "CQL"
	operationLoadCSV           = "LOAD_CSV"
)

type tableSkipper struct {
	entries map[string]struct{}
}

func newTableSkipper(tables []string) *tableSkipper {
	if len(tables) == 0 {
		return nil
	}

	entries := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		keyspace, name := normalizeQualifiedTableName(table)
		if keyspace == "" || name == "" {
			continue
		}
		entries[fmt.Sprintf("%s.%s", keyspace, name)] = struct{}{}
	}

	if len(entries) == 0 {
		return nil
	}

	return &tableSkipper{entries: entries}
}

func (s *tableSkipper) shouldSkip(keyspace, table string) bool {
	if s == nil || len(s.entries) == 0 {
		return false
	}

	normalizedKeyspace := normalizeTableName(keyspace)
	normalizedTable := normalizeTableName(table)
	if normalizedKeyspace == "" || normalizedTable == "" {
		return false
	}

	_, exists := s.entries[fmt.Sprintf("%s.%s", normalizedKeyspace, normalizedTable)]
	return exists
}

func normalizeTableName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeQualifiedTableName(name string) (string, string) {
	normalized := normalizeTableName(name)
	if normalized == "" {
		return "", ""
	}

	parts := strings.Split(normalized, ".")
	if len(parts) != 2 {
		return "", ""
	}

	if parts[0] == "" || parts[1] == "" {
		return "", ""
	}

	return parts[0], parts[1]
}

// Logger is a minimal interface implemented by the standard library logger.
type Logger interface {
	Printf(format string, v ...interface{})
}

func describePlatformKeyspace(platformName, keyspaceName string) string {
	trimmed := strings.TrimSpace(platformName)
	if trimmed == "" {
		return fmt.Sprintf("[No platform found] keyspace %s", keyspaceName)
	}
	return fmt.Sprintf("platform %s, keyspace %s", trimmed, keyspaceName)
}

// Execute performs the requested Cassandra operation for all keyspaces configured in the
// provided platform configuration and returns the outcome of the operation along with its
// data type. The returned value can be consumed by subsequent tasks.
func Execute(ctx context.Context, cfg config.CassandraConfig, op string, skipTables []string, keyspace, command, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
	normalized := normalizeOperation(op)
	switch normalized {
	case operationDropAllObjects:
		if err := dropAllObjects(ctx, cfg, keyspace, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case operationTruncateAllTables:
		if err := truncateAllTables(ctx, cfg, skipTables, keyspace, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case operationListAllTables:
		tablesByKeyspace, err := listAllTables(ctx, cfg, keyspace, logger)
		if err != nil {
			return nil, "", err
		}
		return tablesByKeyspace, flow.ResultTypeJSON, nil
	case operationListAllObjects:
		objectsByKeyspace, err := listAllObjects(ctx, cfg, keyspace, logger)
		if err != nil {
			return nil, "", err
		}
		return objectsByKeyspace, flow.ResultTypeJSON, nil
	case operationCheckConnection:
		if err := checkConnection(ctx, cfg, logger); err != nil {
			return nil, "", err
		}
		return true, flow.ResultTypeBool, nil
	case operationCQL:
		return executeCQL(ctx, cfg, keyspace, command, logger)
	case operationLoadCSV:
		return loadCSV(ctx, cfg, keyspace, tableName, filePath, columns, delimiter, hasHeader, logger)
	default:
		return nil, "", fmt.Errorf("unsupported Cassandra operation %q", op)
	}
}

func checkConnection(ctx context.Context, cfg config.CassandraConfig, logger Logger) error {
	for _, keyspace := range cfg.Keyspaces {
		if !keyspace.Valid() {
			continue
		}

		scope := describePlatformKeyspace(cfg.Name, keyspace.Name)
		logger.Printf("Cassandra: checking connection to %s host: %v", scope, cfg.Hosts)

		session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
		if err != nil {
			return fmt.Errorf("connecting to Cassandra keyspace %s: %w", keyspace.Name, err)
		}

		iter := session.Query("SELECT release_version FROM system.local").WithContext(ctx).Iter()
		if err := iter.Close(); err != nil {
			session.Close()
			return fmt.Errorf("checking connection to Cassandra keyspace %s: %w", keyspace.Name, err)
		}

		logger.Printf("Cassandra: connection to %s successful", scope)

		session.Close()
	}

	return nil
}

func executeCQL(ctx context.Context, cfg config.CassandraConfig, keyspaceName, command string, logger Logger) (any, flow.ResultType, error) {
	trimmedKeyspace := strings.TrimSpace(keyspaceName)
	if trimmedKeyspace == "" {
		return nil, "", fmt.Errorf("cassandra CQL operation: keyspace is required")
	}

	trimmedCommand := strings.TrimSpace(command)
	if trimmedCommand == "" {
		return nil, "", fmt.Errorf("cassandra CQL operation: command is required")
	}

	keyspace, found := findKeyspace(cfg, trimmedKeyspace)
	if !found {
		return nil, "", fmt.Errorf("cassandra CQL operation: keyspace %q not found in platform configuration", trimmedKeyspace)
	}

	session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to Cassandra keyspace %s: %w", keyspace.Name, err)
	}
	defer session.Close()

	logger.Printf("Cassandra: executing CQL command in %s", describePlatformKeyspace(cfg.Name, keyspace.Name))

	iter := session.Query(trimmedCommand).WithContext(ctx).Iter()
	columns := iter.Columns()

	var rows []map[string]any
	if len(columns) > 0 {
		rows = make([]map[string]any, 0)
		row := make(map[string]any, len(columns))
		for iter.MapScan(row) {
			rows = append(rows, copyRow(row))
			row = make(map[string]any, len(columns))
		}
	}

	if err := iter.Close(); err != nil {
		return nil, "", fmt.Errorf("executing CQL command in keyspace %s: %w", keyspace.Name, err)
	}

	if len(columns) > 0 {
		if rows == nil {
			rows = make([]map[string]any, 0)
		}
		logger.Printf("Cassandra: CQL command returned %d row(s)", len(rows))
		return rows, flow.ResultTypeJSON, nil
	}

	logger.Printf("Cassandra: CQL command executed successfully")
	return map[string]any{"success": true}, flow.ResultTypeJSON, nil
}

func loadCSV(ctx context.Context, cfg config.CassandraConfig, keyspaceName, tableName, filePath string, columns []string, delimiter string, hasHeader *bool, logger Logger) (any, flow.ResultType, error) {
	trimmedKeyspace := strings.TrimSpace(keyspaceName)
	if trimmedKeyspace == "" {
		return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: keyspace is required")
	}
	trimmedTable := strings.TrimSpace(tableName)
	if trimmedTable == "" {
		return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: table is required")
	}

	keyspace, found := findKeyspace(cfg, trimmedKeyspace)
	if !found {
		return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: keyspace %q not found in platform configuration", trimmedKeyspace)
	}

	includeHeader := true
	if hasHeader != nil {
		includeHeader = *hasHeader
	}
	csvData, err := common.LoadCSVFile(common.CSVLoadOptions{FilePath: filePath, Columns: columns, Delimiter: delimiter, HeaderInFirstRow: includeHeader})
	if err != nil {
		return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: %w", err)
	}

	session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
	if err != nil {
		return nil, "", fmt.Errorf("connecting to Cassandra keyspace %s: %w", keyspace.Name, err)
	}
	defer session.Close()

	logger.Printf("Cassandra: loading %d CSV row(s) into %s.%s", len(csvData.Rows), keyspace.Name, trimmedTable)

	query := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s)", quoteIdentifier(keyspace.Name), quoteIdentifier(trimmedTable), joinIdentifiers(csvData.Columns), strings.TrimRight(strings.Repeat("?,", len(csvData.Columns)), ","))
	batch := session.NewBatch(gocql.UnloggedBatch)
	loaded := 0
	for index, row := range csvData.Rows {
		values := make([]any, len(row))
		for i := range row {
			values[i] = row[i]
		}
		batch.Query(query, values...)
		if len(batch.Entries) >= 100 {
			if err := session.ExecuteBatch(batch.WithContext(ctx)); err != nil {
				return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: inserting row %d: %w", index+1, err)
			}
			loaded += len(batch.Entries)
			batch = session.NewBatch(gocql.UnloggedBatch)
		}
	}
	if len(batch.Entries) > 0 {
		if err := session.ExecuteBatch(batch.WithContext(ctx)); err != nil {
			return nil, "", fmt.Errorf("cassandra LOAD_CSV operation: inserting remaining rows: %w", err)
		}
		loaded += len(batch.Entries)
	}

	return map[string]any{"success": true, "rows_loaded": loaded, "keyspace": keyspace.Name, "table": trimmedTable}, flow.ResultTypeJSON, nil
}

func joinIdentifiers(identifiers []string) string {
	quoted := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		quoted = append(quoted, quoteIdentifier(identifier))
	}
	return strings.Join(quoted, ",")
}

func dropAllObjects(ctx context.Context, cfg config.CassandraConfig, keyspaceName string, logger Logger) error {
	keyspaces, err := keyspacesForOperation(cfg, keyspaceName, operationDropAllObjects)
	if err != nil {
		return err
	}

	for _, keyspace := range keyspaces {
		logger.Printf("Cassandra: dropping all objects in %s", describePlatformKeyspace(cfg.Name, keyspace.Name))

		session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
		if err != nil {
			return fmt.Errorf("connecting to Cassandra -> host: %s port: %d keyspace: %s: %w", cfg.Hosts, cfg.Port, keyspace.Name, err)
		}

		if err := dropMaterializedViews(ctx, session, keyspace.Name, logger); err != nil {
			session.Close()
			return err
		}
		if err := dropIndexes(ctx, session, keyspace.Name, logger); err != nil {
			session.Close()
			return err
		}
		if err := dropTables(ctx, session, keyspace.Name, logger); err != nil {
			session.Close()
			return err
		}
		if err := dropTypes(ctx, session, keyspace.Name, logger); err != nil {
			session.Close()
			return err
		}
		session.Close()
	}

	return nil
}

func truncateAllTables(ctx context.Context, cfg config.CassandraConfig, skipTables []string, keyspaceName string, logger Logger) error {
	skipper := newTableSkipper(skipTables)

	keyspaces, err := keyspacesForOperation(cfg, keyspaceName, operationTruncateAllTables)
	if err != nil {
		return err
	}

	for _, keyspace := range keyspaces {
		logger.Printf("Cassandra: truncating all tables in %s", describePlatformKeyspace(cfg.Name, keyspace.Name))

		session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
		if err != nil {
			return fmt.Errorf("connecting to Cassandra keyspace %s: %w", keyspace.Name, err)
		}

		if err := truncateTables(ctx, session, keyspace.Name, skipper, logger); err != nil {
			session.Close()
			return err
		}

		if err := truncateMaterializedViews(ctx, session, keyspace.Name, logger); err != nil {
			session.Close()
			return err
		}
		session.Close()
	}

	return nil
}

func truncateTables(ctx context.Context, session *gocql.Session, keyspace string, skipper *tableSkipper, logger Logger) error {
	tables, err := listTables(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if skipper.shouldSkip(keyspace, table) {
			logger.Printf("Cassandra: skipping truncation of table %s.%s", keyspace, table)
			continue
		}

		query := fmt.Sprintf("TRUNCATE %s.%s", quoteIdentifier(keyspace), quoteIdentifier(table))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("truncating table %s.%s: %w", keyspace, table, err)
		}
		logger.Printf("Cassandra: truncated table %s.%s", keyspace, table)
	}

	return nil
}

func truncateMaterializedViews(ctx context.Context, session *gocql.Session, keyspace string, logger Logger) error {
	views, err := listMaterializedViews(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, view := range views {
		query := fmt.Sprintf("TRUNCATE MATERIALIZED VIEW %s.%s", quoteIdentifier(keyspace), quoteIdentifier(view))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("truncating materialized view %s.%s: %w", keyspace, view, err)
		}
		logger.Printf("Cassandra: truncated materialized view %s.%s", keyspace, view)
	}

	return nil
}

func dropTables(ctx context.Context, session *gocql.Session, keyspace string, logger Logger) error {
	tables, err := listTables(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, table := range tables {
		query := fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", quoteIdentifier(keyspace), quoteIdentifier(table))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("dropping table %s.%s: %w", keyspace, table, err)
		}
		logger.Printf("Cassandra: dropped table %s.%s", keyspace, table)
	}

	return nil
}

func dropMaterializedViews(ctx context.Context, session *gocql.Session, keyspace string, logger Logger) error {
	views, err := listMaterializedViews(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, view := range views {
		query := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s.%s", quoteIdentifier(keyspace), quoteIdentifier(view))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("dropping materialized view %s.%s: %w", keyspace, view, err)
		}
		logger.Printf("Cassandra: dropped materialized view %s.%s", keyspace, view)
	}

	return nil
}

func dropIndexes(ctx context.Context, session *gocql.Session, keyspace string, logger Logger) error {
	indexes, err := listIndexes(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, index := range indexes {
		query := fmt.Sprintf("DROP INDEX IF EXISTS %s.%s", quoteIdentifier(keyspace), quoteIdentifier(index))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("dropping index %s.%s: %w", keyspace, index, err)
		}
		logger.Printf("Cassandra: dropped index %s.%s", keyspace, index)
	}

	return nil
}

func dropTypes(ctx context.Context, session *gocql.Session, keyspace string, logger Logger) error {
	types, err := listTypes(ctx, session, keyspace)
	if err != nil {
		return err
	}

	for _, typ := range types {
		query := fmt.Sprintf("DROP TYPE IF EXISTS %s.%s", quoteIdentifier(keyspace), quoteIdentifier(typ))
		if err := session.Query(query).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("dropping type %s.%s: %w", keyspace, typ, err)
		}
		logger.Printf("Cassandra: dropped type %s.%s", keyspace, typ)
	}

	return nil
}

func listTables(ctx context.Context, session *gocql.Session, keyspace string) ([]string, error) {
	iter := session.Query(`SELECT table_name FROM system_schema.tables WHERE keyspace_name = ?`, keyspace).
		WithContext(ctx).Iter()

	var tables []string
	var name string
	for iter.Scan(&name) {
		tables = append(tables, name)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("listing tables for keyspace %s: %w", keyspace, err)
	}

	return tables, nil
}

func listMaterializedViews(ctx context.Context, session *gocql.Session, keyspace string) ([]string, error) {
	views, err := fetchMaterializedViews(ctx, session, keyspace, "system_schema.materialized_views")
	if err == nil {
		return views, nil
	}

	if !isMissingSystemSchemaTable(err, "system_schema.materialized_views") {
		return nil, fmt.Errorf("listing materialized views for keyspace %s: %w", keyspace, err)
	}

	views, err = fetchMaterializedViews(ctx, session, keyspace, "system_schema.views")
	if err != nil {
		return nil, fmt.Errorf("listing materialized views for keyspace %s: %w", keyspace, err)
	}

	return views, nil
}

func fetchMaterializedViews(ctx context.Context, session *gocql.Session, keyspace, table string) ([]string, error) {
	query := fmt.Sprintf("SELECT view_name FROM %s WHERE keyspace_name = ?", table)
	iter := session.Query(query, keyspace).WithContext(ctx).Iter()

	var views []string
	var name string
	for iter.Scan(&name) {
		views = append(views, name)
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return views, nil
}

func listIndexes(ctx context.Context, session *gocql.Session, keyspace string) ([]string, error) {
	indexes, err := fetchIndexes(ctx, session, keyspace, "system_schema.indexes")
	if err == nil {
		return indexes, nil
	}

	if !isMissingSystemSchemaTable(err, "system_schema.indexes") {
		return nil, fmt.Errorf("listing indexes for keyspace %s: %w", keyspace, err)
	}

	query := "SELECT index_name FROM system.schema_columns WHERE keyspace_name = ? AND index_name != ''"
	iter := session.Query(query, keyspace).WithContext(ctx).Iter()

	indexes = nil
	var name string
	for iter.Scan(&name) {
		indexes = append(indexes, name)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("listing indexes for keyspace %s: %w", keyspace, err)
	}

	return indexes, nil
}

func fetchIndexes(ctx context.Context, session *gocql.Session, keyspace, table string) ([]string, error) {
	query := fmt.Sprintf("SELECT index_name FROM %s WHERE keyspace_name = ?", table)
	iter := session.Query(query, keyspace).WithContext(ctx).Iter()

	var indexes []string
	var name string
	for iter.Scan(&name) {
		indexes = append(indexes, name)
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return indexes, nil
}

func listTypes(ctx context.Context, session *gocql.Session, keyspace string) ([]string, error) {
	types, err := fetchTypes(ctx, session, keyspace, "system_schema.types")
	if err == nil {
		return types, nil
	}

	if !isMissingSystemSchemaTable(err, "system_schema.types") {
		return nil, fmt.Errorf("listing types for keyspace %s: %w", keyspace, err)
	}

	types, err = fetchTypes(ctx, session, keyspace, "system.schema_usertypes")
	if err != nil {
		return nil, fmt.Errorf("listing types for keyspace %s: %w", keyspace, err)
	}

	return types, nil
}

func fetchTypes(ctx context.Context, session *gocql.Session, keyspace, table string) ([]string, error) {
	query := fmt.Sprintf("SELECT type_name FROM %s WHERE keyspace_name = ?", table)
	iter := session.Query(query, keyspace).WithContext(ctx).Iter()

	var types []string
	var name string
	for iter.Scan(&name) {
		types = append(types, name)
	}

	if err := iter.Close(); err != nil {
		return nil, err
	}

	return types, nil
}

func isMissingSystemSchemaTable(err error, table string) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	table = strings.ToLower(table)

	if !(strings.Contains(message, "does not exist") || strings.Contains(message, "unconfigured table")) {
		return false
	}

	tablesToCheck := []string{table}
	if dot := strings.LastIndex(table, "."); dot >= 0 && dot < len(table)-1 {
		tablesToCheck = append(tablesToCheck, table[dot+1:])
	}

	for _, tbl := range tablesToCheck {
		if tbl != "" && strings.Contains(message, tbl) {
			return true
		}
	}

	return false
}

func openSession(hosts []string, port int, keyspace config.Keyspace) (*gocql.Session, error) {
	cluster := gocql.NewCluster(hosts...)
	cluster.Port = port
	cluster.Keyspace = keyspace.Name
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: keyspace.Username,
		Password: keyspace.Password,
	}
	cluster.ConnectTimeout = 10 * time.Second
	cluster.Timeout = 10 * time.Second
	cluster.Consistency = gocql.Quorum

	return cluster.CreateSession()
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

func listAllTables(ctx context.Context, cfg config.CassandraConfig, keyspaceName string, logger Logger) (map[string][]string, error) {
	result := make(map[string][]string)

	keyspaces, err := keyspacesForOperation(cfg, keyspaceName, operationListAllTables)
	if err != nil {
		return nil, err
	}

	for _, keyspace := range keyspaces {
		logger.Printf("Cassandra: listing tables in %s", describePlatformKeyspace(cfg.Name, keyspace.Name))

		session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
		if err != nil {
			return nil, fmt.Errorf("connecting to Cassandra -> host: %s port: %d keyspace: %s: %w", cfg.Hosts, cfg.Port, keyspace.Name, err)
		}

		tables, err := listTables(ctx, session, keyspace.Name)
		if err != nil {
			session.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, keyspace.Name, "table", "tables", tables)
		result[keyspace.Name] = append([]string(nil), tables...)

		session.Close()
	}

	return result, nil
}

func listAllObjects(ctx context.Context, cfg config.CassandraConfig, keyspaceName string, logger Logger) (map[string]map[string][]string, error) {
	result := make(map[string]map[string][]string)

	keyspaces, err := keyspacesForOperation(cfg, keyspaceName, operationListAllObjects)
	if err != nil {
		return nil, err
	}

	for _, keyspace := range keyspaces {
		logger.Printf("Cassandra: listing objects in %s", describePlatformKeyspace(cfg.Name, keyspace.Name))

		session, err := openSession(cfg.Hosts, cfg.Port, keyspace)
		if err != nil {
			return nil, fmt.Errorf("connecting to Cassandra -> host: %s port: %d keyspace: %s: %w", cfg.Hosts, cfg.Port, keyspace.Name, err)
		}

		tables, err := listTables(ctx, session, keyspace.Name)
		if err != nil {
			session.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, keyspace.Name, "table", "tables", tables)

		views, err := listMaterializedViews(ctx, session, keyspace.Name)
		if err != nil {
			session.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, keyspace.Name, "materialized view", "materialized views", views)

		indexes, err := listIndexes(ctx, session, keyspace.Name)
		if err != nil {
			session.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, keyspace.Name, "index", "indexes", indexes)

		types, err := listTypes(ctx, session, keyspace.Name)
		if err != nil {
			session.Close()
			return nil, err
		}

		logObjects(logger, cfg.Name, keyspace.Name, "type", "types", types)

		result[keyspace.Name] = map[string][]string{
			"tables":             append([]string(nil), tables...),
			"materialized_views": append([]string(nil), views...),
			"indexes":            append([]string(nil), indexes...),
			"types":              append([]string(nil), types...),
		}

		session.Close()
	}

	return result, nil
}

func keyspacesForOperation(cfg config.CassandraConfig, keyspaceName, operation string) ([]config.Keyspace, error) {
	trimmed := strings.TrimSpace(keyspaceName)
	if trimmed == "" {
		keyspaces := make([]config.Keyspace, 0, len(cfg.Keyspaces))
		for _, keyspace := range cfg.Keyspaces {
			if keyspace.Valid() {
				keyspaces = append(keyspaces, keyspace)
			}
		}
		return keyspaces, nil
	}

	keyspace, found := findKeyspace(cfg, trimmed)
	if !found {
		return nil, fmt.Errorf("cassandra %s operation: keyspace %q not found in platform configuration", describeOperation(operation), trimmed)
	}

	return []config.Keyspace{keyspace}, nil
}

func describeOperation(operation string) string {
	normalized := strings.TrimSpace(operation)
	if normalized == "" {
		return "operation"
	}

	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "_", " ")
	return normalized
}

func logObjects(logger Logger, platformName, keyspace, singular, plural string, names []string) {
	scope := describePlatformKeyspace(platformName, keyspace)
	if len(names) == 0 {
		logger.Printf("Cassandra: no %s found in %s", plural, scope)
		return
	}

	sort.Strings(names)
	for _, name := range names {
		logger.Printf("Cassandra: %s %s.%s", singular, keyspace, name)
	}
}

func findKeyspace(cfg config.CassandraConfig, name string) (config.Keyspace, bool) {
	normalizedTarget := normalizeTableName(name)
	if normalizedTarget == "" {
		return config.Keyspace{}, false
	}

	for _, keyspace := range cfg.Keyspaces {
		if !keyspace.Valid() {
			continue
		}
		if normalizeTableName(keyspace.Name) == normalizedTarget {
			return keyspace, true
		}
	}

	return config.Keyspace{}, false
}

func copyRow(row map[string]any) map[string]any {
	if len(row) == 0 {
		return map[string]any{}
	}

	clone := make(map[string]any, len(row))
	for key, value := range row {
		clone[key] = value
	}
	return clone
}
