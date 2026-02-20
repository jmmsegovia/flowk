package forloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowk/internal/actions/core/variables"
	"flowk/internal/actions/registry"
	"flowk/internal/flow"
	expansion "flowk/internal/shared/expansion"
)

const (
	// ActionName identifies the FOR action in flow definitions.
	ActionName = "FOR"
)

type action struct{}

// Payload describes the configuration supported by the FOR action.
type Payload struct {
	Variable      string        `json:"variable"`
	Initial       json.Number   `json:"initial"`
	Condition     LoopCondition `json:"condition"`
	Step          json.Number   `json:"step"`
	RequireBreak  bool          `json:"require_break,omitempty"`
	MaxIterations *int          `json:"max_iterations,omitempty"`
	Values        []string      `json:"values"`
	Tasks         []flow.Task   `json:"tasks"`
}

// LoopCondition defines the exit condition evaluated on every iteration.
type LoopCondition struct {
	Operator string      `json:"operator"`
	Value    json.Number `json:"value"`
}

// iterationSummary captures the outcome of a single loop iteration.
type iterationSummary struct {
	Index   int              `json:"index"`
	Counter *float64         `json:"counter,omitempty"`
	Value   any              `json:"value,omitempty"`
	Tasks   []subtaskSummary `json:"tasks"`
}

