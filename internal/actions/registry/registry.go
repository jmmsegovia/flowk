package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"flowk/internal/flow"
)

// Logger is implemented by task loggers to provide structured output to actions.
type Logger interface {
	Printf(format string, v ...interface{})
	PrintColored(plain, colored string)
}

// Variable describes a value stored during flow execution that can be consumed by actions.
type Variable struct {
	Name   string
	Type   string
	Value  any
	Secret bool
}

// ExecutionContext exposes runtime information that actions can inspect and mutate.
type ExecutionContext struct {
	Task        *flow.Task
	Tasks       []flow.Task
	Variables   map[string]Variable
	Logger      Logger
	LogDir      string
	ExecuteTask TaskExecutor
}

// TaskExecutionRequest describes a task that should be executed on behalf of an action.
type TaskExecutionRequest struct {
	Task      *flow.Task
	Tasks     []flow.Task
	Variables map[string]Variable
	LogDir    string
}

// TaskExecutionResponse captures the outcome of a delegated task execution.
type TaskExecutionResponse struct {
	Result    Result
	Variables map[string]Variable
}

// TaskExecutor defines the callback contract used by actions to trigger nested task execution.
type TaskExecutor func(ctx context.Context, req TaskExecutionRequest) (TaskExecutionResponse, error)

// Control conveys optional flow control directives emitted by an action.
type Control struct {
	JumpToTaskID string
	Exit         bool
	BreakLoop    bool
}

// Result captures the outcome of an action execution.
type Result struct {
	Value   any
	Type    flow.ResultType
	Control *Control
}

// Action defines the contract implemented by concrete actions.
type Action interface {
	Name() string
	Execute(ctx context.Context, payload json.RawMessage, execCtx *ExecutionContext) (Result, error)
}

// SchemaProvider can be implemented by actions to contribute JSON Schema fragments
// that extend the flow definition validation.
type SchemaProvider interface {
	JSONSchema() (json.RawMessage, error)
}

var (
	mu              sync.RWMutex
	actions         = make(map[string]Action)
	schemaFragments = make(map[string]json.RawMessage)
	schemaVersion   uint64
)

func init() {
	flow.RegisterSchemaProvider(schemaFragmentsSnapshot)
}

// Register stores the supplied action in the global registry.
func Register(action Action) {
	if action == nil {
		panic("registry: cannot register nil action")
	}

	name := strings.TrimSpace(action.Name())
	if name == "" {
		panic("registry: action with empty name")
	}

	key := strings.ToUpper(name)

	mu.Lock()
	defer mu.Unlock()

	if _, exists := actions[key]; exists {
		panic(fmt.Sprintf("registry: action %q already registered", name))
	}

	actions[key] = action

	if provider, ok := action.(SchemaProvider); ok {
		fragment, err := provider.JSONSchema()
		if err != nil {
			panic(fmt.Sprintf("registry: action %q returned invalid schema fragment: %v", name, err))
		}
		if len(fragment) > 0 {
			copied := append(json.RawMessage(nil), fragment...)
			if existing, exists := schemaFragments[key]; !exists || !jsonEqual(existing, copied) {
				schemaFragments[key] = copied
				schemaVersion++
			}
		}
	}
}

// Lookup retrieves an action implementation by name.
func Lookup(name string) (Action, bool) {
	key := strings.ToUpper(strings.TrimSpace(name))
	if key == "" {
		return nil, false
	}

	mu.RLock()
	action, ok := actions[key]
	mu.RUnlock()
	return action, ok
}

// Names returns the registered action names in alphabetical order.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func schemaFragmentsSnapshot() ([]json.RawMessage, uint64) {
	mu.RLock()
	defer mu.RUnlock()

	fragments := make([]json.RawMessage, 0, len(schemaFragments))
	for _, fragment := range schemaFragments {
		if len(fragment) == 0 {
			continue
		}
		fragments = append(fragments, append(json.RawMessage(nil), fragment...))
	}

	return fragments, schemaVersion
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
