package runcontext

import "context"

type resumeKey struct{}

// WithResume flags the context as a resume execution.
func WithResume(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, resumeKey{}, true)
}

// IsResume reports whether the context represents a resume execution.
func IsResume(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	value := ctx.Value(resumeKey{})
	resume, ok := value.(bool)
	return ok && resume
}
