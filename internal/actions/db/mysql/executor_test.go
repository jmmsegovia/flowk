package mysql

import (
	"context"
	"strings"
	"testing"

	"flowk/internal/config"
)

func TestDescribePlatformDatabase(t *testing.T) {
	t.Parallel()

	if got := describePlatformDatabase(" prod ", "main"); got != "platform prod, database main" {
		t.Fatalf("unexpected description: %q", got)
	}
	if got := describePlatformDatabase("", "main"); got != "[No platform found] database main" {
		t.Fatalf("unexpected fallback description: %q", got)
	}
}

func TestExecuteUnsupportedOperation(t *testing.T) {
	t.Parallel()

	_, _, err := Execute(context.Background(), config.MySQLConfig{}, "not-real", nil, "", "", "", "", nil, "", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported MySQL operation") {
		t.Fatalf("expected unsupported operation error, got %v", err)
	}
}

func TestDatabasesForOperation(t *testing.T) {
	t.Parallel()

	cfg := config.MySQLConfig{Databases: []config.MySQLDatabase{{Name: "db1", Username: "u", Password: "p"}, {Name: "db2", Username: "u", Password: "p"}}}

	dbs, err := databasesForOperation(cfg, "", "LIST")
	if err != nil || len(dbs) != 2 {
		t.Fatalf("expected all databases, got %v, err=%v", dbs, err)
	}

	dbs, err = databasesForOperation(cfg, "db2", "LIST")
	if err != nil || len(dbs) != 1 || dbs[0].Name != "db2" {
		t.Fatalf("expected selected database db2, got %v, err=%v", dbs, err)
	}

	_, err = databasesForOperation(cfg, "missing", "LIST")
	if err == nil || !strings.Contains(err.Error(), "database \"missing\" not found") {
		t.Fatalf("expected missing database error, got %v", err)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	t.Parallel()
	if got := quoteIdentifier(`with"quote`); got != "`with\"quote`" {
		t.Fatalf("quoteIdentifier returned %q", got)
	}
}
