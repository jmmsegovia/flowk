package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CassandraConfig contains the information required to connect to the Cassandra cluster
// associated with a platform.
type CassandraConfig struct {
	Name      string
	Hosts     []string
	Port      int
	Keyspaces []Keyspace
}

// Keyspace defines the credentials to connect to a Cassandra keyspace.
type Keyspace struct {
	Name     string
	Type     string
	Username string
	Password string
}

// PostgresConfig contains the information required to connect to a Postgres cluster
// associated with a platform.
type PostgresConfig struct {
	Name      string
	Host      string
	Port      int
	SSLMode   string
	Databases []PostgresDatabase
}

// MySQLConfig contains the information required to connect to a MySQL cluster
// associated with a platform.
type MySQLConfig struct {
	Name      string
	Host      string
	Port      int
	Databases []MySQLDatabase
}

// PostgresDatabase defines the credentials to connect to a Postgres database.
type PostgresDatabase struct {
	Name     string
	Username string
	Password string
	Schema   string
}

// MySQLDatabase defines the credentials to connect to a MySQL database.
type MySQLDatabase struct {
	Name     string
	Username string
	Password string
}

// Valid returns true when the keyspace configuration has all the required fields.
func (k Keyspace) Valid() bool {
	return strings.TrimSpace(k.Name) != "" && strings.TrimSpace(k.Username) != "" && strings.TrimSpace(k.Password) != ""
}

// Valid returns true when the database configuration has all the required fields.
func (d PostgresDatabase) Valid() bool {
	return strings.TrimSpace(d.Name) != "" && strings.TrimSpace(d.Username) != "" && strings.TrimSpace(d.Password) != ""
}

// Valid returns true when the database configuration has all the required fields.
func (d MySQLDatabase) Valid() bool {
	return strings.TrimSpace(d.Name) != "" && strings.TrimSpace(d.Username) != "" && strings.TrimSpace(d.Password) != ""
}

// SchemaOrDefault returns the configured schema name or "public" when empty.
func (d PostgresDatabase) SchemaOrDefault() string {
	if trimmed := strings.TrimSpace(d.Schema); trimmed != "" {
		return trimmed
	}
	return "public"
}

// ParsePlatformConfig builds a Cassandra configuration from a platform definition embedded in a flow variable.
func ParsePlatformConfig(raw json.RawMessage) (CassandraConfig, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return CassandraConfig{}, fmt.Errorf("platform configuration is required")
	}

	if trimmed[0] == '"' {
		var asString string
		if err := json.Unmarshal(trimmed, &asString); err != nil {
			return CassandraConfig{}, fmt.Errorf("decoding platform string: %w", err)
		}
		trimmed = bytes.TrimSpace([]byte(asString))
		if len(trimmed) == 0 {
			return CassandraConfig{}, fmt.Errorf("platform configuration is required")
		}
	}

	var container struct {
		Name      string       `json:"name"`
		Cassandra rawCassandra `json:"cassandra"`
		Values    struct {
			Name      string       `json:"name"`
			Cassandra rawCassandra `json:"cassandra"`
		} `json:"values"`
	}

	if err := json.Unmarshal(trimmed, &container); err != nil {
		return CassandraConfig{}, fmt.Errorf("parsing platform configuration: %w", err)
	}

	platformName := strings.TrimSpace(container.Name)
	if platformName == "" {
		platformName = strings.TrimSpace(container.Values.Name)
	}
	displayName := platformName
	if displayName == "" {
		displayName = "platform variable"
	}

	if !container.Cassandra.isEmpty() {
		return buildCassandraConfig(displayName, platformName, container.Cassandra)
	}

	if !container.Values.Cassandra.isEmpty() {
		return buildCassandraConfig(displayName, platformName, container.Values.Cassandra)
	}

	return CassandraConfig{}, fmt.Errorf("platform %s: Cassandra configuration is required", displayName)
}

