package secretprovidervault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"flowk/internal/actions/registry"
)

const ActionName = "SECRET_PROVIDER_VAULT"

const (
	OperationHealth   = "HEALTH"
	OperationKVPut    = "KV_PUT"
	OperationKVGet    = "KV_GET"
	OperationKVList   = "KV_LIST"
	OperationKVDelete = "KV_DELETE"
)

type Payload struct {
	Operation string         `json:"operation"`
	Address   string         `json:"address"`
	Token     string         `json:"token"`
	KVMount   string         `json:"kv_mount"`
	Path      string         `json:"path"`
	Data      map[string]any `json:"data"`
}

type ExecutionResult struct {
	Operation       string         `json:"operation"`
	Address         string         `json:"address"`
	KVMount         string         `json:"kv_mount,omitempty"`
	Path            string         `json:"path,omitempty"`
	StatusCode      int            `json:"status_code"`
	WrittenKeys     []string       `json:"written_keys,omitempty"`
	SecretData      map[string]any `json:"secret_data,omitempty"`
	SecretKeys      []string       `json:"secret_keys,omitempty"`
	Deleted         bool           `json:"deleted,omitempty"`
	Initialized     bool           `json:"initialized,omitempty"`
	Sealed          bool           `json:"sealed,omitempty"`
	Standby         bool           `json:"standby,omitempty"`
	Version         string         `json:"version,omitempty"`
	DurationSeconds float64        `json:"durationSeconds"`
}

func (p *Payload) Validate() error {
	p.Operation = strings.ToUpper(strings.TrimSpace(p.Operation))
	p.Address = strings.TrimSpace(p.Address)
	p.Token = strings.TrimSpace(p.Token)
	p.KVMount = strings.Trim(strings.TrimSpace(p.KVMount), "/")
	p.Path = strings.Trim(strings.TrimSpace(p.Path), "/")

	if p.Address == "" {
		return fmt.Errorf("vault provider task: address is required")
	}
	if _, err := url.ParseRequestURI(p.Address); err != nil {
		return fmt.Errorf("vault provider task: invalid address %q: %w", p.Address, err)
	}
	if p.Token == "" {
		return fmt.Errorf("vault provider task: token is required")
	}

	if p.KVMount == "" {
		p.KVMount = "secret"
	}

	switch p.Operation {
	case OperationHealth:
		return nil
	case OperationKVPut:
		if p.Path == "" {
			return fmt.Errorf("vault provider task: path is required for %s", p.Operation)
		}
		if len(p.Data) == 0 {
			return fmt.Errorf("vault provider task: data is required for %s", p.Operation)
		}
	case OperationKVGet, OperationKVList, OperationKVDelete:
		if p.Path == "" {
			return fmt.Errorf("vault provider task: path is required for %s", p.Operation)
		}
	default:
		return fmt.Errorf("vault provider task: unsupported operation %q", p.Operation)
	}

	return nil
}

func Execute(ctx context.Context, spec Payload, execCtx *registry.ExecutionContext) (ExecutionResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	base := strings.TrimRight(spec.Address, "/")
	switch spec.Operation {
	case OperationHealth:
		return health(ctx, client, base, spec)
	case OperationKVPut:
		return kvPut(ctx, client, base, spec)
	case OperationKVGet:
		return kvGet(ctx, client, base, spec)
	case OperationKVList:
		return kvList(ctx, client, base, spec)
	case OperationKVDelete:
		return kvDelete(ctx, client, base, spec)
	default:
		return ExecutionResult{}, fmt.Errorf("vault provider task: unsupported operation %q", spec.Operation)
	}
}

