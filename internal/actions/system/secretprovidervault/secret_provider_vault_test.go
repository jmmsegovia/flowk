package secretprovidervault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPayloadValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload Payload
		wantErr string
	}{
		{name: "missing operation", payload: Payload{Address: "http://127.0.0.1:8200", Token: "root"}, wantErr: "unsupported operation"},
		{name: "missing address", payload: Payload{Operation: OperationHealth}, wantErr: "address is required"},
		{name: "missing token", payload: Payload{Operation: OperationHealth, Address: "http://127.0.0.1:8200"}, wantErr: "token is required"},
		{name: "kv put requires path", payload: Payload{Operation: OperationKVPut, Address: "http://127.0.0.1:8200", Token: "root", Data: map[string]any{"k": "v"}}, wantErr: "path is required"},
		{name: "kv put requires data", payload: Payload{Operation: OperationKVPut, Address: "http://127.0.0.1:8200", Token: "root", Path: "apps/demo"}, wantErr: "data is required"},
		{name: "kv get requires path", payload: Payload{Operation: OperationKVGet, Address: "http://127.0.0.1:8200", Token: "root"}, wantErr: "path is required"},
		{name: "kv list requires path", payload: Payload{Operation: OperationKVList, Address: "http://127.0.0.1:8200", Token: "root"}, wantErr: "path is required"},
		{name: "kv delete requires path", payload: Payload{Operation: OperationKVDelete, Address: "http://127.0.0.1:8200", Token: "root"}, wantErr: "path is required"},
		{name: "valid health", payload: Payload{Operation: OperationHealth, Address: "http://127.0.0.1:8200", Token: "root"}},
		{name: "valid kv put", payload: Payload{Operation: OperationKVPut, Address: "http://127.0.0.1:8200", Token: "root", Path: "apps/demo", Data: map[string]any{"api_token": "value"}}},
		{name: "valid kv get", payload: Payload{Operation: OperationKVGet, Address: "http://127.0.0.1:8200", Token: "root", Path: "apps/demo"}},
		{name: "valid kv list", payload: Payload{Operation: OperationKVList, Address: "http://127.0.0.1:8200", Token: "root", Path: "apps"}},
		{name: "valid kv delete", payload: Payload{Operation: OperationKVDelete, Address: "http://127.0.0.1:8200", Token: "root", Path: "apps/demo"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.payload.Validate()
			if tt.wantErr == "" && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestExecuteHealth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sys/health" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if token := r.Header.Get("X-Vault-Token"); token != "root" {
			t.Fatalf("unexpected token %q", token)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"initialized": true,
			"sealed":      false,
			"standby":     false,
			"version":     "1.16.0",
		})
	}))
	defer server.Close()

	spec := Payload{Operation: OperationHealth, Address: server.URL, Token: "root"}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := Execute(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if !result.Initialized || result.Sealed || result.Version == "" {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestExecuteKVPut(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/secret/data/apps/demo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if token := r.Header.Get("X-Vault-Token"); token != "root" {
			t.Fatalf("unexpected token %q", token)
		}
		var payload map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["data"]["api_token"] != "demo-value" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	spec := Payload{
		Operation: OperationKVPut,
		Address:   server.URL,
		Token:     "root",
		Path:      "apps/demo",
		Data:      map[string]any{"api_token": "demo-value"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := Execute(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
	if len(result.WrittenKeys) != 1 || result.WrittenKeys[0] != "api_token" {
		t.Fatalf("unexpected written keys: %+v", result.WrittenKeys)
	}
}

func TestExecuteKVGet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/secret/data/apps/demo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if token := r.Header.Get("X-Vault-Token"); token != "root" {
			t.Fatalf("unexpected token %q", token)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": map[string]any{"api_token": "demo-value", "region": "eu"},
			},
		})
	}))
	defer server.Close()

	spec := Payload{Operation: OperationKVGet, Address: server.URL, Token: "root", Path: "apps/demo"}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := Execute(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if result.SecretData["api_token"] != "demo-value" || result.SecretData["region"] != "eu" {
		t.Fatalf("unexpected secret data: %+v", result.SecretData)
	}
}

func TestExecuteKVList(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/secret/metadata/apps" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("list") != "true" {
			t.Fatalf("expected list=true query, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"keys": []string{"zeta", "alpha"}},
		})
	}))
	defer server.Close()

	spec := Payload{Operation: OperationKVList, Address: server.URL, Token: "root", Path: "apps"}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := Execute(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if len(result.SecretKeys) != 2 || result.SecretKeys[0] != "alpha" || result.SecretKeys[1] != "zeta" {
		t.Fatalf("unexpected keys: %+v", result.SecretKeys)
	}
}

func TestExecuteKVDelete(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/v1/secret/metadata/apps/demo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	spec := Payload{Operation: OperationKVDelete, Address: server.URL, Token: "root", Path: "apps/demo"}
	if err := spec.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	result, err := Execute(context.Background(), spec, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
	if !result.Deleted {
		t.Fatalf("expected deleted=true, got %+v", result)
	}
}
