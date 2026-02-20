package common

import (
	"database/sql"
	"fmt"
	"strings"
)

const (
	OperationDropAllObjects    = "DROP_ALL_OBJECTS"
	OperationTruncateAllTables = "TRUNCATE_ALL_TABLES"
	OperationListAllTables     = "LIST_ALL_TABLES"
	OperationListAllObjects    = "LIST_ALL_OBJECTS"
	OperationCheckConnection   = "CHECK_CONNECTION"
	OperationSQL               = "SQL"
	OperationLoadCSV           = "LOAD_CSV"
)

type TableSkipper struct {
	entries map[string]struct{}
}

func NewTableSkipper(tables []string) *TableSkipper {
	if len(tables) == 0 {
		return nil
	}

	entries := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		schema, name := NormalizeQualifiedTableName(table)
		if schema == "" || name == "" {
			continue
		}
		entries[fmt.Sprintf("%s.%s", schema, name)] = struct{}{}
	}

	if len(entries) == 0 {
		return nil
	}

	return &TableSkipper{entries: entries}
}

func (s *TableSkipper) ShouldSkip(schema, table string) bool {
	if s == nil || len(s.entries) == 0 {
		return false
	}

	normalizedSchema := NormalizeTableName(schema)
	normalizedTable := NormalizeTableName(table)
	if normalizedSchema == "" || normalizedTable == "" {
		return false
	}

	_, exists := s.entries[fmt.Sprintf("%s.%s", normalizedSchema, normalizedTable)]
	return exists
}

func NormalizeTableName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func NormalizeQualifiedTableName(name string) (string, string) {
	normalized := NormalizeTableName(name)
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

func NormalizeOperation(op string) string {
	op = strings.ToUpper(strings.TrimSpace(op))
	op = strings.ReplaceAll(op, " ", "_")
	return op
}

func DescribeOperation(operation string) string {
	normalized := strings.TrimSpace(operation)
	if normalized == "" {
		return "operation"
	}

	normalized = strings.ToLower(normalized)
	normalized = strings.ReplaceAll(normalized, "_", " ")
	return normalized
}

func ScanRow(columns []string, rows *sql.Rows) (map[string]any, error) {
	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	row := make(map[string]any, len(columns))
	for i, name := range columns {
		value := values[i]
		switch typed := value.(type) {
		case []byte:
			row[name] = string(typed)
		default:
			row[name] = typed
		}
	}

	return row, nil
}
