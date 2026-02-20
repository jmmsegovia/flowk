package shell

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
)

func TestExecuteRunsCommand(t *testing.T) {
	payload := Payload{Command: echoCommand("hello")}

	result, err := Execute(context.Background(), payload, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Fatalf("expected stdout to contain hello, got %q", result.Stdout)
	}
}

func TestExecuteAppliesEnvironmentVariables(t *testing.T) {
	payload := Payload{
		Command: envPrintCommand("FOO"),
		Environment: []EnvironmentVariable{
			{Name: "FOO", Value: "bar"},
		},
	}

	result, err := Execute(context.Background(), payload, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	expected := platformLine("bar")
	if strings.TrimSpace(result.Stdout) != expected {
		t.Fatalf("unexpected stdout: got %q want %q", result.Stdout, expected)
	}
}

func TestExecuteInjectsProxyVariables(t *testing.T) {
	execCtx := &registry.ExecutionContext{
		Variables: map[string]registry.Variable{
			"corp_proxy": {
				Name: "corp_proxy",
				Type: "proxy",
				Value: map[string]string{
					"http":  "http://localhost:8080",
					"https": "https://localhost:8443",
					"no":    "example.com",
				},
			},
		},
	}

	payload := Payload{
		Command:        multiEnvEchoCommand([]string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}),
		ProxyVariables: []string{"corp_proxy"},
	}

	result, err := Execute(context.Background(), payload, execCtx)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := normalizeNewlines(result.Stdout)
	if !strings.Contains(output, "http://localhost:8080") {
		t.Fatalf("expected HTTP proxy in output, got %q", result.Stdout)
	}
	if !strings.Contains(output, "https://localhost:8443") {
		t.Fatalf("expected HTTPS proxy in output, got %q", result.Stdout)
	}
	if !strings.Contains(output, "example.com") {
		t.Fatalf("expected NO_PROXY in output, got %q", result.Stdout)
	}

	hasUpper := false
	hasLower := false
	for _, env := range result.Environment {
		switch env.Name {
		case "HTTP_PROXY":
			hasUpper = true
			if env.Value != "http://localhost:8080" {
				t.Fatalf("unexpected HTTP_PROXY value %q", env.Value)
			}
		case "http_proxy":
			hasLower = true
		}
	}
	if !hasUpper || !hasLower {
		t.Fatalf("expected both HTTP_PROXY and http_proxy entries in environment snapshot: %+v", result.Environment)
	}
}

func TestExecuteHonorsContinueOnError(t *testing.T) {
	payload := Payload{Command: failingCommand(5)}

	if _, err := Execute(context.Background(), payload, &registry.ExecutionContext{}); err == nil {
		t.Fatalf("expected error when command exits with non-zero status")
	}

	payload.ContinueOnError = true
	result, err := Execute(context.Background(), payload, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExitCode != 5 {
		t.Fatalf("expected exit code 5, got %d", result.ExitCode)
	}
}

func TestExecuteFailsWithUnknownProxyVariable(t *testing.T) {
	payload := Payload{Command: echoCommand("noop"), ProxyVariables: []string{"missing"}}

	if _, err := Execute(context.Background(), payload, &registry.ExecutionContext{}); err == nil {
		t.Fatalf("expected error for missing proxy variable")
	}
}

func echoCommand(message string) string {
	if runtime.GOOS == "windows" {
		return "echo " + message
	}
	return "echo '" + message + "'"
}

func envPrintCommand(name string) string {
	if runtime.GOOS == "windows" {
		return "echo %" + name + "%"
	}
	return "printf '%s' \"$" + name + "\""
}

func multiEnvEchoCommand(names []string) string {
	if runtime.GOOS == "windows" {
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = "%" + name + "%"
		}
		return "echo " + strings.Join(parts, " ")
	}
	if len(names) != 3 {
		panic("multiEnvEchoCommand expects exactly three names on non-Windows platforms")
	}
	return "printf '%s %s %s' \"$" + names[0] + "\" \"$" + names[1] + "\" \"$" + names[2] + "\""
}

func failingCommand(code int) string {
	if runtime.GOOS == "windows" {
		return "exit /b " + strconv.Itoa(code)
	}
	return "exit " + strconv.Itoa(code)
}

func platformLine(value string) string {
	if runtime.GOOS == "windows" {
		return strings.TrimSpace(value)
	}
	return value
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
