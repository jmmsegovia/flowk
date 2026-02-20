package parallel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestActionExecuteSuccess(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"tasks": []map[string]any{
			{
				"id":          "alpha",
				"description": "first",
				"action":      "PRINT",
			},
			{
				"id":          "bravo",
				"description": "second",
				"action":      "PRINT",
			},
		},
		"merge_order":    []string{"bravo", "alpha"},
		"merge_strategy": "last_write_wins",
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	execCtx := &registry.ExecutionContext{
		Task: &flow.Task{ID: "parent", FlowID: "main"},
		Tasks: []flow.Task{
			{ID: "existing", FlowID: "main"},
		},
		Variables: map[string]registry.Variable{
			"shared": {
				Name:  "shared",
				Type:  "string",
				Value: "base",
			},
		},
		LogDir: t.TempDir(),
	}

	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		switch req.Task.ID {
		case "alpha":
			return registry.TaskExecutionResponse{
				Result: registry.Result{
					Value: map[string]any{"alpha": true},
					Type:  flow.ResultTypeJSON,
				},
				Variables: map[string]registry.Variable{
					"alpha": {
						Name:  "alpha",
						Type:  "string",
						Value: "value-from-alpha",
					},
					"shared": {
						Name:  "shared",
						Type:  "string",
						Value: "alpha-overwrite",
					},
				},
			}, nil
		case "bravo":
			return registry.TaskExecutionResponse{
				Result: registry.Result{
					Value: true,
					Type:  flow.ResultTypeBool,
				},
				Variables: map[string]registry.Variable{
					"bravo": {
						Name:  "bravo",
						Type:  "string",
						Value: "value-from-bravo",
					},
					"shared": {
						Name:  "shared",
						Type:  "string",
						Value: "bravo-overwrite",
					},
				},
			}, nil
		default:
			return registry.TaskExecutionResponse{}, fmt.Errorf("unexpected task %s", req.Task.ID)
		}
	}

	result, err := action{}.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute parallel action: %v", err)
	}

	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", result.Type)
	}

	aggregated, ok := result.Value.(map[string]map[string]any)
	if !ok {
		t.Fatalf("result value type %T, want map[string]map[string]any", result.Value)
	}

	alphaEntry, exists := aggregated["alpha"]
	if !exists {
		t.Fatalf("missing alpha entry in aggregated result")
	}
	if got := alphaEntry["type"]; got != string(flow.ResultTypeJSON) {
		t.Fatalf("alpha type mismatch: %v", got)
	}
	alphaResult, ok := alphaEntry["result"].(map[string]any)
	if !ok {
		t.Fatalf("alpha result type %T", alphaEntry["result"])
	}
	if got := alphaResult["alpha"]; got != true {
		t.Fatalf("alpha result mismatch: %v", got)
	}

	bravoEntry, exists := aggregated["bravo"]
	if !exists {
		t.Fatalf("missing bravo entry in aggregated result")
	}
	if got := bravoEntry["type"]; got != string(flow.ResultTypeBool) {
		t.Fatalf("bravo type mismatch: %v", got)
	}
	if got := bravoEntry["result"]; got != true {
		t.Fatalf("bravo result mismatch: %v", got)
	}

	expectedVars := map[string]registry.Variable{
		"shared": {
			Name:  "shared",
			Type:  "string",
			Value: "alpha-overwrite",
		},
		"alpha": {
			Name:  "alpha",
			Type:  "string",
			Value: "value-from-alpha",
		},
		"bravo": {
			Name:  "bravo",
			Type:  "string",
			Value: "value-from-bravo",
		},
	}

	if len(execCtx.Variables) != len(expectedVars) {
		t.Fatalf("variable count mismatch: got %d want %d", len(execCtx.Variables), len(expectedVars))
	}

	for name, want := range expectedVars {
		got, exists := execCtx.Variables[name]
		if !exists {
			t.Fatalf("missing variable %s", name)
		}
		if got != want {
			t.Fatalf("variable %s mismatch: %#v != %#v", name, got, want)
		}
	}
}

func TestActionExecuteMergeConflict(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"tasks": []map[string]any{
			{
				"id":          "conflict-a",
				"description": "first",
				"action":      "PRINT",
			},
			{
				"id":          "conflict-b",
				"description": "second",
				"action":      "PRINT",
			},
		},
		"merge_strategy": "fail_on_conflict",
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	baseVars := map[string]registry.Variable{
		"shared": {
			Name:  "shared",
			Type:  "string",
			Value: "base",
		},
	}

	execCtx := &registry.ExecutionContext{
		Task:      &flow.Task{ID: "parent", FlowID: "main"},
		Variables: cloneRegistryVariables(baseVars),
		LogDir:    t.TempDir(),
	}

	var calls atomic.Int64

	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		calls.Add(1)
		switch req.Task.ID {
		case "conflict-a":
			return registry.TaskExecutionResponse{
				Result: registry.Result{Type: flow.ResultTypeJSON},
				Variables: map[string]registry.Variable{
					"shared": {
						Name:  "shared",
						Type:  "string",
						Value: "from-a",
					},
				},
			}, nil
		case "conflict-b":
			return registry.TaskExecutionResponse{
				Result: registry.Result{Type: flow.ResultTypeJSON},
				Variables: map[string]registry.Variable{
					"shared": {
						Name:  "shared",
						Type:  "string",
						Value: "from-b",
					},
				},
			}, nil
		default:
			return registry.TaskExecutionResponse{}, errors.New("unexpected task")
		}
	}

	result, err := action{}.Execute(context.Background(), raw, execCtx)
	if err == nil {
		t.Fatalf("expected merge conflict error, got nil")
	}

	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != nil {
		t.Fatalf("expected empty result, got %#v", result.Value)
	}

	if diff := calls.Load(); diff != 2 {
		t.Fatalf("expected two task executions, got %d", diff)
	}

	if len(execCtx.Variables) != len(baseVars) {
		t.Fatalf("variables should not change on conflict")
	}
	for name, want := range baseVars {
		if got := execCtx.Variables[name]; got != want {
			t.Fatalf("variable %s mismatch: %#v != %#v", name, got, want)
		}
	}
}
