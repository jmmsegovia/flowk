package secrets

import (
	"context"
	"fmt"
	"strings"
)

// Resolver resolves secret references for runtime payload expansion.
type Resolver interface {
	Resolve(ctx context.Context, reference string) (string, error)
}

// ResolvePlaceholder resolves placeholders in the form secret:<reference>.
func ResolvePlaceholder(ctx context.Context, resolver Resolver, placeholder string) (string, error) {
	trimmed := strings.TrimSpace(placeholder)
	if !strings.HasPrefix(trimmed, "secret:") {
		return "", fmt.Errorf("unsupported placeholder %q", placeholder)
	}
	if resolver == nil {
		return "", fmt.Errorf("secret resolver is not configured")
	}

	reference := strings.TrimSpace(strings.TrimPrefix(trimmed, "secret:"))
	if reference == "" {
		return "", fmt.Errorf("secret reference is empty")
	}

	return resolver.Resolve(ctx, reference)
}
