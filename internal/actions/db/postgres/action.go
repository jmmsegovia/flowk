package postgres

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
	Database    string          `json:"database"`
	Command     string          `json:"command"`
	Table       string          `json:"table"`
	FilePath    string          `json:"file_path"`
	Columns     []string        `json:"columns"`
	Delimiter   string          `json:"delimiter"`
	HasHeader   *bool           `json:"has_header"`
}

func (c *taskConfig) Validate() error {
	if len(strings.TrimSpace(string(c.RawPlatform))) == 0 {
		return fmt.Errorf("postgres task: platform is required")
	}
	if strings.TrimSpace(c.Operation) == "" {
		return fmt.Errorf("postgres task: operation is required")
	}
	normalizedOperation := strings.ToUpper(strings.TrimSpace(c.Operation))
	if normalizedOperation == "SQL" {
		if strings.TrimSpace(c.Database) == "" {
			return fmt.Errorf("postgres task: database is required for SQL operations")
		}
		if strings.TrimSpace(c.Command) == "" {
			return fmt.Errorf("postgres task: command is required for SQL operations")
		}
	}
	if normalizedOperation == "LOAD_CSV" {
		if strings.TrimSpace(c.Database) == "" {
			return fmt.Errorf("postgres task: database is required for LOAD_CSV operations")
		}
		if strings.TrimSpace(c.Table) == "" {
			return fmt.Errorf("postgres task: table is required for LOAD_CSV operations")
		}
		if strings.TrimSpace(c.FilePath) == "" {
			return fmt.Errorf("postgres task: file_path is required for LOAD_CSV operations")
		}
	}
	return nil
}

func decodeTask(data json.RawMessage) (taskConfig, error) {
	var cfg taskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decoding postgres task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c taskConfig) PostgresConfig() (config.PostgresConfig, error) {
	cfg, err := config.ParsePostgresConfig(c.RawPlatform)
	if err != nil {
		return config.PostgresConfig{}, fmt.Errorf("postgres task: %w", err)
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

	platformCfg, err := cfg.PostgresConfig()
	if err != nil {
		return registry.Result{}, err
	}

	value, resultType, err := Execute(ctx, platformCfg, cfg.Operation, cfg.SkipTables, cfg.Database, cfg.Command, cfg.Table, cfg.FilePath, cfg.Columns, cfg.Delimiter, cfg.HasHeader, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}

	return registry.Result{Value: value, Type: resultType}, nil
}
