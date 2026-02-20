package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowk/internal/flow"
)

type recordingLogger struct {
	entries []string
}

func (l *recordingLogger) Printf(format string, v ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, v...))
}

func TestExecuteHTTP(t *testing.T) {
	t.Helper()

	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")

	var received struct {
		method string
		auth   string
		body   string
		header string
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.method = r.Method
		received.header = r.Header.Get("X-Test")
		received.auth = r.Header.Get("Authorization")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		received.body = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := RequestConfig{
		Protocol: "http",
		Method:   "post",
		URL:      server.URL,
		Headers: map[string]string{
			"X-Test": "value",
		},
		Body:              []byte(`{"name":"test"}`),
		BasicAuthUser:     "user",
		BasicAuthPassword: "pass",
		Timeout:           time.Second,
	}

	logger := &recordingLogger{}

	result, resultType, err := Execute(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultType != flow.ResultTypeJSON {
		t.Fatalf("resultType = %v, want %v", resultType, flow.ResultTypeJSON)
	}

	if received.method != http.MethodPost {
		t.Fatalf("received method = %s, want %s", received.method, http.MethodPost)
	}

	if !strings.HasPrefix(received.auth, "Basic ") {
		t.Fatalf("authorization header = %q, want prefix Basic ", received.auth)
	}

	if received.header != "value" {
		t.Fatalf("header value = %q, want %q", received.header, "value")
	}

	if received.body != `{"name":"test"}` {
		t.Fatalf("body = %s, want %s", received.body, `{"name":"test"}`)
	}

	data, ok := result.(Result)
	if !ok {
		t.Fatalf("result type = %T, want Result", result)
	}

	if data.Response.StatusCode != http.StatusCreated {
		t.Fatalf("status_code = %v, want %d", data.Response.StatusCode, http.StatusCreated)
	}

	if data.Request.Method != http.MethodPost {
		t.Fatalf("logged request method = %s, want %s", data.Request.Method, http.MethodPost)
	}

	if data.Request.URL != server.URL {
		t.Fatalf("logged request url = %s, want %s", data.Request.URL, server.URL)
	}

	if got := data.Request.Headers["X-Test"]; got != "value" {
		t.Fatalf("logged request header X-Test = %q, want %q", got, "value")
	}

	if got := data.Request.Headers["Authorization"]; got != "<secret>" {
		t.Fatalf("logged Authorization header = %q, want <secret>", got)
	}

	if data.Request.BasicAuthUser != "user" {
		t.Fatalf("logged basic auth user = %q, want %q", data.Request.BasicAuthUser, "user")
	}

	if data.Request.BasicAuthPassword != "<secret>" {
		t.Fatalf("logged basic auth password = %q, want <secret>", data.Request.BasicAuthPassword)
	}

	body, ok := data.Response.Body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", data.Response.Body)
	}

	if body["ok"].(bool) != true {
		t.Fatalf("response body ok = %v, want true", body["ok"])
	}

	if !containsLogEntry(logger.entries, "HTTP proxy: not configured") {
		t.Fatalf("expected log entry indicating proxy not configured, got %v", logger.entries)
	}

	payload := getHTTPResultLog(t, logger.entries)

	request, ok := payload["request"].(map[string]any)
	if !ok {
		t.Fatalf("logged request type = %T, want map[string]any", payload["request"])
	}

	if request["method"].(string) != http.MethodPost {
		t.Fatalf("logged request method = %s, want %s", request["method"], http.MethodPost)
	}

	if request["url"].(string) != server.URL {
		t.Fatalf("logged request url = %s, want %s", request["url"], server.URL)
	}

	headers, ok := request["headers"].(map[string]any)
	if !ok {
		t.Fatalf("logged headers type = %T, want map[string]any", request["headers"])
	}

	if headers["X-Test"].(string) != "value" {
		t.Fatalf("logged header X-Test = %s, want value", headers["X-Test"])
	}

	if headers["Authorization"].(string) != "<secret>" {
		t.Fatalf("logged Authorization header = %s, want <secret>", headers["Authorization"])
	}

	if request["basic_auth_user"].(string) != "user" {
		t.Fatalf("logged basic_auth_user = %s, want user", request["basic_auth_user"])
	}

	if request["basic_auth_password"].(string) != "<secret>" {
		t.Fatalf("logged basic_auth_password = %s, want <secret>", request["basic_auth_password"])
	}

	loggedResponse, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("logged response type = %T, want map[string]any", payload["response"])
	}

	if int(loggedResponse["status_code"].(float64)) != http.StatusCreated {
		t.Fatalf("logged status_code = %v, want %d", loggedResponse["status_code"], http.StatusCreated)
	}

	bodyData, ok := loggedResponse["body"].(map[string]any)
	if !ok {
		t.Fatalf("logged body type = %T, want map[string]any", loggedResponse["body"])
	}

	if bodyData["ok"].(bool) != true {
		t.Fatalf("logged response body ok = %v, want true", bodyData["ok"])
	}

	if _, exists := payload["error"]; exists {
		t.Fatalf("unexpected error field in success log: %v", payload)
	}
}

func containsLogEntry(entries []string, needle string) bool {
	for _, entry := range entries {
		if strings.Contains(entry, needle) {
			return true
		}
	}
	return false
}

