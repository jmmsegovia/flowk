package runcontext

import (
	"context"
	"testing"
)

func TestResumeContextHelpers(t *testing.T) {
	t.Parallel()

	if IsResume(nil) {
		t.Fatal("nil context must not be resume")
	}

	ctx := WithResume(context.Background())
	if !IsResume(ctx) {
		t.Fatal("expected context to be marked as resume")
	}
}

func TestStopSignalHelpers(t *testing.T) {
	t.Parallel()

	signal := NewStopSignal()
	if signal.Requested() {
		t.Fatal("new stop signal should not be requested")
	}
	signal.Request()
	if !signal.Requested() {
		t.Fatal("stop signal should be requested after Request")
	}

	ctx := WithStopSignal(context.Background(), signal)
	if StopSignalFromContext(ctx) != signal {
		t.Fatal("stop signal not retrievable from context")
	}
	if !IsStopRequested(ctx) {
		t.Fatal("expected IsStopRequested to return true")
	}
	if IsStopRequested(context.Background()) {
		t.Fatal("background context should not report stop requested")
	}
}

func TestStopAtTaskHelpers(t *testing.T) {
	t.Parallel()

	tracker := NewStopAtTask()
	if tracker.Get() != "" {
		t.Fatal("new tracker should start empty")
	}

	tracker.Set("task-1")
	if tracker.Get() != "task-1" {
		t.Fatalf("unexpected tracker id: %q", tracker.Get())
	}
	tracker.Clear()
	if tracker.Get() != "" {
		t.Fatal("tracker should be empty after Clear")
	}

	ctx := WithStopAtTask(context.Background(), tracker)
	if StopAtTaskFromContext(ctx) != tracker {
		t.Fatal("stop-at-task tracker not retrievable from context")
	}
	if StopAtTaskID(ctx) != "" {
		t.Fatalf("expected empty stop-at-task id, got %q", StopAtTaskID(ctx))
	}
}
