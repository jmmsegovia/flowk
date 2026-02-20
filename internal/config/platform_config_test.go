package config

import (
	"encoding/json"
	"testing"
)

func TestParsePlatformConfigFromObject(t *testing.T) {
	raw := json.RawMessage(`{"cassandra":{"cluster":"host1,host2","port":"9042","keyspaces":[{"name":"ks1","type":"a","user":"user1","password":"pass1"}]}}`)

	cfg, err := ParsePlatformConfig(raw)
	if err != nil {
		t.Fatalf("ParsePlatformConfig() error = %v", err)
	}

	if cfg.Name != "" {
		t.Fatalf("Name = %q, want empty", cfg.Name)
	}

	if cfg.Port != 9042 {
		t.Fatalf("Port = %d, want 9042", cfg.Port)
	}
	if len(cfg.Hosts) != 2 || cfg.Hosts[0] != "host1" || cfg.Hosts[1] != "host2" {
		t.Fatalf("Hosts = %v, want [host1 host2]", cfg.Hosts)
	}
	if len(cfg.Keyspaces) != 1 {
		t.Fatalf("Keyspaces length = %d, want 1", len(cfg.Keyspaces))
	}
}

func TestParsePlatformConfigFromString(t *testing.T) {
	raw := json.RawMessage(`"{\"cassandra\":{\"cluster\":\"host\",\"port\":\"9042\",\"keyspaces\":[{\"name\":\"ks\",\"type\":\"t\",\"user\":\"u\",\"password\":\"p\"}]}}"`)

	cfg, err := ParsePlatformConfig(raw)
	if err != nil {
		t.Fatalf("ParsePlatformConfig() error = %v", err)
	}

	if len(cfg.Hosts) != 1 || cfg.Hosts[0] != "host" {
		t.Fatalf("Hosts = %v, want [host]", cfg.Hosts)
	}
}

func TestParsePlatformConfigExtractsName(t *testing.T) {
	raw := json.RawMessage(`{"name":"platform_all_us-east4","values":{"cassandra":{"cluster":"host","port":"9042","keyspaces":[{"name":"ks","user":"u","password":"p"}]}}}`)

	cfg, err := ParsePlatformConfig(raw)
	if err != nil {
		t.Fatalf("ParsePlatformConfig() error = %v", err)
	}

	if cfg.Name != "platform_all_us-east4" {
		t.Fatalf("Name = %q, want platform_all_us-east4", cfg.Name)
	}
	if len(cfg.Keyspaces) != 1 {
		t.Fatalf("Keyspaces length = %d, want 1", len(cfg.Keyspaces))
	}
}

func TestParsePlatformConfigExtractsNameFromValuesBlock(t *testing.T) {
	raw := json.RawMessage(`{"values":{"name":"platform_all_us-east4","cassandra":{"cluster":"host","port":"9042","keyspaces":[{"name":"ks","user":"u","password":"p"}]}}}`)

	cfg, err := ParsePlatformConfig(raw)
	if err != nil {
		t.Fatalf("ParsePlatformConfig() error = %v", err)
	}

	if cfg.Name != "platform_all_us-east4" {
		t.Fatalf("Name = %q, want platform_all_us-east4", cfg.Name)
	}
	if len(cfg.Keyspaces) != 1 {
		t.Fatalf("Keyspaces length = %d, want 1", len(cfg.Keyspaces))
	}
}

