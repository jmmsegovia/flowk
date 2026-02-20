package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "flowk/internal/actions/auth/gmail"
	_ "flowk/internal/actions/auth/oauth2"
	"flowk/internal/actions/core/evaluate"
	_ "flowk/internal/actions/core/forloop"
	_ "flowk/internal/actions/core/parallel"
	_ "flowk/internal/actions/core/sleep"
	"flowk/internal/actions/core/variables"
	"flowk/internal/actions/db/cassandra"
	_ "flowk/internal/actions/db/mysql"
	_ "flowk/internal/actions/db/postgres"
	_ "flowk/internal/actions/infra/helm"
	_ "flowk/internal/actions/infra/kubernetes"
	_ "flowk/internal/actions/network/httpclient"
	_ "flowk/internal/actions/network/ssh"
	_ "flowk/internal/actions/network/telnet"
	_ "flowk/internal/actions/security/pgp"
	_ "flowk/internal/actions/storage/gcloudstorage"
	_ "flowk/internal/actions/system/base64"
	_ "flowk/internal/actions/system/docker"
	_ "flowk/internal/actions/system/shell"
	"flowk/internal/flow"
	"flowk/internal/shared/runcontext"
)

// Run loads the flow definition and executes the requested actions.
func Run(ctx context.Context, flowPath string, logger cassandra.Logger, startTaskID, singleTaskID, runFlowID, runSubtaskID string) error {
	observer := observerFromContext(ctx)

	definition, err := flow.LoadDefinition(flowPath)
	if err != nil {
		publishEvent(observer, FlowEvent{
			Type:   FlowEventFlowFinished,
			FlowID: "",
			Error:  err.Error(),
		})
		return err
	}

	publishEvent(observer, FlowEvent{
		Type:   FlowEventFlowLoaded,
		FlowID: definition.ID,
	})

	err = runDefinition(ctx, definition, flowPath, logger, startTaskID, singleTaskID, runFlowID, runSubtaskID, observer)
	publishEvent(observer, FlowEvent{
		Type:   FlowEventFlowFinished,
		FlowID: definition.ID,
		Error:  errorMessage(err),
	})

	return err
}

