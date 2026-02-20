package print

import (
	"fmt"
	"testing"

	"flowk/internal/actions/core/variables"
	"flowk/internal/flow"
)

type stubLogger struct {
	messages []string
}

func (l *stubLogger) Printf(format string, v ...interface{}) {
	l.messages = append(l.messages, fmt.Sprintf(format, v...))
}

func TestExecutePrintsVariablesAndTasks(t *testing.T) {
	payload := Payload{
		Entries: []Entry{
			{Message: "Static message"},
			{Message: "Greeting", Variable: "greeting"},
			{Message: "First id", Value: "${from.task:task1.result$.data[0].id}"},
			{TaskID: "task1", Field: "result$.data[0].id", Message: "Legacy id"},
		},
	}

	vars := map[string]variables.Variable{
		"greeting": {
			Name:  "greeting",
			Type:  "string",
			Value: "hola",
		},
	}

	tasks := []flow.Task{
		{
			ID:         "task1",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"data": []any{
					map[string]any{"id": "alpha"},
				},
			},
		},
	}

	logger := &stubLogger{}

	result, resultType, err := Execute(payload, vars, tasks, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultType != flow.ResultTypeJSON {
		t.Fatalf("ResultType = %s, want %s", resultType, flow.ResultTypeJSON)
	}

	entries, ok := result.([]ResultEntry)
	if !ok {
		t.Fatalf("result type = %T, want []ResultEntry", result)
	}

	if len(entries) != 4 {
		t.Fatalf("entries length = %d, want 4", len(entries))
	}

	if entries[0].Message != "Static message" || entries[0].Value != nil {
		t.Fatalf("entries[0] = %+v, want message Static message and nil value", entries[0])
	}

	if entries[1].Message != "Greeting" || entries[1].Value != "hola" {
		t.Fatalf("entries[1] = %+v, want Greeting/hola", entries[1])
	}

	if entries[2].Message != "First id" || entries[2].Value != "alpha" {
		t.Fatalf("entries[2] = %+v, want First id/alpha", entries[2])
	}

	if entries[3].Message != "Legacy id" || entries[3].Value != "alpha" {
		t.Fatalf("entries[3] = %+v, want Legacy id/alpha", entries[3])
	}

	if len(logger.messages) != 4 {
		t.Fatalf("logged messages = %d, want 4", len(logger.messages))
	}
}

