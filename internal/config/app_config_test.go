package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCreatesConfigOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	result, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !result.Loaded {
		t.Fatal("expected config to be marked as loaded")
	}

	expectedPath := filepath.Join(dir, appName, configFileName)
	if result.Path != expectedPath {
		t.Fatalf("config path = %q, want %q", result.Path, expectedPath)
	}

	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "ui:") {
		t.Fatalf("expected config to include ui section, got: %s", content)
	}
	if !strings.Contains(content, DefaultUIDir) {
		t.Fatalf("expected config to include default ui dir, got: %s", content)
	}
	if !strings.Contains(content, "flows_dir: "+DefaultFlowsDir) {
		t.Fatalf("expected config to include default flows_dir, got: %s", content)
	}
}

func TestLoadFromUsesProvidedPath(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "custom.yaml")
	if err := os.WriteFile(customPath, []byte("ui:\n  host: 0.0.0.0\n  port: 9091\n  dir: ui/custom\n"), 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	result, err := LoadFrom(customPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if result.Path != customPath {
		t.Fatalf("config path = %q, want %q", result.Path, customPath)
	}
	if result.Config.UI.Host != "0.0.0.0" {
		t.Fatalf("ui host = %q, want 0.0.0.0", result.Config.UI.Host)
	}
	if result.Config.UI.Port != 9091 {
		t.Fatalf("ui port = %d, want 9091", result.Config.UI.Port)
	}
	if result.Config.UI.Dir != "ui/custom" {
		t.Fatalf("ui dir = %q, want ui/custom", result.Config.UI.Dir)
	}
	if result.Config.FlowsDir != DefaultFlowsDir {
		t.Fatalf("flows_dir = %q, want %q", result.Config.FlowsDir, DefaultFlowsDir)
	}
}

func TestLoadFromParsesFlowsDir(t *testing.T) {
	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "flows.yaml")
	content := "ui:\n  host: 127.0.0.1\n  port: 8080\n  dir: ui/dist\nflows_dir: ./my-flows\n"
	if err := os.WriteFile(customPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	result, err := LoadFrom(customPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if result.Config.FlowsDir != "./my-flows" {
		t.Fatalf("flows_dir = %q, want ./my-flows", result.Config.FlowsDir)
	}
}


func TestLoadFromWithVaultSecrets(t *testing.T) {
	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "vault.yaml")
	content := "ui:\n  host: 127.0.0.1\n  port: 8080\n  dir: ui/dist\nsecrets:\n  provider: vault\n  vault:\n    address: https://vault.local\n    token: s.test\n    kv_mount: apps\n    kv_prefix: prod\n"
	if err := os.WriteFile(customPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	result, err := LoadFrom(customPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if result.Config.Secrets.Provider != "vault" {
		t.Fatalf("secrets.provider = %q, want vault", result.Config.Secrets.Provider)
	}
	if result.Config.Secrets.Vault.Address != "https://vault.local" {
		t.Fatalf("secrets.vault.address = %q, want https://vault.local", result.Config.Secrets.Vault.Address)
	}
}

func TestLoadFromVaultRequiresAddressAndToken(t *testing.T) {
	customDir := t.TempDir()
	customPath := filepath.Join(customDir, "invalid-vault.yaml")
	content := "secrets:\n  provider: vault\n"
	if err := os.WriteFile(customPath, []byte(content), 0o600); err != nil {
		t.Fatalf("writing custom config: %v", err)
	}

	_, err := LoadFrom(customPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
