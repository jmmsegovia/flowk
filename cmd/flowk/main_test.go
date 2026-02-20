package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	actionhelp "flowk/internal/cli/actionhelp"
)

func TestParseRunArgsSupportsFlagsInAnyOrder(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=flow.json", "-begin-from-task", "task42"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.flowPath != "flow.json" {
		t.Fatalf("flowPath = %q, want flow.json", args.flowPath)
	}
	if args.beginFromTask != "task42" {
		t.Fatalf("beginFromTask = %q, want task42", args.beginFromTask)
	}
	if args.runTaskID != "" {
		t.Fatalf("runTask = %q, want empty", args.runTaskID)
	}
	if args.runFlowID != "" {
		t.Fatalf("runFlow = %q, want empty", args.runFlowID)
	}
	if args.runSubtaskID != "" {
		t.Fatalf("runSubtask = %q, want empty", args.runSubtaskID)
	}
}

func TestParseRunArgsSupportsValuesAfterEquals(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=", "flow.json"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.flowPath != "flow.json" {
		t.Fatalf("unexpected flow result: %q", args.flowPath)
	}
	if args.beginFromTask != "" {
		t.Fatalf("beginFromTask = %q, want empty", args.beginFromTask)
	}
	if args.runTaskID != "" {
		t.Fatalf("runTask = %q, want empty", args.runTaskID)
	}
	if args.runFlowID != "" {
		t.Fatalf("runFlow = %q, want empty", args.runFlowID)
	}
	if args.runSubtaskID != "" {
		t.Fatalf("runSubtask = %q, want empty", args.runSubtaskID)
	}
}

func TestParseRunArgsSupportsPositionalArguments(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"flow.json"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.flowPath != "flow.json" {
		t.Fatalf("unexpected flow result: %q", args.flowPath)
	}
	if args.beginFromTask != "" {
		t.Fatalf("beginFromTask = %q, want empty", args.beginFromTask)
	}
	if args.runTaskID != "" {
		t.Fatalf("runTask = %q, want empty", args.runTaskID)
	}
	if args.runFlowID != "" {
		t.Fatalf("runFlow = %q, want empty", args.runFlowID)
	}
	if args.runSubtaskID != "" {
		t.Fatalf("runSubtask = %q, want empty", args.runSubtaskID)
	}
}

func TestParseRunArgsUnexpectedArguments(t *testing.T) {
	setTempConfigHome(t)
	_, err := parseRunArgs([]string{"-flow=flow.json", "extra"})
	if err == nil {
		t.Fatal("parseRunArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("error message = %q, want unexpected arguments", err)
	}
}

func TestParseRunArgsRunTask(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=flow.json", "-run-task", "task99"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.beginFromTask != "" {
		t.Fatalf("beginFromTask = %q, want empty", args.beginFromTask)
	}
	if args.runTaskID != "task99" {
		t.Fatalf("runTask = %q, want task99", args.runTaskID)
	}
	if args.flowPath != "flow.json" {
		t.Fatalf("unexpected flow result: %q", args.flowPath)
	}
	if args.runFlowID != "" {
		t.Fatalf("runFlow = %q, want empty", args.runFlowID)
	}
	if args.runSubtaskID != "" {
		t.Fatalf("runSubtask = %q, want empty", args.runSubtaskID)
	}
}

func TestParseRunArgsRunTaskConflictsWithBeginFromTask(t *testing.T) {
	setTempConfigHome(t)
	_, err := parseRunArgs([]string{"-flow=flow.json", "-run-task", "task99", "-begin-from-task", "task1"})
	if err == nil {
		t.Fatal("parseRunArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("error message = %q, want mention of conflict", err)
	}
}

func TestParseRunArgsRunFlow(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=flow.json", "-run-flow", "subflow"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.flowPath != "flow.json" {
		t.Fatalf("unexpected flow result: %q", args.flowPath)
	}
	if args.beginFromTask != "" {
		t.Fatalf("beginFromTask = %q, want empty", args.beginFromTask)
	}
	if args.runTaskID != "" {
		t.Fatalf("runTask = %q, want empty", args.runTaskID)
	}
	if args.runFlowID != "subflow" {
		t.Fatalf("runFlow = %q, want subflow", args.runFlowID)
	}
	if args.runSubtaskID != "" {
		t.Fatalf("runSubtask = %q, want empty", args.runSubtaskID)
	}
}

