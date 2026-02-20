package cassandra

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"flowk/internal/actions/registry"
	"flowk/internal/config"
)

type taskConfig struct {
	RawPlatform json.RawMessage `json:"platform"`
	Operation   string          `json:"operation"`
	SkipTables  []string        `json:"skip_tables"`
	Keyspace    string          `json:"keyspace"`
	Command     string          `json:"command"`
	Table       string          `json:"table"`
	FilePath    string          `json:"file_path"`
	Columns     []string        `json:"columns"`
	Delimiter   string          `json:"delimiter"`
	HasHeader   *bool           `json:"has_header"`
}

func (c *taskConfig) Validate() error {
	if len(strings.TrimSpace(string(c.RawPlatform))) == 0 {
		return fmt.Errorf("cassandra task: platform is required")
	}
	if strings.TrimSpace(c.Operation) == "" {
		return fmt.Errorf("cassandra task: operation is required")
	}
	normalizedOperation := strings.ToUpper(strings.TrimSpace(c.Operation))
	if normalizedOperation == "CQL" {
		if strings.TrimSpace(c.Keyspace) == "" {
			return fmt.Errorf("cassandra task: keyspace is required for CQL operations")
		}
		if strings.TrimSpace(c.Command) == "" {
			return fmt.Errorf("cassandra task: command is required for CQL operations")
		}
	}
	if normalizedOperation == "LOAD_CSV" {
		if strings.TrimSpace(c.Keyspace) == "" {
			return fmt.Errorf("cassandra task: keyspace is required for LOAD_CSV operations")
		}
		if strings.TrimSpace(c.Table) == "" {
			return fmt.Errorf("cassandra task: table is required for LOAD_CSV operations")
		}
		if strings.TrimSpace(c.FilePath) == "" {
			return fmt.Errorf("cassandra task: file_path is required for LOAD_CSV operations")
		}
	}
	return nil
}

func decodeTask(data json.RawMessage) (taskConfig, error) {
	var cfg taskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decoding cassandra task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c taskConfig) CassandraConfig() (config.CassandraConfig, error) {
	cfg, err := config.ParsePlatformConfig(c.RawPlatform)
	if err != nil {
		return config.CassandraConfig{}, fmt.Errorf("cassandra task: %w", err)
	}
	return cfg, nil
}

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	cfg, err := decodeTask(payload)
	if err != nil {
		return registry.Result{}, err
	}

	platformCfg, err := cfg.CassandraConfig()
	if err != nil {
		return registry.Result{}, err
	}

	value, resultType, err := Execute(ctx, platformCfg, cfg.Operation, cfg.SkipTables, cfg.Keyspace, cfg.Command, cfg.Table, cfg.FilePath, cfg.Columns, cfg.Delimiter, cfg.HasHeader, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}

	return registry.Result{Value: value, Type: resultType}, nil
}
