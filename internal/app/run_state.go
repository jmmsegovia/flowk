package app

import (
	"context"
	"strings"
	"sync"

	"flowk/internal/flow"
)

// RunState captures the in-memory execution state for a flow run.
type RunState struct {
	mu       sync.RWMutex
	Vars     map[string]Variable
	Tasks    map[string]TaskSnapshot
	Subtasks map[string]map[string]TaskSnapshot
}

// NewRunState creates an initialized RunState.
func NewRunState() *RunState {
	return &RunState{
		Vars:     make(map[string]Variable),
		Tasks:    make(map[string]TaskSnapshot),
		Subtasks: make(map[string]map[string]TaskSnapshot),
	}
}

// HasData reports whether the run state stores any data.
func (rs *RunState) HasData() bool {
	if rs == nil {
		return false
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.Vars) > 0 || len(rs.Tasks) > 0 || len(rs.Subtasks) > 0
}

// HasVariables reports whether the run state has stored variables.
func (rs *RunState) HasVariables() bool {
	if rs == nil {
		return false
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.Vars) > 0
}

// TaskSnapshot returns the snapshot for a task ID if it exists.
func (rs *RunState) TaskSnapshot(taskID string) (TaskSnapshot, bool) {
	if rs == nil {
		return TaskSnapshot{}, false
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.Tasks == nil {
		return TaskSnapshot{}, false
	}

	snapshot, ok := rs.Tasks[taskID]
	return snapshot, ok
}

// SubtaskSnapshot returns the snapshot for a subtask ID if it exists.
func (rs *RunState) SubtaskSnapshot(subtaskID string) (TaskSnapshot, bool) {
	if rs == nil {
		return TaskSnapshot{}, false
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.Subtasks == nil {
		return TaskSnapshot{}, false
	}

	for _, children := range rs.Subtasks {
		if snapshot, ok := children[subtaskID]; ok {
			return snapshot, true
		}
	}

	return TaskSnapshot{}, false
}

// Reset clears the stored run state.
func (rs *RunState) Reset() {
	if rs == nil {
		return
	}

	rs.mu.Lock()
	rs.Vars = make(map[string]Variable)
	rs.Tasks = make(map[string]TaskSnapshot)
	rs.Subtasks = make(map[string]map[string]TaskSnapshot)
	rs.mu.Unlock()
}

// SnapshotVariables returns a copy of the stored variables.
func (rs *RunState) SnapshotVariables() map[string]Variable {
	if rs == nil {
		return map[string]Variable{}
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if len(rs.Vars) == 0 {
		return map[string]Variable{}
	}

	copyVars := make(map[string]Variable, len(rs.Vars))
	for name, variable := range rs.Vars {
		copyVars[name] = variable
	}
	return copyVars
}

// UpdateVariables stores the provided variables snapshot.
func (rs *RunState) UpdateVariables(vars map[string]Variable) {
	if rs == nil {
		return
	}

	updated := make(map[string]Variable, len(vars))
	for name, variable := range vars {
		updated[name] = variable
	}

	rs.mu.Lock()
	if rs.Vars == nil {
		rs.Vars = make(map[string]Variable)
	}
	rs.Vars = updated
	rs.mu.Unlock()
}

// RecordTask stores a snapshot for the provided task and its subtasks.
func (rs *RunState) RecordTask(task *flow.Task) {
	if rs == nil || task == nil {
		return
	}

	snapshot := snapshotTask(task)
	if snapshot == nil {
		return
	}

	subtaskSnapshots := map[string]TaskSnapshot{}
	children, err := extractSubtasks(task)
	if err == nil && len(children) > 0 {
		for i := range children {
			child := children[i]
			if strings.TrimSpace(child.FlowID) == "" {
				child.FlowID = task.FlowID
			}
			if childSnapshot := snapshotTask(&child); childSnapshot != nil {
				subtaskSnapshots[child.ID] = *childSnapshot
			}
		}
	}

	rs.mu.Lock()
	if rs.Tasks == nil {
		rs.Tasks = make(map[string]TaskSnapshot)
	}
	rs.Tasks[task.ID] = *snapshot
	if len(subtaskSnapshots) > 0 {
		if rs.Subtasks == nil {
			rs.Subtasks = make(map[string]map[string]TaskSnapshot)
		}
		rs.Subtasks[task.ID] = subtaskSnapshots
	}
	rs.mu.Unlock()
}

// ApplyToDefinition overlays stored task snapshots onto the definition.
func (rs *RunState) ApplyToDefinition(def *flow.Definition) {
	if rs == nil || def == nil {
		return
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if len(rs.Tasks) == 0 {
		return
	}

	for i := range def.Tasks {
		task := &def.Tasks[i]
		snapshot, ok := rs.Tasks[task.ID]
		if !ok {
			continue
		}
		task.Status = snapshot.Status
		task.Success = snapshot.Success
		task.StartTimestamp = snapshot.StartTimestamp
		task.EndTimestamp = snapshot.EndTimestamp
		task.DurationSeconds = snapshot.DurationSeconds
		task.ResultType = snapshot.ResultType
		task.Result = snapshot.Result
	}
}

type runStateContextKey struct{}

// WithRunState stores a RunState in the context.
func WithRunState(ctx context.Context, state *RunState) context.Context {
	if ctx == nil || state == nil {
		return ctx
	}
	return context.WithValue(ctx, runStateContextKey{}, state)
}

// RunStateFromContext loads a RunState from the context.
func RunStateFromContext(ctx context.Context) *RunState {
	if ctx == nil {
		return nil
	}
	value := ctx.Value(runStateContextKey{})
	if state, ok := value.(*RunState); ok {
		return state
	}
	return nil
}

func updateRunStateFromContext(ctx context.Context, task *flow.Task, vars map[string]Variable) {
	state := RunStateFromContext(ctx)
	if state == nil {
		return
	}

	state.UpdateVariables(vars)
	state.RecordTask(task)
}
