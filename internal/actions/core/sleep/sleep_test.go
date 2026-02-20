package sleep

import (
	"context"
	"errors"
	"testing"
	"time"

	"flowk/internal/flow"
)

func TestExecuteWaitsForDuration(t *testing.T) {
	ctx := context.Background()
	start := time.Now()

	result, resultType, err := Execute(ctx, 0.05, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultType != flow.ResultTypeFloat {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeFloat)
	}

	if v, ok := result.(float64); !ok || v != 0.05 {
		t.Fatalf("Execute() result = %#v, want 0.05", result)
	}

	if time.Since(start) < 45*time.Millisecond {
		t.Fatalf("Execute() returned too early")
	}
}

func TestExecuteImmediateForZero(t *testing.T) {
	ctx := context.Background()

	result, resultType, err := Execute(ctx, 0, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultType != flow.ResultTypeFloat {
		t.Fatalf("Execute() resultType = %q, want %q", resultType, flow.ResultTypeFloat)
	}

	if v, ok := result.(float64); !ok || v != 0 {
		t.Fatalf("Execute() result = %#v, want 0", result)
	}
}

func TestExecuteReturnsErrorOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := Execute(ctx, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestExecuteRejectsNegativeSeconds(t *testing.T) {
	ctx := context.Background()

	if _, _, err := Execute(ctx, -1, nil); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
