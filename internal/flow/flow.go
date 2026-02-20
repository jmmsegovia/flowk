package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResultType identifies the supported data types that a task can expose as the
// outcome of its execution. These values are leveraged by subsequent tasks
// that may depend on previous results.
type ResultType string

const (
	ResultTypeBool   ResultType = "bool"
	ResultTypeString ResultType = "string"
	ResultTypeInt    ResultType = "int"
	ResultTypeFloat  ResultType = "float"
	ResultTypeJSON   ResultType = "json"
)

// Definition contains the ordered list of actions that must be executed by the tool.
type Definition struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	// Imports expands referenced flow definitions before executing the current
	// flow. Paths are resolved relative to the directory of the main flow
	// definition.
	Imports []string `json:"imports,omitempty"`
	Tasks   []Task   `json:"tasks"`

	// OnErrorFlow is executed when any task in the flow fails. If provided,
	// execution jumps directly to the referenced flow after the first
	// failure instead of stopping immediately.
	OnErrorFlow string `json:"on_error_flow,omitempty"`
	// FinallyFlow is executed once after the flow finishes, regardless of success or failure.
	FinallyFlow string `json:"finally_flow,omitempty"`
	// FinallyTask is executed once after the flow finishes, regardless of success or failure.
	FinallyTask string `json:"finally_task,omitempty"`

	// FlowImports maps a flow identifier to the list of flow identifiers it imports.
	// The map is populated when loading a definition and is not part of the JSON payload.
	FlowImports map[string][]string `json:"-"`
}

// TaskStatus identifies the lifecycle state of a task within a flow definition.
type TaskStatus string

const (
	TaskStatusNotStarted TaskStatus = "not started"
	TaskStatusInProgress TaskStatus = "in progress"
	TaskStatusPaused     TaskStatus = "paused"
	TaskStatusCompleted  TaskStatus = "completed"
)

// Task represents a single operation within a flow definition.
type Task struct {
	ID              string          `json:"id"`
	Description     string          `json:"description"`
	Action          string          `json:"action"`
	FlowID          string          `json:"-"`
	Status          TaskStatus      `json:"status,omitempty"`
	StartTimestamp  time.Time       `json:"-"`
	EndTimestamp    time.Time       `json:"-"`
	DurationSeconds float64         `json:"-"`
	Success         bool            `json:"-"`
	Result          any             `json:"-"`
	ResultType      ResultType      `json:"-"`
	Payload         json.RawMessage `json:"-"`
}

// UnmarshalJSON extracts the metadata fields of a task and retains the original payload.
func (t *Task) UnmarshalJSON(data []byte) error {
	type alias struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Action      string `json:"action"`
	}

	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}

	t.ID = a.ID
	t.Description = a.Description
	t.Action = a.Action
	t.Payload = append(t.Payload[:0], data...)

	return nil
}

// LoadDefinition parses the JSON flow definition stored at the provided path.
func LoadDefinition(path string) (*Definition, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving action flow path: %w", err)
	}

	baseDir := filepath.Dir(absPath)
	def, err := loadDefinitionRecursive(absPath, baseDir, map[string]struct{}{}, map[string]string{})
	if err != nil {
		return nil, err
	}

	if err := validateTasks(def); err != nil {
		return nil, err
	}

	return def, nil
}

func loadDefinitionRecursive(path, baseDir string, visiting map[string]struct{}, flowIDs map[string]string) (*Definition, error) {
	if _, seen := visiting[path]; seen {
		return nil, fmt.Errorf("flow import cycle detected: %s", path)
	}

	visiting[path] = struct{}{}
	defer delete(visiting, path)

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading action flow %s: %w", path, err)
	}

	if err := validateDefinitionAgainstSchema(path, content); err != nil {
		return nil, err
	}

	var def Definition
	if err := json.Unmarshal(content, &def); err != nil {
		return nil, fmt.Errorf("parsing action flow %s: %w", path, err)
	}

	def.ID = strings.TrimSpace(def.ID)
	if def.ID == "" {
		return nil, fmt.Errorf("%s: id is required", path)
	}

	if existingPath, exists := flowIDs[def.ID]; exists && existingPath != path {
		return nil, fmt.Errorf("flow id %q is duplicated (previously defined at %s)", def.ID, existingPath)
	}
	flowIDs[def.ID] = path

	if def.FlowImports == nil {
		def.FlowImports = make(map[string][]string)
	}
	if _, exists := def.FlowImports[def.ID]; !exists {
		def.FlowImports[def.ID] = nil
	}

	for i := range def.Tasks {
		def.Tasks[i].FlowID = def.ID
	}

	var combined []Task
	for idx, importPath := range def.Imports {
		resolved := strings.TrimSpace(importPath)
		if resolved == "" {
			return nil, fmt.Errorf("imports[%d]: path is required", idx)
		}

		if !filepath.IsAbs(resolved) {
			relative := resolved
			candidate := filepath.Join(baseDir, relative)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				resolved = candidate
			} else if cwd, err := os.Getwd(); err == nil {
				alt := filepath.Join(cwd, relative)
				if info, err := os.Stat(alt); err == nil && !info.IsDir() {
					resolved = alt
				} else {
					resolved = candidate
				}
			} else {
				resolved = candidate
			}
		}

		absImport, err := filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("imports[%d]: resolving path %q: %w", idx, importPath, err)
		}

		importedDef, err := loadDefinitionRecursive(absImport, baseDir, visiting, flowIDs)
		if err != nil {
			return nil, fmt.Errorf("imports[%d]: loading %q: %w", idx, importPath, err)
		}

		combined = append(combined, importedDef.Tasks...)

		if def.FlowImports == nil {
			def.FlowImports = make(map[string][]string)
		}
		def.FlowImports[def.ID] = append(def.FlowImports[def.ID], importedDef.ID)
		mergeFlowImports(def.FlowImports, importedDef.FlowImports)
	}

	combined = append(combined, def.Tasks...)
	def.Tasks = combined

	return &def, nil
}

