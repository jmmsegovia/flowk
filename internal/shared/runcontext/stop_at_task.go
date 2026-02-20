package runcontext

import (
	"context"
	"sync/atomic"
)

type stopAtTaskKey struct{}

// StopAtTask stores a task identifier that should pause the flow after it completes.
type StopAtTask struct {
	target atomic.Value
}

// NewStopAtTask creates a new stop-at-task tracker.
func NewStopAtTask() *StopAtTask {
	tracker := &StopAtTask{}
	tracker.target.Store("")
	return tracker
}

// Set updates the task identifier to stop after.
func (s *StopAtTask) Set(taskID string) {
	if s == nil {
		return
	}
	s.target.Store(taskID)
}

// Clear removes any stop-at-task marker.
func (s *StopAtTask) Clear() {
	if s == nil {
		return
	}
	s.target.Store("")
}

// Get returns the configured stop-at-task identifier.
func (s *StopAtTask) Get() string {
	if s == nil {
		return ""
	}
	if value := s.target.Load(); value != nil {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}

// WithStopAtTask stores a stop-at-task tracker in the context.
func WithStopAtTask(ctx context.Context, stopAt *StopAtTask) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, stopAtTaskKey{}, stopAt)
}

// StopAtTaskFromContext returns the stop-at-task tracker stored in the context.
func StopAtTaskFromContext(ctx context.Context) *StopAtTask {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(stopAtTaskKey{}).(*StopAtTask)
	return tracker
}

// StopAtTaskID returns the configured stop-at-task identifier from context.
func StopAtTaskID(ctx context.Context) string {
	return StopAtTaskFromContext(ctx).Get()
}
