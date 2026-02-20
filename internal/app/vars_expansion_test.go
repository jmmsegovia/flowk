package app

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	expansion "flowk/internal/shared/expansion"
)

func TestExpandEvaluateTaskPayloadPreservesConditions(t *testing.T) {
	raw := json.RawMessage(`{
        "if_conditions": [
            {"left": "${loopvar}", "operation": "=", "right": "${other}"}
        ],
        "then": {"continue": "${loopvar}"},
        "else": {"continue": "no-op"}
    }`)

	vars := map[string]Variable{
		"loopvar": {Name: "loopvar", Value: "hola"},
		"other":   {Name: "other", Value: "mundo"},
	}

	expanded, err := expansion.ExpandEvaluateTaskPayload(raw, vars, nil)
	if err != nil {
		t.Fatalf("expandEvaluateTaskPayload() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(expanded, &decoded); err != nil {
		t.Fatalf("unmarshal expanded payload: %v", err)
	}

	conditions, ok := decoded["if_conditions"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatalf("if_conditions missing or unexpected type: %+v", decoded["if_conditions"])
	}

	condition, ok := conditions[0].(map[string]any)
	if !ok {
		t.Fatalf("condition is not a map: %+v", conditions[0])
	}

	if got := condition["left"]; got != "${loopvar}" {
		t.Fatalf("condition left = %v, want ${loopvar}", got)
	}
	if got := condition["right"]; got != "${other}" {
		t.Fatalf("condition right = %v, want ${other}", got)
	}

	thenBranch, ok := decoded["then"].(map[string]any)
	if !ok {
		t.Fatalf("then branch not found: %+v", decoded["then"])
	}
	if got := thenBranch["continue"]; got != "hola" {
		t.Fatalf("then.continue = %v, want hola", got)
	}
}

func TestExpandTaskPayloadExpandsSkipTables(t *testing.T) {
	raw := json.RawMessage(`{
        "platform": "${platform_config}",
        "operation": "TRUNCATE_ALL_TABLES",
        "skip_tables": [
            "tic_${platform_name}_mgr.tic_${platform_name}_mgr_version",
            "tic_${platform_name}_disp.tic_${platform_name}_disp_version"
        ]
    }`)

	vars := map[string]Variable{
		"platform_name": {Name: "platform_name", Value: "dev02"},
		"platform_config": {
			Name: "platform_config",
			Value: map[string]any{
				"cassandra": map[string]any{
					"keyspaces": []any{
						map[string]any{
							"name": "tic_${platform_name}_mgr",
						},
					},
				},
			},
		},
	}

	expanded, err := expansion.ExpandTaskPayload(raw, vars, nil)
	if err != nil {
		t.Fatalf("expandTaskPayload() error = %v", err)
	}

	var payload struct {
		SkipTables []string `json:"skip_tables"`
		Platform   struct {
			Cassandra struct {
				Keyspaces []struct {
					Name string `json:"name"`
				} `json:"keyspaces"`
			} `json:"cassandra"`
		} `json:"platform"`
	}

	if err := json.Unmarshal(expanded, &payload); err != nil {
		t.Fatalf("unmarshal expanded payload: %v", err)
	}

	wantTables := []string{
		"tic_dev02_mgr.tic_dev02_mgr_version",
		"tic_dev02_disp.tic_dev02_disp_version",
	}

	if diff := cmp.Diff(wantTables, payload.SkipTables); diff != "" {
		t.Fatalf("skip_tables diff (-want +got):\n%s", diff)
	}

	if len(payload.Platform.Cassandra.Keyspaces) != 1 {
		t.Fatalf("expected one keyspace, got %d", len(payload.Platform.Cassandra.Keyspaces))
	}

	if got := payload.Platform.Cassandra.Keyspaces[0].Name; got != "tic_dev02_mgr" {
		t.Fatalf("keyspace name = %q, want tic_dev02_mgr", got)
	}
}
