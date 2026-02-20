package app

import (
	"strings"
	"sync"

	"flowk/internal/actions/core/variables"
	"flowk/internal/actions/registry"
	"flowk/internal/flow"
	expansion "flowk/internal/shared/expansion"
)

// Variable describes a runtime variable stored during a flow execution.
type Variable = variables.Variable

// RunContext contains the state shared by all actions during a flow execution.
type RunContext struct {
	mu   sync.RWMutex
	Vars map[string]Variable
}

// Snapshot returns a deep copy of the current variables map.
func (rc *RunContext) Snapshot() map[string]Variable {
	if rc == nil {
		return map[string]Variable{}
	}

	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if len(rc.Vars) == 0 {
		return map[string]Variable{}
	}

	snapshot := make(map[string]Variable, len(rc.Vars))
	for name, variable := range rc.Vars {
		snapshot[name] = variable
	}
	return snapshot
}

// Replace overwrites the current variables map with a copy of the provided values.
func (rc *RunContext) Replace(vars map[string]Variable) {
	if rc == nil {
		return
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if len(vars) == 0 {
		rc.Vars = map[string]Variable{}
		return
	}

	updated := make(map[string]Variable, len(vars))
	for name, variable := range vars {
		updated[name] = variable
	}
	rc.Vars = updated
}

// ExecutionContext builds the registry execution context used by actions.
func (rc *RunContext) ExecutionContext(task *flow.Task, tasks []flow.Task, logger registry.Logger) *registry.ExecutionContext {
	snapshot := rc.Snapshot()
	vars := make(map[string]registry.Variable, len(snapshot))
	for name, variable := range snapshot {
		vars[name] = registry.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	return &registry.ExecutionContext{
		Task:      task,
		Tasks:     tasks,
		Variables: vars,
		Logger:    logger,
	}
}

// UpdateFromExecutionContext synchronises the run context after an action completes.
func (rc *RunContext) UpdateFromExecutionContext(execCtx *registry.ExecutionContext) {
	if rc == nil || execCtx == nil || execCtx.Variables == nil {
		return
	}

	rc.Replace(registryVariablesToRun(execCtx.Variables))
}

func registryVariablesToRun(vars map[string]registry.Variable) map[string]Variable {
	if len(vars) == 0 {
		return map[string]Variable{}
	}

	converted := make(map[string]Variable, len(vars))
	for name, variable := range vars {
		updated := Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
		if strings.TrimSpace(updated.Name) == "" {
			updated.Name = name
		}
		converted[name] = updated
	}

	return converted
}

func runVariablesToRegistry(vars map[string]Variable) map[string]registry.Variable {
	if len(vars) == 0 {
		return map[string]registry.Variable{}
	}

	converted := make(map[string]registry.Variable, len(vars))
	for name, variable := range vars {
		converted[name] = registry.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	return converted
}

func runVariablesToExpansion(vars map[string]Variable) map[string]expansion.Variable {
	if len(vars) == 0 {
		return map[string]expansion.Variable{}
	}

	converted := make(map[string]expansion.Variable, len(vars))
	for name, variable := range vars {
		converted[name] = expansion.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}

	return converted
}
