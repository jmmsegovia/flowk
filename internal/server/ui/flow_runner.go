package ui

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"flowk/internal/actions/db/cassandra"
	"flowk/internal/app"
	"flowk/internal/flow"
	"flowk/internal/shared/runcontext"
)

var (
	// ErrRunInProgress indicates that a flow execution is already active.
	ErrRunInProgress = errors.New("flow execution already in progress")
	// ErrRunnerUnavailable indicates that the flow runner is not configured.
	ErrRunnerUnavailable = errors.New("flow runner is not configured")
	// ErrNoRunInProgress indicates that no flow run is currently active.
	ErrNoRunInProgress = errors.New("no flow execution in progress")
	// ErrFlowPathRequired indicates that the runner was created without a flow path.
	ErrFlowPathRequired = errors.New("flow path is required to execute the run")
	// ErrNoRunState indicates that there is no stored run state to resume from.
	ErrNoRunState = errors.New("no previous run state available to resume from")
	// ErrResumeConflict indicates conflicting run options were provided with a resume request.
	ErrResumeConflict = errors.New("resume request cannot be combined with other run options")
	// ErrResumeTaskNotFound indicates that the requested task was not executed previously.
	ErrResumeTaskNotFound = errors.New("requested resume task was not executed previously")
	// ErrResumeTaskNotCompleted indicates that the requested task has not completed yet.
	ErrResumeTaskNotCompleted = errors.New("requested resume task has not completed yet")
)

// FlowRunner coordinates flow executions so only one run happens at a time.
type RunOptions struct {
	BeginFromTask    string
	RunTaskID        string
	RunFlowID        string
	RunSubtaskID     string
	ResumeFromTaskID string
}

type FlowRunner struct {
	ctx                 context.Context
	observer            app.FlowObserver
	flowPath            string
	defaultBeginTaskID  string
	defaultRunTaskID    string
	defaultRunFlowID    string
	defaultRunSubtaskID string
	logger              cassandra.Logger
	lastRunState        *app.RunState
	stopSignal          *runcontext.StopSignal
	stopAtTask          *runcontext.StopAtTask

	mu      sync.Mutex
	running bool
}

// NewFlowRunner creates a runner that executes flows using the provided context and observer.
func NewFlowRunner(ctx context.Context, observer app.FlowObserver, flowPath, beginFromTask, runTaskID, runFlowID, runSubtaskID string, logger cassandra.Logger) *FlowRunner {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &FlowRunner{
		ctx:                 ctx,
		observer:            observer,
		flowPath:            flowPath,
		defaultBeginTaskID:  beginFromTask,
		defaultRunTaskID:    runTaskID,
		defaultRunFlowID:    runFlowID,
		defaultRunSubtaskID: runSubtaskID,
		logger:              logger,
		stopAtTask:          runcontext.NewStopAtTask(),
	}
}

// Start launches a new flow execution and returns a channel that resolves when the run finishes.
func (r *FlowRunner) Start(options *RunOptions) (<-chan error, error) {
	if r == nil {
		return nil, ErrRunnerUnavailable
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return nil, ErrRunInProgress
	}

	flowPath := strings.TrimSpace(r.flowPath)
	if flowPath == "" {
		r.mu.Unlock()
		return nil, ErrFlowPathRequired
	}

	r.running = true
	stopSignal := runcontext.NewStopSignal()
	r.stopSignal = stopSignal
	done := make(chan error, 1)
	ctx := r.ctx
	observer := r.observer
	logger := r.logger
	beginFromTask := r.defaultBeginTaskID
	runTaskID := r.defaultRunTaskID
	runFlowID := r.defaultRunFlowID
	runSubtaskID := r.defaultRunSubtaskID
	var runState *app.RunState
	var resumeFromTaskID string
	var hasExplicitRunOption bool
	if options != nil {
		if trimmed := strings.TrimSpace(options.BeginFromTask); trimmed != "" {
			beginFromTask = trimmed
			hasExplicitRunOption = true
		}
		if trimmed := strings.TrimSpace(options.RunTaskID); trimmed != "" {
			runTaskID = trimmed
			hasExplicitRunOption = true
		}
		if trimmed := strings.TrimSpace(options.RunFlowID); trimmed != "" {
			runFlowID = trimmed
			hasExplicitRunOption = true
		}
		if trimmed := strings.TrimSpace(options.RunSubtaskID); trimmed != "" {
			runSubtaskID = trimmed
			hasExplicitRunOption = true
		}
		resumeFromTaskID = strings.TrimSpace(options.ResumeFromTaskID)
	}

	if resumeFromTaskID != "" {
		if hasExplicitRunOption {
			r.mu.Unlock()
			return nil, ErrResumeConflict
		}
		beginFromTask = ""
		runTaskID = ""
		runFlowID = ""
		runSubtaskID = ""
		if r.lastRunState == nil || !r.lastRunState.HasData() {
			r.mu.Unlock()
			return nil, ErrNoRunState
		}
		if resumeFromTaskID != "" {
			snapshot, ok := r.lastRunState.TaskSnapshot(resumeFromTaskID)
			if !ok {
				r.mu.Unlock()
				return nil, ErrResumeTaskNotFound
			}
			if snapshot.Status != flow.TaskStatusCompleted {
				r.mu.Unlock()
				return nil, ErrResumeTaskNotCompleted
			}
			beginFromTask = resumeFromTaskID
		}
		runState = r.lastRunState
	} else if strings.TrimSpace(runTaskID) != "" && r.lastRunState != nil && r.lastRunState.HasData() {
		runState = r.lastRunState
	} else {
		runState = app.NewRunState()
	}
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.lastRunState = runState
			r.stopSignal = nil
			r.mu.Unlock()
		}()

		runCtx := ctx
		runCtx = app.WithRunState(runCtx, runState)
		runCtx = runcontext.WithStopSignal(runCtx, stopSignal)
		runCtx = runcontext.WithStopAtTask(runCtx, r.stopAtTask)
		if observer != nil {
			runCtx = app.WithObserver(runCtx, observer)
		}

		err := app.Run(runCtx, flowPath, logger, beginFromTask, runTaskID, runFlowID, runSubtaskID)
		done <- err
		close(done)
	}()

	return done, nil
}

// Trigger launches a new run without waiting for the outcome.
func (r *FlowRunner) Trigger(options *RunOptions) error {
	_, err := r.Start(options)
	return err
}

// Running reports whether a run is currently in progress.
func (r *FlowRunner) Running() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// RequestStop asks the runner to stop after the current task completes.
func (r *FlowRunner) RequestStop() error {
	if r == nil {
		return ErrRunnerUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.stopSignal == nil {
		return ErrNoRunInProgress
	}
	r.stopSignal.Request()
	return nil
}

// SetStopAtTask configures a task ID that should stop the flow after it completes.
func (r *FlowRunner) SetStopAtTask(taskID string) error {
	if r == nil {
		return ErrRunnerUnavailable
	}
	trimmed := strings.TrimSpace(taskID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopAtTask == nil {
		r.stopAtTask = runcontext.NewStopAtTask()
	}
	if trimmed == "" {
		r.stopAtTask.Clear()
		return nil
	}
	r.stopAtTask.Set(trimmed)
	return nil
}

// UpdateFlowPath sets the path to the flow definition used for subsequent runs.
func (r *FlowRunner) UpdateFlowPath(path string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.flowPath = strings.TrimSpace(path)
	r.mu.Unlock()
}
