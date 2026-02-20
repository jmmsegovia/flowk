package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
	"flowk/internal/logging/colors"
)

const sampleJSON = `{
  "firstName": "John",
  "lastName" : "doe",
  "age"      : 26,
  "address"  : {
    "streetAddress": "naist street",
    "city"         : "Nara",
    "postalCode"   : "630-0192"
  },
  "phoneNumbers": [
    {
      "type"  : "iPhone",
      "number": "0123-4567-8888"
    },
    {
      "type"  : "home",
      "number": "0123-4567-8910"
    }
  ]
}`

type coloredMessage struct {
	plain   string
	colored string
}

type stubLogger struct {
	messages        []string
	coloredMessages []coloredMessage
}

func (l *stubLogger) Printf(format string, args ...any) {
	l.messages = append(l.messages, fmt.Sprintf(format, args...))
}

func (l *stubLogger) contains(substr string) bool {
	for _, msg := range l.messages {
		if strings.Contains(msg, substr) {
			return true
		}
	}
	return false
}

func (l *stubLogger) containsWithSuffix(substr, suffix string) bool {
	for _, msg := range l.messages {
		if strings.Contains(msg, substr) && strings.HasSuffix(msg, suffix) {
			return true
		}
	}
	return false
}

func (l *stubLogger) containsANSIInMessages() bool {
	for _, msg := range l.messages {
		if strings.Contains(msg, "\033") {
			return true
		}
	}
	return false
}

func (l *stubLogger) coloredContains(substr string) bool {
	for _, msg := range l.coloredMessages {
		if strings.Contains(msg.colored, substr) {
			return true
		}
	}
	return false
}

func (l *stubLogger) PrintColored(plain, colored string) {
	l.coloredMessages = append(l.coloredMessages, coloredMessage{plain: plain, colored: colored})
	l.messages = append(l.messages, plain)
}

func newStubLogger() *stubLogger {
	return &stubLogger{messages: make([]string, 0), coloredMessages: make([]coloredMessage, 0)}
}

func unmarshalSample(t *testing.T) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(sampleJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func TestExecuteReturnsTrueWhenConditionsMet(t *testing.T) {
	logger := newStubLogger()

	task := &flow.Task{
		ID:          "truncate_all_tables_us",
		Description: "Truncate all tables in the all_us-east4 platform.",
		Action:      "DB_CASSANDRA_OPERATION",
		Status:      flow.TaskStatusCompleted,
		Success:     true,
		ResultType:  flow.ResultTypeJSON,
		Result:      unmarshalSample(t),
	}

	conditions := []Condition{
		{Left: "success", Operation: "=", Right: true},
		{Left: "result$.phoneNumbers[:1].type", Operation: "=", Right: "iPhone"},
	}

	result, resultType, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result {
		t.Fatalf("Execute() result = false, want true")
	}
	if resultType != flow.ResultTypeBool {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeBool)
	}

	if !logger.containsWithSuffix("Condition [01]", "OK") {
		t.Fatalf("logger does not contain Condition [01] OK message: %v", logger.messages)
	}
	if !logger.containsWithSuffix("Condition [02]", "OK") {
		t.Fatalf("logger does not contain Condition [02] OK message: %v", logger.messages)
	}
	if logger.containsANSIInMessages() {
		t.Fatalf("plain messages should not contain ANSI codes: %v", logger.messages)
	}
	coloredOK := fmt.Sprintf("%sOK%s", colors.Green, colors.Reset)
	if !logger.coloredContains(coloredOK) {
		t.Fatalf("colored messages should contain green OK: %+v", logger.coloredMessages)
	}
}

