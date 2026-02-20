package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowk/internal/actions/core/evaluate"
	"flowk/internal/actions/core/forloop"
	"flowk/internal/actions/core/parallel"
	"flowk/internal/actions/core/print"
	"flowk/internal/actions/core/variables"
	"flowk/internal/actions/db/cassandra"
	"flowk/internal/actions/registry"
	"flowk/internal/flow"
	"flowk/internal/logging/colors"
	expansion "flowk/internal/shared/expansion"
)

type taskDirectoryAllocator struct {
	mu      sync.Mutex
	counter int
}

func (a *taskDirectoryAllocator) allocate(parentDir, taskID string) (string, error) {
	trimmedParent := strings.TrimSpace(parentDir)
	if trimmedParent == "" {
		return "", fmt.Errorf("task directory allocator: parent directory is required")
	}

	if err := os.MkdirAll(trimmedParent, 0o755); err != nil {
		return "", fmt.Errorf("creating parent directory %q: %w", trimmedParent, err)
	}

	sanitized := sanitizeForDirectory(taskID)
	if sanitized == "" {
		sanitized = "task"
	}

	a.mu.Lock()
	idx := a.counter
	a.counter++
	a.mu.Unlock()

	dirName := fmt.Sprintf("task-%04d-%s", idx, sanitized)
	taskDir := filepath.Join(trimmedParent, dirName)

	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return "", fmt.Errorf("creating task directory %q: %w", taskDir, err)
	}

	return taskDir, nil
}