// ParsePostgresConfig builds a Postgres configuration from a platform definition embedded in a flow variable.
func ParsePostgresConfig(raw json.RawMessage) (PostgresConfig, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return PostgresConfig{}, fmt.Errorf("platform configuration is required")
	}

	if trimmed[0] == '"' {
		var asString string
		if err := json.Unmarshal(trimmed, &asString); err != nil {
			return PostgresConfig{}, fmt.Errorf("decoding platform string: %w", err)
		}
		trimmed = bytes.TrimSpace([]byte(asString))
		if len(trimmed) == 0 {
			return PostgresConfig{}, fmt.Errorf("platform configuration is required")
		}
	}

	var container struct {
		Name     string      `json:"name"`
		Postgres rawPostgres `json:"postgres"`
		Values   struct {
			Name     string      `json:"name"`
			Postgres rawPostgres `json:"postgres"`
		} `json:"values"`
	}

	if err := json.Unmarshal(trimmed, &container); err != nil {
		return PostgresConfig{}, fmt.Errorf("parsing platform configuration: %w", err)
	}

	platformName := strings.TrimSpace(container.Name)
	if platformName == "" {
		platformName = strings.TrimSpace(container.Values.Name)
	}
	displayName := platformName
	if displayName == "" {
		displayName = "platform variable"
	}

	if !container.Postgres.isEmpty() {
		return buildPostgresConfig(displayName, platformName, container.Postgres)
	}

	if !container.Values.Postgres.isEmpty() {
		return buildPostgresConfig(displayName, platformName, container.Values.Postgres)
	}

	return PostgresConfig{}, fmt.Errorf("platform %s: Postgres configuration is required", displayName)
}

// ParseMySQLConfig builds a MySQL configuration from a platform definition embedded in a flow variable.
func ParseMySQLConfig(raw json.RawMessage) (MySQLConfig, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return MySQLConfig{}, fmt.Errorf("platform configuration is required")
	}

	if trimmed[0] == '"' {
		var asString string
		if err := json.Unmarshal(trimmed, &asString); err != nil {
			return MySQLConfig{}, fmt.Errorf("decoding platform string: %w", err)
		}
		trimmed = bytes.TrimSpace([]byte(asString))
		if len(trimmed) == 0 {
			return MySQLConfig{}, fmt.Errorf("platform configuration is required")
		}
	}

	var container struct {
		Name   string   `json:"name"`
		MySQL  rawMySQL `json:"mysql"`
		Values struct {
			Name  string   `json:"name"`
			MySQL rawMySQL `json:"mysql"`
		} `json:"values"`
	}

	if err := json.Unmarshal(trimmed, &container); err != nil {
		return MySQLConfig{}, fmt.Errorf("parsing platform configuration: %w", err)
	}

	platformName := strings.TrimSpace(container.Name)
	if platformName == "" {
		platformName = strings.TrimSpace(container.Values.Name)
	}
	displayName := platformName
	if displayName == "" {
		displayName = "platform variable"
	}

	if !container.MySQL.isEmpty() {
		return buildMySQLConfig(displayName, platformName, container.MySQL)
	}

	if !container.Values.MySQL.isEmpty() {
		return buildMySQLConfig(displayName, platformName, container.Values.MySQL)
	}

	return MySQLConfig{}, fmt.Errorf("platform %s: MySQL configuration is required", displayName)
}

func buildCassandraConfig(displayName, platformName string, values rawCassandra) (CassandraConfig, error) {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "platform variable"
	}

	port, err := strconv.Atoi(strings.TrimSpace(values.Port))
	if err != nil {
		return CassandraConfig{}, fmt.Errorf("platform %s: invalid Cassandra port: %w", name, err)
	}

	hosts := parseHosts(values.Cluster)
	if len(hosts) == 0 {
		return CassandraConfig{}, fmt.Errorf("platform %s: Cassandra cluster hosts are required", name)
	}

	keyspaces := make([]Keyspace, 0, len(values.Keyspaces))
	for _, rawKeyspace := range values.Keyspaces {
		if ks := buildKeyspace(rawKeyspace); ks.Valid() {
			keyspaces = append(keyspaces, ks)
		}
	}

	if len(keyspaces) == 0 {
		return CassandraConfig{}, fmt.Errorf("platform %s: no Cassandra keyspaces configured", name)
	}

	return CassandraConfig{
		Name:      strings.TrimSpace(platformName),
		Hosts:     hosts,
		Port:      port,
		Keyspaces: keyspaces,
	}, nil
}

func buildKeyspace(raw rawKeyspace) Keyspace {
	name := strings.TrimSpace(raw.Name)
	user := strings.TrimSpace(raw.User)
	password := strings.TrimSpace(raw.Password)
	keyspaceType := strings.TrimSpace(raw.Type)
	return Keyspace{Name: name, Type: keyspaceType, Username: user, Password: password}
}

