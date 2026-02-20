package cassandra

import (
	"strings"
	"testing"

	"flowk/internal/config"
)

func TestTableSkipperShouldSkip(t *testing.T) {
	skipper := newTableSkipper([]string{"ks_one.table_one", " dispatcher_europe-west1.audit_history ", ""})

	testCases := []struct {
		name     string
		keyspace string
		table    string
		want     bool
	}{
		{name: "skip by qualified name", keyspace: "ks_one", table: "table_one", want: true},
		{name: "skip by qualified name with hyphen", keyspace: "dispatcher_europe-west1", table: "audit_history", want: true},
		{name: "different keyspace", keyspace: "ks_two", table: "table_one", want: false},
		{name: "case insensitive", keyspace: "KS_ONE", table: "TABLE_ONE", want: true},
		{name: "not skipped", keyspace: "ks_one", table: "table_three", want: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := skipper.shouldSkip(tc.keyspace, tc.table); got != tc.want {
				t.Fatalf("shouldSkip(%q, %q) = %v, want %v", tc.keyspace, tc.table, got, tc.want)
			}
		})
	}
}

func TestTableSkipperIgnoresInvalidTables(t *testing.T) {
	skipper := newTableSkipper([]string{"table_one", "ks_one", ""})

	if skipper != nil {
		t.Fatal("expected skipper to be nil when no valid table names are provided")
	}
}

func TestTableSkipperNil(t *testing.T) {
	var skipper *tableSkipper

	if skipper.shouldSkip("ks", "table") {
		t.Fatal("expected nil skipper to never skip tables")
	}
}

func TestKeyspacesForOperationAllKeyspaces(t *testing.T) {
	cfg := config.CassandraConfig{
		Keyspaces: []config.Keyspace{
			{Name: "ks_one", Username: "user1", Password: "pass1"},
			{Name: "ks_two", Username: "user2", Password: "pass2"},
			{Name: "invalid", Username: "", Password: ""},
		},
	}

	keyspaces, err := keyspacesForOperation(cfg, "", operationDropAllObjects)
	if err != nil {
		t.Fatalf("keyspacesForOperation returned error: %v", err)
	}

	if len(keyspaces) != 2 {
		t.Fatalf("expected 2 keyspaces, got %d", len(keyspaces))
	}

	names := map[string]struct{}{}
	for _, ks := range keyspaces {
		names[ks.Name] = struct{}{}
	}

	if _, ok := names["ks_one"]; !ok {
		t.Fatalf("expected ks_one to be included: %#v", names)
	}
	if _, ok := names["ks_two"]; !ok {
		t.Fatalf("expected ks_two to be included: %#v", names)
	}
	if _, ok := names["invalid"]; ok {
		t.Fatalf("did not expect invalid keyspace to be included: %#v", names)
	}
}

func TestKeyspacesForOperationSpecificKeyspace(t *testing.T) {
	cfg := config.CassandraConfig{
		Keyspaces: []config.Keyspace{
			{Name: "ks_one", Username: "user1", Password: "pass1"},
			{Name: "ks_two", Username: "user2", Password: "pass2"},
		},
	}

	keyspaces, err := keyspacesForOperation(cfg, "KS_TWO", operationTruncateAllTables)
	if err != nil {
		t.Fatalf("keyspacesForOperation returned error: %v", err)
	}

	if len(keyspaces) != 1 {
		t.Fatalf("expected 1 keyspace, got %d", len(keyspaces))
	}

	if got := keyspaces[0].Name; got != "ks_two" {
		t.Fatalf("expected keyspace name ks_two, got %q", got)
	}
}

func TestKeyspacesForOperationMissingKeyspace(t *testing.T) {
	cfg := config.CassandraConfig{
		Keyspaces: []config.Keyspace{
			{Name: "ks_one", Username: "user1", Password: "pass1"},
		},
	}

	_, err := keyspacesForOperation(cfg, "missing", operationListAllTables)
	if err == nil {
		t.Fatal("expected error when keyspace is not found")
	}

	if !strings.Contains(err.Error(), "list all tables") {
		t.Fatalf("expected error message to reference operation, got: %v", err)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error message to reference missing keyspace, got: %v", err)
	}
}