func TestExecuteReturnsFalseWhenConditionFails(t *testing.T) {
	logger := newStubLogger()

	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		Success:    true,
		ResultType: flow.ResultTypeJSON,
		Result:     unmarshalSample(t),
	}

	conditions := []Condition{
		{Left: "success", Operation: "=", Right: true},
		{Left: "result$.phoneNumbers[:1].type", Operation: "=", Right: "Android"},
	}

	result, resultType, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result {
		t.Fatalf("Execute() result = true, want false")
	}
	if resultType != flow.ResultTypeBool {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeBool)
	}

	if !logger.containsWithSuffix("Condition [02]", "KO") {
		t.Fatalf("logger does not contain Condition [02] KO message: %v", logger.messages)
	}
	if logger.containsANSIInMessages() {
		t.Fatalf("plain messages should not contain ANSI codes: %v", logger.messages)
	}
	coloredKO := fmt.Sprintf("%sKO%s", colors.Red, colors.Reset)
	if !logger.coloredContains(coloredKO) {
		t.Fatalf("colored messages should contain red KO: %+v", logger.coloredMessages)
	}
}

func TestExecuteComparesStringVariables(t *testing.T) {
	logger := newStubLogger()

	variables := map[string]any{
		"loopvar": "hola",
	}

	conditions := []Condition{
		{Left: "${loopvar}", Operation: "=", Right: "hola"},
	}

	ok, _, err := Execute(nil, nil, variables, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteErrorsOnUnsupportedField(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{Status: flow.TaskStatusCompleted}
	conditions := []Condition{
		{Left: "unknown", Operation: "=", Right: "value"},
	}

	if _, _, err := Execute(task, nil, nil, conditions, logger); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteErrorsWhenJSONResultRequired(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeString,
		Result:     "text",
	}
	conditions := []Condition{
		{Left: "result$.value", Operation: "=", Right: "text"},
	}

	if _, _, err := Execute(task, nil, nil, conditions, logger); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteSupportsJSONArrayBody(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"body": []any{
				map[string]any{
					"name":  "key1",
					"state": "ACTIVE",
				},
				map[string]any{
					"name":  "key3",
					"state": "ACTIVE",
				},
			},
		},
	}

	conditions := []Condition{
		{Left: "result$.body[?(@.name == 'key1')].state", Operation: "=", Right: "ACTIVE"},
	}

	ok, _, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteHandlesThenBranchFieldsInJSON(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		Success:    true,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"then": map[string]any{
				"continue": "",
			},
			"else": map[string]any{
				"sleep":      5,
				"gototaskid": "list_tenant1_keys",
			},
		},
	}

	conditions := []Condition{
		{Left: "success", Operation: "=", Right: true},
		{Left: "result$.then.continue", Operation: "=", Right: ""},
		{Left: "result$.else.sleep", Operation: "=", Right: float64(5)},
		{Left: "result$.else.gototaskid", Operation: "=", Right: "list_tenant1_keys"},
	}

	ok, resultType, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
	if resultType != flow.ResultTypeBool {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeBool)
	}
}

func TestExecuteLogsWhenJSONPathReturnsEmptyCollection(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"body": []any{},
		},
	}

	conditions := []Condition{
		{Left: "result$.body[?(@.name == 'missing')].state", Operation: "=", Right: "ACTIVE"},
	}

	ok, resultType, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if ok {
		t.Fatalf("Execute() result = true, want false")
	}
	if resultType != flow.ResultTypeBool {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeBool)
	}
	if !logger.contains("did not return any results") {
		t.Fatalf("expected log about empty results, got %v", logger.messages)
	}
}

func TestExecuteSupportsLengthExtensionInPlaceholders(t *testing.T) {
	logger := newStubLogger()

	tasks := []flow.Task{
		{
			ID:         "http.obtener_breeds",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"data": []any{
						map[string]any{"id": "1"},
						map[string]any{"id": "2"},
					},
				},
			},
		},
	}

	conditions := []Condition{
		{Left: "${from.task:http.obtener_breeds.result$.body.data.length()}", Operation: ">", Right: 0},
	}

	ok, _, err := Execute(nil, tasks, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestResolveConditionFieldValueSupportsPlaceholders(t *testing.T) {
	tasks := []flow.Task{
		{
			ID:         "previous",
			Status:     flow.TaskStatusCompleted,
			Success:    true,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"count": float64(42),
				},
			},
		},
	}

	tests := []struct {
		name  string
		field string
		want  any
	}{
		{name: "success", field: "${from.task:previous.success}", want: true},
		{name: "json path", field: "${from.task:previous$.body.count}", want: float64(42)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveConditionFieldValue(nil, tasks, tt.field)
			if err != nil {
				t.Fatalf("resolveConditionFieldValue returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			if fmt.Sprintf("%T", got) != fmt.Sprintf("%T", tt.want) {
				t.Fatalf("expected type %T, got %T", tt.want, got)
			}
		})
	}
}

