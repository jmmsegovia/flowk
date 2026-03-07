package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func installSchemaProviderHarness(t *testing.T) func(values ...[]byte) {
	t.Helper()

	var mu sync.RWMutex
	var fragments []json.RawMessage
	var version uint64

	RegisterSchemaProvider(func() ([]json.RawMessage, uint64) {
		mu.RLock()
		defer mu.RUnlock()
		copied := make([]json.RawMessage, len(fragments))
		copy(copied, fragments)
		return copied, version
	})

	schemaCache = sync.Map{}

	t.Cleanup(func() {
		ResetSchemaProviderForTesting()
		SetupSchemaProviderForTesting(t)
	})

	return func(values ...[]byte) {
		mu.Lock()
		defer mu.Unlock()

		fragments = make([]json.RawMessage, len(values))
		for i, data := range values {
			fragments[i] = append(json.RawMessage(nil), data...)
		}
		version++
	}
}

func TestValidateDefinitionAgainstSchema_ActionRemoval(t *testing.T) {
	setFragments := installSchemaProviderHarness(t)

	sleepFragment, err := os.ReadFile(filepath.Join("..", "actions", "core", "sleep", "schema.json"))
	if err != nil {
		t.Fatalf("reading sleep fragment: %v", err)
	}
	httpFragment, err := os.ReadFile(filepath.Join("..", "actions", "network", "httpclient", "schema.json"))
	if err != nil {
		t.Fatalf("reading http fragment: %v", err)
	}

	setFragments(sleepFragment, httpFragment)

	tmpDir := t.TempDir()

	sleepFlow := []byte(`{
      "description": "sleep test",
      "id": "sleep-flow",
      "name": "sleep-flow",
      "tasks": [
        {
          "action": "SLEEP",
          "description": "sleep action",
          "id": "sleep",
          "name": "sleep",
          "seconds": 1
        }
      ]
    }`)
	sleepPath := filepath.Join(tmpDir, "sleep.json")
	if err := os.WriteFile(sleepPath, sleepFlow, 0o600); err != nil {
		t.Fatalf("writing sleep flow: %v", err)
	}
	if err := validateDefinitionAgainstSchema(sleepPath, sleepFlow); err != nil {
		t.Fatalf("sleep flow validation failed: %v", err)
	}

	httpFlow := []byte(`{
      "description": "http test",
      "id": "http-flow",
      "name": "http-flow",
      "tasks": [
        {
          "action": "HTTP_REQUEST",
          "description": "http action",
          "id": "http",
          "method": "GET",
          "name": "http",
          "protocol": "HTTP",
          "url": "https://example.com"
        }
      ]
    }`)
	httpPath := filepath.Join(tmpDir, "http.json")
	if err := os.WriteFile(httpPath, httpFlow, 0o600); err != nil {
		t.Fatalf("writing http flow: %v", err)
	}
	if err := validateDefinitionAgainstSchema(httpPath, httpFlow); err != nil {
		t.Fatalf("http flow validation failed before removal: %v", err)
	}

	setFragments(sleepFragment)

	if err := validateDefinitionAgainstSchema(httpPath, httpFlow); err == nil {
		t.Fatalf("expected validation error for removed action, got nil")
	}
}

func TestValidateDefinitionAgainstSchema_ActionSpecificOperations(t *testing.T) {
	setFragments := installSchemaProviderHarness(t)

	cassandraFragment, err := os.ReadFile(filepath.Join("..", "actions", "db", "cassandra", "schema.json"))
	if err != nil {
		t.Fatalf("reading cassandra fragment: %v", err)
	}
	kubernetesFragment, err := os.ReadFile(filepath.Join("..", "actions", "infra", "kubernetes", "schema.json"))
	if err != nil {
		t.Fatalf("reading kubernetes fragment: %v", err)
	}

	setFragments(cassandraFragment, kubernetesFragment)

	tmpDir := t.TempDir()

	flowContent := []byte(`{
      "description": "mixed cassandra and kubernetes operations",
      "id": "platform-ops",
      "name": "platform-ops",
      "tasks": [
        {
          "action": "DB_CASSANDRA_OPERATION",
          "description": "ensure connectivity",
          "id": "cassandra.check",
          "name": "cassandra.check",
          "operation": "CHECK_CONNECTION",
          "platform": "prod"
        },
        {
          "action": "KUBERNETES",
          "context": "dev-context",
          "deployments": [
            "api"
          ],
          "description": "scale api deployment",
          "id": "kubernetes.scale",
          "name": "kubernetes.scale",
          "namespace": "default",
          "operation": "SCALE",
          "replicas": 3
        }
      ]
    }`)

	flowPath := filepath.Join(tmpDir, "flow.json")
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	if err := validateDefinitionAgainstSchema(flowPath, flowContent); err != nil {
		t.Fatalf("flow validation failed: %v", err)
	}
}

func TestValidateDefinitionAgainstSchema_HelmFragmentDoesNotRestrictOtherOperations(t *testing.T) {
	setFragments := installSchemaProviderHarness(t)

	helmFragment, err := os.ReadFile(filepath.Join("..", "actions", "infra", "helm", "schema.json"))
	if err != nil {
		t.Fatalf("reading helm fragment: %v", err)
	}
	dockerFragment, err := os.ReadFile(filepath.Join("..", "actions", "system", "docker", "schema.json"))
	if err != nil {
		t.Fatalf("reading docker fragment: %v", err)
	}

	setFragments(helmFragment, dockerFragment)

	tmpDir := t.TempDir()
	flowContent := []byte(`{
      "description": "validate docker operations with helm schema present",
      "id": "docker-only",
      "name": "docker-only",
      "tasks": [
        {
          "action": "DOCKER",
          "id": "docker.pull",
          "image": "alpine:3.19",
          "name": "docker.pull",
          "operation": "IMAGE_PULL"
        }
      ]
    }`)
	flowPath := filepath.Join(tmpDir, "flow.json")
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	if err := validateDefinitionAgainstSchema(flowPath, flowContent); err != nil {
		t.Fatalf("flow validation failed: %v", err)
	}
}

func TestValidateDefinitionAgainstSchema_SecretProviderVaultAction(t *testing.T) {
	setFragments := installSchemaProviderHarness(t)

	vaultFragment, err := os.ReadFile(filepath.Join("..", "actions", "system", "secretprovidervault", "schema.json"))
	if err != nil {
		t.Fatalf("reading secret provider vault fragment: %v", err)
	}

	setFragments(vaultFragment)

	tmpDir := t.TempDir()
	flowContent := []byte(`{
      "description": "validate secret provider vault action",
      "id": "vault-health",
      "name": "vault-health",
      "tasks": [
        {
          "action": "SECRET_PROVIDER_VAULT",
          "id": "vault.health",
          "name": "vault.health",
          "operation": "HEALTH",
          "address": "http://127.0.0.1:8200",
          "token": "root"
        }
      ]
    }`)
	flowPath := filepath.Join(tmpDir, "flow.json")
	if err := os.WriteFile(flowPath, flowContent, 0o600); err != nil {
		t.Fatalf("writing flow: %v", err)
	}

	if err := validateDefinitionAgainstSchema(flowPath, flowContent); err != nil {
		t.Fatalf("flow validation failed: %v", err)
	}
}
