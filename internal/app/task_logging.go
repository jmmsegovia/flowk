package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowk/internal/actions/db/cassandra"
	"flowk/internal/flow"
	expansion "flowk/internal/shared/expansion"
)

type taskLogger struct {
	base     cassandra.Logger
	mu       sync.Mutex
	logs     []string
	observer FlowObserver
	task     *flow.Task
}

func newTaskLogger(base cassandra.Logger, observer FlowObserver, task *flow.Task) *taskLogger {
	return &taskLogger{base: base, observer: observer, task: task}
}

func (l *taskLogger) Printf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.logMessage(message, "")
}

func (l *taskLogger) PrintColored(plain, colored string) {
	l.logMessage(plain, colored)
}

func (l *taskLogger) Logs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	copied := make([]string, len(l.logs))
	copy(copied, l.logs)
	return copied
}

func prepareFlowLogsDir(flowPath string, resume bool) (string, error) {
	flowFile := filepath.Base(flowPath)
	flowName := strings.TrimSuffix(flowFile, filepath.Ext(flowFile))
	flowName = sanitizeForDirectory(flowName)
	if flowName == "" {
		flowName = "flow"
	}

	root := "logs"
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("creating logs root directory: %w", err)
	}

	flowDir := filepath.Join(root, flowName)
	if !resume {
		if err := os.RemoveAll(flowDir); err != nil {
			return "", fmt.Errorf("cleaning logs directory for flow %q: %w", flowName, err)
		}
	}

	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		return "", fmt.Errorf("creating logs directory for flow %q: %w", flowName, err)
	}

	return flowDir, nil
}

func sanitizeForDirectory(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
	)

	sanitized := replacer.Replace(trimmed)
	sanitized = strings.ReplaceAll(sanitized, "..", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")

	return sanitized
}

func (l *taskLogger) logMessage(plain, colored string) {
	if l == nil {
		return
	}

	if colored == "" {
		colored = plain
	}

	if l.base != nil {
		l.base.Printf("%s", colored)
	}

	l.mu.Lock()
	l.logs = append(l.logs, plain)
	l.mu.Unlock()

	if l.observer != nil {
		publishEvent(l.observer, FlowEvent{
			Type:    FlowEventTaskLog,
			FlowID:  l.task.FlowID,
			Task:    snapshotTask(l.task),
			Message: plain,
		})
	}
}

func writeTaskArtifacts(dir string, task *flow.Task, logs []string, vars map[string]Variable, errMessage string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensuring task directory %q: %w", dir, err)
	}

	payload := taskLogPayload{
		ID:              task.ID,
		Description:     task.Description,
		Action:          task.Action,
		Status:          task.Status,
		Success:         task.Success,
		StartTimestamp:  task.StartTimestamp,
		EndTimestamp:    task.EndTimestamp,
		DurationSeconds: task.DurationSeconds,
		ResultType:      task.ResultType,
		Result:          task.Result,
		Error:           strings.TrimSpace(errMessage),
		Logs:            logs,
	}

	if resMap, ok := task.Result.(map[string]any); ok {
		payload.Variables = resMap
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling task log: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "task_log.json"), data, 0o644); err != nil {
		return fmt.Errorf("writing task log: %w", err)
	}

	snapshot := snapshotVariables(vars)
	snapshotData, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling variables snapshot: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "environment_variables.json"), snapshotData, 0o644); err != nil {
		return fmt.Errorf("writing variables snapshot: %w", err)
	}

	return nil
}

type taskLogPayload struct {
	ID              string          `json:"id"`
	Description     string          `json:"description"`
	Action          string          `json:"action"`
	Status          flow.TaskStatus `json:"status"`
	Success         bool            `json:"success"`
	StartTimestamp  time.Time       `json:"start_timestamp"`
	EndTimestamp    time.Time       `json:"end_timestamp"`
	DurationSeconds float64         `json:"duration_seconds"`
	ResultType      flow.ResultType `json:"result_type"`
	Result          any             `json:"result"`
	Error           string          `json:"error,omitempty"`
	Variables       map[string]any  `json:"variables,omitempty"`
	Logs            []string        `json:"logs"`
}

type variableSnapshot struct {
	Type   string `json:"type"`
	Secret bool   `json:"secret"`
	Value  any    `json:"value"`
}

func snapshotVariables(vars map[string]Variable) map[string]variableSnapshot {
	if len(vars) == 0 {
		return map[string]variableSnapshot{}
	}

	masked := make(map[string]Variable, len(vars))
	for name, variable := range vars {
		maskedVar := variable
		if maskedVar.Secret {
			maskedVar.Value = "<secret>"
		}
		masked[name] = maskedVar
	}

	snapshot := make(map[string]variableSnapshot, len(vars))
	for name, variable := range vars {
		value := any("<secret>")
		if !variable.Secret {
			expanded, err := expansion.ExpandValue(variable.Value, masked)
			if err != nil {
				expanded = variable.Value
			}
			value = expanded
		}

		snapshot[name] = variableSnapshot{
			Type:   variable.Type,
			Secret: variable.Secret,
			Value:  value,
		}
	}

	return snapshot
}