func buildPostgresConfig(displayName, platformName string, values rawPostgres) (PostgresConfig, error) {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "platform variable"
	}

	port, err := strconv.Atoi(strings.TrimSpace(values.Port))
	if err != nil {
		return PostgresConfig{}, fmt.Errorf("platform %s: invalid Postgres port: %w", name, err)
	}

	host := strings.TrimSpace(values.Host)
	if host == "" {
		return PostgresConfig{}, fmt.Errorf("platform %s: Postgres host is required", name)
	}

	databases := make([]PostgresDatabase, 0, len(values.Databases))
	for _, rawDatabase := range values.Databases {
		if db := buildDatabase(rawDatabase); db.Valid() {
			databases = append(databases, db)
		}
	}

	if len(databases) == 0 {
		return PostgresConfig{}, fmt.Errorf("platform %s: no Postgres databases configured", name)
	}

	return PostgresConfig{
		Name:      strings.TrimSpace(platformName),
		Host:      host,
		Port:      port,
		SSLMode:   strings.TrimSpace(values.SSLMode),
		Databases: databases,
	}, nil
}

func buildMySQLConfig(displayName, platformName string, values rawMySQL) (MySQLConfig, error) {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "platform variable"
	}

	port, err := strconv.Atoi(strings.TrimSpace(values.Port))
	if err != nil {
		return MySQLConfig{}, fmt.Errorf("platform %s: invalid MySQL port: %w", name, err)
	}

	host := strings.TrimSpace(values.Host)
	if host == "" {
		return MySQLConfig{}, fmt.Errorf("platform %s: MySQL host is required", name)
	}

	databases := make([]MySQLDatabase, 0, len(values.Databases))
	for _, rawDatabase := range values.Databases {
		if db := buildMySQLDatabase(rawDatabase); db.Valid() {
			databases = append(databases, db)
		}
	}

	if len(databases) == 0 {
		return MySQLConfig{}, fmt.Errorf("platform %s: no MySQL databases configured", name)
	}

	return MySQLConfig{
		Name:      strings.TrimSpace(platformName),
		Host:      host,
		Port:      port,
		Databases: databases,
	}, nil
}

func parseHosts(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\t', '\n':
			return true
		default:
			return false
		}
	})

	hosts := make([]string, 0, len(fields))
	for _, f := range fields {
		if trimmed := strings.TrimSpace(f); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}

	return hosts
}

type rawCassandra struct {
	Cluster   string        `json:"cluster"`
	Port      string        `json:"port"`
	Keyspaces []rawKeyspace `json:"keyspaces"`
}

type rawPostgres struct {
	Host      string                `json:"host"`
	Port      string                `json:"port"`
	SSLMode   string                `json:"sslmode"`
	Databases []rawPostgresDatabase `json:"databases"`
}

type rawMySQL struct {
	Host      string             `json:"host"`
	Port      string             `json:"port"`
	Databases []rawMySQLDatabase `json:"databases"`
}

type rawKeyspace struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type rawPostgresDatabase struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
	Schema   string `json:"schema"`
}

type rawMySQLDatabase struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password"`
}

func (r rawCassandra) isEmpty() bool {
	if strings.TrimSpace(r.Cluster) != "" {
		return false
	}
	if strings.TrimSpace(r.Port) != "" {
		return false
	}
	return len(r.Keyspaces) == 0
}

func (r rawPostgres) isEmpty() bool {
	if strings.TrimSpace(r.Host) != "" {
		return false
	}
	if strings.TrimSpace(r.Port) != "" {
		return false
	}
	if strings.TrimSpace(r.SSLMode) != "" {
		return false
	}
	return len(r.Databases) == 0
}

func (r rawMySQL) isEmpty() bool {
	if strings.TrimSpace(r.Host) != "" {
		return false
	}
	if strings.TrimSpace(r.Port) != "" {
		return false
	}
	return len(r.Databases) == 0
}

func buildDatabase(raw rawPostgresDatabase) PostgresDatabase {
	name := strings.TrimSpace(raw.Name)
	user := strings.TrimSpace(raw.User)
	password := strings.TrimSpace(raw.Password)
	schema := strings.TrimSpace(raw.Schema)
	return PostgresDatabase{Name: name, Username: user, Password: password, Schema: schema}
}

func buildMySQLDatabase(raw rawMySQLDatabase) MySQLDatabase {
	name := strings.TrimSpace(raw.Name)
	user := strings.TrimSpace(raw.User)
	password := strings.TrimSpace(raw.Password)
	return MySQLDatabase{Name: name, Username: user, Password: password}
}
