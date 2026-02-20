package variables

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"flowk/internal/flow"
	jsonpathutil "flowk/internal/shared/jsonpathutil"
)

func TestExecuteCreatesVariables(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "baseUrl", Type: "string", Value: "https://example.org"},
			{Name: "retryLimit", Type: "number", Value: 3},
			{Name: "feature", Type: "bool", Value: true},
		},
	}

	existing := make(map[string]Variable)
	result, resultType, err := Execute(payload, existing, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resultType != flow.ResultTypeJSON {
		t.Fatalf("expected result type json, got %s", resultType)
	}
	if len(existing) != 3 {
		t.Fatalf("expected 3 variables stored, got %d", len(existing))
	}
	if got := existing["baseUrl"].Value; got != "https://example.org" {
		t.Fatalf("unexpected baseUrl value: %v", got)
	}
	if got := existing["retryLimit"].Value; got != float64(3) {
		t.Fatalf("unexpected retryLimit value: %v", got)
	}
	if result["feature"] != true {
		t.Fatalf("expected feature result true, got %v", result["feature"])
	}
}

func TestExecuteAppliesMathOperation(t *testing.T) {
	existing := map[string]Variable{
		"counter": {Name: "counter", Type: "number", Value: float64(10)},
		"step":    {Name: "step", Type: "number", Value: float64(2)},
	}

	payload := Payload{
		Scope:     scopeFlow,
		Overwrite: true,
		Vars: []VariableConfig{
			{
				Name:  "factor",
				Type:  "number",
				Value: 3,
			},
			{
				Name: "counter",
				Type: "number",
				Operation: &MathOperation{
					Operator: "multiply",
					Variable: "factor",
				},
			},
		},
	}

	result, resultType, err := Execute(payload, existing, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resultType != flow.ResultTypeJSON {
		t.Fatalf("expected json result type, got %s", resultType)
	}

	got := existing["counter"].Value
	want := float64(10 * 3)
	if got != want {
		t.Fatalf("expected counter %v, got %v", want, got)
	}

	if result["counter"] != want {
		t.Fatalf("expected result counter %v, got %v", want, result["counter"])
	}

	second := Payload{
		Scope:     scopeFlow,
		Overwrite: true,
		Vars: []VariableConfig{{
			Name: "counter",
			Type: "number",
			Operation: &MathOperation{
				Operator: "add",
				Variable: "step",
			},
		}},
	}

	result, resultType, err = Execute(second, existing, nil)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if resultType != flow.ResultTypeJSON {
		t.Fatalf("expected json result type, got %s", resultType)
	}

	got = existing["counter"].Value
	want = float64((10 * 3) + 2)
	if got != want {
		t.Fatalf("expected counter %v, got %v", want, got)
	}
	if result["counter"] != want {
		t.Fatalf("expected second result counter %v, got %v", want, result["counter"])
	}
}

func TestExecuteMathOperationErrors(t *testing.T) {
	tests := []struct {
		name     string
		payload  Payload
		existing map[string]Variable
		wantErr  string
	}{
		{
			name: "missing base variable",
			payload: Payload{
				Scope:     scopeFlow,
				Overwrite: true,
				Vars: []VariableConfig{{
					Name:      "counter",
					Type:      "number",
					Operation: &MathOperation{Operator: "add", Variable: "step"},
				}},
			},
			existing: map[string]Variable{
				"step": {Name: "step", Type: "number", Value: float64(1)},
			},
			wantErr: "operation requires existing variable",
		},
		{
			name: "missing operand variable",
			payload: Payload{
				Scope:     scopeFlow,
				Overwrite: true,
				Vars: []VariableConfig{{
					Name:      "counter",
					Type:      "number",
					Operation: &MathOperation{Operator: "add", Variable: "missing"},
				}},
			},
			existing: map[string]Variable{
				"counter": {Name: "counter", Type: "number", Value: float64(1)},
			},
			wantErr: "referenced variable \"missing\" not found",
		},
		{
			name: "division by zero",
			payload: Payload{
				Scope:     scopeFlow,
				Overwrite: true,
				Vars: []VariableConfig{{
					Name:      "counter",
					Type:      "number",
					Operation: &MathOperation{Operator: "divide", Variable: "zero"},
				}},
			},
			existing: map[string]Variable{
				"counter": {Name: "counter", Type: "number", Value: float64(4)},
				"zero":    {Name: "zero", Type: "number", Value: float64(0)},
			},
			wantErr: "division by zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Execute(tt.payload, tt.existing, nil)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestExecuteCoercesTypes(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "retry", Type: "number", Value: "5"},
			{Name: "enabled", Type: "bool", Value: "true"},
			{Name: "ids", Type: "array", Value: []any{float64(1), float64(2)}},
			{Name: "meta", Type: "object", Value: map[string]any{"env": "test"}},
		},
	}

	existing := make(map[string]Variable)
	if _, _, err := Execute(payload, existing, nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if v := existing["retry"].Value; v != float64(5) {
		t.Fatalf("expected retry to be 5, got %v", v)
	}
	if v := existing["enabled"].Value; v != true {
		t.Fatalf("expected enabled to be true, got %v", v)
	}
	if v := existing["ids"].Value; len(v.([]any)) != 2 {
		t.Fatalf("expected ids length 2, got %v", v)
	}
	if v := existing["meta"].Value.(map[string]any)["env"]; v != "test" {
		t.Fatalf("expected meta.env to be test, got %v", v)
	}
}

func TestExecuteResolvesTaskPlaceholders(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "token", Type: "string", Value: "${from.task:http.login$.body.token}"},
			{Name: "count", Type: "number", Value: "${from.task:http.login$.body.count}"},
			{Name: "enabled", Type: "bool", Value: "${from.task:http.login$.body.enabled}"},
		},
	}

	tasks := []flow.Task{
		{
			ID:         "http.login",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"token":   "abc123",
					"count":   float64(7),
					"enabled": true,
				},
			},
		},
	}

	existing := make(map[string]Variable)
	if _, _, err := Execute(payload, existing, tasks); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if v := existing["token"].Value; v != "abc123" {
		t.Fatalf("expected token to be abc123, got %v", v)
	}
	if v := existing["count"].Value; v != float64(7) {
		t.Fatalf("expected count to be 7, got %v", v)
	}
	if v := existing["enabled"].Value; v != true {
		t.Fatalf("expected enabled to be true, got %v", v)
	}
}

