package expansion

import (
	"encoding/json"
	"testing"
)

func TestExpandParallelTaskPayloadSkipsNestedTasks(t *testing.T) {
	raw := json.RawMessage(`{
        "fail_fast": "${parallel_fail_fast}",
        "merge_strategy": "${merge_strategy}",
        "tasks": [
            {
                "id": "loop-task",
                "action": "PRINT",
                "entries": [
                    {"message": "Iteration ${loop_counter}"}
                ]
            }
        ]
    }`)

	vars := map[string]Variable{
		"parallel_fail_fast": {Name: "parallel_fail_fast", Value: true},
		"merge_strategy":     {Name: "merge_strategy", Value: "last_write_wins"},
	}

	expanded, err := ExpandParallelTaskPayload(raw, vars, nil)
	if err != nil {
		t.Fatalf("ExpandParallelTaskPayload() error = %v", err)
	}

	var payload struct {
		FailFast      bool   `json:"fail_fast"`
		MergeStrategy string `json:"merge_strategy"`
		Tasks         []struct {
			Entries []struct {
				Message string `json:"message"`
			} `json:"entries"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal(expanded, &payload); err != nil {
		t.Fatalf("unmarshal expanded payload: %v", err)
	}

	if !payload.FailFast {
		t.Fatalf("fail_fast = %v, want true", payload.FailFast)
	}

	if payload.MergeStrategy != "last_write_wins" {
		t.Fatalf("merge_strategy = %q, want last_write_wins", payload.MergeStrategy)
	}

	if len(payload.Tasks) != 1 || len(payload.Tasks[0].Entries) != 1 {
		t.Fatalf("unexpected tasks payload: %+v", payload.Tasks)
	}

	if payload.Tasks[0].Entries[0].Message != "Iteration ${loop_counter}" {
		t.Fatalf("nested task message = %q, want Iteration ${loop_counter}", payload.Tasks[0].Entries[0].Message)
	}
}
