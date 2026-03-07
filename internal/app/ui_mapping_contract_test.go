package app

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"flowk/internal/actions/registry"
)

var uppercaseObjectKeyPattern = regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]+)\s*:`)

func TestTaskTypeIconCoversEveryRegisteredSchemaAction(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "ui", "src", "components", "flow", "TaskTypeIcon.tsx")

	present, err := uppercaseObjectKeys(path)
	if err != nil {
		t.Fatalf("reading TaskTypeIcon action variants: %v", err)
	}

	required := registeredSchemaActions(t)
	missing := missingActionMappings(required, present)
	if len(missing) > 0 {
		t.Fatalf(
			"missing TaskTypeIcon ACTION_VARIANTS mappings for actions: %s",
			strings.Join(missing, ", "),
		)
	}
}

func TestActionGuideCategoryMapCoversEveryRegisteredSchemaAction(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "ui", "src", "pages", "ActionGuidePage.tsx")

	present, err := uppercaseObjectKeys(path)
	if err != nil {
		t.Fatalf("reading ActionGuidePage category map: %v", err)
	}

	required := registeredSchemaActions(t)
	missing := missingActionMappings(required, present)
	if len(missing) > 0 {
		t.Fatalf(
			"missing ActionGuidePage ACTION_CATEGORY_MAP mappings for actions: %s",
			strings.Join(missing, ", "),
		)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func uppercaseObjectKeys(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	matches := uppercaseObjectKeyPattern.FindAllStringSubmatch(string(data), -1)
	keys := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			continue
		}
		keys[key] = struct{}{}
	}

	return keys, nil
}

func registeredSchemaActions(t *testing.T) []string {
	t.Helper()

	names := registry.Names()
	required := make([]string, 0, len(names))
	for _, name := range names {
		action, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("registered action %q is not retrievable from registry lookup", name)
		}
		if _, ok := action.(registry.SchemaProvider); !ok {
			continue
		}
		required = append(required, name)
	}

	sort.Strings(required)
	return required
}

func missingActionMappings(required []string, present map[string]struct{}) []string {
	missing := make([]string, 0)
	for _, action := range required {
		if _, ok := present[action]; !ok {
			missing = append(missing, action)
		}
	}
	return missing
}
