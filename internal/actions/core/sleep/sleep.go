package sleep

import (
	"context"
	"fmt"
	"time"

	"flowk/internal/flow"
)

const (
	// ActionName identifies the Sleep action in the flow file.
	ActionName = "SLEEP"
)

type Logger interface {
	Printf(format string, v ...interface{})
}

// Execute pauses the execution for the configured number of seconds. The function
// respects the provided context and returns when the timer completes or the
// context is cancelled.
func Execute(ctx context.Context, seconds float64, logger Logger) (any, flow.ResultType, error) {
	if seconds < 0 {
		return nil, "", fmt.Errorf("sleep seconds must be non-negative")
	}

	duration := time.Duration(seconds * float64(time.Second))
	if duration <= 0 {
		return seconds, flow.ResultTypeFloat, nil
	}

	if logger != nil {
		logger.Printf("Sleeping for %.2f seconds", seconds)
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-timer.C:
		return seconds, flow.ResultTypeFloat, nil
	}
}
