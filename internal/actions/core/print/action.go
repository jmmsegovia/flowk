package print

import (
	"context"
	"encoding/json"
	"fmt"

	"flowk/internal/actions/core/variables"
	"flowk/internal/actions/registry"
)

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(_ context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var cfg Payload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("decoding print task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return registry.Result{}, err
	}

	vars := make(map[string]variables.Variable, len(execCtx.Variables))
	for name, variable := range execCtx.Variables {
		vars[name] = variables.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	value, resultType, err := Execute(cfg, vars, execCtx.Tasks, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}
	return registry.Result{Value: value, Type: resultType}, nil
}