func TestParseRunArgsRunFlowConflicts(t *testing.T) {
	setTempConfigHome(t)
	_, err := parseRunArgs([]string{"-flow=flow.json", "-run-flow", "subflow", "-run-task", "task1"})
	if err == nil {
		t.Fatal("parseRunArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run-flow") {
		t.Fatalf("error message = %q, want mention of run-flow conflict", err)
	}
}

func TestParseRunArgsRunSubtask(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=flow.json", "-run-subtask", "task99"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.runSubtaskID != "task99" {
		t.Fatalf("runSubtask = %q, want task99", args.runSubtaskID)
	}
	if args.beginFromTask != "" {
		t.Fatalf("beginFromTask = %q, want empty", args.beginFromTask)
	}
	if args.runTaskID != "" {
		t.Fatalf("runTask = %q, want empty", args.runTaskID)
	}
	if args.runFlowID != "" {
		t.Fatalf("runFlow = %q, want empty", args.runFlowID)
	}
}

func TestParseRunArgsRunSubtaskConflictsWithBeginFromTask(t *testing.T) {
	setTempConfigHome(t)
	_, err := parseRunArgs([]string{"-flow=flow.json", "-run-subtask", "task99", "-begin-from-task", "task1"})
	if err == nil {
		t.Fatal("parseRunArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run-subtask") {
		t.Fatalf("error message = %q, want mention of run-subtask conflict", err)
	}
}

func TestParseRunArgsValidateOnly(t *testing.T) {
	setTempConfigHome(t)
	args, err := parseRunArgs([]string{"-flow=flow.json", "-validate-only"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if !args.validateOnly {
		t.Fatal("validateOnly flag not enabled")
	}
}

func TestParseRunArgsValidateOnlyConflictsWithServeUI(t *testing.T) {
	setTempConfigHome(t)
	_, err := parseRunArgs([]string{"-flow=flow.json", "-validate-only", "-serve-ui"})
	if err == nil {
		t.Fatal("parseRunArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "validate-only") {
		t.Fatalf("error message = %q, want mention of validate-only conflict", err)
	}
}

func TestParseRunArgsServeUIOptions(t *testing.T) {
	configHome := setTempConfigHome(t)
	writeConfig(t, configHome, "ui:\n  host: 0.0.0.0\n  port: 9090\n  dir: ui/custom\n")
	args, err := parseRunArgs([]string{"-flow", "flow.json", "-serve-ui"})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if !args.serveUI {
		t.Fatal("serveUI flag not enabled")
	}
	if args.uiAddress != "0.0.0.0:9090" {
		t.Fatalf("uiAddress = %q, want 0.0.0.0:9090", args.uiAddress)
	}
	if args.uiDir != "ui/custom" {
		t.Fatalf("uiDir = %q, want ui/custom", args.uiDir)
	}
}

func TestParseRunArgsConfigOverride(t *testing.T) {
	xdgHome := setTempConfigHome(t)
	writeConfig(t, xdgHome, "ui:\n  host: 127.0.0.1\n  port: 8080\n  dir: ui/default\n")

	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "custom.yaml")
	if err := os.WriteFile(customPath, []byte("ui:\n  host: 0.0.0.0\n  port: 9091\n  dir: ui/custom\n"), 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	args, err := parseRunArgs([]string{"-flow", "flow.json", "-serve-ui", "-config", customPath})
	if err != nil {
		t.Fatalf("parseRunArgs() error = %v", err)
	}
	if args.uiAddress != "0.0.0.0:9091" {
		t.Fatalf("uiAddress = %q, want 0.0.0.0:9091", args.uiAddress)
	}
	if args.uiDir != "ui/custom" {
		t.Fatalf("uiDir = %q, want ui/custom", args.uiDir)
	}
}

func TestBuildActionHelpPrint(t *testing.T) {
	help, err := actionhelp.Build("print")
	if err != nil {
		t.Fatalf("buildActionHelp() error = %v", err)
	}

	if !strings.Contains(help, "Action PRINT") {
		t.Fatalf("help output missing action header: %s", help)
	}

	if !strings.Contains(help, "Required fields:") {
		t.Fatalf("help output missing required fields section: %s", help)
	}

	if !strings.Contains(help, "entries") {
		t.Fatalf("help output missing entries field description: %s", help)
	}
}

func TestBuildActionHelpForLoopOptionalField(t *testing.T) {
	help, err := actionhelp.Build("for")
	if err != nil {
		t.Fatalf("buildActionHelp() error = %v", err)
	}

	if !strings.Contains(help, "Optional fields:") {
		t.Fatalf("help output missing optional fields section: %s", help)
	}

	if !strings.Contains(help, "max_iterations") {
		t.Fatalf("help output missing max_iterations optional field: %s", help)
	}
}

func TestExecuteActionHelpUnknownAction(t *testing.T) {
	err := executeActionHelp("flowk", []string{"does-not-exist"})
	if err == nil {
		t.Fatal("executeActionHelp() error = nil, want usage error")
	}

	var usageErr *usageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("error = %T, want *usageError", err)
	}

	if usageErr.helpMessage == "" {
		t.Fatal("usage error missing help message")
	}
}

func TestRunFlowWithServeUIKeepsServerRunningUntilContextCancelled(t *testing.T) {
	dir := t.TempDir()
	flowPath := filepath.Join(dir, "flow.json")

	flowContent := []byte(`{
                "id": "ui.test.flow",
                "description": "UI test flow",
                "tasks": [
                        {"id":"sleep","description":"short pause","action":"SLEEP","seconds":0.01}
                ]
        }`)
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	args := runArguments{
		flowPath:  flowPath,
		serveUI:   true,
		uiAddress: addr,
		uiDir:     dir,
	}

	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatalf("writing ui index: %v", err)
	}

	origWriter := log.Writer()
	buf := &threadSafeBuffer{}
	log.SetOutput(buf)
	defer log.SetOutput(origWriter)

	errCh := make(chan error, 1)
	go func() {
		errCh <- runFlowWithOptions(ctx, args)
	}()

	time.Sleep(500 * time.Millisecond)
	t.Logf("logs after sleep: %s", buf.String())

	wantLog := "The Flowk UI will remain available"
	if !strings.Contains(buf.String(), wantLog) {
		cancel()
		t.Fatalf("expected log message %q, got logs: %s", wantLog, buf.String())
	}
	t.Log("confirmed flow completion log")

	select {
	case err := <-errCh:
		t.Fatalf("runFlowWithOptions returned before context cancellation: %v", err)
	default:
	}
	t.Log("runFlowWithOptions still active after initial check")

	time.Sleep(200 * time.Millisecond)

	select {
	case err := <-errCh:
		t.Fatalf("runFlowWithOptions returned early after flow completion: %v", err)
	default:
	}
	t.Log("runFlowWithOptions still active after delay")

	cancel()
	t.Log("cancelled context")

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("runFlowWithOptions() error = %v, want context.Canceled", err)
		}
		t.Logf("runFlowWithOptions exited with: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runFlowWithOptions to exit after cancellation")
	}
}

