package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	testSchemaProviderOnce sync.Once
	testSchemaMu           sync.RWMutex
	testBaseFragments      [][]byte
	testExtraFragments     [][]byte
	testSchemaVersion      uint64
)

func ensureTestSchemaProvider(tb testing.TB) {
	tb.Helper()

	testSchemaProviderOnce.Do(func() {
		_, currentFile, _, ok := runtime.Caller(0)
		if !ok {
			tb.Fatalf("failed to determine test file path")
		}

		root := filepath.Join(filepath.Dir(currentFile), "..", "..")
		schemaFiles, err := filepath.Glob(filepath.Join(root, "internal", "actions", "*", "schema.json"))
		if err != nil {
			tb.Fatalf("failed to locate action schemas: %v", err)
		}

		fragments := make([][]byte, 0, len(schemaFiles))
		for _, path := range schemaFiles {
			data, err := os.ReadFile(path)
			if err != nil {
				tb.Fatalf("failed to read action schema %s: %v", path, err)
			}
			fragments = append(fragments, append([]byte(nil), data...))
		}

		testSchemaMu.Lock()
		testBaseFragments = fragments
		testExtraFragments = nil
		testSchemaVersion = 1
		testSchemaMu.Unlock()

		RegisterSchemaProvider(func() ([]json.RawMessage, uint64) {
			testSchemaMu.RLock()
			defer testSchemaMu.RUnlock()

			combined := make([]json.RawMessage, 0, len(testBaseFragments)+len(testExtraFragments))
			for _, data := range testBaseFragments {
				combined = append(combined, json.RawMessage(append([]byte(nil), data...)))
			}
			for _, data := range testExtraFragments {
				combined = append(combined, json.RawMessage(append([]byte(nil), data...)))
			}

			version := testSchemaVersion
			if version == 0 {
				version = 1
			}
			return combined, version
		})
		schemaCache = sync.Map{}
	})
}

// SetupSchemaProviderForTesting ensures the flow package uses the action schema fragments when running tests.
func SetupSchemaProviderForTesting(tb testing.TB) {
	ensureTestSchemaProvider(tb)
}

// RegisterSchemaFragmentForTesting adds an additional schema fragment to the merged schema used during tests.
func RegisterSchemaFragmentForTesting(fragment []byte) {
	testSchemaMu.Lock()
	testExtraFragments = append(testExtraFragments, append([]byte(nil), fragment...))
	testSchemaVersion++
	testSchemaMu.Unlock()
	schemaCache = sync.Map{}
}

// ResetSchemaProviderForTesting clears the cached schema fragments so they can be reloaded.
func ResetSchemaProviderForTesting() {
	testSchemaMu.Lock()
	testBaseFragments = nil
	testExtraFragments = nil
	testSchemaVersion = 0
	testSchemaMu.Unlock()
	testSchemaProviderOnce = sync.Once{}
	schemaCache = sync.Map{}
	RegisterSchemaProvider(nil)
}

// SchemaFragmentsForTesting returns the schema fragments currently configured for test validation.
func SchemaFragmentsForTesting() [][]byte {
	testSchemaMu.RLock()
	defer testSchemaMu.RUnlock()

	combined := make([][]byte, 0, len(testBaseFragments)+len(testExtraFragments))
	for _, data := range testBaseFragments {
		combined = append(combined, append([]byte(nil), data...))
	}
	for _, data := range testExtraFragments {
		combined = append(combined, append([]byte(nil), data...))
	}
	return combined
}

func setupSchemaProvider(t *testing.T) {
	t.Helper()
	SetupSchemaProviderForTesting(t)
}

func TestTaskUnmarshalStoresPayload(t *testing.T) {
	setupSchemaProvider(t)
	data := []byte(`{"id":"one","description":"sleep task","action":"SLEEP","seconds":3}`)
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatalf("Task.UnmarshalJSON() error = %v", err)
	}

	if got, want := string(task.Payload), string(data); got != want {
		t.Fatalf("task payload = %s, want %s", got, want)
	}
}

