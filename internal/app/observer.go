package app

import (
	"context"
	"time"

	"flowk/internal/flow"
)

type FlowEventType string

const (
	FlowEventFlowLoaded    FlowEventType = "flow_loaded"
	FlowEventFlowStarted   FlowEventType = "flow_started"
	FlowEventFlowFinished  FlowEventType = "flow_finished"
	FlowEventTaskStarted   FlowEventType = "task_started"
	FlowEventTaskCompleted FlowEventType = "task_completed"
	FlowEventTaskFailed    FlowEventType = "task_failed"
	FlowEventTaskLog       FlowEventType = "task_log"
)

type FlowEvent struct {
	Type      FlowEventType `json:"type"`
	Timestamp time.Time     `json:"timestamp"`
	FlowID    string        `json:"flowId"`
	Task      *TaskSnapshot `json:"task,omitempty"`
	Message   string        `json:"message,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type TaskSnapshot struct {
	ID              string          `json:"id"`
	FlowID          string          `json:"flowId"`
	Description     string          `json:"description,omitempty"`
	Action          string          `json:"action"`
	Status          flow.TaskStatus `json:"status"`
	Success         bool            `json:"success"`
	StartTimestamp  time.Time       `json:"startTimestamp"`
	EndTimestamp    time.Time       `json:"endTimestamp"`
	DurationSeconds float64         `json:"durationSeconds"`
	ResultType      flow.ResultType `json:"resultType"`
	Result          any             `json:"result,omitempty"`
}

type FlowObserver interface {
	OnEvent(event FlowEvent)
}

type observerContextKey struct{}

func WithObserver(ctx context.Context, observer FlowObserver) context.Context {
	if ctx == nil || observer == nil {
		return ctx
	}
	return context.WithValue(ctx, observerContextKey{}, observer)
}

func observerFromContext(ctx context.Context) FlowObserver {
	if ctx == nil {
		return nil
	}
	value := ctx.Value(observerContextKey{})
	if observer, ok := value.(FlowObserver); ok {
		return observer
	}
	return nil
}

func publishEvent(observer FlowObserver, evt FlowEvent) {
	if observer == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	observer.OnEvent(evt)
}

func snapshotTask(task *flow.Task) *TaskSnapshot {
	if task == nil {
		return nil
	}

	snapshot := &TaskSnapshot{
		ID:              task.ID,
		FlowID:          task.FlowID,
		Description:     task.Description,
		Action:          task.Action,
		Status:          task.Status,
		Success:         task.Success,
		StartTimestamp:  task.StartTimestamp,
		EndTimestamp:    task.EndTimestamp,
		DurationSeconds: task.DurationSeconds,
		ResultType:      task.ResultType,
	}

	if task.Result != nil {
		snapshot.Result = task.Result
	}

	return snapshot
}
