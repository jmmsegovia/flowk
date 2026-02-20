package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
		return registry.Result{}, fmt.Errorf("shell: decode payload: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return registry.Result{}, err
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if spec.TimeoutSeconds > 0 {
		timeout := time.Duration(spec.TimeoutSeconds * float64(time.Second))
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := Execute(runCtx, spec, execCtx)
	if err != nil {
		return registry.Result{}, err
	}

	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}
