package variables

import (
	"context"
	"encoding/json"
	"fmt"

	"flowk/internal/actions/registry"
	"flowk/internal/shared/runcontext"
)

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var cfg Payload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("decoding variables task payload: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return registry.Result{}, err
	}
	if runcontext.IsResume(ctx) {
		cfg.Overwrite = true
	}

	existing := cloneVariables(execCtx)
	value, resultType, err := Execute(cfg, existing, execCtx.Tasks)
	if err != nil {
		return registry.Result{}, err
	}

	if execCtx.Variables == nil {
		execCtx.Variables = make(map[string]registry.Variable, len(existing))
	} else {
		for name := range execCtx.Variables {
			delete(execCtx.Variables, name)
		}
	}
	for name, variable := range existing {
		execCtx.Variables[name] = registry.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	return registry.Result{Value: value, Type: resultType}, nil
}

func cloneVariables(execCtx *registry.ExecutionContext) map[string]Variable {
	if execCtx == nil || execCtx.Variables == nil {
		return make(map[string]Variable)
	}

	vars := make(map[string]Variable, len(execCtx.Variables))
	for name, variable := range execCtx.Variables {
		vars[name] = Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}
	return vars
}
