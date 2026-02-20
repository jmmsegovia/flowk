package parallel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

const (
	// ActionName identifies the parallel action in flow definitions.
	ActionName = "PARALLEL"

	mergeStrategyLastWrite      = "last_write_wins"
	mergeStrategyFailOnConflict = "fail_on_conflict"
)

// Payload describes the configuration supported by the PARALLEL action.
type Payload struct {
	Tasks         []flow.Task `json:"tasks"`
	FailFast      bool        `json:"fail_fast"`
	MergeStrategy string      `json:"merge_strategy"`
	MergeOrder    []string    `json:"merge_order"`
}

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	if execCtx == nil || execCtx.ExecuteTask == nil {
		return registry.Result{}, fmt.Errorf("parallel action: task executor unavailable")
	}

	var cfg Payload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("parallel action: decoding payload: %w", err)
	}

	if len(cfg.Tasks) == 0 {
		return registry.Result{}, fmt.Errorf("parallel action: tasks is required")
	}

	strategy := strings.ToLower(strings.TrimSpace(cfg.MergeStrategy))
	switch strategy {
	case "", mergeStrategyLastWrite:
		strategy = mergeStrategyLastWrite
	case mergeStrategyFailOnConflict:
	default:
		return registry.Result{}, fmt.Errorf("parallel action: unsupported merge_strategy %q", cfg.MergeStrategy)
	}

	taskIDs := make(map[string]struct{}, len(cfg.Tasks))
	for i := range cfg.Tasks {
		cfg.Tasks[i].ID = strings.TrimSpace(cfg.Tasks[i].ID)
		if cfg.Tasks[i].ID == "" {
			return registry.Result{}, fmt.Errorf("parallel action: tasks[%d]: id is required", i)
		}
		taskIDs[cfg.Tasks[i].ID] = struct{}{}
		if cfg.Tasks[i].FlowID == "" && execCtx.Task != nil {
			cfg.Tasks[i].FlowID = execCtx.Task.FlowID
		}
	}

	mergeSequence, err := buildMergeSequence(cfg.MergeOrder, cfg.Tasks, taskIDs)
	if err != nil {
		return registry.Result{}, err
	}

	baseVariables := cloneRegistryVariables(execCtx.Variables)
	baseTasks := append([]flow.Task(nil), execCtx.Tasks...)
	parallelDir := filepath.Join(execCtx.LogDir, "task_parallel")

	results := make(map[string]registry.Result, len(cfg.Tasks))
	variables := make(map[string]map[string]registry.Variable, len(cfg.Tasks))
	taskErrors := make(map[string]error, len(cfg.Tasks))

	var mu sync.Mutex

	ctxForTasks := ctx
	var cancel context.CancelFunc
	if cfg.FailFast {
		ctxForTasks, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	var wg sync.WaitGroup

	for i := range cfg.Tasks {
		taskCopy := cfg.Tasks[i]
		wg.Add(1)

		go func(task flow.Task) {
			defer wg.Done()

			req := registry.TaskExecutionRequest{
				Task:      &task,
				Tasks:     baseTasks,
				Variables: cloneRegistryVariables(baseVariables),
				LogDir:    parallelDir,
			}

			resp, execErr := execCtx.ExecuteTask(ctxForTasks, req)

			mu.Lock()
			defer mu.Unlock()

			if execErr != nil {
				taskErrors[task.ID] = execErr
				if cfg.FailFast && cancel != nil {
					cancel()
				}
				return
			}

			results[task.ID] = resp.Result
			variables[task.ID] = cloneRegistryVariables(resp.Variables)
		}(taskCopy)
	}

	wg.Wait()

	merged, err := mergeVariables(strategy, mergeSequence, baseVariables, variables, taskErrors)
	if err != nil {
		return registry.Result{}, err
	}

	execCtx.Variables = merged

	aggregated := aggregateResults(cfg.Tasks, results, taskErrors)
	finalResult := registry.Result{
		Value: aggregated,
		Type:  flow.ResultTypeJSON,
	}

	if len(taskErrors) > 0 {
		failures := make([]string, 0, len(taskErrors))
		for _, task := range cfg.Tasks {
			if err := taskErrors[task.ID]; err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", task.ID, err))
			}
		}
		return finalResult, fmt.Errorf("parallel action: %d subtasks failed (%s)", len(taskErrors), strings.Join(failures, "; "))
	}

	return finalResult, nil
}

func buildMergeSequence(mergeOrder []string, tasks []flow.Task, taskIDs map[string]struct{}) ([]string, error) {
	order := make([]string, 0, len(tasks))
	seen := make(map[string]struct{}, len(mergeOrder))

	for idx, id := range mergeOrder {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return nil, fmt.Errorf("parallel action: merge_order[%d]: id is required", idx)
		}
		if _, exists := taskIDs[trimmed]; !exists {
			return nil, fmt.Errorf("parallel action: merge_order[%d]: unknown task id %q", idx, trimmed)
		}
		if _, dup := seen[trimmed]; dup {
			return nil, fmt.Errorf("parallel action: merge_order[%d]: duplicate task id %q", idx, trimmed)
		}
		seen[trimmed] = struct{}{}
		order = append(order, trimmed)
	}

	for _, task := range tasks {
		if _, exists := seen[task.ID]; exists {
			continue
		}
		order = append(order, task.ID)
	}

	return order, nil
}

func mergeVariables(strategy string, sequence []string, base map[string]registry.Variable, updates map[string]map[string]registry.Variable, taskErrors map[string]error) (map[string]registry.Variable, error) {
	merged := cloneRegistryVariables(base)
	origin := make(map[string]string)

	for name := range merged {
		origin[name] = "base"
	}

	for _, taskID := range sequence {
		if taskErrors[taskID] != nil {
			continue
		}

		vars := updates[taskID]
		if len(vars) == 0 {
			continue
		}

		for name, variable := range vars {
			if existing, exists := merged[name]; exists {
				if strategy == mergeStrategyFailOnConflict && !registryVariableEqual(existing, variable) {
					return nil, fmt.Errorf("parallel action: variable %q conflict between tasks %s and %s", name, origin[name], taskID)
				}
			}

			merged[name] = variable
			origin[name] = taskID
		}
	}

	return merged, nil
}

func aggregateResults(tasks []flow.Task, results map[string]registry.Result, taskErrors map[string]error) map[string]map[string]any {
	aggregated := make(map[string]map[string]any, len(tasks))

	for _, task := range tasks {
		entry := map[string]any{}
		if res, exists := results[task.ID]; exists {
			entry["result"] = res.Value
			entry["type"] = string(res.Type)
		}
		if err := taskErrors[task.ID]; err != nil {
			entry["error"] = err.Error()
		}
		aggregated[task.ID] = entry
	}

	return aggregated
}

func cloneRegistryVariables(vars map[string]registry.Variable) map[string]registry.Variable {
	if len(vars) == 0 {
		return map[string]registry.Variable{}
	}

	cloned := make(map[string]registry.Variable, len(vars))
	for name, variable := range vars {
		cloned[name] = registry.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}
	return cloned
}

func registryVariableEqual(a, b registry.Variable) bool {
	if a.Secret != b.Secret || a.Type != b.Type {
		return false
	}
	return reflect.DeepEqual(a.Value, b.Value)
}