func mergeFlowImports(dst map[string][]string, src map[string][]string) {
	if len(src) == 0 {
		return
	}

	for flowID, imports := range src {
		if len(imports) == 0 {
			if _, exists := dst[flowID]; !exists {
				dst[flowID] = nil
			}
			continue
		}

		copied := append([]string(nil), imports...)

		if existing, exists := dst[flowID]; exists {
			dst[flowID] = appendUnique(existing, copied)
			continue
		}

		dst[flowID] = copied
	}
}

func appendUnique(dst, values []string) []string {
	if len(values) == 0 {
		return dst
	}

	seen := make(map[string]struct{}, len(dst))
	for _, v := range dst {
		seen[v] = struct{}{}
	}

	for _, v := range values {
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}

	return dst
}

// FlowsForExecution returns the flow identifiers that must be executed when the
// provided flow ID is requested. The result includes the requested flow and all
// of its transitive imports.
func (d *Definition) FlowsForExecution(flowID string) (map[string]struct{}, error) {
	if d == nil {
		return nil, fmt.Errorf("flow definition is nil")
	}

	trimmed := strings.TrimSpace(flowID)
	if trimmed == "" {
		return nil, fmt.Errorf("flow id is required")
	}

	if len(d.FlowImports) == 0 {
		return nil, fmt.Errorf("no flow metadata available for id %q", trimmed)
	}

	if _, exists := d.FlowImports[trimmed]; !exists {
		return nil, fmt.Errorf("flow id %q not found in definition", trimmed)
	}

	selected := make(map[string]struct{})
	var visit func(id string)
	visit = func(id string) {
		if _, seen := selected[id]; seen {
			return
		}
		selected[id] = struct{}{}
		for _, imported := range d.FlowImports[id] {
			visit(imported)
		}
	}
	visit(trimmed)

	return selected, nil
}

func validateTasks(def *Definition) error {
	ids := make(map[string]int)
	for i := range def.Tasks {
		task := &def.Tasks[i]

		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("tasks[%d]: id is required", i)
		}
		if prevIdx, exists := ids[task.ID]; exists {
			return fmt.Errorf("tasks[%d]: id %q is duplicated (previously defined at tasks[%d])", i, task.ID, prevIdx)
		}
		ids[task.ID] = i

		action := strings.TrimSpace(task.Action)
		if action == "" {
			return fmt.Errorf("tasks[%d]: action is required", i)
		}

		task.Status = TaskStatusNotStarted
	}

	trimmedOnError := strings.TrimSpace(def.OnErrorFlow)
	if trimmedOnError != "" {
		if len(def.FlowImports) == 0 {
			return fmt.Errorf("on_error_flow %q not found in flow metadata", trimmedOnError)
		}
		if _, exists := def.FlowImports[trimmedOnError]; !exists {
			return fmt.Errorf("on_error_flow %q not found in flow definition", trimmedOnError)
		}
	}

	trimmedFinallyFlow := strings.TrimSpace(def.FinallyFlow)
	if trimmedFinallyFlow != "" {
		if len(def.FlowImports) == 0 {
			return fmt.Errorf("finally_flow %q not found in flow metadata", trimmedFinallyFlow)
		}
		if _, exists := def.FlowImports[trimmedFinallyFlow]; !exists {
			return fmt.Errorf("finally_flow %q not found in flow definition", trimmedFinallyFlow)
		}
	}

	trimmedFinallyTask := strings.TrimSpace(def.FinallyTask)
	if trimmedFinallyTask != "" {
		found := false
		for i := range def.Tasks {
			if def.Tasks[i].ID == trimmedFinallyTask {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("finally_task %q not found in flow definition", trimmedFinallyTask)
		}
	}

	return nil
}