func TestParsePlatformConfigValidation(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"cassandra":{"cluster":"","port":"9042","keyspaces":[{"name":"ks","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"cassandra":{"cluster":"host","port":"","keyspaces":[{"name":"ks","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"cassandra":{"cluster":"host","port":"9042","keyspaces":[{"name":"","user":"u","password":"p"}]}}`),
	}

	for i, raw := range tests {
		if _, err := ParsePlatformConfig(raw); err == nil {
			t.Fatalf("ParsePlatformConfig() test %d error = nil, want error", i)
		}
	}
}

func TestParsePostgresConfigFromObject(t *testing.T) {
	raw := json.RawMessage(`{"postgres":{"host":"db-host","port":"5432","sslmode":"require","databases":[{"name":"app","user":"user1","password":"pass1","schema":"public"}]}}`)

	cfg, err := ParsePostgresConfig(raw)
	if err != nil {
		t.Fatalf("ParsePostgresConfig() error = %v", err)
	}

	if cfg.Name != "" {
		t.Fatalf("Name = %q, want empty", cfg.Name)
	}
	if cfg.Host != "db-host" {
		t.Fatalf("Host = %q, want db-host", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Fatalf("Port = %d, want 5432", cfg.Port)
	}
	if cfg.SSLMode != "require" {
		t.Fatalf("SSLMode = %q, want require", cfg.SSLMode)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParsePostgresConfigFromString(t *testing.T) {
	raw := json.RawMessage(`"{\"postgres\":{\"host\":\"db-host\",\"port\":\"5432\",\"databases\":[{\"name\":\"app\",\"user\":\"user1\",\"password\":\"pass1\"}]}}"`)

	cfg, err := ParsePostgresConfig(raw)
	if err != nil {
		t.Fatalf("ParsePostgresConfig() error = %v", err)
	}

	if cfg.Host != "db-host" {
		t.Fatalf("Host = %q, want db-host", cfg.Host)
	}
}

func TestParsePostgresConfigExtractsName(t *testing.T) {
	raw := json.RawMessage(`{"name":"platform_pg","values":{"postgres":{"host":"db-host","port":"5432","databases":[{"name":"app","user":"user1","password":"pass1"}]}}}`)

	cfg, err := ParsePostgresConfig(raw)
	if err != nil {
		t.Fatalf("ParsePostgresConfig() error = %v", err)
	}

	if cfg.Name != "platform_pg" {
		t.Fatalf("Name = %q, want platform_pg", cfg.Name)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParsePostgresConfigExtractsNameFromValuesBlock(t *testing.T) {
	raw := json.RawMessage(`{"values":{"name":"platform_pg","postgres":{"host":"db-host","port":"5432","databases":[{"name":"app","user":"user1","password":"pass1"}]}}}`)

	cfg, err := ParsePostgresConfig(raw)
	if err != nil {
		t.Fatalf("ParsePostgresConfig() error = %v", err)
	}

	if cfg.Name != "platform_pg" {
		t.Fatalf("Name = %q, want platform_pg", cfg.Name)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParsePostgresConfigValidation(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"postgres":{"host":"","port":"5432","databases":[{"name":"app","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"postgres":{"host":"db-host","port":"","databases":[{"name":"app","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"postgres":{"host":"db-host","port":"5432","databases":[{"name":"","user":"u","password":"p"}]}}`),
	}

	for i, raw := range tests {
		if _, err := ParsePostgresConfig(raw); err == nil {
			t.Fatalf("ParsePostgresConfig() test %d error = nil, want error", i)
		}
	}
}

func TestParseMySQLConfigFromObject(t *testing.T) {
	raw := json.RawMessage(`{"mysql":{"host":"db-host","port":"3306","databases":[{"name":"app","user":"user1","password":"pass1"}]}}`)

	cfg, err := ParseMySQLConfig(raw)
	if err != nil {
		t.Fatalf("ParseMySQLConfig() error = %v", err)
	}

	if cfg.Name != "" {
		t.Fatalf("Name = %q, want empty", cfg.Name)
	}
	if cfg.Host != "db-host" {
		t.Fatalf("Host = %q, want db-host", cfg.Host)
	}
	if cfg.Port != 3306 {
		t.Fatalf("Port = %d, want 3306", cfg.Port)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParseMySQLConfigFromString(t *testing.T) {
	raw := json.RawMessage(`"{\"mysql\":{\"host\":\"db-host\",\"port\":\"3306\",\"databases\":[{\"name\":\"app\",\"user\":\"user1\",\"password\":\"pass1\"}]}}"`)

	cfg, err := ParseMySQLConfig(raw)
	if err != nil {
		t.Fatalf("ParseMySQLConfig() error = %v", err)
	}

	if cfg.Host != "db-host" {
		t.Fatalf("Host = %q, want db-host", cfg.Host)
	}
}

func TestParseMySQLConfigExtractsName(t *testing.T) {
	raw := json.RawMessage(`{"name":"platform_mysql","values":{"mysql":{"host":"db-host","port":"3306","databases":[{"name":"app","user":"user1","password":"pass1"}]}}}`)

	cfg, err := ParseMySQLConfig(raw)
	if err != nil {
		t.Fatalf("ParseMySQLConfig() error = %v", err)
	}

	if cfg.Name != "platform_mysql" {
		t.Fatalf("Name = %q, want platform_mysql", cfg.Name)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParseMySQLConfigExtractsNameFromValuesBlock(t *testing.T) {
	raw := json.RawMessage(`{"values":{"name":"platform_mysql","mysql":{"host":"db-host","port":"3306","databases":[{"name":"app","user":"user1","password":"pass1"}]}}}`)

	cfg, err := ParseMySQLConfig(raw)
	if err != nil {
		t.Fatalf("ParseMySQLConfig() error = %v", err)
	}

	if cfg.Name != "platform_mysql" {
		t.Fatalf("Name = %q, want platform_mysql", cfg.Name)
	}
	if len(cfg.Databases) != 1 {
		t.Fatalf("Databases length = %d, want 1", len(cfg.Databases))
	}
}

func TestParseMySQLConfigValidation(t *testing.T) {
	tests := []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"mysql":{"host":"","port":"3306","databases":[{"name":"app","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"mysql":{"host":"db-host","port":"","databases":[{"name":"app","user":"u","password":"p"}]}}`),
		json.RawMessage(`{"mysql":{"host":"db-host","port":"3306","databases":[{"name":"","user":"u","password":"p"}]}}`),
	}

	for i, raw := range tests {
		if _, err := ParseMySQLConfig(raw); err == nil {
			t.Fatalf("ParseMySQLConfig() test %d error = nil, want error", i)
		}
	}
}
