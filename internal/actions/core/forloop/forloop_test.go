package forloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestPayloadNormalizeValidation(t *testing.T) {
	execCtx := &registry.ExecutionContext{Task: &flow.Task{FlowID: "flow"}}

	tests := []struct {
		name    string
		payload Payload
		wantErr string
	}{
		{
			name: "missing variable",
			payload: Payload{
				Initial:   json.Number("0"),
				Step:      json.Number("1"),
				Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
				Tasks:     []flow.Task{{ID: "child"}},
			},
			wantErr: "variable is required",
		},
		{
			name: "zero step",
			payload: Payload{
				Variable:  "counter",
				Initial:   json.Number("0"),
				Step:      json.Number("0"),
				Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
				Tasks:     []flow.Task{{ID: "child"}},
			},
			wantErr: "step cannot be zero",
		},
		{
			name: "invalid operator",
			payload: Payload{
				Variable:  "counter",
				Initial:   json.Number("0"),
				Step:      json.Number("1"),
				Condition: LoopCondition{Operator: "~=", Value: json.Number("1")},
				Tasks:     []flow.Task{{ID: "child"}},
			},
			wantErr: "unsupported condition.operator",
		},
		{
			name: "empty tasks",
			payload: Payload{
				Variable:  "counter",
				Initial:   json.Number("0"),
				Step:      json.Number("1"),
				Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
			},
			wantErr: "tasks is required",
		},
		{
			name: "duplicate ids",
			payload: Payload{
				Variable:  "counter",
				Initial:   json.Number("0"),
				Step:      json.Number("1"),
				Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
				Tasks:     []flow.Task{{ID: "dup"}, {ID: "dup"}},
			},
			wantErr: "duplicated",
		},
		{
			name: "values with numeric fields",
			payload: Payload{
				Variable: "item",
				Initial:  json.Number("0"),
				Values:   []string{"a"},
				Tasks:    []flow.Task{{ID: "child"}},
			},
			wantErr: "values cannot be combined",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.payload.normalize(execCtx)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestPayloadNormalizeFlowInheritance(t *testing.T) {
	execCtx := &registry.ExecutionContext{Task: &flow.Task{FlowID: "parent"}}
	payload := Payload{
		Variable:  "counter",
		Initial:   json.Number("0"),
		Step:      json.Number("1"),
		Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
		Tasks:     []flow.Task{{ID: "child"}},
	}

	normalized, err := payload.normalize(execCtx)
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if normalized.tasks[0].FlowID != "parent" {
		t.Fatalf("expected flow id to be inherited, got %q", normalized.tasks[0].FlowID)
	}
}

func TestPayloadNormalizeValues(t *testing.T) {
	execCtx := &registry.ExecutionContext{Task: &flow.Task{FlowID: "parent"}}
	payload := Payload{
		Variable: "item",
		Values:   []string{"apple", "banana"},
		Tasks:    []flow.Task{{ID: "child"}},
	}

	normalized, err := payload.normalize(execCtx)
	if err != nil {
		t.Fatalf("normalize returned error: %v", err)
	}
	if normalized.numeric != nil {
		t.Fatalf("expected numeric configuration to be nil")
	}
	if normalized.values == nil || len(normalized.values) != 2 {
		t.Fatalf("expected two values, got %+v", normalized.values)
	}
	if normalized.values[0] != "apple" || normalized.values[1] != "banana" {
		t.Fatalf("unexpected values slice: %+v", normalized.values)
	}
	if normalized.tasks[0].FlowID != "parent" {
		t.Fatalf("expected flow id to be inherited, got %q", normalized.tasks[0].FlowID)
	}
}

func TestExecuteIncrementsAndPersistsVariables(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:      &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks:     []flow.Task{{ID: "parent", FlowID: "flow"}},
		Variables: map[string]registry.Variable{"existing": {Name: "existing", Type: "string", Value: "keep"}},
		LogDir:    filepath.Join(t.TempDir(), "parent"),
	}

	call := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		counter, ok := req.Variables["loop_counter"]
		if !ok {
			t.Fatalf("iteration %d missing loop counter", call)
		}
		if counter.Type != "number" {
			t.Fatalf("iteration %d unexpected counter type %q", call, counter.Type)
		}

		expectedDir := filepath.Join(execCtx.LogDir, "task_for", fmt.Sprintf("%d", call))
		if req.LogDir != expectedDir {
			t.Fatalf("iteration %d expected log dir %q, got %q", call, expectedDir, req.LogDir)
		}

		respVars := cloneVariables(req.Variables)
		respVars["shared"] = registry.Variable{Name: "shared", Type: "number", Value: float64(call)}

		result := registry.Result{Value: map[string]any{"iteration": call}, Type: flow.ResultTypeJSON}
		call++
		return registry.TaskExecutionResponse{Result: result, Variables: respVars}, nil
	}

	payload := Payload{
		Variable:  "loop_counter",
		Initial:   json.Number("0"),
		Step:      json.Number("1"),
		Condition: LoopCondition{Operator: "<", Value: json.Number("3")},
		Tasks:     []flow.Task{{ID: "child"}},
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(summaries))
	}
	for i, summary := range summaries {
		if summary.Index != i {
			t.Errorf("iteration %d summary index mismatch: %d", i, summary.Index)
		}
		if summary.Counter == nil || *summary.Counter != float64(i) {
			t.Errorf("iteration %d counter mismatch: %v", i, summary.Counter)
		}
		if value, ok := summary.Value.(float64); !ok || value != float64(i) {
			t.Errorf("iteration %d value mismatch: %v", i, summary.Value)
		}
		if len(summary.Tasks) != 1 {
			t.Fatalf("iteration %d expected 1 subtask result, got %d", i, len(summary.Tasks))
		}
		if summary.Tasks[0].ResultType != flow.ResultTypeJSON {
			t.Fatalf("iteration %d unexpected result type %s", i, summary.Tasks[0].ResultType)
		}
	}

	if got := execCtx.Variables["loop_counter"].Value; got != float64(2) {
		t.Fatalf("expected final counter 2, got %v", got)
	}
	if got := execCtx.Variables["shared"].Value; got != float64(2) {
		t.Fatalf("expected shared variable to persist value 2, got %v", got)
	}
}