func TestRunFlowWithServeUIPortConflictLogsAndStops(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatalf("writing ui index: %v", err)
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	defer occupied.Close()

	args := runArguments{
		serveUI:   true,
		uiAddress: occupied.Addr().String(),
		uiDir:     dir,
	}

	origWriter := log.Writer()
	buf := &threadSafeBuffer{}
	log.SetOutput(buf)
	defer log.SetOutput(origWriter)

	err = runFlowWithOptions(context.Background(), args)
	if err == nil {
		t.Fatal("runFlowWithOptions() error = nil, want bind conflict")
	}

	logs := buf.String()
	if !strings.Contains(logs, "UI server failed to bind") {
		t.Fatalf("expected bind conflict log, got logs: %s", logs)
	}
	if strings.Contains(logs, "Flowk UI is available") {
		t.Fatalf("unexpected availability log on bind failure: %s", logs)
	}
}

type threadSafeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *threadSafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *threadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestExecuteActionHelpWithoutArgsListsActions(t *testing.T) {
	output := captureStdout(t, func() {
		if err := executeActionHelp("flowk", nil); err != nil {
			t.Fatalf("executeActionHelp() error = %v", err)
		}
	})

	if !strings.Contains(output, "Available actions:") {
		t.Fatalf("output missing action list header: %s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe creation failed: %v", err)
	}

	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = original

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading stdout failed: %v", err)
	}
	r.Close()

	return string(data)
}

func setTempConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func writeConfig(t *testing.T, configHome, contents string) string {
	t.Helper()
	configDir := filepath.Join(configHome, "flowk")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	path := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}
