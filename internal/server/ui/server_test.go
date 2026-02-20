package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "flowk/internal/app"
)

func TestStoreFlowDefinitionCopiesImports(t *testing.T) {
	repo := t.TempDir()

	subflowDir := filepath.Join(repo, "flows", "complete", "subflows", "tic")
	if err := os.MkdirAll(subflowDir, 0o755); err != nil {
		t.Fatalf("creating subflow dir: %v", err)
	}

	subflowPath := filepath.Join(subflowDir, "child.json")
	subflowContent := `{
  "id": "child_flow",
  "description": "sub flow",
  "tasks": [
    {
      "id": "child_task",
      "description": "child task",
      "action": "PRINT",
      "entries": [
        {"message": "child"}
      ]
    }
  ]
}`
	if err := os.WriteFile(subflowPath, []byte(subflowContent), 0o600); err != nil {
		t.Fatalf("writing subflow: %v", err)
	}

	rootFlow := `{
  "id": "root_flow",
  "description": "root",
  "imports": ["subflows/tic/child.json"],
  "tasks": [
    {
      "id": "root_task",
      "description": "root task",
      "action": "PRINT",
      "entries": [
        {"message": "root"}
      ]
    }
  ]
}`

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	srv, err := NewServer(Config{
		Address:       "127.0.0.1:0",
		FlowUploadDir: filepath.Join(repo, "uploads"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	path, def, err := srv.storeFlowDefinition([]byte(rootFlow))
	if err != nil {
		t.Fatalf("storeFlowDefinition error: %v", err)
	}
	if def == nil {
		t.Fatal("definition is nil")
	}

	if len(def.Tasks) != 2 {
		t.Fatalf("expected 2 tasks after import, got %d", len(def.Tasks))
	}

	importedPath := filepath.Join(filepath.Dir(path), "subflows", "tic", "child.json")
	if _, err := os.Stat(importedPath); err != nil {
		t.Fatalf("expected copied import at %s: %v", importedPath, err)
	}
}

func TestStoreFlowDefinitionRewritesEscapingImport(t *testing.T) {
	repo := t.TempDir()

	sharedDir := filepath.Join(repo, "shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("creating shared dir: %v", err)
	}

	childPath := filepath.Join(sharedDir, "child.json")
	childContent := `{
  "id": "child_flow",
  "description": "child",
  "tasks": [
    {
      "id": "child_task",
      "description": "child task",
      "action": "PRINT",
      "entries": [
        {"message": "child"}
      ]
    }
  ]
}`
	if err := os.WriteFile(childPath, []byte(childContent), 0o600); err != nil {
		t.Fatalf("writing child flow: %v", err)
	}

	rootFlow := `{
  "id": "root_flow",
  "description": "root",
  "imports": ["../shared/child.json"],
  "tasks": [
    {
      "id": "root_task",
      "description": "root task",
      "action": "PRINT",
      "entries": [
        {"message": "root"}
      ]
    }
  ]
}`

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	srv, err := NewServer(Config{
		Address:       "127.0.0.1:0",
		FlowUploadDir: filepath.Join(repo, "uploads"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	path, def, err := srv.storeFlowDefinition([]byte(rootFlow))
	if err != nil {
		t.Fatalf("storeFlowDefinition error: %v", err)
	}
	if def == nil {
		t.Fatal("definition is nil")
	}
	if len(def.Tasks) != 2 {
		t.Fatalf("expected 2 tasks after import, got %d", len(def.Tasks))
	}

	importedPath := filepath.Join(filepath.Dir(path), "shared", "child.json")
	if _, err := os.Stat(importedPath); err != nil {
		t.Fatalf("expected copied import at %s: %v", importedPath, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading uploaded flow: %v", err)
	}
	if strings.Contains(string(data), "../shared/child.json") {
		t.Fatalf("expected import path to be rewritten, got: %s", string(data))
	}
	if !strings.Contains(string(data), "\"shared/child.json\"") {
		t.Fatalf("expected rewritten import path, got: %s", string(data))
	}
}

func TestStoreFlowDefinitionRefreshesImports(t *testing.T) {
	repo := t.TempDir()

	subflowDir := filepath.Join(repo, "flows", "complete", "subflows", "tic")
	if err := os.MkdirAll(subflowDir, 0o755); err != nil {
		t.Fatalf("creating subflow dir: %v", err)
	}

	subflowPath := filepath.Join(subflowDir, "child.json")
	subflowContentV1 := `{
  "id": "child_flow",
  "description": "sub flow v1",
  "tasks": [
    {
      "id": "child_task",
      "description": "child task",
      "action": "PRINT",
      "entries": [
        {"message": "child-v1"}
      ]
    }
  ]
}`
	if err := os.WriteFile(subflowPath, []byte(subflowContentV1), 0o600); err != nil {
		t.Fatalf("writing subflow v1: %v", err)
	}

	rootFlow := `{
  "id": "root_flow",
  "description": "root",
  "imports": ["subflows/tic/child.json"],
  "tasks": [
    {
      "id": "root_task",
      "description": "root task",
      "action": "PRINT",
      "entries": [
        {"message": "root"}
      ]
    }
  ]
}`

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	srv, err := NewServer(Config{
		Address:       "127.0.0.1:0",
		FlowUploadDir: filepath.Join(repo, "uploads"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	firstPath, _, err := srv.storeFlowDefinition([]byte(rootFlow))
	if err != nil {
		t.Fatalf("storeFlowDefinition v1 error: %v", err)
	}

	importedPath := filepath.Join(filepath.Dir(firstPath), "subflows", "tic", "child.json")
	data, err := os.ReadFile(importedPath)
	if err != nil {
		t.Fatalf("reading copied import: %v", err)
	}
	if !strings.Contains(string(data), "child-v1") {
		t.Fatalf("expected v1 import content, got: %s", string(data))
	}

	subflowContentV2 := strings.ReplaceAll(subflowContentV1, "child-v1", "child-v2")
	subflowContentV2 = strings.ReplaceAll(subflowContentV2, "v1", "v2")
	if err := os.WriteFile(subflowPath, []byte(subflowContentV2), 0o600); err != nil {
		t.Fatalf("writing subflow v2: %v", err)
	}

	secondPath, _, err := srv.storeFlowDefinition([]byte(rootFlow))
	if err != nil {
		t.Fatalf("storeFlowDefinition v2 error: %v", err)
	}

	importedPathV2 := filepath.Join(filepath.Dir(secondPath), "subflows", "tic", "child.json")
	data, err = os.ReadFile(importedPathV2)
	if err != nil {
		t.Fatalf("reading refreshed import: %v", err)
	}
	if !strings.Contains(string(data), "child-v2") {
		t.Fatalf("expected v2 import content, got: %s", string(data))
	}
}

func TestActionsGuideEndpoint(t *testing.T) {
	srv, err := NewServer(Config{
		Address: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/actions/guide", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	srv.Handle().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload struct {
		Primer   string `json:"primer"`
		Markdown string `json:"markdown"`
		Actions  []any  `json:"actions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if strings.TrimSpace(payload.Primer) == "" {
		t.Fatalf("expected primer in response, got empty")
	}

	if len(payload.Actions) == 0 {
		t.Fatalf("expected at least one action in response")
	}

	if !strings.Contains(payload.Markdown, "FlowK") {
		t.Fatalf("markdown field missing expected content: %s", payload.Markdown)
	}
}

func TestStoreFlowDefinitionMissingImport(t *testing.T) {
	repo := t.TempDir()

	rootFlow := `{
  "id": "root_flow",
  "description": "root",
  "imports": ["subflows/tic/missing.json"],
  "tasks": [
    {
      "id": "root_task",
      "description": "root task",
      "action": "PRINT",
      "entries": [
        {"message": "root"}
      ]
    }
  ]
}`

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	srv, err := NewServer(Config{
		Address:       "127.0.0.1:0",
		FlowUploadDir: filepath.Join(repo, "uploads"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	_, _, err = srv.storeFlowDefinition([]byte(rootFlow))
	if err == nil {
		t.Fatal("expected error for missing import, got nil")
	}
	if !strings.Contains(err.Error(), "no se encontrA3 el archivo requerido") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStoreFlowDefinitionSearchesParentDirForImports(t *testing.T) {
	repo := t.TempDir()

	flowkRoot := filepath.Join(repo, "flowk")
	externalRoot := filepath.Join(repo, "flow_complete", "complete")

	if err := os.MkdirAll(flowkRoot, 0o755); err != nil {
		t.Fatalf("creating flowk root: %v", err)
	}

	subflowDir := filepath.Join(externalRoot, "subflows", "tic")
	if err := os.MkdirAll(subflowDir, 0o755); err != nil {
		t.Fatalf("creating external subflow dir: %v", err)
	}

	subflowPath := filepath.Join(subflowDir, "child.json")
	subflowContent := `{
  "id": "child_flow",
  "description": "sub flow external",
  "tasks": [
    {
      "id": "child_task",
      "description": "child task",
      "action": "PRINT",
      "entries": [
        {"message": "child"}
      ]
    }
  ]
}`
	if err := os.WriteFile(subflowPath, []byte(subflowContent), 0o600); err != nil {
		t.Fatalf("writing external subflow: %v", err)
	}

	rootFlow := `{
  "id": "root_flow",
  "description": "root",
  "imports": ["subflows/tic/child.json"],
  "tasks": [
    {
      "id": "root_task",
      "description": "root task",
      "action": "PRINT",
      "entries": [
        {"message": "root"}
      ]
    }
  ]
}`

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := os.Chdir(flowkRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	srv, err := NewServer(Config{
		Address:       "127.0.0.1:0",
		FlowUploadDir: filepath.Join(flowkRoot, "uploads"),
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	path, def, err := srv.storeFlowDefinition([]byte(rootFlow))
	if err != nil {
		t.Fatalf("storeFlowDefinition error: %v", err)
	}
	if def == nil {
		t.Fatal("definition is nil")
	}

	importedPath := filepath.Join(filepath.Dir(path), "subflows", "tic", "child.json")
	if _, err := os.Stat(importedPath); err != nil {
		t.Fatalf("expected copied import at %s: %v", importedPath, err)
	}
}