func TestExecuteComparesAcrossTasks(t *testing.T) {
	logger := newStubLogger()

	tasks := []flow.Task{
		{
			ID:         "first",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"value": 5,
			},
		},
		{
			ID:         "second",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"value": 5,
			},
		},
	}

	conditions := []Condition{{
		Left:      "${from.task:first.result$.value}",
		Operation: "=",
		Right:     "${from.task:second.result$.value}",
	}}

	ok, _, err := Execute(nil, tasks, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteResolvesVariablePlaceholders(t *testing.T) {
	logger := newStubLogger()

	tasks := []flow.Task{{
		ID:         "counter",
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"count": float64(7),
		},
	}}

	variables := map[string]any{
		"threshold": float64(5),
	}

	conditions := []Condition{{
		Left:      "${from.task:counter.result$.count}",
		Operation: ">",
		Right:     "${threshold}",
	}}

	ok, _, err := Execute(nil, tasks, variables, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteSupportsFieldExpectedWithWhitespace(t *testing.T) {
	logger := newStubLogger()

	tasks := []flow.Task{{
		ID:      "http.obtener_breeds",
		Status:  flow.TaskStatusCompleted,
		Success: true,
	}}

	conditions := []Condition{{
		Field:     "${  from.task:http.obtener_breeds.success  }",
		Operation: "=",
		Expected:  true,
	}}

	ok, _, err := Execute(nil, tasks, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteAllowsEmptyCollectionComparison(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"items": []any{},
		},
	}

	conditions := []Condition{
		{Left: "result$.items", Operation: "=", Right: []any{}},
	}

	ok, _, err := Execute(task, nil, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteHandlesNumericComparisons(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"value": 42,
		},
	}

	tests := []struct {
		name       string
		conditions []Condition
	}{
		{
			name: "equality with json.Number",
			conditions: []Condition{
				{Left: "result$.value", Operation: "=", Right: json.Number("42")},
			},
		},
		{
			name: "greater than",
			conditions: []Condition{
				{Left: "result$.value", Operation: ">", Right: 10},
			},
		},
		{
			name: "greater than or equal",
			conditions: []Condition{
				{Left: "result$.value", Operation: ">=", Right: 42},
			},
		},
		{
			name: "less than",
			conditions: []Condition{
				{Left: "result$.value", Operation: "<", Right: 100},
			},
		},
		{
			name: "less than or equal",
			conditions: []Condition{
				{Left: "result$.value", Operation: "<=", Right: 42},
			},
		},
	}

	for _, tt := range tests {
		ok, _, err := Execute(task, nil, nil, tt.conditions, logger)
		if err != nil {
			t.Fatalf("%s: Execute() error = %v", tt.name, err)
		}
		if !ok {
			t.Fatalf("%s: Execute() result = false, want true", tt.name)
		}
	}
}

func TestExecuteErrorsOnNonNumericComparison(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"value": "not-a-number",
		},
	}

	conditions := []Condition{{Left: "result$.value", Operation: ">", Right: 10}}

	if _, _, err := Execute(task, nil, nil, conditions, logger); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteErrorsWhenTaskIsNil(t *testing.T) {
	logger := newStubLogger()
	conditions := []Condition{{Left: "success", Operation: "=", Right: true}}

	if _, _, err := Execute(nil, nil, nil, conditions, logger); err == nil || !strings.Contains(err.Error(), "unsupported field") {
		t.Fatalf("expected unsupported field error, got %v", err)
	}
}

func TestExecuteResolvesPlaceholderFields(t *testing.T) {
	logger := newStubLogger()
	tasks := []flow.Task{{
		ID:         "http.task",
		Status:     flow.TaskStatusCompleted,
		Success:    true,
		ResultType: flow.ResultTypeJSON,
		Result:     unmarshalSample(t),
	}}

	conditions := []Condition{
		{Left: "${from.task:http.task.success}", Operation: "=", Right: true},
		{Left: "${from.task:http.task.result$.phoneNumbers[:1].type}", Operation: "=", Right: "iPhone"},
	}

	ok, _, err := Execute(nil, tasks, nil, conditions, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !ok {
		t.Fatalf("Execute() result = false, want true")
	}
}

func TestExecuteErrorsWhenPlaceholderTaskMissing(t *testing.T) {
	logger := newStubLogger()
	conditions := []Condition{
		{Left: "${from.task:missing.result$.response.status}", Operation: "=", Right: "200 OK"},
	}

	if _, _, err := Execute(nil, nil, nil, conditions, logger); err == nil || !strings.Contains(err.Error(), "referenced task \"missing\" not found") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}

func TestExecuteErrorsWithoutTaskAndPlaceholder(t *testing.T) {
	logger := newStubLogger()
	conditions := []Condition{{Left: "success", Operation: "=", Right: true}}

	if _, _, err := Execute(nil, nil, nil, conditions, logger); err == nil || !strings.Contains(err.Error(), "unsupported field") {
		t.Fatalf("expected unsupported field error, got %v", err)
	}
}

func TestConditionValidateErrors(t *testing.T) {
	tests := []struct {
		name    string
		cond    Condition
		expects string
	}{
		{name: "missing field", cond: Condition{Operation: "="}, expects: "field is required"},
		{name: "missing operation", cond: Condition{Left: "success"}, expects: "operation is required"},
		{name: "unsupported operation", cond: Condition{Left: "success", Operation: "INVALID_OP"}, expects: "unsupported operation"},
	}

	for _, tt := range tests {
		if err := tt.cond.Validate(); err == nil || !strings.Contains(err.Error(), tt.expects) {
			t.Fatalf("%s: expected error containing %q, got %v", tt.name, tt.expects, err)
		}
	}
}

func TestConditionValidateSupportsComparisons(t *testing.T) {
	operations := []string{"=", ">", "<", ">=", "<="}

	for _, op := range operations {
		cond := Condition{Left: "success", Operation: op, Right: true}
		if err := cond.Validate(); err != nil {
			t.Fatalf("operation %q: expected no error, got %v", op, err)
		}
	}
}

func TestConditionValidateSupportsLegacyFieldAlias(t *testing.T) {
	cond := Condition{Field: "success", Operation: "=", Expected: true}
	if err := cond.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestActionExecuteThenBranchBreaks(t *testing.T) {
	act := action{}
	logger := newStubLogger()
	payload := map[string]any{
		"if_conditions": []map[string]any{
			{"left": "${flag}", "operation": "=", "right": true},
		},
		"then": map[string]any{
			"break": "stop now",
		},
		"else": map[string]any{},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	execCtx := &registry.ExecutionContext{
		Task:  &flow.Task{ID: "evaluate"},
		Tasks: []flow.Task{{ID: "evaluate"}},
		Variables: map[string]registry.Variable{
			"flag": {Name: "flag", Type: "bool", Value: true},
		},
		Logger: logger,
	}

	result, err := act.Execute(context.Background(), raw, execCtx)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Control == nil || !result.Control.BreakLoop {
		t.Fatalf("expected break control, got %#v", result.Control)
	}
	if !logger.contains("Evaluate then branch break: stop now") {
		t.Fatalf("expected break log message, got %v", logger.messages)
	}
}

func TestDecodeTaskRejectsBreakWithGoto(t *testing.T) {
	payload := map[string]any{
		"if_conditions": []map[string]any{
			{"left": "success", "operation": "=", "right": true},
		},
		"then": map[string]any{
			"break":    "stop",
			"gototask": "next",
		},
		"else": map[string]any{},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := decodeTask(raw); err == nil || !strings.Contains(err.Error(), ".break cannot be combined with goto") {
		t.Fatalf("expected break/goto validation error, got %v", err)
	}
}

func TestExecuteHandlesExtendedOperators(t *testing.T) {
	logger := newStubLogger()
	task := &flow.Task{
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result: map[string]any{
			"name":      "Green Day",
			"score":     100,
			"tags":      []any{"punk", "rock", "90s"},
			"meta":      map[string]any{"id": "123"},
			"emptyList": []any{},
		},
	}

	tests := []struct {
		name       string
		conditions []Condition
		wantResult bool
	}{
		// !=
		{
			name:       "not equal success",
			conditions: []Condition{{Left: "result$.score", Operation: "!=", Right: 50}},
			wantResult: true,
		},
		{
			name:       "not equal failure",
			conditions: []Condition{{Left: "result$.score", Operation: "!=", Right: 100}},
			wantResult: false,
		},
		// STARTS_WITH
		{
			name:       "starts with success",
			conditions: []Condition{{Left: "result$.name", Operation: "STARTS_WITH", Right: "Green"}},
			wantResult: true,
		},
		{
			name:       "starts with failure",
			conditions: []Condition{{Left: "result$.name", Operation: "STARTS_WITH", Right: "Blue"}},
			wantResult: false,
		},
		// ENDS_WITH
		{
			name:       "ends with success",
			conditions: []Condition{{Left: "result$.name", Operation: "ENDS_WITH", Right: "Day"}},
			wantResult: true,
		},
		{
			name:       "ends with failure",
			conditions: []Condition{{Left: "result$.name", Operation: "ENDS_WITH", Right: "Night"}},
			wantResult: false,
		},
		// CONTAINS
		{
			name:       "contains success",
			conditions: []Condition{{Left: "result$.name", Operation: "CONTAINS", Right: "een"}},
			wantResult: true,
		},
		{
			name:       "contains failure",
			conditions: []Condition{{Left: "result$.name", Operation: "CONTAINS", Right: "xyz"}},
			wantResult: false,
		},
		// MATCHES
		{
			name:       "matches success",
			conditions: []Condition{{Left: "result$.name", Operation: "MATCHES", Right: "^Green.*"}},
			wantResult: true,
		},
		{
			name:       "matches failure",
			conditions: []Condition{{Left: "result$.name", Operation: "MATCHES", Right: "^[0-9]+$"}},
			wantResult: false,
		},
		// IN (Field IN List)
		{
			name:       "in success",
			conditions: []Condition{{Left: "result$.name", Operation: "IN", Right: []any{"Green Day", "The Offspring"}}},
			wantResult: true,
		},
		{
			name:       "in failure",
			conditions: []Condition{{Left: "result$.name", Operation: "IN", Right: []any{"Metallica", "Slayer"}}},
			wantResult: false,
		},
		// NOT_IN (Field NOT_IN List)
		{
			name:       "not in success",
			conditions: []Condition{{Left: "result$.name", Operation: "NOT_IN", Right: []any{"Metallica", "Slayer"}}},
			wantResult: true,
		},
		{
			name:       "not in failure",
			conditions: []Condition{{Left: "result$.name", Operation: "NOT_IN", Right: []any{"Green Day", "The Offspring"}}},
			wantResult: false,
		},
		// CONTAINS (List CONTAINS Item)
		{
			name:       "list contains success",
			conditions: []Condition{{Left: "result$.tags", Operation: "CONTAINS", Right: "punk"}},
			wantResult: true,
		},
		{
			name:       "list contains failure",
			conditions: []Condition{{Left: "result$.tags", Operation: "CONTAINS", Right: "jazz"}},
			wantResult: false,
		},
		// Empty List NOT_IN
		// We want to check if "anything" is NOT IN emptyList.
		// Since we can't put literal "anything" on Left, let's use a field we know exists but isn't in the list?
		// actually, let's test that emptyList CONTAINS "anything" is false.
		{
			name:       "empty list contains failure",
			conditions: []Condition{{Left: "result$.emptyList", Operation: "CONTAINS", Right: "anything"}},
			wantResult: false,
		},
	}

	// We don't need the ${from.task...} placeholders for these simple tests if we rely on the implicit task context.
	// But Execute signature is (task, tasks, variables...).
	// If we pass 'task' as the first arg, simple field references like "result$.name" work.

	// Ensure we pass the main task as the first argument
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, err := Execute(task, nil, nil, tt.conditions, logger)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if ok != tt.wantResult {
				t.Fatalf("Execute() result = %v, want %v", ok, tt.wantResult)
			}
		})
	}
}