func executeTask(
	ctx context.Context,
	runCtx *RunContext,
	task *flow.Task,
	tasks []flow.Task,
	logger cassandra.Logger,
	parentDir string,
	allocator *taskDirectoryAllocator,
	observer FlowObserver,
) (registry.Result, string, error) {
	if task == nil {
		return registry.Result{}, "", fmt.Errorf("executeTask: task is required")
	}
	if allocator == nil {
		return registry.Result{}, "", fmt.Errorf("executeTask: directory allocator is required")
	}

	taskDir, err := allocator.allocate(parentDir, task.ID)
	if err != nil {
		return registry.Result{}, "", err
	}

	task.Status = flow.TaskStatusInProgress
	task.StartTimestamp = time.Now()

	publishEvent(observer, FlowEvent{
		Type:   FlowEventTaskStarted,
		FlowID: task.FlowID,
		Task:   snapshotTask(task),
	})

	taskLogger := newTaskLogger(logger, observer, task)
	taskLogPrefix := fmt.Sprintf("flow: %s task: %s", task.FlowID, task.ID)

	expandedDescription := task.Description
	if strings.Contains(task.Description, "${") {
		expanded, err := expansion.ExpandString(task.Description, runVariablesToExpansion(runCtx.Snapshot()))
		if err != nil {
			taskLogger.Printf("expanding task description: %v", err)
		} else {
			expandedDescription = expanded
			task.Description = expanded
		}
	}

	startPlain := fmt.Sprintf("[[ Executing %s ]] %s", taskLogPrefix, expandedDescription)
	startColored := fmt.Sprintf("%s[[ Executing %s ]]%s %s", colors.BrightWhite, taskLogPrefix, colors.Reset, expandedDescription)
	taskLogger.PrintColored(startPlain, startColored)

	var (
		expandedPayload json.RawMessage = task.Payload
		actionResult    registry.Result
		execErr         error
		resultType      flow.ResultType
	)

	switch {
	case strings.EqualFold(task.Action, evaluate.ActionName):
		expandedPayload, execErr = expansion.ExpandEvaluateTaskPayload(task.Payload, runCtx.Snapshot(), tasks)
	case strings.EqualFold(task.Action, print.ActionName):
	// PRINT tasks handle interpolation at execution time.
	case strings.EqualFold(task.Action, variables.ActionName):
	// VARIABLES tasks operate on the raw payload to evaluate intra-task references.
	case strings.EqualFold(task.Action, forloop.ActionName):
	// FOR tasks manage variable evaluation within nested executions.
	case strings.EqualFold(task.Action, parallel.ActionName):
		expandedPayload, execErr = expansion.ExpandParallelTaskPayload(task.Payload, runCtx.Snapshot(), tasks)
	default:
		expandedPayload, execErr = expansion.ExpandTaskPayload(task.Payload, runCtx.Snapshot(), tasks)
	}

	if execErr != nil {
		execErr = fmt.Errorf("expanding variables: %w", execErr)
		return finalizeTask(ctx, task, taskLogger, taskLogPrefix, taskDir, runCtx.Snapshot(), execErr, observer)
	}

	actionImpl, found := registry.Lookup(task.Action)
	if !found {
		execErr = fmt.Errorf("unsupported action %q", task.Action)
		return finalizeTask(ctx, task, taskLogger, taskLogPrefix, taskDir, runCtx.Snapshot(), execErr, observer)
	}

	execCtx := runCtx.ExecutionContext(task, tasks, taskLogger)
	execCtx.LogDir = taskDir
	execCtx.ExecuteTask = func(childCtx context.Context, req registry.TaskExecutionRequest) (registry.TaskExecutionResponse, error) {
		if req.Task == nil {
			return registry.TaskExecutionResponse{}, fmt.Errorf("executeTask: nested task is required")
		}

		childRunCtx := &RunContext{}
		if len(req.Variables) == 0 {
			childRunCtx.Replace(runCtx.Snapshot())
		} else {
			childRunCtx.Replace(registryVariablesToRun(req.Variables))
		}

		nestedTasks := req.Tasks
		if len(nestedTasks) == 0 {
			nestedTasks = tasks
		}

		nestedParent := strings.TrimSpace(req.LogDir)
		if nestedParent == "" {
			nestedParent = taskDir
		}

		nestedResult, _, nestedErr := executeTask(childCtx, childRunCtx, req.Task, nestedTasks, logger, nestedParent, allocator, observer)
		if nestedErr != nil {
			return registry.TaskExecutionResponse{}, nestedErr
		}

		return registry.TaskExecutionResponse{
			Result:    nestedResult,
			Variables: runVariablesToRegistry(childRunCtx.Snapshot()),
		}, nil
	}

	actionResult, execErr = actionImpl.Execute(ctx, expandedPayload, execCtx)
	if execErr != nil {
		return finalizeTask(ctx, task, taskLogger, taskLogPrefix, taskDir, runCtx.Snapshot(), execErr, observer)
	}

	runCtx.UpdateFromExecutionContext(execCtx)

	task.EndTimestamp = time.Now()
	task.DurationSeconds = task.EndTimestamp.Sub(task.StartTimestamp).Seconds()
	task.Success = true
	task.Result = actionResult.Value
	task.ResultType = actionResult.Type
	task.Status = flow.TaskStatusCompleted

	resultType = actionResult.Type

	if err := writeTaskArtifacts(taskDir, task, taskLogger.Logs(), runCtx.Snapshot(), ""); err != nil {
		execErr = fmt.Errorf("writing task artifacts: %w", err)
		return finalizeTask(ctx, task, taskLogger, taskLogPrefix, taskDir, runCtx.Snapshot(), execErr, observer)
	}

	taskLogger.PrintColored(
		fmt.Sprintf("[[ %s executed with SUCCESS ]]", taskLogPrefix),
		fmt.Sprintf("%s[[ %s executed with SUCCESS ]]%s", colors.Green, taskLogPrefix, colors.Reset),
	)

	if task.Result == nil {
		task.ResultType = resultType
	}

	publishEvent(observer, FlowEvent{
		Type:   FlowEventTaskCompleted,
		FlowID: task.FlowID,
		Task:   snapshotTask(task),
	})

	updateRunStateFromContext(ctx, task, runCtx.Snapshot())

	return actionResult, taskDir, nil
}

func finalizeTask(ctx context.Context, task *flow.Task, taskLogger *taskLogger, prefix, taskDir string, vars map[string]Variable, err error, observer FlowObserver) (registry.Result, string, error) {
	task.EndTimestamp = time.Now()
	task.DurationSeconds = task.EndTimestamp.Sub(task.StartTimestamp).Seconds()
	task.Success = false
	task.Status = flow.TaskStatusCompleted
	if task.Result == nil {
		task.ResultType = ""
	}

	failurePlain := fmt.Sprintf("[[ %s executed with ERRORS ]]", prefix)
	failureColored := fmt.Sprintf("%s[[ %s executed with ERRORS ]]%s", colors.Red, prefix, colors.Reset)

	if writeErr := writeTaskArtifacts(taskDir, task, taskLogger.Logs(), vars, errorMessage(err)); writeErr != nil {
		taskLogger.PrintColored(failurePlain, failureColored)
		return registry.Result{}, taskDir, fmt.Errorf("writing task artifacts: %v (original error: %w)", writeErr, err)
	}

	taskLogger.PrintColored(failurePlain, failureColored)

	publishEvent(observer, FlowEvent{
		Type:   FlowEventTaskFailed,
		FlowID: task.FlowID,
		Task:   snapshotTask(task),
		Error:  errorMessage(err),
	})

	updateRunStateFromContext(ctx, task, vars)

	return registry.Result{}, taskDir, err
}