func TestExecuteResolvesVariablePlaceholders(t *testing.T) {
	existing := map[string]Variable{
		"limit": {Name: "limit", Type: "number", Value: float64(5)},
	}

	payload := Payload{
		Scope:     scopeFlow,
		Overwrite: true,
		Vars: []VariableConfig{
			{Name: "platform_name", Type: "string", Value: "tic-dev08"},
			{Name: "namespace", Type: "string", Value: "tic-${platform_name}"},
			{Name: "limit_copy", Type: "number", Value: "${limit}"},
		},
	}

	result, resultType, err := Execute(payload, existing, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resultType != flow.ResultTypeJSON {
		t.Fatalf("expected json result type, got %s", resultType)
	}

	if got := existing["namespace"].Value; got != "tic-tic-dev08" {
		t.Fatalf("expected namespace to be tic-tic-dev08, got %v", got)
	}
	if got := result["namespace"]; got != "tic-tic-dev08" {
		t.Fatalf("expected namespace result tic-tic-dev08, got %v", got)
	}

	if got := existing["limit_copy"].Value; got != float64(5) {
		t.Fatalf("expected limit_copy to be 5, got %v", got)
	}
	if got := result["limit_copy"]; got != float64(5) {
		t.Fatalf("expected limit_copy result 5, got %v", got)
	}
}

func TestExecuteResolvesVariablePlaceholdersInsideTaskPlaceholder(t *testing.T) {
	northernID := "8000793f-a1ae-4ec4-8d55-ef83f1f644e5"

	existing := map[string]Variable{
		"northernWorkingGroupId": {Name: "northernWorkingGroupId", Type: "string", Value: northernID},
	}

	tasks := []flow.Task{
		{
			ID:         "dog_api.fetch_page",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"data": []any{
						map[string]any{
							"attributes": map[string]any{
								"name": "Akita",
							},
							"relationships": map[string]any{
								"group": map[string]any{
									"data": map[string]any{
										"id": northernID,
									},
								},
							},
						},
						map[string]any{
							"attributes": map[string]any{
								"name": "Beagle",
							},
							"relationships": map[string]any{
								"group": map[string]any{
									"data": map[string]any{
										"id": "other-group",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	payload := Payload{
		Scope:     scopeFlow,
		Overwrite: true,
		Vars: []VariableConfig{
			{
				Name:  "northernWorkingBreeds",
				Type:  "array",
				Value: "${from.task:dog_api.fetch_page.result$.body.data[?(@.relationships.group.data.id == '${northernWorkingGroupId}')].attributes.name}",
			},
		},
	}

	result, resultType, err := Execute(payload, existing, tasks)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if resultType != flow.ResultTypeJSON {
		t.Fatalf("expected json result type, got %s", resultType)
	}

	got, ok := existing["northernWorkingBreeds"].Value.([]any)
	if !ok {
		t.Fatalf("expected northernWorkingBreeds to be an array, got %T", existing["northernWorkingBreeds"].Value)
	}
	if len(got) != 1 || got[0] != "Akita" {
		t.Fatalf("expected northernWorkingBreeds to contain Akita, got %v", got)
	}

	res, ok := result["northernWorkingBreeds"].([]any)
	if !ok {
		t.Fatalf("expected result northernWorkingBreeds to be an array, got %T", result["northernWorkingBreeds"])
	}
	if len(res) != 1 || res[0] != "Akita" {
		t.Fatalf("expected result northernWorkingBreeds to contain Akita, got %v", res)
	}
}

func TestExecuteVariablePlaceholderMissing(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{{
			Name:  "namespace",
			Type:  "string",
			Value: "tic-${missing}",
		}},
	}

	_, _, err := Execute(payload, make(map[string]Variable), nil)
	if err == nil {
		t.Fatalf("expected error when referencing missing variable")
	}
	if !strings.Contains(err.Error(), "variable \"missing\" not defined") {
		t.Fatalf("expected error mentioning missing variable, got %v", err)
	}
}

func TestResolveTaskPlaceholders(t *testing.T) {
	tasks := []flow.Task{
		{
			ID:         "http.login",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"token":   "abc123",
					"count":   float64(7),
					"enabled": true,
				},
			},
		},
	}

	tests := []struct {
		name  string
		input string
		want  any
	}{
		{name: "string", input: "${from.task:http.login$.body.token}", want: "abc123"},
		{name: "number", input: "${from.task:http.login$.body.count}", want: float64(7)},
		{name: "bool", input: "${from.task:http.login$.body.enabled}", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTaskPlaceholders(tt.input, tasks)
			if err != nil {
				t.Fatalf("resolveTaskPlaceholders returned error: %v", err)
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

func TestResolveFromTaskPlaceholderLengthExtension(t *testing.T) {
	tasks := []flow.Task{
		{
			ID:         "http.obtener_breeds",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"data": []any{
						map[string]any{"id": 1},
						map[string]any{"id": 2},
					},
					"message": "hola",
					"meta": map[string]any{
						"total": 2,
					},
				},
			},
		},
	}

	tests := []struct {
		name    string
		expr    string
		want    any
		wantErr string
	}{
		{
			name: "array length",
			expr: "http.obtener_breeds$.body.data.length()",
			want: float64(2),
		},
		{
			name: "string length",
			expr: "http.obtener_breeds$.body.message.length()",
			want: float64(len("hola")),
		},
		{
			name:    "unsupported type",
			expr:    "http.obtener_breeds$.body.meta.total.length()",
			wantErr: "length() unsupported",
		},
		{
			name: "object length",
			expr: "http.obtener_breeds$.body.meta.length()",
			want: float64(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveFromTaskPlaceholder(tt.expr, tasks)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestExecuteHonorsOverwriteFlag(t *testing.T) {
	existing := map[string]Variable{
		"retry": {Name: "retry", Type: "number", Value: float64(2)},
	}

	payload := Payload{
		Scope: scopeFlow,
		Vars:  []VariableConfig{{Name: "retry", Type: "number", Value: 5}},
	}

	if _, _, err := Execute(payload, existing, nil); err == nil {
		t.Fatalf("expected error when overwriting without flag")
	}

	payload.Overwrite = true
	if _, _, err := Execute(payload, existing, nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if v := existing["retry"].Value; v != float64(5) {
		t.Fatalf("expected retry to be 5 after overwrite, got %v", v)
	}
}

func TestExecuteMasksSecretValues(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars:  []VariableConfig{{Name: "token", Type: "secret", Value: "s3cr3t"}},
	}

	existing := make(map[string]Variable)
	result, _, err := Execute(payload, existing, nil)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if v := existing["token"].Value; v != "s3cr3t" {
		t.Fatalf("expected stored secret value, got %v", v)
	}
	if res := result["token"]; res != "****" {
		t.Fatalf("expected masked secret in result, got %v", res)
	}
}

func TestExecuteMissingTaskPlaceholder(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars:  []VariableConfig{{Name: "token", Type: "string", Value: "${from.task:unknown$.body.token}"}},
	}

	existing := make(map[string]Variable)
	if _, _, err := Execute(payload, existing, nil); err == nil {
		t.Fatalf("expected error when resolving unknown task")
	}
}

func TestExecuteInvalidBool(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars:  []VariableConfig{{Name: "flag", Type: "bool", Value: "notabool"}},
	}

	existing := make(map[string]Variable)
	if _, _, err := Execute(payload, existing, nil); err == nil {
		t.Fatalf("expected error for invalid bool value")
	}
}

func TestValidateUnsupportedScope(t *testing.T) {
	payload := Payload{Scope: "session", Vars: []VariableConfig{{Name: "foo", Type: "string", Value: "bar"}}}
	err := payload.Validate()
	if err == nil {
		t.Fatalf("expected validation error for unsupported scope")
	}
	if !strings.Contains(err.Error(), "unsupported scope") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidateRequiresVars(t *testing.T) {
	payload := Payload{Scope: scopeFlow}
	if err := payload.Validate(); err == nil {
		t.Fatalf("expected error when vars list is empty")
	}
}

func TestValidateDetectsDuplicateNames(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "foo", Type: "string", Value: "a"},
			{Name: "foo", Type: "string", Value: "b"},
		},
	}

	err := payload.Validate()
	if err == nil {
		t.Fatalf("expected validation error for duplicate names")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVariableConfigValidateRejectsInvalidNamesAndTypes(t *testing.T) {
	tests := []struct {
		name string
		cfg  VariableConfig
	}{
		{
			name: "empty name",
			cfg:  VariableConfig{Name: " ", Type: "string", Value: "a"},
		},
		{
			name: "invalid character",
			cfg:  VariableConfig{Name: "bad*name", Type: "string", Value: "a"},
		},
		{
			name: "unsupported type",
			cfg:  VariableConfig{Name: "foo", Type: "bytes", Value: []byte("a")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatalf("expected validation failure for %s", tt.name)
			}
		})
	}
}

func TestResolveValueWithEmbeddedPlaceholder(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "message", Type: "string", Value: "token=${from.task:http.login$.body.token}"},
		},
	}

	tasks := []flow.Task{
		{
			ID:         "http.login",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{"token": "abc123"},
			},
		},
	}

	existing := make(map[string]Variable)
	result, _, err := Execute(payload, existing, tasks)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if got := existing["message"].Value; got != "token=abc123" {
		t.Fatalf("unexpected stored value: %v", got)
	}
	if got := result["message"]; got != "token=abc123" {
		t.Fatalf("unexpected result value: %v", got)
	}
}

func TestResolveValueRendersNonStringPlaceholder(t *testing.T) {
	payload := Payload{
		Scope: scopeFlow,
		Vars: []VariableConfig{
			{Name: "ids", Type: "string", Value: "${from.task:list$.body}"},
		},
	}

	tasks := []flow.Task{
		{
			ID:         "list",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": []any{map[string]any{"id": 1}},
			},
		},
	}

	existing := make(map[string]Variable)
	if _, _, err := Execute(payload, existing, tasks); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := existing["ids"].Value; got != `[{"id":1}]` {
		t.Fatalf("unexpected rendered value: %v", got)
	}
}

func TestResolveFromTaskPlaceholderSupportsResultSuffix(t *testing.T) {
	tasks := []flow.Task{
		{
			ID:         "task.one",
			Status:     flow.TaskStatusCompleted,
			ResultType: flow.ResultTypeJSON,
			Result: map[string]any{
				"body": map[string]any{
					"meta": map[string]any{"current": float64(3)},
				},
			},
		},
	}

	value, err := resolveFromTaskPlaceholder("task.one.result$.body.meta.current", tasks)
	if err != nil {
		t.Fatalf("resolveFromTaskPlaceholder returned error: %v", err)
	}
	if value != float64(3) {
		t.Fatalf("unexpected value: %v", value)
	}

	value, err = resolveFromTaskPlaceholder("task.one.result$body.meta.current", tasks)
	if err != nil {
		t.Fatalf("resolveFromTaskPlaceholder without dot returned error: %v", err)
	}
	if value != float64(3) {
		t.Fatalf("unexpected value without dot: %v", value)
	}
}

func TestResolveFromTaskPlaceholderErrors(t *testing.T) {
	baseTask := flow.Task{
		ID:         "task.one",
		Status:     flow.TaskStatusCompleted,
		ResultType: flow.ResultTypeJSON,
		Result:     map[string]any{"data": 5},
	}

	tests := []struct {
		name    string
		expr    string
		mutate  func(task *flow.Task) []flow.Task
		wantErr string
	}{
		{name: "empty", expr: "  ", mutate: func(task *flow.Task) []flow.Task { return []flow.Task{*task} }, wantErr: "empty"},
		{name: "missing path", expr: "task.one", mutate: func(task *flow.Task) []flow.Task { return []flow.Task{*task} }, wantErr: "missing json path"},
		{name: "missing task", expr: "missing$.data", mutate: func(task *flow.Task) []flow.Task { return []flow.Task{*task} }, wantErr: "not found"},
		{name: "not completed", expr: "task.one$.data", mutate: func(task *flow.Task) []flow.Task {
			clone := *task
			clone.Status = flow.TaskStatusInProgress
			return []flow.Task{clone}
		}, wantErr: "not completed"},
		{name: "non json", expr: "task.one$.data", mutate: func(task *flow.Task) []flow.Task {
			clone := *task
			clone.ResultType = flow.ResultTypeString
			return []flow.Task{clone}
		}, wantErr: "does not contain json"},
		{name: "json path error", expr: "task.one$.unknown", mutate: func(task *flow.Task) []flow.Task { return []flow.Task{*task} }, wantErr: "evaluating json path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks := tt.mutate(&baseTask)
			if _, err := resolveFromTaskPlaceholder(tt.expr, tasks); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNormalizeJSONPath(t *testing.T) {
	tests := map[string]string{
		"$.foo":    "$.foo",
		"foo":      "$.foo",
		".foo":     "$.foo",
		" [0] ":    "$[0]",
		"$['id']":  `$["id"]`,
		"$body":    "$.body",
		"$body.id": "$.body.id",
	}

	for input, want := range tests {
		if got := normalizeJSONPath(input); got != want {
			t.Fatalf("normalizeJSONPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeJSONContainerHandlesEncodings(t *testing.T) {
	raw := json.RawMessage(`{"value":1}`)
	data := []byte(`{"flag":true}`)

	type sample struct {
		Count int `json:"count"`
	}

	normalized := jsonpathutil.NormalizeContainer(map[string]any{
		"raw":    raw,
		"data":   data,
		"struct": sample{Count: 7},
		"slice":  []any{raw},
	}).(map[string]any)

	if got := normalized["raw"].(map[string]any)["value"]; got != float64(1) {
		t.Fatalf("unexpected raw value: %v", got)
	}
	if got := normalized["data"].(map[string]any)["flag"]; got != true {
		t.Fatalf("unexpected data value: %v", got)
	}
	if got := normalized["struct"].(map[string]any)["count"]; got != float64(7) {
		t.Fatalf("unexpected struct value: %v", got)
	}
	if _, ok := normalized["slice"].([]any)[0].(map[string]any); !ok {
		t.Fatalf("expected slice element to be normalized map")
	}
}

func TestExecuteStoresProxyVariables(t *testing.T) {
        payload := Payload{
                Scope: scopeFlow,
                Vars: []VariableConfig{
                        {
                                Name: "corporate_proxy",
                                Type: "proxy",
                                Value: map[string]any{
                                        "http":         "http://proxy.internal:8080",
                                        "HTTPS_PROXY":  "https://proxy.internal:8443",
                                        "no":           "localhost,127.0.0.1",
                                        "EXTRA_PROXY":  "socks5://proxy.internal:1080",
                                        "custom_field": "ignored",
                                },
                        },
                },
        }

        existing := make(map[string]Variable)
        result, resultType, err := Execute(payload, existing, nil)
        if err != nil {
                t.Fatalf("Execute() error = %v", err)
        }
        if resultType != flow.ResultTypeJSON {
                t.Fatalf("unexpected result type: %s", resultType)
        }

        variable, ok := existing["corporate_proxy"]
        if !ok {
                t.Fatalf("expected proxy variable to be stored")
        }
        if !strings.EqualFold(variable.Type, "proxy") {
                t.Fatalf("expected proxy variable type, got %q", variable.Type)
        }

        proxies, ok := variable.Value.(map[string]string)
        if !ok {
                t.Fatalf("expected proxy variable value to be map[string]string, got %T", variable.Value)
        }

        expected := map[string]string{
                "http":         "http://proxy.internal:8080",
                "HTTPS_PROXY":  "https://proxy.internal:8443",
                "no":           "localhost,127.0.0.1",
                "EXTRA_PROXY":  "socks5://proxy.internal:1080",
                "custom_field": "ignored",
        }
        for key, want := range expected {
                if got := proxies[key]; got != want {
                        t.Fatalf("proxy %q mismatch: got %q want %q", key, got, want)
                }
        }

        formatted, ok := result["corporate_proxy"].(map[string]string)
        if !ok {
                t.Fatalf("expected proxy result to be map[string]string, got %T", result["corporate_proxy"])
        }
        for key, want := range expected {
                if got := formatted[key]; got != want {
                        t.Fatalf("result proxy %q mismatch: got %q want %q", key, got, want)
                }
        }
}

func TestCoerceValueErrors(t *testing.T) {
        if _, err := coerceValue("array", "not array"); err == nil {
                t.Fatalf("expected error coercing array")
        }
        if _, err := coerceValue("object", "not object"); err == nil {
                t.Fatalf("expected error coercing object")
        }
        if _, err := coerceValue("unknown", "value"); err == nil {
                t.Fatalf("expected error for unsupported type")
        }
}

func TestToNumberRejectsEmptyString(t *testing.T) {
	if _, err := toNumber(""); err == nil {
		t.Fatalf("expected error parsing empty string")
	}
}
