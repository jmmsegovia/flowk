package flow_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestLoadDefinitionAcceptsRegisteredExternalAction(t *testing.T) {
	dir := t.TempDir()
	flow.SetupSchemaProviderForTesting(t)

	ensureExternalActionRegistered()

	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"external.action","description":"test","tasks":[{"id":"ext","description":"external task","action":"EXTERNAL_TEST_ACTION"}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := flow.LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
}

func TestLoadDefinitionRejectsUnknownExternalAction(t *testing.T) {
	dir := t.TempDir()
	flow.SetupSchemaProviderForTesting(t)

	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"external.unknown","description":"test","tasks":[{"id":"ext","description":"external task","action":"MISSING_EXTERNAL_ACTION"}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := flow.LoadDefinition(path); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

const externalTestActionName = "EXTERNAL_TEST_ACTION"

var registerExternalActionOnce sync.Once

type externalTestAction struct{}

func (externalTestAction) Name() string { return externalTestActionName }

func (externalTestAction) Execute(context.Context, json.RawMessage, *registry.ExecutionContext) (registry.Result, error) {
	return registry.Result{}, nil
}

func (externalTestAction) JSONSchema() (json.RawMessage, error) {
	fragment := map[string]any{
		"definitions": map[string]any{
			"task": map[string]any{
				"properties": map[string]any{
					"action": map[string]any{
						"enum": []string{externalTestActionName},
					},
				},
				"allOf": []any{
					map[string]any{
						"if": map[string]any{
							"properties": map[string]any{
								"action": map[string]any{
									"const": externalTestActionName,
								},
							},
							"required": []any{"action"},
						},
						"then": map[string]any{
							"required": []any{"id", "description", "action"},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(fragment)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(data), nil
}

func ensureExternalActionRegistered() {
	registerExternalActionOnce.Do(func() {
		fragment, err := externalTestAction{}.JSONSchema()
		if err != nil {
			panic(err)
		}
		registry.Register(externalTestAction{})
		flow.RegisterSchemaFragmentForTesting(fragment)
		found := false
		for _, existing := range flow.SchemaFragmentsForTesting() {
			if fragmentHasAction(existing, externalTestActionName) {
				found = true
				break
			}
		}
		if !found {
			panic("external action fragment not registered")
		}
	})
}

func fragmentHasAction(data []byte, name string) bool {
	var doc struct {
		Definitions struct {
			Task struct {
				Properties struct {
					Action struct {
						Enum []string `json:"enum"`
					} `json:"action"`
				} `json:"properties"`
			} `json:"task"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for _, value := range doc.Definitions.Task.Properties.Action.Enum {
		if value == name {
			return true
		}
	}
	return false
}