func health(ctx context.Context, client *http.Client, baseURL string, spec Payload) (ExecutionResult, error) {
	endpoint := baseURL + "/v1/sys/health"
	start := time.Now()
	respBody, statusCode, err := doRequest(ctx, client, requestSpec{endpoint: endpoint, token: spec.Token})
	if err != nil {
		return ExecutionResult{}, err
	}

	var body struct {
		Initialized bool   `json:"initialized"`
		Sealed      bool   `json:"sealed"`
		Standby     bool   `json:"standby"`
		Version     string `json:"version"`
	}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &body); err != nil {
			return ExecutionResult{}, fmt.Errorf("vault provider task: decode health response: %w", err)
		}
	}

	return ExecutionResult{
		Operation:       spec.Operation,
		Address:         strings.TrimRight(spec.Address, "/"),
		StatusCode:      statusCode,
		Initialized:     body.Initialized,
		Sealed:          body.Sealed,
		Standby:         body.Standby,
		Version:         body.Version,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func kvPut(ctx context.Context, client *http.Client, baseURL string, spec Payload) (ExecutionResult, error) {
	start := time.Now()
	requestBody := map[string]any{"data": spec.Data}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("vault provider task: encode request body: %w", err)
	}

	endpoint := buildKVEndpoint(baseURL, spec.KVMount, "data", spec.Path)
	_, statusCode, err := doRequest(ctx, client, requestSpec{endpoint: endpoint, token: spec.Token, method: http.MethodPost, body: encoded})
	if err != nil {
		return ExecutionResult{}, err
	}

	keys := make([]string, 0, len(spec.Data))
	for key := range spec.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return ExecutionResult{
		Operation:       spec.Operation,
		Address:         strings.TrimRight(spec.Address, "/"),
		KVMount:         spec.KVMount,
		Path:            spec.Path,
		StatusCode:      statusCode,
		WrittenKeys:     keys,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func kvGet(ctx context.Context, client *http.Client, baseURL string, spec Payload) (ExecutionResult, error) {
	start := time.Now()
	endpoint := buildKVEndpoint(baseURL, spec.KVMount, "data", spec.Path)
	respBody, statusCode, err := doRequest(ctx, client, requestSpec{endpoint: endpoint, token: spec.Token})
	if err != nil {
		return ExecutionResult{}, err
	}

	var body struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &body); err != nil {
		return ExecutionResult{}, fmt.Errorf("vault provider task: decode kv get response: %w", err)
	}

	return ExecutionResult{
		Operation:       spec.Operation,
		Address:         strings.TrimRight(spec.Address, "/"),
		KVMount:         spec.KVMount,
		Path:            spec.Path,
		StatusCode:      statusCode,
		SecretData:      body.Data.Data,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func kvList(ctx context.Context, client *http.Client, baseURL string, spec Payload) (ExecutionResult, error) {
	start := time.Now()
	endpoint := buildKVEndpoint(baseURL, spec.KVMount, "metadata", spec.Path)
	respBody, statusCode, err := doRequest(ctx, client, requestSpec{endpoint: endpoint, token: spec.Token, method: http.MethodGet, query: map[string]string{"list": "true"}})
	if err != nil {
		return ExecutionResult{}, err
	}

	var body struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &body); err != nil {
		return ExecutionResult{}, fmt.Errorf("vault provider task: decode kv list response: %w", err)
	}
	sort.Strings(body.Data.Keys)

	return ExecutionResult{
		Operation:       spec.Operation,
		Address:         strings.TrimRight(spec.Address, "/"),
		KVMount:         spec.KVMount,
		Path:            spec.Path,
		StatusCode:      statusCode,
		SecretKeys:      body.Data.Keys,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func kvDelete(ctx context.Context, client *http.Client, baseURL string, spec Payload) (ExecutionResult, error) {
	start := time.Now()
	endpoint := buildKVEndpoint(baseURL, spec.KVMount, "metadata", spec.Path)
	_, statusCode, err := doRequest(ctx, client, requestSpec{endpoint: endpoint, token: spec.Token, method: http.MethodDelete})
	if err != nil {
		return ExecutionResult{}, err
	}

	return ExecutionResult{
		Operation:       spec.Operation,
		Address:         strings.TrimRight(spec.Address, "/"),
		KVMount:         spec.KVMount,
		Path:            spec.Path,
		StatusCode:      statusCode,
		Deleted:         true,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func buildKVEndpoint(baseURL, mount, segment, secretPath string) string {
	endpoint := fmt.Sprintf("%s/v1/%s/%s/%s", baseURL, path.Clean(mount), segment, path.Clean("/"+secretPath))
	return strings.Replace(endpoint, "/"+segment+"//", "/"+segment+"/", 1)
}

type requestSpec struct {
	endpoint string
	token    string
	method   string
	body     []byte
	query    map[string]string
}

func doRequest(ctx context.Context, client *http.Client, spec requestSpec) ([]byte, int, error) {
	method := spec.method
	if method == "" {
		if len(spec.body) > 0 {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	endpoint := spec.endpoint
	if len(spec.query) > 0 {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, 0, fmt.Errorf("vault provider task: invalid endpoint %q: %w", endpoint, err)
		}
		query := parsed.Query()
		for key, value := range spec.query {
			query.Set(key, value)
		}
		parsed.RawQuery = query.Encode()
		endpoint = parsed.String()
	}

	var bodyReader io.Reader
	if len(spec.body) > 0 {
		bodyReader = bytes.NewReader(spec.body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("vault provider task: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", spec.token)
	if len(spec.body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("vault provider task: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf(
			"vault provider task: request to %q failed with %s: %s",
			endpoint,
			resp.Status,
			strings.TrimSpace(string(respBody)),
		)
	}

	return respBody, resp.StatusCode, nil
}
