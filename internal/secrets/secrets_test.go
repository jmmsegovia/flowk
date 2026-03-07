package secrets

import (
	"context"
	"testing"
)

type stubResolver struct {
	value string
	err   error
}

func (s stubResolver) Resolve(_ context.Context, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.value, nil
}

func TestResolvePlaceholder(t *testing.T) {
	got, err := ResolvePlaceholder(context.Background(), stubResolver{value: "ok"}, "secret:vault:app/path#token")
	if err != nil {
		t.Fatalf("ResolvePlaceholder() error = %v", err)
	}
	if got != "ok" {
		t.Fatalf("ResolvePlaceholder() = %q, want ok", got)
	}
}

func TestResolvePlaceholderMissingResolver(t *testing.T) {
	_, err := ResolvePlaceholder(context.Background(), nil, "secret:vault:app/path#token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParseVaultReference(t *testing.T) {
	path, field, err := parseVaultReference("apps/service#password")
	if err != nil {
		t.Fatalf("parseVaultReference() error = %v", err)
	}
	if path != "apps/service" || field != "password" {
		t.Fatalf("parseVaultReference() = (%q,%q), want (apps/service,password)", path, field)
	}
}

func TestBuildResolverNone(t *testing.T) {
	resolver, err := BuildResolver(Config{Provider: "none"})
	if err != nil {
		t.Fatalf("BuildResolver() error = %v", err)
	}
	if resolver != nil {
		t.Fatalf("resolver = %#v, want nil", resolver)
	}
}
