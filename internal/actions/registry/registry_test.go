package registry

import (
	"context"
	"encoding/json"
	"testing"

	"flowk/internal/flow"
)

type testAction struct {
	name string
}

func (a testAction) Name() string { return a.name }
func (a testAction) Execute(context.Context, json.RawMessage, *ExecutionContext) (Result, error) {
	return Result{}, nil
}

func resetRegistryState(t *testing.T) {
	t.Helper()

	mu.Lock()
	prevActions := actions
	prevFragments := schemaFragments
	prevVersion := schemaVersion
	actions = make(map[string]Action)
	schemaFragments = make(map[string]json.RawMessage)
	schemaVersion = 0
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		actions = prevActions
		schemaFragments = prevFragments
		schemaVersion = prevVersion
		mu.Unlock()
	})
}

func TestRegisterLookupAndNames(t *testing.T) {
	resetRegistryState(t)

	Register(testAction{name: "zeta"})
	Register(testAction{name: "alpha"})

	if _, ok := Lookup(""); ok {
		t.Fatal("Lookup with empty key should fail")
	}

	action, ok := Lookup("ALPHA")
	if !ok || action.Name() != "alpha" {
		t.Fatalf("Lookup did not return alpha action: %#v, ok=%v", action, ok)
	}

	names := Names()
	if len(names) != 2 || names[0] != "ALPHA" || names[1] != "ZETA" {
		t.Fatalf("unexpected names ordering: %v", names)
	}
}

func TestRegisterPanicsOnInvalidInput(t *testing.T) {
	resetRegistryState(t)

	t.Run("nil action", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nil action")
			}
		}()
		Register(nil)
	})

	t.Run("duplicate action", func(t *testing.T) {
		Register(testAction{name: "alpha"})
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for duplicate action")
			}
		}()
		Register(testAction{name: "alpha"})
	})
}

func TestJSONEqual(t *testing.T) {
	a := json.RawMessage(`{"a":1}`)
	b := json.RawMessage(`{"a":1}`)
	c := json.RawMessage(`{"a":2}`)
	if !jsonEqual(a, b) {
		t.Fatal("expected equal JSON slices")
	}
	if jsonEqual(a, c) {
		t.Fatal("expected non-equal JSON slices")
	}
	if flow.ResultTypeJSON == "" {
		t.Fatal("sanity check for imported flow package")
	}
}
