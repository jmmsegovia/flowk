package runcontext

import (
	"context"
	"sync/atomic"
)

type stopKey struct{}

// StopSignal tracks a stop request for the active flow run.
type StopSignal struct {
	requested atomic.Bool
}

// NewStopSignal creates a new stop request tracker.
func NewStopSignal() *StopSignal {
	return &StopSignal{}
}

// Request marks the stop request as active.
func (s *StopSignal) Request() {
	if s == nil {
		return
	}
	s.requested.Store(true)
}

// Requested reports whether a stop request has been made.
func (s *StopSignal) Requested() bool {
	if s == nil {
		return false
	}
	return s.requested.Load()
}

// WithStopSignal stores a stop signal in the context.
func WithStopSignal(ctx context.Context, signal *StopSignal) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, stopKey{}, signal)
}

// StopSignalFromContext returns the stop signal stored in the context.
func StopSignalFromContext(ctx context.Context) *StopSignal {
	if ctx == nil {
		return nil
	}
	signal, _ := ctx.Value(stopKey{}).(*StopSignal)
	return signal
}

// IsStopRequested reports whether a stop has been requested for the current run.
func IsStopRequested(ctx context.Context) bool {
	return StopSignalFromContext(ctx).Requested()
}
