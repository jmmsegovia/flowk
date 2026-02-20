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
}
