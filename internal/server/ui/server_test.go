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
    "description": "sub flow",
    "id": "child_flow",
    "name": "child_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "child task",
        "entries": [
          {
            "message": "child"
          }
        ],
        "id": "child_task",
        "name": "child_task"
      }
    ]
  }`
	if err := os.WriteFile(subflowPath, []byte(subflowContent), 0o600); err != nil {
		t.Fatalf("writing subflow: %v", err)
	}

	rootFlow := `{
    "description": "root",
    "id": "root_flow",
    "imports": [
      "subflows/tic/child.json"
    ],
    "name": "root_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "root task",
        "entries": [
          {
            "message": "root"
          }
        ],
        "id": "root_task",
        "name": "root_task"
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
    "description": "child",
    "id": "child_flow",
    "name": "child_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "child task",
        "entries": [
          {
            "message": "child"
          }
        ],
        "id": "child_task",
        "name": "child_task"
      }
    ]
  }`
	if err := os.WriteFile(childPath, []byte(childContent), 0o600); err != nil {
		t.Fatalf("writing child flow: %v", err)
	}

	rootFlow := `{
    "description": "root",
    "id": "root_flow",
    "imports": [
      "../shared/child.json"
    ],
    "name": "root_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "root task",
        "entries": [
          {
            "message": "root"
          }
        ],
        "id": "root_task",
        "name": "root_task"
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
    "description": "sub flow v1",
    "id": "child_flow",
    "name": "child_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "child task",
        "entries": [
          {
            "message": "child-v1"
          }
        ],
        "id": "child_task",
        "name": "child_task"
      }
    ]
  }`
	if err := os.WriteFile(subflowPath, []byte(subflowContentV1), 0o600); err != nil {
		t.Fatalf("writing subflow v1: %v", err)
	}

	rootFlow := `{
    "description": "root",
    "id": "root_flow",
    "imports": [
      "subflows/tic/child.json"
    ],
    "name": "root_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "root task",
        "entries": [
          {
            "message": "root"
          }
        ],
        "id": "root_task",
        "name": "root_task"
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

func TestOpenAPIEndpoint(t *testing.T) {
	srv, err := NewServer(Config{
		Address: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}

	srv.Handle().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if payload["openapi"] != "3.0.3" {
		t.Fatalf("unexpected openapi version: %v", payload["openapi"])
	}

	paths, ok := payload["paths"].(map[string]any)
	if !ok {
		t.Fatalf("expected paths object in response")
	}

	if _, ok := paths["/api/run/events"]; !ok {
		t.Fatalf("expected /api/run/events path in contract")
	}
}

func TestStoreFlowDefinitionMissingImport(t *testing.T) {
	repo := t.TempDir()

	rootFlow := `{
    "description": "root",
    "id": "root_flow",
    "imports": [
      "subflows/tic/missing.json"
    ],
    "name": "root_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "root task",
        "entries": [
          {
            "message": "root"
          }
        ],
        "id": "root_task",
        "name": "root_task"
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
	if !strings.Contains(err.Error(), "required file") {
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
    "description": "sub flow external",
    "id": "child_flow",
    "name": "child_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "child task",
        "entries": [
          {
            "message": "child"
          }
        ],
        "id": "child_task",
        "name": "child_task"
      }
    ]
  }`
	if err := os.WriteFile(subflowPath, []byte(subflowContent), 0o600); err != nil {
		t.Fatalf("writing external subflow: %v", err)
	}

	rootFlow := `{
    "description": "root",
    "id": "root_flow",
    "imports": [
      "subflows/tic/child.json"
    ],
    "name": "root_flow",
    "tasks": [
      {
        "action": "PRINT",
        "description": "root task",
        "entries": [
          {
            "message": "root"
          }
        ],
        "id": "root_task",
        "name": "root_task"
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

func TestHandleFlowsListsRecursiveDirectory(t *testing.T) {
	repo := t.TempDir()
	flowsRoot := filepath.Join(repo, "flows")

	rootFlowPath := filepath.Join(flowsRoot, "root.json")
	nestedFlowPath := filepath.Join(flowsRoot, "nested", "child.json")
	if err := os.MkdirAll(filepath.Dir(nestedFlowPath), 0o755); err != nil {
		t.Fatalf("creating nested dir: %v", err)
	}

	rootFlow := `{
		"id": "root.flow",
		"name": "root.flow",
		"description": "root flow",
		"tasks": [
			{ "id": "t1", "name": "t1", "description": "task", "action": "PRINT", "entries": [{"message": "root"}] }
		]
	}`
	nestedFlow := `{
		"id": "child.flow",
		"name": "child.flow",
		"description": "child flow",
		"tasks": [
			{ "id": "t1", "name": "t1", "description": "task", "action": "PRINT", "entries": [{"message": "child"}] }
		]
	}`

	if err := os.WriteFile(rootFlowPath, []byte(rootFlow), 0o600); err != nil {
		t.Fatalf("writing root flow: %v", err)
	}
	if err := os.WriteFile(nestedFlowPath, []byte(nestedFlow), 0o600); err != nil {
		t.Fatalf("writing nested flow: %v", err)
	}

	srv, err := NewServer(Config{
		Address:     "127.0.0.1:0",
		FlowRootDir: flowsRoot,
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/flows", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	srv.Handle().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload struct {
		RootDir string `json:"rootDir"`
		Flows   []struct {
			ID         string `json:"id"`
			SourceName string `json:"sourceName"`
		} `json:"flows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(payload.Flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(payload.Flows))
	}
	if payload.RootDir != flowsRoot {
		t.Fatalf("rootDir = %q, want %q", payload.RootDir, flowsRoot)
	}

	if payload.Flows[0].SourceName != "nested/child.json" || payload.Flows[1].SourceName != "root.json" {
		t.Fatalf("unexpected flow order/sources: %+v", payload.Flows)
	}
}

func TestHandleFlowsSkipsSubflowsInListing(t *testing.T) {
	repo := t.TempDir()
	flowsRoot := filepath.Join(repo, "flows")

	mainPath := filepath.Join(flowsRoot, "main.json")
	importedPath := filepath.Join(flowsRoot, "subflows", "imported.json")
	markedSubflowPath := filepath.Join(flowsRoot, "marked_subflow.json")
	if err := os.MkdirAll(filepath.Dir(importedPath), 0o755); err != nil {
		t.Fatalf("creating subflows dir: %v", err)
	}

	mainFlow := `{
		"id": "main.flow",
		"name": "main.flow",
		"description": "main flow",
		"imports": ["./subflows/imported.json"],
		"tasks": [
			{ "id": "main.task", "name": "main.task", "description": "task", "action": "PRINT", "entries": [{"message": "main"}] }
		]
	}`
	importedFlow := `{
		"id": "imported.flow",
		"name": "imported.flow",
		"description": "imported flow",
		"tasks": [
			{ "id": "imported.task", "name": "imported.task", "description": "task", "action": "PRINT", "entries": [{"message": "sub"}] }
		]
	}`
	markedSubflow := `{
		"id": "marked.subflow",
		"name": "marked.subflow",
		"description": "marked subflow",
		"is_subflow": true,
		"tasks": [
			{ "id": "t1", "name": "t1", "description": "task", "action": "PRINT", "entries": [{"message": "marked"}] }
		]
	}`

	if err := os.WriteFile(mainPath, []byte(mainFlow), 0o600); err != nil {
		t.Fatalf("writing main flow: %v", err)
	}
	if err := os.WriteFile(importedPath, []byte(importedFlow), 0o600); err != nil {
		t.Fatalf("writing imported flow: %v", err)
	}
	if err := os.WriteFile(markedSubflowPath, []byte(markedSubflow), 0o600); err != nil {
		t.Fatalf("writing marked subflow: %v", err)
	}

	srv, err := NewServer(Config{
		Address:     "127.0.0.1:0",
		FlowRootDir: flowsRoot,
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/api/flows", nil)
	if err != nil {
		t.Fatalf("creating request: %v", err)
	}
	srv.Handle().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload struct {
		Flows []struct {
			ID         string `json:"id"`
			SourceName string `json:"sourceName"`
		} `json:"flows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(payload.Flows) != 1 {
		t.Fatalf("expected only main flow in listing, got %d", len(payload.Flows))
	}
	if payload.Flows[0].ID != "main.flow" || payload.Flows[0].SourceName != "main.json" {
		t.Fatalf("unexpected listed flow: %+v", payload.Flows[0])
	}
}

func TestHandleOpenFlowActivatesSelectedSource(t *testing.T) {
	repo := t.TempDir()
	flowsRoot := filepath.Join(repo, "flows")
	flowPath := filepath.Join(flowsRoot, "demo", "demo.json")
	if err := os.MkdirAll(filepath.Dir(flowPath), 0o755); err != nil {
		t.Fatalf("creating flow dir: %v", err)
	}

	flowJSON := `{
		"id": "demo.flow",
		"name": "demo.flow",
		"description": "demo flow",
		"tasks": [
			{ "id": "t1", "name": "t1", "description": "task", "action": "PRINT", "entries": [{"message": "demo"}] }
		]
	}`
	if err := os.WriteFile(flowPath, []byte(flowJSON), 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	srv, err := NewServer(Config{
		Address:     "127.0.0.1:0",
		FlowRootDir: flowsRoot,
	})
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	openReq := `{"sourceName":"demo/demo.json"}`
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/api/flows/open", strings.NewReader(openReq))
	if err != nil {
		t.Fatalf("creating open request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	srv.Handle().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req, err = http.NewRequest(http.MethodGet, "/api/flow", nil)
	if err != nil {
		t.Fatalf("creating get flow request: %v", err)
	}
	srv.Handle().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	var payload struct {
		ID         string `json:"id"`
		SourceName string `json:"sourceName"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if payload.ID != "demo.flow" {
		t.Fatalf("id = %q, want demo.flow", payload.ID)
	}
	if payload.SourceName != "demo/demo.json" {
		t.Fatalf("sourceName = %q, want demo/demo.json", payload.SourceName)
	}
}
