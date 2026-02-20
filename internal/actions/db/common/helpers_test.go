package common

import (
	"os"
	"testing"
)

func TestNormalizeQualifiedTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		schema string
		table  string
	}{
		{name: "valid and trimmed", input: "  Public.Users ", schema: "public", table: "users"},
		{name: "missing separator", input: "users"},
		{name: "too many parts", input: "a.b.c"},
		{name: "empty schema", input: ".users"},
		{name: "empty table", input: "public."},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema, table := NormalizeQualifiedTableName(tt.input)
			if schema != tt.schema || table != tt.table {
				t.Fatalf("NormalizeQualifiedTableName(%q) = (%q,%q), want (%q,%q)", tt.input, schema, table, tt.schema, tt.table)
			}
		})
	}
}

func TestTableSkipperShouldSkip(t *testing.T) {
	t.Parallel()

	skipper := NewTableSkipper([]string{"Public.Users", "analytics.events", "invalid"})
	if skipper == nil {
		t.Fatal("NewTableSkipper returned nil for valid entries")
	}

	tests := []struct {
		name   string
		schema string
		table  string
		want   bool
	}{
		{name: "matched entry", schema: " public ", table: " USERS ", want: true},
		{name: "matched second entry", schema: "analytics", table: "events", want: true},
		{name: "different table", schema: "public", table: "orders", want: false},
		{name: "invalid names", schema: "", table: "users", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := skipper.ShouldSkip(tt.schema, tt.table); got != tt.want {
				t.Fatalf("ShouldSkip(%q,%q) = %v, want %v", tt.schema, tt.table, got, tt.want)
			}
		})
	}

	if got := (*TableSkipper)(nil).ShouldSkip("public", "users"); got {
		t.Fatalf("nil skipper ShouldSkip returned true, want false")
	}
}

func TestOperationFormattingHelpers(t *testing.T) {
	t.Parallel()

	if got := NormalizeOperation(" list all tables "); got != "LIST_ALL_TABLES" {
		t.Fatalf("NormalizeOperation returned %q", got)
	}

	if got := DescribeOperation(" LIST_ALL_TABLES "); got != "list all tables" {
		t.Fatalf("DescribeOperation returned %q", got)
	}

	if got := DescribeOperation("   "); got != "operation" {
		t.Fatalf("DescribeOperation empty returned %q", got)
	}
}

func TestLoadCSVFile(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp(t.TempDir(), "*.csv")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	content := "id,name\n1,Alice\n2,Bob\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := LoadCSVFile(CSVLoadOptions{FilePath: tmpFile.Name(), HeaderInFirstRow: true})
	if err != nil {
		t.Fatalf("LoadCSVFile: %v", err)
	}
	if len(data.Columns) != 2 || data.Columns[0] != "id" || data.Columns[1] != "name" {
		t.Fatalf("unexpected columns: %#v", data.Columns)
	}
	if len(data.Rows) != 2 {
		t.Fatalf("unexpected rows length: %d", len(data.Rows))
	}
}
