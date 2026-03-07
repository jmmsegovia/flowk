package expansion

import (
	"context"
	"errors"
	"testing"
)

type secretStub struct {
	value string
	err   error
}

func (s secretStub) Resolve(_ context.Context, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.value, nil
}

func TestExpandStringResolvesSecretPlaceholder(t *testing.T) {
	t.Cleanup(func() { SetSecretResolver(nil) })
	SetSecretResolver(secretStub{value: "super-secret"})

	got, err := ExpandString("token=${secret:vault:apps/api#token}", nil)
	if err != nil {
		t.Fatalf("ExpandString() error = %v", err)
	}
	if got != "token=super-secret" {
		t.Fatalf("ExpandString() = %q, want token=super-secret", got)
	}
}

func TestExpandStringSecretPlaceholderWithoutResolver(t *testing.T) {
	t.Cleanup(func() { SetSecretResolver(nil) })
	SetSecretResolver(nil)

	_, err := ExpandString("${secret:vault:apps/api#token}", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExpandStringSecretPlaceholderResolverError(t *testing.T) {
	t.Cleanup(func() { SetSecretResolver(nil) })
	SetSecretResolver(secretStub{err: errors.New("boom")})

	_, err := ExpandString("${secret:vault:apps/api#token}", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