func TestExecuteIteratesValues(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:   &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks:  []flow.Task{{ID: "parent", FlowID: "flow"}},
		LogDir: filepath.Join(t.TempDir(), "parent"),
	}

	var seen []string
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		variable, ok := req.Variables["fruit"]
		if !ok {
			t.Fatalf("missing fruit variable in iteration")
		}
		if variable.Type != "string" {
			t.Fatalf("unexpected variable type %q", variable.Type)
		}
		value, _ := variable.Value.(string)
		seen = append(seen, value)

		expectedDir := filepath.Join(execCtx.LogDir, "task_for", fmt.Sprintf("%d", len(seen)-1))
		if req.LogDir != expectedDir {
			t.Fatalf("iteration %d expected log dir %q, got %q", len(seen)-1, expectedDir, req.LogDir)
		}

		return registry.TaskExecutionResponse{
			Result:    registry.Result{Value: value, Type: flow.ResultTypeString},
			Variables: cloneVariables(req.Variables),
		}, nil
	}

	raw, _ := json.Marshal(map[string]any{
		"variable": "fruit",
		"values":   []string{"apple", "banana", "cherry"},
		"tasks":    []map[string]any{{"id": "child"}},
	})
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(summaries))
	}
	expected := []string{"apple", "banana", "cherry"}
	for i, summary := range summaries {
		if summary.Index != i {
			t.Errorf("iteration %d summary index mismatch: %d", i, summary.Index)
		}
		if summary.Counter != nil {
			t.Errorf("iteration %d expected nil counter, got %v", i, summary.Counter)
		}
		if summaryValue, _ := summary.Value.(string); summaryValue != expected[i] {
			t.Errorf("iteration %d expected value %q, got %v", i, expected[i], summary.Value)
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("expected %d executions, got %d", len(expected), len(seen))
	}
	if got := execCtx.Variables["fruit"]; got.Type != "string" || got.Value != "cherry" {
		t.Fatalf("expected final fruit to be cherry string, got %+v", got)
	}
}

