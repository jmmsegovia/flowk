package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

type bufferLogger struct {
	mu     sync.Mutex
	buffer []string
}

func (b *bufferLogger) Printf(format string, args ...interface{}) {
	b.mu.Lock()
	b.buffer = append(b.buffer, fmt.Sprintf(format, args...))
	b.mu.Unlock()
}

func (b *bufferLogger) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.buffer, "\n")
}

func findTaskDir(t *testing.T, flowDir, taskID string) string {
	t.Helper()

	entries, err := os.ReadDir(flowDir)
	if err != nil {
		t.Fatalf("reading flow directory: %v", err)
	}

	sanitized := sanitizeForDirectory(taskID)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.Contains(entry.Name(), sanitized) {
			return filepath.Join(flowDir, entry.Name())
		}
	}

	t.Fatalf("task directory for %s not found in %s", taskID, flowDir)
	return ""
}

func TestRunBeginsFromSpecifiedTask(t *testing.T) {
	flowPath := writeFlow(t)

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "task2", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if !strings.Contains(logs, "Task task1 (First sleep task) - Status: not started") {
		t.Fatalf("expected task1 to remain not started, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task task2 (Second sleep task) - Status: completed") {
		t.Fatalf("expected task2 to complete, logs: %s", logs)
	}
}

func TestTaskDescriptionsExpandVariables(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "description.expansion",
                "description": "Expand task descriptions",
                "tasks": [
                        {
                                "id": "loop",
                                "description": "Iterate over key names",
                                "action": "FOR",
                                "variable": "key_name",
                                "values": [
                                        "VALUE1"
                                ],
                                "tasks": [
                                        {
                                                "id": "export_key",
                                                "description": "Export key ${key_name}",
                                                "action": "PRINT",
                                                "entries": [
                                                        {"message": "Key ${key_name}"}
                                                ]
                                        }
                                ]
                        }
                ]
        }`)

	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if !strings.Contains(logs, "Export key VALUE1") {
		t.Fatalf("expected description to expand variable, logs: %s", logs)
	}
	if strings.Contains(logs, "${key_name}") {
		t.Fatalf("expected placeholders to be expanded in description, logs: %s", logs)
	}
}

func TestRunFailsWhenBeginTaskNotFound(t *testing.T) {
	flowPath := writeFlow(t)

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "missing", "", "", ""); err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}

func TestRunEvaluateBranchActions(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "evaluate.branch.actions",
                "description": "evaluate branch actions",
                "tasks": [
                        {"id":"sleep1","description":"Initial sleep","action":"SLEEP","seconds":0.01},
                        {"id":"eval_then","description":"Evaluate success continue","action":"EVALUATE","if_conditions":[{"field":"${from.task:sleep1.success}","operation":"=","expected":true}],"then":{"continue":"All keys ACTIVE"},"else":{"continue":""}},
                        {"id":"sleep2","description":"Second sleep","action":"SLEEP","seconds":0.01},
                        {"id":"eval_else","description":"Evaluate failure branch","action":"EVALUATE","if_conditions":[{"field":"${from.task:sleep1.success}","operation":"=","expected":false}],"then":{"continue":""},"else":{"sleep":0.01,"gototask":"sleep4"}},
                        {"id":"sleep3","description":"Skipped sleep","action":"SLEEP","seconds":0.01},
                        {"id":"sleep4","description":"Final sleep","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if count := strings.Count(logs, "Sleeping for 0.01 seconds"); count != 4 {
		t.Fatalf("expected 4 sleep logs, got %d, logs: %s", count, logs)
	}
	if !strings.Contains(logs, "Task sleep2 (Second sleep) - Status: completed") {
		t.Fatalf("expected sleep2 to complete via continue branch, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task sleep3 (Skipped sleep) - Status: not started") {
		t.Fatalf("expected sleep3 to be skipped by goto branch, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task sleep4 (Final sleep) - Status: completed") {
		t.Fatalf("expected sleep4 to complete after goto, logs: %s", logs)
	}
	if !strings.Contains(logs, "Evaluate then branch continue: All keys ACTIVE") {
		t.Fatalf("expected continue message to be logged, logs: %s", logs)
	}
}

func TestRunEvaluateExitStopsFlow(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "evaluate.exit.action",
                "description": "evaluate exit action",
                "tasks": [
                        {"id":"sleep1","description":"Initial sleep","action":"SLEEP","seconds":0.01},
                        {"id":"eval_exit","description":"Evaluate exit","action":"EVALUATE","if_conditions":[{"field":"${from.task:sleep1.success}","operation":"=","expected":true}],"then":{"exit":"exiting from test because of ERROR 0012545"},"else":{"continue":""}},
                        {"id":"sleep2","description":"Should not run","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if strings.Contains(logs, "Task sleep2 (Should not run) - Status: completed") {
		t.Fatalf("expected sleep2 to be skipped due to exit, logs: %s", logs)
	}
	if !strings.Contains(logs, "Evaluate then branch exit: exiting from test because of ERROR 0012545") {
		t.Fatalf("expected exit message to be logged, logs: %s", logs)
	}
}

func TestRunExecutesOnErrorFlow(t *testing.T) {
	dir := t.TempDir()
	cleanupPath := filepath.Join(dir, "cleanup.json")
	cleanupContent := []byte(`{"id":"cleanup.flow","description":"cleanup flow","tasks":[{"id":"cleanup","description":"run cleanup","action":"PRINT","entries":[{"message":"cleanup"}]}]}`)
	if err := os.WriteFile(cleanupPath, cleanupContent, 0o600); err != nil {
		t.Fatalf("writing cleanup flow: %v", err)
	}

	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "onerror.cleanup",
                "description": "ensure cleanup runs",
                "imports": ["cleanup.json"],
                "on_error_flow": "cleanup.flow",
                "tasks": [
                        {"id":"fail","description":"first task fails","action":"SHELL","command":["false"]},
                        {"id":"skipped","description":"should be skipped","action":"PRINT","entries":[{"message":"skip"}]}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := Run(ctx, flowPath, logger, "", "", "", "")
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}

	logs := logger.String()
	t.Logf("Run error: %v", err)
	if !strings.Contains(logs, "Task cleanup (run cleanup) - Status: completed") {
		t.Fatalf("expected cleanup task to execute, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task skipped (should be skipped) - Status: not started") {
		t.Fatalf("expected skipped task to remain not started, logs: %s", logs)
	}
}

func TestRunVariablesTaskSupportsIntraTaskReferences(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "variables.intra.references",
                "description": "variables with intra-task references",
                "tasks": [
                        {
                                "id": "vars.namespace",
                                "description": "Declare namespace used in tests",
                                "action": "VARIABLES",
                                "scope": "flow",
                                "overwrite": true,
                                "vars": [
                                        {"name": "platform_name", "type": "string", "value": "tic-dev08"},
                                        {"name": "k8_namespace", "type": "string", "value": "tic-${platform_name}"}
                                ]
                        },
                        {
                                "id": "print.namespace",
                                "description": "Log namespace",
                                "action": "PRINT",
                                "entries": [
                                        {"message": "Namespace", "value": "${k8_namespace}"}
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := flow.LoadDefinition(flowPath); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if err := Run(ctx, flowPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if !strings.Contains(logs, "Namespace: tic-tic-dev08") {
		t.Fatalf("expected namespace log, got logs: %s", logs)
	}
}

func TestParallelActionExecutesSubtasks(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "parallel_flow.json")

	flowContent := []byte(`{
                "id": "parallel.flow.test",
                "description": "parallel action demo",
                "tasks": [
                        {
                                "id": "setup",
                                "description": "Initialize variables",
                                "action": "VARIABLES",
                                "scope": "flow",
                                "overwrite": true,
                                "vars": [
                                        {"name": "initial_value", "type": "string", "value": "base"}
                                ]
                        },
                        {
                                "id": "parallel.work",
                                "description": "Execute subtasks concurrently",
                                "action": "PARALLEL",
                                "fail_fast": false,
                                "merge_strategy": "last_write_wins",
                                "merge_order": ["parallel.a", "parallel.b"],
                                "tasks": [
                                        {
                                                "id": "parallel.a",
                                                "description": "Set value from branch A",
                                                "action": "VARIABLES",
                                                "scope": "flow",
                                                "overwrite": true,
                                                "vars": [
                                                        {"name": "parallel_value", "type": "string", "value": "from_a"}
                                                ]
                                        },
                                        {
                                                "id": "parallel.b",
                                                "description": "Set value from branch B",
                                                "action": "VARIABLES",
                                                "scope": "flow",
                                                "overwrite": true,
                                                "vars": [
                                                        {"name": "parallel_value", "type": "string", "value": "from_b"}
                                                ]
                                        },
                                        {
                                                "id": "parallel.log",
                                                "description": "Log the base value",
                                                "action": "PRINT",
                                                "entries": [
                                                        {"message": "Base value", "value": "${initial_value}"}
                                                ]
                                        }
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	def, err := flow.LoadDefinition(flowPath)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runDefinition(ctx, def, flowPath, logger, "", "", "", "", nil); err != nil {
		t.Fatalf("runDefinition() error = %v", err)
	}

	parallelTask := findTaskByID(def.Tasks, "parallel.work")
	if parallelTask == nil {
		t.Fatalf("expected to find parallel.work task")
	}

	if parallelTask.ResultType != flow.ResultTypeJSON {
		t.Fatalf("parallel task result type = %s, want %s", parallelTask.ResultType, flow.ResultTypeJSON)
	}

	aggregated, ok := parallelTask.Result.(map[string]map[string]any)
	if !ok {
		t.Fatalf("parallel task result = %T, want map[string]map[string]any", parallelTask.Result)
	}

	entryA, ok := aggregated["parallel.a"]
	if !ok {
		t.Fatalf("parallel.a entry missing or invalid: %v", aggregated["parallel.a"])
	}
	resultA, ok := entryA["result"].(map[string]any)
	if !ok || resultA["parallel_value"] != "from_a" {
		t.Fatalf("parallel.a result = %v, want parallel_value=from_a", entryA["result"])
	}

	entryB, ok := aggregated["parallel.b"]
	if !ok {
		t.Fatalf("parallel.b entry missing or invalid: %v", aggregated["parallel.b"])
	}
	resultB, ok := entryB["result"].(map[string]any)
	if !ok || resultB["parallel_value"] != "from_b" {
		t.Fatalf("parallel.b result = %v, want parallel_value=from_b", entryB["result"])
	}

	if entryLog, ok := aggregated["parallel.log"]; !ok || len(entryLog) == 0 {
		t.Fatalf("parallel.log entry missing or empty: %v", entryLog)
	}

	flowName := strings.TrimSuffix(filepath.Base(flowPath), filepath.Ext(flowPath))
	parallelDir := filepath.Join("logs", sanitizeForDirectory(flowName), fmt.Sprintf("task-%04d-%s", 1, sanitizeForDirectory("parallel.work")))
	defer os.RemoveAll(filepath.Join("logs", sanitizeForDirectory(flowName)))

	envData, err := os.ReadFile(filepath.Join(parallelDir, "environment_variables.json"))
	if err != nil {
		t.Fatalf("reading environment snapshot: %v", err)
	}

	var env map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(envData, &env); err != nil {
		t.Fatalf("unmarshalling environment snapshot: %v", err)
	}

	if got := env["parallel_value"].Value; got != "from_b" {
		t.Fatalf("parallel_value after merge = %v, want from_b", got)
	}
	if got := env["initial_value"].Value; got != "base" {
		t.Fatalf("initial_value lost after merge: %v", got)
	}

	varsDir := filepath.Join(parallelDir, "variables")
	if _, err := os.Stat(varsDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no per-variable snapshots directory in %s", varsDir)
	}

	subTasksDir := filepath.Join(parallelDir, "task_parallel")
	entries, err := os.ReadDir(subTasksDir)
	if err != nil {
		t.Fatalf("reading subtask directories: %v", err)
	}

	expectedSubtasks := map[string]bool{
		sanitizeForDirectory("parallel.a"):   false,
		sanitizeForDirectory("parallel.b"):   false,
		sanitizeForDirectory("parallel.log"): false,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for id := range expectedSubtasks {
			if strings.Contains(entry.Name(), id) {
				expectedSubtasks[id] = true
			}
		}

		taskDir := filepath.Join(subTasksDir, entry.Name())
		if _, err := os.Stat(filepath.Join(taskDir, "task_log.json")); err != nil {
			t.Fatalf("expected task_log.json in %s: %v", taskDir, err)
		}

		if _, err := os.Stat(filepath.Join(taskDir, "environment_variables.json")); err != nil {
			t.Fatalf("expected environment snapshot in %s: %v", taskDir, err)
		}

		if _, err := os.Stat(filepath.Join(taskDir, "variables")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected variables directory to be removed in %s", taskDir)
		}

		logsDir := filepath.Join(taskDir, "logs")
		if _, err := os.Stat(logsDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected logs directory to be removed in %s", taskDir)
		}

		if strings.Contains(entry.Name(), sanitizeForDirectory("parallel.log")) {
			taskLogPath := filepath.Join(taskDir, "task_log.json")
			data, err := os.ReadFile(taskLogPath)
			if err != nil {
				t.Fatalf("reading task log: %v", err)
			}

			var payload struct {
				Logs []string `json:"logs"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshalling task log: %v", err)
			}

			found := false
			for _, entry := range payload.Logs {
				if strings.Contains(entry, "Base value") {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected task log to contain base value message, got %v", payload.Logs)
			}
		}
	}

	for id, found := range expectedSubtasks {
		if !found {
			t.Fatalf("expected subtask directory containing %s", id)
		}
	}
}

func TestRunSubtaskExecutesParallelChild(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "parallel.subtask.run",
                "description": "run a parallel subtask directly",
                "tasks": [
                        {
                                "id": "parallel.work",
                                "description": "Execute subtasks concurrently",
                                "action": "PARALLEL",
                                "tasks": [
                                        {
                                                "id": "parallel.a",
                                                "description": "Set value from subtask",
                                                "action": "VARIABLES",
                                                "scope": "flow",
                                                "overwrite": true,
                                                "vars": [
                                                        {"name": "subtask_value", "type": "string", "value": "ok"}
                                                ]
                                        },
                                        {
                                                "id": "parallel.b",
                                                "description": "Noop",
                                                "action": "PRINT",
                                                "entries": [
                                                        {"message": "noop"}
                                                ]
                                        }
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", "parallel.a"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	flowName := strings.TrimSuffix(filepath.Base(flowPath), filepath.Ext(flowPath))
	subtaskDir := filepath.Join("logs", sanitizeForDirectory(flowName), fmt.Sprintf("task-%04d-%s", 0, sanitizeForDirectory("parallel.a")))
	defer os.RemoveAll(filepath.Join("logs", sanitizeForDirectory(flowName)))

	envData, err := os.ReadFile(filepath.Join(subtaskDir, "environment_variables.json"))
	if err != nil {
		t.Fatalf("reading environment snapshot: %v", err)
	}

	var env map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(envData, &env); err != nil {
		t.Fatalf("unmarshalling environment snapshot: %v", err)
	}

	if got := env["subtask_value"].Value; got != "ok" {
		t.Fatalf("subtask_value = %v, want ok", got)
	}
}

func TestRunSubtaskExecutesFlowAndParentVariables(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "subtask.variables.run",
                "description": "run subtask with flow and parent variables",
                "tasks": [
                        {
                                "id": "vars.flow",
                                "description": "Set base value",
                                "action": "VARIABLES",
                                "scope": "flow",
                                "vars": [
                                        {"name": "base", "type": "string", "value": "ok"}
                                ]
                        },
                        {
                                "id": "parallel.work",
                                "description": "Execute subtasks concurrently",
                                "action": "PARALLEL",
                                "tasks": [
                                        {
                                                "id": "vars.inner",
                                                "description": "Set inner value",
                                                "action": "VARIABLES",
                                                "scope": "flow",
                                                "vars": [
                                                        {"name": "inner", "type": "string", "value": "${base}"}
                                                ]
                                        },
                                        {
                                                "id": "parallel.target",
                                                "description": "Target subtask",
                                                "action": "PRINT",
                                                "entries": [
                                                        {"message": "inner=${inner}"}
                                                ]
                                        }
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", "parallel.target"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	flowName := strings.TrimSuffix(filepath.Base(flowPath), filepath.Ext(flowPath))
	flowDir := filepath.Join("logs", sanitizeForDirectory(flowName))
	defer os.RemoveAll(flowDir)

	taskDir := findTaskDir(t, flowDir, "parallel.target")
	envData, err := os.ReadFile(filepath.Join(taskDir, "environment_variables.json"))
	if err != nil {
		t.Fatalf("reading environment snapshot: %v", err)
	}

	var env map[string]struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(envData, &env); err != nil {
		t.Fatalf("unmarshalling environment snapshot: %v", err)
	}

	if got := env["base"].Value; got != "ok" {
		t.Fatalf("base = %v, want ok", got)
	}
	if got := env["inner"].Value; got != "ok" {
		t.Fatalf("inner = %v, want ok", got)
	}
}

func TestRunFlowFlagExecutesSelectedFlowAndImports(t *testing.T) {
	dir := t.TempDir()

	nestedPath := filepath.Join(dir, "nested.json")
	nestedContent := []byte(`{"id":"nested.flow","description":"nested","tasks":[{"id":"nested.task","description":"Nested sleep","action":"SLEEP","seconds":0.01}]}`)
	if err := os.WriteFile(nestedPath, nestedContent, 0o600); err != nil {
		t.Fatalf("writing nested flow: %v", err)
	}

	importedPath := filepath.Join(dir, "imported.json")
	importedContent := []byte(`{"id":"imported.flow","description":"imported","imports":["nested.json"],"tasks":[{"id":"imported.task","description":"Imported sleep","action":"SLEEP","seconds":0.01}]}`)
	if err := os.WriteFile(importedPath, importedContent, 0o600); err != nil {
		t.Fatalf("writing imported flow: %v", err)
	}

	rootPath := filepath.Join(dir, "root.json")
	rootContent := []byte(`{"id":"root.flow","description":"root","imports":["imported.json"],"tasks":[{"id":"local.task","description":"Local sleep","action":"SLEEP","seconds":0.01}]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("writing root flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, rootPath, logger, "", "", "imported.flow", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if !strings.Contains(logs, "Task imported.task (Imported sleep) - Status: completed") {
		t.Fatalf("expected imported task to run, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task nested.task (Nested sleep) - Status: completed") {
		t.Fatalf("expected nested task to run via import, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task local.task (Local sleep) - Status: not started") {
		t.Fatalf("expected local task to remain not started, logs: %s", logs)
	}
	if strings.Contains(logs, "[[ Executing flow: root.flow task: local.task ]]") {
		t.Fatalf("local task should not execute when running imported flow, logs: %s", logs)
	}
}

func TestRunExecutesRegisteredAction(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	action := ensureTestActionRegistered()
	action.reset()

	flowContent := []byte(`{
                "id": "registered.action",
                "description": "invoke registered action",
                "tasks": [
                        {"id":"custom","description":"Custom","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	definition, err := flow.LoadDefinition(flowPath)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
	definition.Tasks[0].Action = "TEST_ACTION"

	if err := runDefinition(ctx, definition, flowPath, logger, "", "", "", "", nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !action.executed() {
		t.Fatal("expected registered action to execute")
	}
}

func TestRunFailsForUnknownAction(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "unknown.action",
                "description": "unknown action",
                "tasks": [
                        {"id":"custom","description":"Custom","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	definition, err := flow.LoadDefinition(flowPath)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
	definition.Tasks[0].Action = "MISSING_ACTION"

	err = runDefinition(ctx, definition, flowPath, logger, "", "", "", "", nil)
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported action \"MISSING_ACTION\"") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunFlowExecutesPriorVariablesTasks(t *testing.T) {
	dir := t.TempDir()

	varsPath := filepath.Join(dir, "vars.json")
	varsContent := []byte(`{
                "id": "vars.flow",
                "description": "declare vars",
                "tasks": [
                        {
                                "id": "vars.declare",
                                "description": "Declare username",
                                "action": "VARIABLES",
                                "scope": "flow",
                                "overwrite": true,
                                "vars": [
                                        {"name": "manager_user", "type": "string", "value": "admin"}
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(varsPath, varsContent, 0o600); err != nil {
		t.Fatalf("writing vars flow: %v", err)
	}

	targetPath := filepath.Join(dir, "target.json")
	targetContent := []byte(`{
                "id": "target.flow",
                "description": "target flow",
                "tasks": [
                        {
                                "id": "print.user",
                                "description": "Print manager user",
                                "action": "PRINT",
                                "entries": [
                                        {"message": "user", "value": "${manager_user}"}
                                ]
                        }
                ]
        }`)
	if err := os.WriteFile(targetPath, targetContent, 0o600); err != nil {
		t.Fatalf("writing target flow: %v", err)
	}

	rootPath := filepath.Join(dir, "root.json")
	rootContent := []byte(`{
                "id": "root.flow",
                "description": "root",
                "imports": ["vars.json", "target.json"],
                "tasks": []
        }`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("writing root flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Run(ctx, rootPath, logger, "", "", "target.flow", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	logs := logger.String()
	if !strings.Contains(logs, "Task vars.declare (Declare username) - Status: completed") {
		t.Fatalf("expected variables task to execute before selected flow, logs: %s", logs)
	}
	if !strings.Contains(logs, "Task print.user (Print manager user) - Status: completed") {
		t.Fatalf("expected target flow task to execute, logs: %s", logs)
	}
}

func TestRunCreatesTaskLogs(t *testing.T) {
	t.Helper()

	if err := os.RemoveAll("logs"); err != nil {
		t.Fatalf("removing logs directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll("logs")
	})

	flowPath := writeFlow(t)

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Run(ctx, flowPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	flowName := sanitizeForDirectory(strings.TrimSuffix(filepath.Base(flowPath), filepath.Ext(flowPath)))
	if flowName == "" {
		flowName = "flow"
	}

	flowDir := filepath.Join("logs", flowName)
	if info, err := os.Stat(flowDir); err != nil {
		t.Fatalf("expected flow log directory %q: %v", flowDir, err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", flowDir)
	}

	taskDir := filepath.Join(flowDir, "task-0000-task1")
	if info, err := os.Stat(taskDir); err != nil {
		t.Fatalf("expected task directory %q: %v", taskDir, err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", taskDir)
	}

	logData, err := os.ReadFile(filepath.Join(taskDir, "task_log.json"))
	if err != nil {
		t.Fatalf("reading task log: %v", err)
	}

	var taskLog struct {
		Logs   []string `json:"logs"`
		Result any      `json:"result"`
	}
	if err := json.Unmarshal(logData, &taskLog); err != nil {
		t.Fatalf("unmarshalling task log: %v", err)
	}
	if len(taskLog.Logs) == 0 {
		t.Fatal("expected task log to contain at least one entry")
	}

	varsData, err := os.ReadFile(filepath.Join(taskDir, "environment_variables.json"))
	if err != nil {
		t.Fatalf("reading environment variables: %v", err)
	}

	var snapshot map[string]map[string]any
	if err := json.Unmarshal(varsData, &snapshot); err != nil {
		t.Fatalf("unmarshalling variables snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected variables snapshot to be present")
	}
}

func TestRunCreatesSubflowTaskLogsInFlowDirectory(t *testing.T) {
	t.Helper()

	if err := os.RemoveAll("logs"); err != nil {
		t.Fatalf("removing logs directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll("logs")
	})

	dir := t.TempDir()

	subflowPath := filepath.Join(dir, "sub.json")
	subflowContent := []byte(`{
                "id": "sub.flow",
                "description": "sub flow",
                "tasks": [
                        {"id":"sub.task","description":"Sub task","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(subflowPath, subflowContent, 0o600); err != nil {
		t.Fatalf("writing subflow: %v", err)
	}

	rootPath := filepath.Join(dir, "root.json")
	rootContent := []byte(`{
                "id": "root.flow",
                "description": "root flow",
                "imports": ["sub.json"],
                "tasks": [
                        {"id":"root.task","description":"Root task","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("writing root flow: %v", err)
	}

	logger := &bufferLogger{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Run(ctx, rootPath, logger, "", "", "", ""); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	flowName := sanitizeForDirectory(strings.TrimSuffix(filepath.Base(rootPath), filepath.Ext(rootPath)))
	if flowName == "" {
		flowName = "flow"
	}

	flowDir := filepath.Join("logs", flowName)

	subflowDir := filepath.Join(flowDir, sanitizeForDirectory("sub.flow"))
	if info, err := os.Stat(subflowDir); err != nil {
		t.Fatalf("expected subflow directory %q: %v", subflowDir, err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", subflowDir)
	}

	subTaskDir := filepath.Join(subflowDir, "task-0000-sub.task")
	if info, err := os.Stat(subTaskDir); err != nil {
		t.Fatalf("expected subflow task directory %q: %v", subTaskDir, err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", subTaskDir)
	}

	rootTaskDir := filepath.Join(flowDir, "task-0001-root.task")
	if info, err := os.Stat(rootTaskDir); err != nil {
		t.Fatalf("expected root task directory %q: %v", rootTaskDir, err)
	} else if !info.IsDir() {
		t.Fatalf("expected %q to be a directory", rootTaskDir)
	}
}

func writeFlow(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{"id":"writeflow.test","description":"test","tasks":[{"id":"task1","description":"First sleep task","action":"SLEEP","seconds":0.01},{"id":"task2","description":"Second sleep task","action":"SLEEP","seconds":0.01}]}`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	return flowPath
}

var (
	registerTestActionOnce sync.Once
	testActionInstance     *testRegistryAction
)

type testRegistryAction struct {
	mu           sync.Mutex
	executedFlag bool
}

func (a *testRegistryAction) Name() string {
	return "TEST_ACTION"
}

func (a *testRegistryAction) Execute(_ context.Context, _ json.RawMessage, _ *registry.ExecutionContext) (registry.Result, error) {
	a.mu.Lock()
	a.executedFlag = true
	a.mu.Unlock()
	return registry.Result{Value: true, Type: flow.ResultTypeBool}, nil
}

func (a *testRegistryAction) reset() {
	a.mu.Lock()
	a.executedFlag = false
	a.mu.Unlock()
}

func (a *testRegistryAction) executed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.executedFlag
}

func ensureTestActionRegistered() *testRegistryAction {
	registerTestActionOnce.Do(func() {
		testActionInstance = &testRegistryAction{}
		registry.Register(testActionInstance)
	})
	return testActionInstance
}
