package jsonpathutil

import "testing"

func TestEvaluateSupportsComparisonFilters(t *testing.T) {
	container := map[string]any{
		"items": []any{
			map[string]any{"name": "alpha", "value": 1},
			map[string]any{"name": "bravo", "value": 2},
			map[string]any{"name": "charlie", "value": 3},
		},
	}

	normalized := NormalizeContainer(container)

	result, err := Evaluate("$.items[?(@.value >= 2)].name", normalized)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	names, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any result, got %T", result)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	if names[0] != "bravo" || names[1] != "charlie" {
		t.Fatalf("unexpected names: %#v", names)
	}
}

func TestNormalizeContainerHandlesTypedSlices(t *testing.T) {
	type deployment struct {
		Name  string `json:"name"`
		Ready int    `json:"readyReplicas"`
	}

	container := []deployment{{Name: "tic-alpha", Ready: 1}, {Name: "tic-bravo", Ready: 0}}

	normalized := NormalizeContainer(container)

	result, err := Evaluate("$[*].name", normalized)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	names, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any result, got %T", result)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	if names[0] != "tic-alpha" || names[1] != "tic-bravo" {
		t.Fatalf("unexpected names: %#v", names)
	}
}