// ValidateFlow loads the flow definition to ensure it is structurally valid.
func ValidateFlow(flowPath string) error {
	_, err := flow.LoadDefinition(flowPath)
	return err
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runDefinition(ctx context.Context, definition *flow.Definition, flowPath string, logger cassandra.Logger, startTaskID, singleTaskID, runFlowID, runSubtaskID string, observer FlowObserver) error {
	if definition == nil {
		return fmt.Errorf("definition is required")
	}

	var (
		allowedFlows     map[string]struct{}
		firstAllowedTask int = -1
		err              error
	)
	if trimmed := strings.TrimSpace(runFlowID); trimmed != "" {
		if strings.TrimSpace(startTaskID) != "" || strings.TrimSpace(singleTaskID) != "" || strings.TrimSpace(runSubtaskID) != "" {
			return fmt.Errorf("run-flow cannot be combined with begin-from-task, run-task, or run-subtask")
		}

		allowedFlows, err = definition.FlowsForExecution(trimmed)
		if err != nil {
			return err
		}

		for idx := range definition.Tasks {
			if _, run := allowedFlows[definition.Tasks[idx].FlowID]; run {
				firstAllowedTask = idx
				break
			}
		}
	}

	runCtx := RunContext{
		Vars: make(map[string]Variable),
	}
	runState := RunStateFromContext(ctx)
	resumeRequested := strings.TrimSpace(startTaskID) != "" ||
		strings.TrimSpace(singleTaskID) != "" ||
		strings.TrimSpace(runFlowID) != "" ||
		strings.TrimSpace(runSubtaskID) != ""
	isResume := runState != nil && runState.HasData() && resumeRequested
	if isResume {
		ctx = runcontext.WithResume(ctx)
	}
	if runState != nil && !isResume {
		runState.Reset()
	}
	if runState != nil && runState.HasVariables() {
		runCtx.Replace(runState.SnapshotVariables())
	}
	if runState != nil {
		runState.ApplyToDefinition(definition)
	}

	if trimmed := strings.TrimSpace(runSubtaskID); trimmed != "" {
		if strings.TrimSpace(startTaskID) != "" || strings.TrimSpace(singleTaskID) != "" {
			return fmt.Errorf("run-subtask cannot be combined with begin-from-task or run-task")
		}
	}

	flowLogsDir, err := prepareFlowLogsDir(flowPath, isResume)
	if err != nil {
		return err
	}

	flowDirectories := map[string]string{
		definition.ID: flowLogsDir,
	}

	flowParents := make(map[string]string)
	for parent, imports := range definition.FlowImports {
		for _, imported := range imports {
			if _, exists := flowParents[imported]; !exists {
				flowParents[imported] = parent
			}
		}
	}

	var resolveFlowDir func(string) (string, error)
	resolveFlowDir = func(flowID string) (string, error) {
		if dir, exists := flowDirectories[flowID]; exists {
			return dir, nil
		}

		parentDir := flowLogsDir
		if parentID, hasParent := flowParents[flowID]; hasParent {
			resolved, err := resolveFlowDir(parentID)
			if err != nil {
				return "", err
			}
			parentDir = resolved
		}

		sanitized := sanitizeForDirectory(flowID)
		if sanitized == "" {
			sanitized = "flow"
		}

		dir := filepath.Join(parentDir, sanitized)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating logs directory for flow %q: %w", flowID, err)
		}

		flowDirectories[flowID] = dir
		return dir, nil
	}

	startIdx := 0
	endIdx := len(definition.Tasks)
	requestedStartIdx := 0

	cleanupFlowID := strings.TrimSpace(definition.OnErrorFlow)
	cleanupStartIdx, cleanupEndIdx := -1, -1
	if cleanupFlowID != "" {
		cleanupStartIdx, cleanupEndIdx = findFlowTaskRange(definition.Tasks, cleanupFlowID)
		if cleanupStartIdx < 0 {
			return fmt.Errorf("on_error_flow %q not found in flow definition", cleanupFlowID)
		}
	}

	finallyFlowID := strings.TrimSpace(definition.FinallyFlow)
	finallyFlowStartIdx, finallyFlowEndIdx := -1, -1
	if finallyFlowID != "" {
		finallyFlowStartIdx, finallyFlowEndIdx = findFlowTaskRange(definition.Tasks, finallyFlowID)
		if finallyFlowStartIdx < 0 {
			return fmt.Errorf("finally_flow %q not found in flow definition", finallyFlowID)
		}
	}

	finallyTaskID := strings.TrimSpace(definition.FinallyTask)
	finallyTaskIdx := -1
	if finallyTaskID != "" {
		finallyTaskIdx = findTaskIndexByID(definition.Tasks, finallyTaskID)
		if finallyTaskIdx < 0 {
			return fmt.Errorf("finally_task %q not found in flow definition", finallyTaskID)
		}
	}

	var (
		originalErr               error
		cleanupScheduled          bool
		cleanupFlowExplicitlyUsed bool = strings.TrimSpace(runFlowID) == cleanupFlowID
	)

	if trimmed := strings.TrimSpace(singleTaskID); trimmed != "" {
		targetIdx := findTaskIndexByID(definition.Tasks, trimmed)
		if targetIdx < 0 {
			return fmt.Errorf("run-task: task id %q not found in flow definition", trimmed)
		}
		startIdx = targetIdx
		endIdx = targetIdx + 1
		requestedStartIdx = targetIdx
	} else if trimmed := strings.TrimSpace(startTaskID); trimmed != "" {
		targetIdx := findTaskIndexByID(definition.Tasks, trimmed)
		if targetIdx < 0 {
			return fmt.Errorf("begin-from-task: task id %q not found in flow definition", trimmed)
		}
		startIdx = targetIdx
		requestedStartIdx = targetIdx
	}

	allocator := &taskDirectoryAllocator{}
	runVariableTask := func(task *flow.Task, label string) error {
		if task == nil || !strings.EqualFold(task.Action, variables.ActionName) {
			return nil
		}

		taskFlowDir, err := resolveFlowDir(task.FlowID)
		if err != nil {
			return fmt.Errorf("%s: resolving flow directory: %w", label, err)
		}
		_, _, execErr := executeTask(ctx, &runCtx, task, definition.Tasks, logger, taskFlowDir, allocator, observer)
		if execErr != nil {
			return fmt.Errorf("%s: %w", label, execErr)
		}
		return nil
	}

	runFlowVariablesBefore := func(endIdx int) error {
		if endIdx <= 0 {
			return nil
		}
		for i := 0; i < endIdx; i++ {
			if err := runVariableTask(&definition.Tasks[i], fmt.Sprintf("tasks[%d]", i)); err != nil {
				return err
			}
		}
		return nil
	}

	runParentVariablesBefore := func(parent *flow.Task, subtaskID string) error {
		if parent == nil {
			return nil
		}

		children, err := extractSubtasks(parent)
		if err != nil {
			return fmt.Errorf("subtask %q: %w", subtaskID, err)
		}

		found := false
		for i := range children {
			child := children[i]
			if strings.TrimSpace(child.FlowID) == "" {
				child.FlowID = parent.FlowID
				children[i] = child
			}

			if strings.TrimSpace(child.ID) == subtaskID {
				found = true
				break
			}

			if err := runVariableTask(&children[i], fmt.Sprintf("subtask %q", child.ID)); err != nil {
				return err
			}
		}

		if !found {
			return fmt.Errorf("run-subtask: subtask id %q not found in parent task %q", subtaskID, parent.ID)
		}

		return nil
	}

	finallyExecuted := false
	runFinally := func(prevErr error) error {
		if finallyExecuted || (finallyFlowStartIdx < 0 && finallyTaskIdx < 0) {
			return prevErr
		}
		finallyExecuted = true

		runTask := func(task *flow.Task, idx int) error {
			taskFlowDir, err := resolveFlowDir(task.FlowID)
			if err != nil {
				return fmt.Errorf("tasks[%d]: resolving flow directory: %w", idx, err)
			}
			_, _, execErr := executeTask(ctx, &runCtx, task, definition.Tasks, logger, taskFlowDir, allocator, observer)
			if execErr != nil {
				return fmt.Errorf("tasks[%d]: %w", idx, execErr)
			}
			return nil
		}

		if finallyFlowStartIdx >= 0 {
			for i := finallyFlowStartIdx; i <= finallyFlowEndIdx; i++ {
				if err := runTask(&definition.Tasks[i], i); err != nil {
					if prevErr != nil {
						return fmt.Errorf("finally_flow %q failed: %v (original error: %w)", finallyFlowID, err, prevErr)
					}
					return err
				}
			}
		}

		if finallyTaskIdx >= 0 {
			if err := runTask(&definition.Tasks[finallyTaskIdx], finallyTaskIdx); err != nil {
				if prevErr != nil {
					return fmt.Errorf("finally_task %q failed: %v (original error: %w)", finallyTaskID, err, prevErr)
				}
				return err
			}
		}

		return prevErr
	}

	publishEvent(observer, FlowEvent{
		Type:   FlowEventFlowStarted,
		FlowID: definition.ID,
	})

	if trimmed := strings.TrimSpace(runSubtaskID); trimmed != "" {
		match, err := findSubtaskForRun(definition.Tasks, trimmed)
		if err != nil {
			return err
		}

		subtask := match.task
		if strings.TrimSpace(subtask.FlowID) == "" {
			subtask.FlowID = definition.ID
		}

		if match.root != nil {
			rootIdx := findTaskIndexByID(definition.Tasks, match.root.ID)
			if rootIdx >= 0 {
				if err := runFlowVariablesBefore(rootIdx); err != nil {
					return runFinally(err)
				}
			}
		}

		if err := runParentVariablesBefore(match.parent, trimmed); err != nil {
			return runFinally(err)
		}

		taskFlowDir, err := resolveFlowDir(subtask.FlowID)
		if err != nil {
			return fmt.Errorf("subtask %q: resolving flow directory: %w", trimmed, err)
		}

		_, _, execErr := executeTask(ctx, &runCtx, &subtask, definition.Tasks, logger, taskFlowDir, allocator, observer)
		if execErr != nil {
			return runFinally(execErr)
		}
		return runFinally(nil)
	}

	loopStartIdx := startIdx
	if requestedStartIdx > 0 {
		loopStartIdx = 0
	}

	stopAtTaskID := strings.TrimSpace(runcontext.StopAtTaskID(ctx))
	skipStopAtOnce := stopAtTaskID != "" && stopAtTaskID == strings.TrimSpace(startTaskID)

	stopRequested := false
	for idx := loopStartIdx; idx < endIdx; idx++ {
		if runcontext.IsStopRequested(ctx) {
			stopRequested = true
			break
		}
		task := &definition.Tasks[idx]

		if cleanupFlowID != "" && !cleanupScheduled && !cleanupFlowExplicitlyUsed && task.FlowID == cleanupFlowID {
			continue
		}

		if len(allowedFlows) > 0 {
			if _, run := allowedFlows[task.FlowID]; !run {
				if firstAllowedTask >= 0 && idx < firstAllowedTask && strings.EqualFold(task.Action, variables.ActionName) {
					// Allow variable declarations that precede the first selected flow task
					// so the requested flow has access to the required values.
				} else {
					continue
				}
			}
		}

		if idx < requestedStartIdx && !strings.EqualFold(task.Action, variables.ActionName) {
			continue
		}

		taskFlowDir, err := resolveFlowDir(task.FlowID)
		if err != nil {
			return fmt.Errorf("tasks[%d]: resolving flow directory: %w", idx, err)
		}

		actionResult, _, err := executeTask(ctx, &runCtx, task, definition.Tasks, logger, taskFlowDir, allocator, observer)
		if err != nil {
			wrappedErr := fmt.Errorf("tasks[%d]: %w", idx, err)
			if originalErr == nil {
				originalErr = wrappedErr
			}

			if cleanupStartIdx >= 0 && (idx < cleanupStartIdx || idx > cleanupEndIdx) {
				if !cleanupScheduled {
					cleanupScheduled = true
					endIdx = cleanupEndIdx + 1
				}
				idx = cleanupStartIdx - 1
				continue
			}

			if cleanupScheduled && idx >= cleanupStartIdx && idx <= cleanupEndIdx {
				return runFinally(fmt.Errorf("on_error_flow %q failed: %v (original error: %w)", cleanupFlowID, err, originalErr))
			}

			return runFinally(originalErr)
		}

		if stopAtTaskID != "" && stopAtTaskID == task.ID {
			if skipStopAtOnce {
				skipStopAtOnce = false
			} else if stopSignal := runcontext.StopSignalFromContext(ctx); stopSignal != nil {
				stopSignal.Request()
			}
		}

		if runcontext.IsStopRequested(ctx) {
			stopRequested = true
			break
		}

		if control := actionResult.Control; control != nil {
			if trimmedID := strings.TrimSpace(control.JumpToTaskID); trimmedID != "" {
				targetIdx := findTaskIndexByID(definition.Tasks, trimmedID)
				if targetIdx < 0 {
					var controlErr error
					if strings.EqualFold(task.Action, evaluate.ActionName) {
						controlErr = fmt.Errorf("evaluate task: branch requested to go to unknown task %q", trimmedID)
					} else {
						controlErr = fmt.Errorf("action %q requested to go to unknown task %q", task.Action, trimmedID)
					}
					return fmt.Errorf("tasks[%d]: %w", idx, controlErr)
				}
				idx = targetIdx - 1
			}
			if control.Exit {
				idx = endIdx
			}
		}
	}

	logFlowSummary(logger, definition.Tasks)

	if stopRequested {
		return nil
	}

	if originalErr != nil {
		return runFinally(originalErr)
	}

	return runFinally(nil)
}

func findTaskByID(tasks []flow.Task, id string) *flow.Task {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil
	}

	for i := range tasks {
		if tasks[i].ID == trimmedID {
			return &tasks[i]
		}
	}

	return nil
}

func findTaskIndexByID(tasks []flow.Task, id string) int {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return -1
	}

	for i := range tasks {
		if tasks[i].ID == trimmedID {
			return i
		}
	}

	return -1
}

func findFlowTaskRange(tasks []flow.Task, flowID string) (int, int) {
	trimmedID := strings.TrimSpace(flowID)
	if trimmedID == "" {
		return -1, -1
	}

	startIdx, endIdx := -1, -1
	for i := range tasks {
		if tasks[i].FlowID != trimmedID {
			continue
		}
		if startIdx == -1 {
			startIdx = i
		}
		endIdx = i
	}

	return startIdx, endIdx
}

func logFlowSummary(logger cassandra.Logger, tasks []flow.Task) {
	if logger == nil {
		return
	}

	summaryLogger := newTaskLogger(logger, nil, nil)
	for i := range tasks {
		task := tasks[i]
		summaryLogger.Printf("Task %s (%s) - Status: %s", task.ID, task.Description, task.Status)
	}
}