func TestExecuteMasksSecretVariables(t *testing.T) {
	payload := Payload{Entries: []Entry{{Variable: "token"}}}
	vars := map[string]variables.Variable{
		"token": {
			Name:   "token",
			Type:   "secret",
			Value:  "super-secret",
			Secret: true,
		},
	}

	tasks := []flow.Task{}
	logger := &stubLogger{}

	result, _, err := Execute(payload, vars, tasks, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	entries := result.([]ResultEntry)
	if entries[0].Value != "****" {
		t.Fatalf("secret variable was not masked: %+v", entries[0])
	}
}

func TestExecuteInterpolatesPlaceholdersInMessageAndValue(t *testing.T) {
	payload := Payload{Entries: []Entry{{
		Message: "Hello ${first} ${last} from ${from.task:task1.result$.country}",
		Value:   "Message: ${greeting} - ${from.task:task1.result$.country}",
	}}}

	vars := map[string]variables.Variable{
		"first": {Name: "first", Type: "string", Value: "Ada"},
		"last":  {Name: "last", Type: "string", Value: "Lovelace"},
		"greeting": {
			Name:  "greeting",
			Type:  "string",
			Value: "Hola",
		},
	}

	tasks := []flow.Task{{
		ID:         "task1",
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"country": "Peru",
		},
	}}

	logger := &stubLogger{}

	result, _, err := Execute(payload, vars, tasks, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	entries := result.([]ResultEntry)
	if len(entries) != 1 {
		t.Fatalf("entries length = %d, want 1", len(entries))
	}

	expectedMessage := "Hello Ada Lovelace from Peru"
	expectedValue := "Message: Hola - Peru"

	if entries[0].Message != expectedMessage {
		t.Fatalf("message = %q, want %q", entries[0].Message, expectedMessage)
	}
	if entries[0].Value != expectedValue {
		t.Fatalf("value = %q, want %q", entries[0].Value, expectedValue)
	}
	if len(logger.messages) != 1 {
		t.Fatalf("logged messages = %d, want 1", len(logger.messages))
	}
	if logger.messages[0] != expectedMessage+": "+expectedValue {
		t.Fatalf("logged message = %q, want %q", logger.messages[0], expectedMessage+": "+expectedValue)
	}
}

func TestExecuteMasksSecretPlaceholders(t *testing.T) {
	payload := Payload{Entries: []Entry{
		{Message: "Token ${token}"},
		{Value: "Secret is ${token}"},
	}}

	vars := map[string]variables.Variable{
		"token": {
			Name:   "token",
			Type:   "secret",
			Value:  "super-secret",
			Secret: true,
		},
	}

	logger := &stubLogger{}

	result, _, err := Execute(payload, vars, nil, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	entries := result.([]ResultEntry)
	if len(entries) != 2 {
		t.Fatalf("entries length = %d, want 2", len(entries))
	}

	if entries[0].Message != "Token ****" {
		t.Fatalf("entries[0].Message = %q, want Token ****", entries[0].Message)
	}
	if entries[0].Value != nil {
		t.Fatalf("entries[0].Value = %v, want nil", entries[0].Value)
	}

	if entries[1].Value != "Secret is ****" {
		t.Fatalf("entries[1].Value = %v, want Secret is ****", entries[1].Value)
	}

	if len(logger.messages) != 2 {
		t.Fatalf("logged messages = %d, want 2", len(logger.messages))
	}
	if logger.messages[0] != "Token ****" {
		t.Fatalf("logger.messages[0] = %q, want Token ****", logger.messages[0])
	}
	if logger.messages[1] != "Secret is ****" {
		t.Fatalf("logger.messages[1] = %q, want Secret is ****", logger.messages[1])
	}
}

func TestExecuteResolvesFromTaskPlaceholders(t *testing.T) {
	payload := Payload{Entries: []Entry{
		{Message: "Count ${from.task:task1.result$.count}"},
		{Message: "Result", Value: "${from.task:task1.result$.data[0].id}"},
	}}

	tasks := []flow.Task{{
		ID:         "task1",
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"count": float64(3),
			"data": []any{
				map[string]any{"id": "alpha"},
			},
		},
	}}

	logger := &stubLogger{}

	result, _, err := Execute(payload, nil, tasks, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	entries := result.([]ResultEntry)
	if len(entries) != 2 {
		t.Fatalf("entries length = %d, want 2", len(entries))
	}

	if entries[0].Message != "Count 3" {
		t.Fatalf("entries[0].Message = %q, want Count 3", entries[0].Message)
	}
	if entries[0].Value != nil {
		t.Fatalf("entries[0].Value = %v, want nil", entries[0].Value)
	}

	if id, ok := entries[1].Value.(string); !ok || id != "alpha" {
		t.Fatalf("entries[1].Value = %#v, want string alpha", entries[1].Value)
	}

	if len(logger.messages) != 2 {
		t.Fatalf("logged messages = %d, want 2", len(logger.messages))
	}
	if logger.messages[0] != "Count 3" {
		t.Fatalf("logger.messages[0] = %q, want Count 3", logger.messages[0])
	}
	if logger.messages[1] != "Result: alpha" {
		t.Fatalf("logger.messages[1] = %q, want Result: alpha", logger.messages[1])
	}
}

func TestExecuteErrors(t *testing.T) {
	tests := []struct {
		name    string
		payload Payload
	}{
		{
			name:    "missing variable",
			payload: Payload{Entries: []Entry{{Variable: "missing"}}},
		},
		{
			name:    "missing task",
			payload: Payload{Entries: []Entry{{TaskID: "unknown"}}},
		},
		{
			name:    "missing task placeholder",
			payload: Payload{Entries: []Entry{{Value: "${from.task:unknown.result$}"}}},
		},
	}

	tasks := []flow.Task{}
	vars := map[string]variables.Variable{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Execute(tc.payload, vars, tasks, nil); err == nil {
				t.Fatalf("Execute() error = nil, want error")
			}
		})
	}
}
