package sleep

import (
	"context"
	"encoding/json"
	"fmt"

	"flowk/internal/actions/registry"
)

type taskConfig struct {
	Seconds float64 `json:"seconds"`
}

func (c *taskConfig) Validate() error {
	if c.Seconds <= 0 {
		return fmt.Errorf("sleep task: seconds must be greater than zero")
	}
	return nil
}

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var cfg taskConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("decoding sleep task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return registry.Result{}, err
	}

	value, resultType, err := Execute(ctx, cfg.Seconds, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}
	return registry.Result{Value: value, Type: resultType}, nil
}