func TestExecuteValuesExpandsVariables(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
		Variables: map[string]registry.Variable{
			"prefix": {Name: "prefix", Type: "string", Value: "PRE_"},
		},
	}

	var seen []string
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		variable, ok := req.Variables["letter"]
		if !ok {
			t.Fatalf("missing iteration variable")
		}
		if variable.Type != "string" {
			t.Fatalf("unexpected variable type %q", variable.Type)
		}
		value, _ := variable.Value.(string)
		seen = append(seen, value)

		resp := registry.TaskExecutionResponse{
			Result:    registry.Result{Value: value, Type: flow.ResultTypeString},
			Variables: cloneVariables(req.Variables),
		}
		return resp, nil
	}

	raw, _ := json.Marshal(map[string]any{
		"variable": "letter",
		"values":   []string{"${prefix}A", "${prefix}B"},
		"tasks":    []map[string]any{{"id": "child"}},
	})

	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(summaries))
	}

	expected := []string{"PRE_A", "PRE_B"}
	if len(seen) != len(expected) {
		t.Fatalf("expected %d executions, got %d", len(expected), len(seen))
	}
	for i, summary := range summaries {
		if summary.Index != i {
			t.Errorf("iteration %d summary index mismatch: %d", i, summary.Index)
		}
		if summary.Counter != nil {
			t.Errorf("iteration %d expected nil counter, got %v", i, summary.Counter)
		}
		if summaryValue, _ := summary.Value.(string); summaryValue != expected[i] {
			t.Errorf("iteration %d expected value %q, got %v", i, expected[i], summary.Value)
		}
		if seen[i] != expected[i] {
			t.Errorf("iteration %d expected execution value %q, got %q", i, expected[i], seen[i])
		}
	}

	final, ok := execCtx.Variables["letter"]
	if !ok {
		t.Fatalf("expected final iteration variable to be present")
	}
	if final.Type != "string" || final.Value != "PRE_B" {
		t.Fatalf("expected final iteration variable to be PRE_B string, got %+v", final)
	}
}

func TestExecuteExposesCompletedSubtasksWithinIteration(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:   &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks:  []flow.Task{{ID: "parent", FlowID: "flow"}},
		LogDir: t.TempDir(),
	}

	iteration := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		switch req.Task.ID {
		case "produce":
			result := map[string]any{"value": float64(iteration)}
			req.Task.Status = flow.TaskStatusCompleted
			req.Task.Success = true
			req.Task.Result = result
			req.Task.ResultType = flow.ResultTypeJSON
			iteration++
			return registry.TaskExecutionResponse{
				Result:    registry.Result{Value: result, Type: flow.ResultTypeJSON},
				Variables: cloneVariables(req.Variables),
			}, nil
		case "evaluate":
			var previous *flow.Task
			for i := range req.Tasks {
				task := &req.Tasks[i]
				if task.ID == "produce" {
					previous = task
					break
				}
			}
			if previous == nil {
				t.Fatalf("evaluate task did not receive previous subtask")
			}
			if previous.Status != flow.TaskStatusCompleted {
				t.Fatalf("expected previous subtask to be completed, got %s", previous.Status)
			}
			if !previous.Success {
				t.Fatalf("expected previous subtask to succeed")
			}
			if previous.ResultType != flow.ResultTypeJSON {
				t.Fatalf("expected previous subtask to expose json result, got %s", previous.ResultType)
			}
			result, ok := previous.Result.(map[string]any)
			if !ok {
				t.Fatalf("expected previous result to be a map, got %T", previous.Result)
			}
			if value, ok := result["value"].(float64); !ok || value != 0 {
				t.Fatalf("expected previous result value 0, got %v", result["value"])
			}

			return registry.TaskExecutionResponse{
				Result:    registry.Result{Value: true, Type: flow.ResultTypeBool},
				Variables: cloneVariables(req.Variables),
			}, nil
		default:
			t.Fatalf("unexpected task id %q", req.Task.ID)
		}
		return registry.TaskExecutionResponse{}, nil
	}

	payload := Payload{
		Variable:  "item",
		Initial:   json.Number("0"),
		Step:      json.Number("1"),
		Condition: LoopCondition{Operator: "<", Value: json.Number("1")},
		Tasks: []flow.Task{
			{ID: "produce", Action: "VARIABLES"},
			{ID: "evaluate", Action: "EVALUATE"},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected a single iteration, got %d", len(summaries))
	}
	if len(summaries[0].Tasks) != 2 {
		t.Fatalf("expected two subtasks in summary, got %d", len(summaries[0].Tasks))
	}
}

