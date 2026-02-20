package base64

import (
	"context"
	"encoding/json"
	"fmt"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

type Action struct{}

func init() {
	registry.Register(Action{})
}

func (Action) Name() string {
	return ActionName
}

func (Action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var spec Payload
	if err := json.Unmarshal(payload, &spec); err != nil {
		return registry.Result{}, fmt.Errorf("base64: decode payload: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return registry.Result{}, err
	}

	result, err := Execute(ctx, spec, execCtx)
	if err != nil {
		return registry.Result{}, err
	}

	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}