// subtaskSummary stores the outcome of an individual subtask execution.
type subtaskSummary struct {
	TaskID     string            `json:"task_id"`
	Result     any               `json:"result,omitempty"`
	ResultType flow.ResultType   `json:"result_type,omitempty"`
	Control    *registry.Control `json:"control,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (cfg Payload) normalize(execCtx *registry.ExecutionContext) (normalizedPayload, error) {
	variable := strings.TrimSpace(cfg.Variable)
	if variable == "" {
		return normalizedPayload{}, fmt.Errorf("for action: variable is required")
	}

	if cfg.MaxIterations != nil && *cfg.MaxIterations < 0 {
		return normalizedPayload{}, fmt.Errorf("for action: max_iterations must be non-negative")
	}

	if len(cfg.Tasks) == 0 {
		return normalizedPayload{}, fmt.Errorf("for action: tasks is required")
	}

	taskIDs := make(map[string]struct{}, len(cfg.Tasks))
	tasks := make([]flow.Task, len(cfg.Tasks))
	for i := range cfg.Tasks {
		task := cfg.Tasks[i]
		task.ID = strings.TrimSpace(task.ID)
		if task.ID == "" {
			return normalizedPayload{}, fmt.Errorf("for action: tasks[%d]: id is required", i)
		}
		if _, exists := taskIDs[task.ID]; exists {
			return normalizedPayload{}, fmt.Errorf("for action: tasks[%d]: id %q is duplicated", i, task.ID)
		}
		taskIDs[task.ID] = struct{}{}
		if task.FlowID == "" && execCtx != nil && execCtx.Task != nil {
			task.FlowID = execCtx.Task.FlowID
		}
		tasks[i] = task
	}

	if cfg.Values != nil {
		if cfg.Initial != "" || cfg.Step != "" || strings.TrimSpace(cfg.Condition.Operator) != "" || cfg.Condition.Value != "" {
			return normalizedPayload{}, fmt.Errorf("for action: values cannot be combined with numeric loop configuration")
		}

		// Resolve task placeholders to support dynamic lists like ${from.task:...result$.items[*].field}
		resolvedValues := make([]string, 0)
		for i := range cfg.Values {
			raw := cfg.Values[i]
			if execCtx != nil && len(execCtx.Tasks) > 0 {
				if anyVal, err := variables.ResolveTaskPlaceholders(raw, execCtx.Tasks); err == nil {
					switch v := anyVal.(type) {
					case []any:
						for _, item := range v {
							resolvedValues = append(resolvedValues, fmt.Sprintf("%v", item))
						}
						continue
					case []string:
						resolvedValues = append(resolvedValues, v...)
						continue
					default:
						resolvedValues = append(resolvedValues, fmt.Sprintf("%v", v))
						continue
					}
				}
			}
			resolvedValues = append(resolvedValues, raw)
		}

		return normalizedPayload{
			variable:      variable,
			requireBreak:  cfg.RequireBreak,
			maxIterations: cfg.MaxIterations,
			values:        resolvedValues,
			tasks:         tasks,
		}, nil
	}

	initial, err := toFloat(cfg.Initial, "initial")
	if err != nil {
		return normalizedPayload{}, err
	}

	step, err := toFloat(cfg.Step, "step")
	if err != nil {
		return normalizedPayload{}, err
	}
	if step == 0 {
		return normalizedPayload{}, fmt.Errorf("for action: step cannot be zero")
	}

	operator := strings.TrimSpace(cfg.Condition.Operator)
	if operator == "" {
		return normalizedPayload{}, fmt.Errorf("for action: condition.operator is required")
	}
	if !isSupportedOperator(operator) {
		return normalizedPayload{}, fmt.Errorf("for action: unsupported condition.operator %q", cfg.Condition.Operator)
	}

	target, err := toFloat(cfg.Condition.Value, "condition.value")
	if err != nil {
		return normalizedPayload{}, err
	}

	return normalizedPayload{
		variable: variable,
		numeric: &numericLoop{
			initial:      initial,
			conditionOp:  operator,
			conditionVal: target,
			step:         step,
		},
		requireBreak:  cfg.RequireBreak,
		maxIterations: cfg.MaxIterations,
		tasks:         tasks,
	}, nil
}

type normalizedPayload struct {
	variable      string
	numeric       *numericLoop
	maxIterations *int
	requireBreak  bool
	values        []string
	tasks         []flow.Task
}

type numericLoop struct {
	initial      float64
	conditionOp  string
	conditionVal float64
	step         float64
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	if execCtx == nil || execCtx.ExecuteTask == nil {
		return registry.Result{}, fmt.Errorf("for action: task executor unavailable")
	}

	var cfg Payload
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return registry.Result{}, fmt.Errorf("for action: decoding payload: %w", err)
	}

	normalized, err := cfg.normalize(execCtx)
	if err != nil {
		return registry.Result{}, err
	}

	baseVariables := cloneVariables(execCtx.Variables)
	baseTasks := append([]flow.Task(nil), execCtx.Tasks...)

	loopDir := strings.TrimSpace(execCtx.LogDir)
	if loopDir != "" {
		loopDir = filepath.Join(loopDir, "task_for")
	}

	currentVariables := cloneVariables(baseVariables)
	summaries := make([]iterationSummary, 0)
	var finalControl *registry.Control
	loopBroken := false

	if normalized.values != nil {
		executed := false
		var lastAssigned registry.Variable

		for iteration := 0; iteration < len(normalized.values); iteration++ {
			if normalized.maxIterations != nil && *normalized.maxIterations > 0 && iteration >= *normalized.maxIterations {
				break
			}

			value, err := expandLoopValue(normalized.values[iteration], currentVariables)
			if err != nil {
				execCtx.Variables = currentVariables
				return registry.Result{}, fmt.Errorf("for action: evaluating values[%d]: %w", iteration, err)
			}
			executed = true

			assignment := registry.Variable{
				Name:  normalized.variable,
				Type:  "string",
				Value: value,
			}
			lastAssigned = assignment
			currentVariables[normalized.variable] = assignment

			iterDir := loopDir
			if iterDir != "" {
				iterDir = filepath.Join(loopDir, fmt.Sprintf("%d", iteration))
				if err := os.MkdirAll(iterDir, 0o755); err != nil {
					execCtx.Variables = currentVariables
					return registry.Result{}, fmt.Errorf("for action: creating iteration log dir: %w", err)
				}
			}

			summary := iterationSummary{
				Index: iteration,
				Value: value,
			}

			taskSummaries, updatedVars, ctrl, brokeLoop, execErr := executeIterationTasks(ctx, iterDir, normalized.tasks, baseTasks, currentVariables, execCtx, assignment)
			summary.Tasks = append(summary.Tasks, taskSummaries...)
			currentVariables = updatedVars

			if execErr != nil {
				summaries = append(summaries, summary)
				execCtx.Variables = currentVariables
				return registry.Result{Value: summaries, Type: flow.ResultTypeJSON}, execErr
			}

			summaries = append(summaries, summary)
			if ctrl != nil {
				finalControl = ctrl
				if ctrl.BreakLoop {
					loopBroken = true
				}
				break
			}
			if brokeLoop {
				loopBroken = true
				break
			}
		}

		if executed {
			currentVariables[normalized.variable] = lastAssigned
		}

		if normalized.requireBreak && !loopBroken {
			execCtx.Variables = currentVariables
			return registry.Result{Value: summaries, Type: flow.ResultTypeJSON}, fmt.Errorf("for action: require_break enabled but loop ended without break after %d iterations", len(summaries))
		}

		execCtx.Variables = currentVariables

		return registry.Result{
			Value:   summaries,
			Type:    flow.ResultTypeJSON,
			Control: finalControl,
		}, nil
	}

	numeric := normalized.numeric
	if numeric == nil {
		execCtx.Variables = currentVariables
		return registry.Result{Value: summaries, Type: flow.ResultTypeJSON}, nil
	}

	counter := numeric.initial
	iteration := 0
	executed := false
	var lastAssigned registry.Variable

	for {
		if normalized.maxIterations != nil && *normalized.maxIterations > 0 && iteration >= *normalized.maxIterations {
			break
		}
		if !evaluateCondition(counter, numeric.conditionVal, numeric.conditionOp) {
			break
		}

		executed = true
		assignment := registry.Variable{
			Name:  normalized.variable,
			Type:  "number",
			Value: counter,
		}
		lastAssigned = assignment
		currentVariables[normalized.variable] = assignment

		iterDir := loopDir
		if iterDir != "" {
			iterDir = filepath.Join(loopDir, fmt.Sprintf("%d", iteration))
			if err := os.MkdirAll(iterDir, 0o755); err != nil {
				execCtx.Variables = currentVariables
				return registry.Result{}, fmt.Errorf("for action: creating iteration log dir: %w", err)
			}
		}

		counterCopy := counter
		summary := iterationSummary{
			Index:   iteration,
			Counter: &counterCopy,
			Value:   counter,
		}

		taskSummaries, updatedVars, ctrl, brokeLoop, execErr := executeIterationTasks(ctx, iterDir, normalized.tasks, baseTasks, currentVariables, execCtx, assignment)
		summary.Tasks = append(summary.Tasks, taskSummaries...)
		currentVariables = updatedVars

		if execErr != nil {
			summaries = append(summaries, summary)
			execCtx.Variables = currentVariables
			return registry.Result{Value: summaries, Type: flow.ResultTypeJSON}, execErr
		}

		summaries = append(summaries, summary)
		if ctrl != nil {
			finalControl = ctrl
			if ctrl.BreakLoop {
				loopBroken = true
			}
			break
		}
		if brokeLoop {
			loopBroken = true
			break
		}

		iteration++
		counter += numeric.step
	}

	if executed {
		currentVariables[normalized.variable] = lastAssigned
	}

	if normalized.requireBreak && !loopBroken {
		execCtx.Variables = currentVariables
		return registry.Result{Value: summaries, Type: flow.ResultTypeJSON}, fmt.Errorf("for action: require_break enabled but loop ended without break after %d iterations", len(summaries))
	}

	execCtx.Variables = currentVariables

	return registry.Result{
		Value:   summaries,
		Type:    flow.ResultTypeJSON,
		Control: finalControl,
	}, nil
}

func cloneVariables(vars map[string]registry.Variable) map[string]registry.Variable {
	if len(vars) == 0 {
		return make(map[string]registry.Variable)
	}
	cloned := make(map[string]registry.Variable, len(vars))
	for key, value := range vars {
		cloned[key] = value
	}
	return cloned
}

func executeIterationTasks(
	ctx context.Context,
	iterDir string,
	tasks []flow.Task,
	baseTasks []flow.Task,
	currentVariables map[string]registry.Variable,
	execCtx *registry.ExecutionContext,
	assignment registry.Variable,
) ([]subtaskSummary, map[string]registry.Variable, *registry.Control, bool, error) {
	summaries := make([]subtaskSummary, 0, len(tasks))
	updated := currentVariables
	var finalControl *registry.Control
	brokeLoop := false
	executedTasks := make([]flow.Task, 0, len(tasks))

	for _, task := range tasks {
		visibleTasks := make([]flow.Task, 0, len(baseTasks)+len(executedTasks))
		if len(baseTasks) > 0 {
			visibleTasks = append(visibleTasks, baseTasks...)
		}
		if len(executedTasks) > 0 {
			visibleTasks = append(visibleTasks, executedTasks...)
		}

		req := registry.TaskExecutionRequest{
			Task:      &task,
			Tasks:     visibleTasks,
			Variables: cloneVariables(updated),
			LogDir:    iterDir,
		}

		resp, execErr := execCtx.ExecuteTask(ctx, req)
		if execErr != nil {
			summaries = append(summaries, subtaskSummary{
				TaskID: task.ID,
				Error:  execErr.Error(),
			})
			updated[assignment.Name] = assignment
			return summaries, updated, nil, false, fmt.Errorf("for action: executing task %s: %w", task.ID, execErr)
		}

		summaries = append(summaries, subtaskSummary{
			TaskID:     task.ID,
			Result:     resp.Result.Value,
			ResultType: resp.Result.Type,
			Control:    resp.Result.Control,
		})

		if len(resp.Variables) > 0 {
			updated = cloneVariables(resp.Variables)
		}
		updated[assignment.Name] = assignment

		executedTasks = append(executedTasks, task)

		if ctrl := resp.Result.Control; ctrl != nil {
			if ctrl.Exit || ctrl.JumpToTaskID != "" {
				finalControl = ctrl
				break
			}
			if ctrl.BreakLoop {
				brokeLoop = true
				break
			}
		}
	}

	return summaries, updated, finalControl, brokeLoop, nil
}

func expandLoopValue(template string, vars map[string]registry.Variable) (string, error) {
	if template == "" {
		return template, nil
	}

	expanded, err := expansion.ExpandString(template, toExpansionVariables(vars))
	if err != nil {
		return "", err
	}
	return expanded, nil
}

func toExpansionVariables(vars map[string]registry.Variable) map[string]expansion.Variable {
	if len(vars) == 0 {
		return map[string]expansion.Variable{}
	}

	converted := make(map[string]expansion.Variable, len(vars))
	for name, variable := range vars {
		converted[name] = expansion.Variable{
			Name:   chooseNonEmpty(variable.Name, name),
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	return converted
}

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func toFloat(number json.Number, field string) (float64, error) {
	if number == "" {
		return 0, fmt.Errorf("for action: %s is required", field)
	}
	value, err := number.Float64()
	if err != nil {
		return 0, fmt.Errorf("for action: %s must be a number", field)
	}
	return value, nil
}

func isSupportedOperator(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func evaluateCondition(counter, target float64, operator string) bool {
	switch operator {
	case "==":
		return counter == target
	case "!=":
		return counter != target
	case "<":
		return counter < target
	case "<=":
		return counter <= target
	case ">":
		return counter > target
	case ">=":
		return counter >= target
	default:
		return false
	}
}