func TestExecuteDecrement(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:   &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks:  []flow.Task{{ID: "parent", FlowID: "flow"}},
		LogDir: t.TempDir(),
	}

	var seen []float64
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		value := req.Variables["counter"].Value.(float64)
		seen = append(seen, value)
		return registry.TaskExecutionResponse{Result: registry.Result{Value: "ok", Type: flow.ResultTypeString}, Variables: cloneVariables(req.Variables)}, nil
	}

	payload := Payload{
		Variable:  "counter",
		Initial:   json.Number("5"),
		Step:      json.Number("-2"),
		Condition: LoopCondition{Operator: ">=", Value: json.Number("0")},
		Tasks:     []flow.Task{{ID: "child"}},
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries := result.Value.([]iterationSummary)
	if len(summaries) != 3 {
		t.Fatalf("expected 3 iterations, got %d", len(summaries))
	}
	expected := []float64{5, 3, 1}
	for i, want := range expected {
		if summaries[i].Counter == nil || *summaries[i].Counter != want {
			t.Errorf("iteration %d counter mismatch: got %v want %v", i, summaries[i].Counter, want)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(seen))
	}
	if got := execCtx.Variables["counter"].Value; got != float64(1) {
		t.Fatalf("expected final counter 1, got %v", got)
	}
}

func TestExecuteMaxIterations(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	runs := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		runs++
		return registry.TaskExecutionResponse{Result: registry.Result{Value: nil, Type: flow.ResultTypeJSON}, Variables: cloneVariables(req.Variables)}, nil
	}

	max := 2
	payload := Payload{
		Variable:      "counter",
		Initial:       json.Number("0"),
		Step:          json.Number("1"),
		Condition:     LoopCondition{Operator: "<", Value: json.Number("10")},
		MaxIterations: &max,
		Tasks:         []flow.Task{{ID: "child"}},
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	summaries := result.Value.([]iterationSummary)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(summaries))
	}
	if runs != 2 {
		t.Fatalf("expected execute task called twice, got %d", runs)
	}
}

func TestExecuteValuesMaxIterations(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	runs := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		runs++
		return registry.TaskExecutionResponse{Result: registry.Result{Value: req.Variables["fruit"].Value, Type: flow.ResultTypeString}, Variables: cloneVariables(req.Variables)}, nil
	}

	max := 2
	raw, _ := json.Marshal(map[string]any{
		"variable":       "fruit",
		"values":         []string{"apple", "banana", "cherry"},
		"max_iterations": max,
		"tasks":          []map[string]any{{"id": "child"}},
	})
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	summaries := result.Value.([]iterationSummary)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(summaries))
	}
	if runs != 2 {
		t.Fatalf("expected execute task called twice, got %d", runs)
	}
	if got := execCtx.Variables["fruit"]; got.Type != "string" || got.Value != "banana" {
		t.Fatalf("expected final fruit to be banana, got %+v", got)
	}
}

func TestExecutePropagatesControl(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	count := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		ctrl := &registry.Control{}
		if count == 1 {
			ctrl.Exit = true
		}
		resp := registry.TaskExecutionResponse{
			Result:    registry.Result{Value: count, Type: flow.ResultTypeInt, Control: ctrl},
			Variables: cloneVariables(req.Variables),
		}
		count++
		return resp, nil
	}

	payload := Payload{
		Variable:  "counter",
		Initial:   json.Number("0"),
		Step:      json.Number("1"),
		Condition: LoopCondition{Operator: "<", Value: json.Number("5")},
		Tasks:     []flow.Task{{ID: "child"}},
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if result.Control == nil || !result.Control.Exit {
		t.Fatalf("expected exit control propagated, got %+v", result.Control)
	}
	summaries := result.Value.([]iterationSummary)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 iterations before exit, got %d", len(summaries))
	}
}