func TestLoadDefinitionInitializesTaskStatus(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"load.status","description":"test","tasks":[{"id":"one","description":"sleep once","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Tasks) != 1 {
		t.Fatalf("unexpected number of tasks: got %d, want 1", len(def.Tasks))
	}

	if def.Tasks[0].Status != TaskStatusNotStarted {
		t.Fatalf("task status = %q, want %q", def.Tasks[0].Status, TaskStatusNotStarted)
	}
}

func TestLoadDefinitionSleepActionPayload(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"load.payload","description":"test","tasks":[{"id":"sleep","description":"sleep for two seconds","action":"SLEEP","seconds":2}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Tasks) != 1 {
		t.Fatalf("unexpected number of tasks: got %d, want 1", len(def.Tasks))
	}

	var payload struct {
		Seconds float64 `json:"seconds"`
	}
	if err := json.Unmarshal(def.Tasks[0].Payload, &payload); err != nil {
		t.Fatalf("decoding payload: %v", err)
	}
	if payload.Seconds != 2 {
		t.Fatalf("payload seconds = %v, want 2", payload.Seconds)
	}
}

func TestLoadDefinitionPrintTaskAllowsEmptyDescription(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"print.no.description","description":"test","tasks":[{"id":"print","action":"PRINT","entries":[{"message":"hello"}]}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Tasks) != 1 {
		t.Fatalf("unexpected number of tasks: got %d, want 1", len(def.Tasks))
	}

	if def.Tasks[0].Description != "" {
		t.Fatalf("expected empty description, got %q", def.Tasks[0].Description)
	}
}

func TestLoadDefinitionTaskAllowsEmptyDescription(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"sleep.no.description","description":"test","tasks":[{"id":"sleep","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Tasks) != 1 {
		t.Fatalf("unexpected number of tasks: got %d, want 1", len(def.Tasks))
	}

	if def.Tasks[0].Description != "" {
		t.Fatalf("expected empty description, got %q", def.Tasks[0].Description)
	}
}

func TestLoadDefinitionRequiresID(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"missing.task.id","description":"test","tasks":[{"description":"missing id","action":"SLEEP","seconds":2}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := LoadDefinition(path); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

func TestLoadDefinitionRequiresAction(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"missing.task.action","description":"test","tasks":[{"id":"sleep","description":"missing action","seconds":2}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := LoadDefinition(path); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

func TestLoadDefinitionRejectsDuplicateIDs(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"duplicate.task.ids","description":"test","tasks":[{"id":"dup","description":"first","action":"SLEEP","seconds":2},{"id":"dup","description":"second","action":"SLEEP","seconds":3}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := LoadDefinition(path); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

func TestLoadDefinitionUsesEmbeddedSchema(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"schema.missing","description":"test","tasks":[{"id":"sleep","description":"sleep task","action":"SLEEP","seconds":2}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}
	if _, err := LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
}

func TestLoadDefinitionAllowsEmptyStringEvaluateRight(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"evaluate.empty","description":"test","tasks":[{"id":"seed","action":"VARIABLES","scope":"flow","overwrite":true,"vars":[{"name":"sample","type":"string","value":""}]},{"id":"eval","action":"EVALUATE","if_conditions":[{"left":"${sample}","operation":"!=","right":""}],"then":{"continue":"ok"},"else":{"exit":"fail"}}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
}

func TestLoadDefinitionMergesImportedTasks(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	importedDir := filepath.Join(dir, "sub")
	if err := os.Mkdir(importedDir, 0o755); err != nil {
		t.Fatalf("failed to create import directory: %v", err)
	}

	importedPath := filepath.Join(importedDir, "imported.json")
	importedContent := []byte(`{"id":"imported.flow","description":"imported","tasks":[{"id":"imported","description":"from import","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(importedPath, importedContent, 0o600); err != nil {
		t.Fatalf("failed to write imported flow: %v", err)
	}

	rootPath := filepath.Join(dir, "flow.json")
	rootContent := []byte(`{"id":"root.flow","description":"root","imports":["sub/imported.json"],"tasks":[{"id":"local","description":"local task","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("failed to write root flow: %v", err)
	}

	def, err := LoadDefinition(rootPath)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if len(def.Tasks) != 2 {
		t.Fatalf("unexpected number of tasks: got %d, want 2", len(def.Tasks))
	}

	if def.Tasks[0].ID != "imported" {
		t.Fatalf("expected first task from import, got %q", def.Tasks[0].ID)
	}

	for i, task := range def.Tasks {
		if task.Status != TaskStatusNotStarted {
			t.Fatalf("tasks[%d].Status = %q, want %q", i, task.Status, TaskStatusNotStarted)
		}
	}
}

func TestLoadDefinitionValidatesOnErrorFlow(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	flowPath := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"onerror.missing","description":"test","on_error_flow":"missing.flow","tasks":[{"id":"main","description":"main task","action":"SLEEP","seconds":0.01}]}`)
	if err := os.WriteFile(flowPath, content, 0o600); err != nil {
		t.Fatalf("failed to write flow definition: %v", err)
	}

	if _, err := LoadDefinition(flowPath); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	} else if !strings.Contains(err.Error(), "on_error_flow") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDefinitionFailsForMissingImport(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	rootPath := filepath.Join(dir, "flow.json")
	rootContent := []byte(`{"id":"missing.import","description":"root","imports":["missing.json"],"tasks":[{"id":"local","description":"local","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("failed to write root flow: %v", err)
	}

	if _, err := LoadDefinition(rootPath); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

func TestLoadDefinitionDetectsDuplicateIDsAcrossImports(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	importedPath := filepath.Join(dir, "imported.json")
	importedContent := []byte(`{"id":"dup.flow.imported","description":"imported","tasks":[{"id":"dup","description":"duplicate","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(importedPath, importedContent, 0o600); err != nil {
		t.Fatalf("failed to write imported flow: %v", err)
	}

	rootPath := filepath.Join(dir, "flow.json")
	rootContent := []byte(`{"id":"dup.flow.root","description":"root","imports":["imported.json"],"tasks":[{"id":"dup","description":"local","action":"SLEEP","seconds":1}]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("failed to write root flow: %v", err)
	}

	if _, err := LoadDefinition(rootPath); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	}
}

func TestLoadDefinitionDetectsImportCycles(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	secondPath := filepath.Join(dir, "second.json")
	secondContent := []byte(`{"id":"second.flow","description":"second","imports":["flow.json"],"tasks":[]}`)
	if err := os.WriteFile(secondPath, secondContent, 0o600); err != nil {
		t.Fatalf("failed to write second flow: %v", err)
	}

	rootPath := filepath.Join(dir, "flow.json")
	rootContent := []byte(`{"id":"cycle.root","description":"root","imports":["second.json"],"tasks":[]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("failed to write root flow: %v", err)
	}

	if _, err := LoadDefinition(rootPath); err == nil {
		t.Fatal("LoadDefinition() error = nil, want error")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func FlowKsForExecutionIncludesTransitiveImports(t *testing.T) {
	dir := t.TempDir()

	nestedPath := filepath.Join(dir, "nested.json")
	nestedContent := []byte(`{"id":"nested.flow","description":"nested","tasks":[]}`)
	if err := os.WriteFile(nestedPath, nestedContent, 0o600); err != nil {
		t.Fatalf("failed to write nested flow: %v", err)
	}

	importedPath := filepath.Join(dir, "imported.json")
	importedContent := []byte(`{"id":"imported.flow","description":"imported","imports":["nested.json"],"tasks":[]}`)
	if err := os.WriteFile(importedPath, importedContent, 0o600); err != nil {
		t.Fatalf("failed to write imported flow: %v", err)
	}

	rootPath := filepath.Join(dir, "root.json")
	rootContent := []byte(`{"id":"root.flow","description":"root","imports":["imported.json"],"tasks":[]}`)
	if err := os.WriteFile(rootPath, rootContent, 0o600); err != nil {
		t.Fatalf("failed to write root flow: %v", err)
	}

	def, err := LoadDefinition(rootPath)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	selected, err := def.FlowsForExecution("imported.flow")
	if err != nil {
		t.Fatalf("FlowsForExecution() error = %v", err)
	}

	if len(selected) != 2 {
		t.Fatalf("expected 2 flows (imported + nested), got %d", len(selected))
	}
	if _, ok := selected["imported.flow"]; !ok {
		t.Fatalf("expected imported.flow to be selected, got %v", selected)
	}
	if _, ok := selected["nested.flow"]; !ok {
		t.Fatalf("expected nested.flow to be selected, got %v", selected)
	}
}

func FlowKsForExecutionUnknownFlow(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"root.flow","description":"root","tasks":[]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow: %v", err)
	}

	def, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}

	if _, err := def.FlowsForExecution("missing.flow"); err == nil {
		t.Fatal("FlowsForExecution() error = nil, want error")
	}
}

func TestLoadDefinitionSupportsParallelAction(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"parallel.flow","description":"parallel","tasks":[{"id":"parallel","description":"parallel work","action":"PARALLEL","tasks":[{"id":"sleep","description":"sleep","action":"SLEEP","seconds":0.01}]}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow: %v", err)
	}

	if _, err := LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
}

func TestLoadDefinitionParallelWithoutTasksIsAccepted(t *testing.T) {
	setupSchemaProvider(t)
	dir := t.TempDir()

	path := filepath.Join(dir, "flow.json")
	content := []byte(`{"id":"parallel.invalid","description":"parallel","tasks":[{"id":"parallel","description":"missing tasks","action":"PARALLEL"}]}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write flow: %v", err)
	}

	if _, err := LoadDefinition(path); err != nil {
		t.Fatalf("LoadDefinition() error = %v", err)
	}
}
