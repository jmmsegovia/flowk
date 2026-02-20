package helm

import (
	"context"
	"encoding/json"
	"fmt"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, _ *registry.ExecutionContext) (registry.Result, error) {
	var spec Payload
	if err := json.Unmarshal(payload, &spec); err != nil {
		return registry.Result{}, fmt.Errorf("helm: decode payload: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return registry.Result{}, err
	}

	result, err := Execute(ctx, spec)
	if err != nil {
		return registry.Result{}, err
	}

	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}