func TestExecuteStopsLoopOnBreakControl(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	runs := 0
	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		if req.Task.ID != "breaker" {
			t.Fatalf("unexpected task id %q", req.Task.ID)
		}
		runs++
		return registry.TaskExecutionResponse{
			Result:    registry.Result{Value: true, Type: flow.ResultTypeBool, Control: &registry.Control{BreakLoop: true}},
			Variables: cloneVariables(req.Variables),
		}, nil
	}

	raw := []byte("{\"variable\":\"item\",\"values\":[\"first\",\"second\"],\"tasks\":[{\"id\":\"breaker\",\"action\":\"EVALUATE\"}]}")

	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if runs != 1 {
		t.Fatalf("expected a single iteration, got %d", runs)
	}
	if result.Control != nil {
		t.Fatalf("expected no control propagated, got %+v", result.Control)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 iteration summary, got %d", len(summaries))
	}
	if len(summaries[0].Tasks) != 1 {
		t.Fatalf("expected 1 subtask summary, got %d", len(summaries[0].Tasks))
	}
	if ctrl := summaries[0].Tasks[0].Control; ctrl == nil || !ctrl.BreakLoop {
		t.Fatalf("expected subtask summary to capture break control, got %+v", ctrl)
	}
}

func TestExecutePropagatesErrors(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		if req.Variables["counter"].Value == float64(1) {
			return registry.TaskExecutionResponse{}, errors.New("boom")
		}
		return registry.TaskExecutionResponse{Result: registry.Result{Value: "ok", Type: flow.ResultTypeString}, Variables: cloneVariables(req.Variables)}, nil
	}

	payload := Payload{
		Variable:  "counter",
		Initial:   json.Number("0"),
		Step:      json.Number("1"),
		Condition: LoopCondition{Operator: "<", Value: json.Number("5")},
		Tasks:     []flow.Task{{ID: "child"}},
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err == nil || !strings.Contains(err.Error(), "executing task") {
		t.Fatalf("expected error from failing subtask, got %v", err)
	}
	summaries := result.Value.([]iterationSummary)
	if len(summaries) != 2 {
		t.Fatalf("expected summaries for two iterations, got %d", len(summaries))
	}
	if summaries[1].Tasks[0].Error == "" {
		t.Fatalf("expected error captured in summary")
	}
}

func TestExecuteFailsWhenRequireBreakNotTriggered(t *testing.T) {
	act := action{}
	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "parent", FlowID: "flow"},
		Tasks: []flow.Task{{ID: "parent", FlowID: "flow"}},
	}

	execCtx.ExecuteTask = func(ctx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		if req.Task.ID != "child" {
			return registry.TaskExecutionResponse{}, fmt.Errorf("unexpected task id %s", req.Task.ID)
		}
		return registry.TaskExecutionResponse{Result: registry.Result{Value: true, Type: flow.ResultTypeBool}, Variables: cloneVariables(req.Variables)}, nil
	}

	max := 2
	payload := Payload{
		Variable:      "counter",
		Initial:       json.Number("0"),
		Step:          json.Number("1"),
		Condition:     LoopCondition{Operator: "<", Value: json.Number("2")},
		Tasks:         []flow.Task{{ID: "child"}},
		RequireBreak:  true,
		MaxIterations: &max,
	}

	raw, _ := json.Marshal(payload)
	result, err := act.Execute(context.Background(), raw, execCtx)
	if err == nil || !strings.Contains(err.Error(), "require_break") {
		t.Fatalf("expected error due to missing break, got %v", err)
	}

	summaries, ok := result.Value.([]iterationSummary)
	if !ok {
		t.Fatalf("unexpected result type: %#v", result.Value)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 iterations, got %d", len(summaries))
	}
}