func getHTTPResultLog(t *testing.T, entries []string) map[string]any {
	t.Helper()

	const prefix = "HTTP result: "

	for _, entry := range entries {
		if strings.HasPrefix(entry, prefix) {
			var payload map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(entry, prefix)), &payload); err != nil {
				t.Fatalf("parsing HTTP result log: %v", err)
			}
			return payload
		}
	}

	t.Fatalf("HTTP result log entry not found in %v", entries)
	return nil
}

func TestExecuteLogsProxyUsageWithEnvironment(t *testing.T) {
	t.Helper()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"proxied":true}`))
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("NO_PROXY", "")

	cfg := RequestConfig{Protocol: "http", Method: "GET", URL: "http://example.com"}

	logger := &recordingLogger{}

	result, resultType, err := Execute(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if resultType != flow.ResultTypeJSON {
		t.Fatalf("resultType = %v, want %v", resultType, flow.ResultTypeJSON)
	}

	response, ok := result.(Result)
	if !ok {
		t.Fatalf("result type = %T, want Result", result)
	}

	if response.Response.StatusCode != http.StatusOK {
		t.Fatalf("status_code = %v, want %d", response.Response.StatusCode, http.StatusOK)
	}

	if !containsLogEntry(logger.entries, "HTTP proxy: ") {
		t.Fatalf("expected log entry with proxy information, got %v", logger.entries)
	}

	payload := getHTTPResultLog(t, logger.entries)
	loggedResponse, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("logged response type = %T, want map[string]any", payload["response"])
	}

	if int(loggedResponse["status_code"].(float64)) != http.StatusOK {
		t.Fatalf("logged status_code = %v, want %d", loggedResponse["status_code"], http.StatusOK)
	}
}

func TestExecuteHTTPParsesJSONArray(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"key1","state":"ACTIVE"}]`))
	}))
	defer server.Close()

	cfg := RequestConfig{Protocol: "http", Method: "GET", URL: server.URL}

	result, _, err := Execute(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	response, ok := result.(Result)
	if !ok {
		t.Fatalf("result type = %T, want Result", result)
	}

	body, ok := response.Response.Body.([]any)
	if !ok {
		t.Fatalf("body type = %T, want []any", response.Response.Body)
	}

	if len(body) != 1 {
		t.Fatalf("body length = %d, want 1", len(body))
	}

	entry, ok := body[0].(map[string]any)
	if !ok {
		t.Fatalf("body[0] type = %T, want map[string]any", body[0])
	}

	if entry["state"].(string) != "ACTIVE" {
		t.Fatalf("state = %s, want ACTIVE", entry["state"])
	}
}

func TestExecuteHTTPHonorsAcceptedStatusCodes(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := RequestConfig{
		Protocol:            "http",
		Method:              "GET",
		URL:                 server.URL,
		AcceptedStatusCodes: []int{http.StatusAccepted},
	}

	logger := &recordingLogger{}

	result, _, err := Execute(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	response, ok := result.(Result)
	if !ok {
		t.Fatalf("result type = %T, want Result", result)
	}

	if response.Response.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", response.Response.StatusCode, http.StatusAccepted)
	}

	payload := getHTTPResultLog(t, logger.entries)
	loggedResponse, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("logged response type = %T, want map[string]any", payload["response"])
	}

	if int(loggedResponse["status_code"].(float64)) != http.StatusAccepted {
		t.Fatalf("logged status_code = %v, want %d", loggedResponse["status_code"], http.StatusAccepted)
	}
}

func TestExecuteHTTPRejectedStatusCode(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := RequestConfig{
		Protocol:            "http",
		Method:              "GET",
		URL:                 server.URL,
		AcceptedStatusCodes: []int{http.StatusOK},
	}

	logger := &recordingLogger{}

	result, _, err := Execute(context.Background(), cfg, logger)
	if err == nil {
		t.Fatalf("Execute() error = nil, want error")
	}

	response, ok := result.(Result)
	if !ok {
		t.Fatalf("result type = %T, want Result", result)
	}

	if response.Response.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", response.Response.StatusCode, http.StatusBadRequest)
	}

	if !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("error = %v, want unexpected status code", err)
	}

	payload := getHTTPResultLog(t, logger.entries)
	errValue, ok := payload["error"].(string)
	if !ok || strings.TrimSpace(errValue) == "" {
		t.Fatalf("expected error information in log, got %v", payload)
	}
}

func TestExecuteHTTPSWithCustomCA(t *testing.T) {
	t.Helper()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	server.TLS = &tls.Config{
		ClientAuth: tls.NoClientCert,
	}
	server.StartTLS()
	defer server.Close()

	certDER := server.TLS.Certificates[0].Certificate[0]
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}

	cfg := RequestConfig{
		Protocol:            "HTTPS",
		Method:              "GET",
		URL:                 server.URL,
		CACertPath:          caPath,
		AcceptedStatusCodes: []int{http.StatusNoContent},
	}

	if _, _, err := Execute(context.Background(), cfg, nil); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecuteRejectsInvalidProtocol(t *testing.T) {
	cfg := RequestConfig{Protocol: "ftp", Method: "GET", URL: "http://example.com"}
	if _, _, err := Execute(context.Background(), cfg, nil); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRejectsInvalidMethod(t *testing.T) {
	cfg := RequestConfig{Protocol: "http", Method: "TRACE", URL: "http://example.com"}
	if _, _, err := Execute(context.Background(), cfg, nil); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}

func TestExecuteRequiresHost(t *testing.T) {
	cfg := RequestConfig{Protocol: "http", Method: "GET", URL: "/path"}
	if _, _, err := Execute(context.Background(), cfg, nil); err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
}
