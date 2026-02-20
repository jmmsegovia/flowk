package helm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestActionExecuteReturnsJSONResult(t *testing.T) {

	original := commandRunner
	t.Cleanup(func() { commandRunner = original })
	commandRunner = func(_ context.Context, _ Payload, _ []string) (commandOutput, error) {
		return commandOutput{stdout: "[]"}, nil
	}

	a := action{}
	payload := json.RawMessage(`{"id":"repo_list","action":"HELM","operation":"REPO_LIST"}`)
	result, err := a.Execute(context.Background(), payload, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("expected result type %q, got %q", flow.ResultTypeJSON, result.Type)
	}
}

func TestActionExecuteInvalidPayload(t *testing.T) {

	a := action{}
	_, err := a.Execute(context.Background(), json.RawMessage(`{"action":"HELM"}`), &registry.ExecutionContext{})
	if err == nil || !strings.Contains(err.Error(), "operation is required") {
		t.Fatalf("expected operation required error, got %v", err)
	}
}
