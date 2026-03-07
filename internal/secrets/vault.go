package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const providerVault = "vault"

// VaultConfig configures the native Vault secret resolver.
type VaultConfig struct {
	Address  string
	Token    string
	KVMount  string
	KVPrefix string
}

// VaultResolver reads secrets from HashiCorp Vault KV v2.
type VaultResolver struct {
	address  string
	token    string
	kvMount  string
	kvPrefix string
	client   *http.Client
}

func NewVaultResolver(cfg VaultConfig) (*VaultResolver, error) {
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		return nil, fmt.Errorf("vault: address is required")
	}
	if _, err := url.ParseRequestURI(address); err != nil {
		return nil, fmt.Errorf("vault: invalid address %q: %w", address, err)
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("vault: token is required")
	}

	mount := strings.Trim(strings.TrimSpace(cfg.KVMount), "/")
	if mount == "" {
		mount = "kv"
	}

	return &VaultResolver{
		address:  strings.TrimRight(address, "/"),
		token:    token,
		kvMount:  mount,
		kvPrefix: strings.Trim(strings.TrimSpace(cfg.KVPrefix), "/"),
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Resolve resolves a secret reference in the format vault:<path>#<field>.
func (r *VaultResolver) Resolve(ctx context.Context, reference string) (string, error) {
	trimmed := strings.TrimSpace(reference)
	if !strings.HasPrefix(trimmed, providerVault+":") {
		return "", fmt.Errorf("vault: unsupported reference %q", reference)
	}

	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, providerVault+":"))
	secretPath, field, err := parseVaultReference(payload)
	if err != nil {
		return "", err
	}

	if r.kvPrefix != "" {
		secretPath = r.kvPrefix + "/" + secretPath
	}

	resolved, err := r.readKVv2(ctx, secretPath, field)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func (r *VaultResolver) readKVv2(ctx context.Context, secretPath, field string) (string, error) {
	endpoint := fmt.Sprintf("%s/v1/%s/data/%s", r.address, path.Clean(r.kvMount), path.Clean("/"+secretPath))
	endpoint = strings.Replace(endpoint, "/data//", "/data/", 1)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("vault: building request: %w", err)
	}
	req.Header.Set("X-Vault-Token", r.token)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return "", fmt.Errorf("vault: reading %q returned %s: %s", secretPath, resp.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("vault: decoding response: %w", err)
	}

	value, ok := payload.Data.Data[field]
	if !ok {
		return "", fmt.Errorf("vault: field %q not found at %q", field, secretPath)
	}

	rendered, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("vault: field %q at %q is not a string", field, secretPath)
	}
	return rendered, nil
}

func parseVaultReference(reference string) (string, string, error) {
	parts := strings.SplitN(reference, "#", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("vault: reference %q must use format vault:<path>#<field>", reference)
	}

	secretPath := strings.Trim(strings.TrimSpace(parts[0]), "/")
	field := strings.TrimSpace(parts[1])
	if secretPath == "" {
		return "", "", fmt.Errorf("vault: secret path is required")
	}
	if field == "" {
		return "", "", fmt.Errorf("vault: secret field is required")
	}
	return secretPath, field, nil
}
