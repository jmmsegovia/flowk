package app

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestCombinedSchemaIncludesEveryRegisteredAction(t *testing.T) {
	combined, err := flow.CombinedSchema()
	if err != nil {
		t.Fatalf("loading combined schema: %v", err)
	}

	var schemaDoc struct {
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
	if err := json.Unmarshal(combined, &schemaDoc); err != nil {
		t.Fatalf("decoding combined schema: %v", err)
	}

	schemaActions := make(map[string]struct{}, len(schemaDoc.Definitions.Task.Properties.Action.Enum))
	for _, action := range schemaDoc.Definitions.Task.Properties.Action.Enum {
		key := strings.ToUpper(strings.TrimSpace(action))
		if key == "" {
			continue
		}
		schemaActions[key] = struct{}{}
	}

	var missing []string
	for _, name := range registry.Names() {
		action, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("registered action %q is not retrievable from registry lookup", name)
		}
		if _, ok := action.(registry.SchemaProvider); !ok {
			continue
		}
		if _, ok := schemaActions[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf(
			"registered actions missing from definitions.task.properties.action.enum: %s. "+
				"Each action schema fragment must include properties.action.enum with its action name.",
			strings.Join(missing, ", "),
		)
	}
}
