package oauth2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidateRequiredFieldsPerOperation(t *testing.T) {
	t.Helper()

	cases := []struct {
		name      string
		operation string
		payload   map[string]any
		wantErr   string
	}{
		{name: "authorize", operation: "AUTHORIZE_URL", payload: map[string]any{"client_id": "c", "redirect_uri": "r", "scopes": "a"}, wantErr: "auth_url is required"},
		{name: "exchange", operation: "EXCHANGE_CODE", payload: map[string]any{"token_url": "u", "client_id": "c", "redirect_uri": "r"}, wantErr: "code is required"},
		{name: "refresh", operation: "REFRESH_TOKEN", payload: map[string]any{"token_url": "u", "client_id": "c"}, wantErr: "refresh_token is required"},
		{name: "device code", operation: "DEVICE_CODE", payload: map[string]any{"device_url": "u", "client_id": "c"}, wantErr: "scopes is required"},
		{name: "device token", operation: "DEVICE_TOKEN", payload: map[string]any{"token_url": "u", "client_id": "c"}, wantErr: "device_code is required"},
		{name: "client credentials", operation: "CLIENT_CREDENTIALS", payload: map[string]any{"token_url": "u", "client_id": "c"}, wantErr: "client_secret is required"},
		{name: "password", operation: "PASSWORD", payload: map[string]any{"token_url": "u", "client_id": "c", "username": "u"}, wantErr: "password is required"},
		{name: "introspect", operation: "INTROSPECT", payload: map[string]any{"introspect_url": "u"}, wantErr: "token is required"},
		{name: "revoke", operation: "REVOKE", payload: map[string]any{"revoke_url": "u"}, wantErr: "token is required"},
	}

	a := action{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()
			tc.payload["operation"] = tc.operation
			raw, _ := json.Marshal(tc.payload)
			_, err := a.Execute(context.Background(), raw, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestAuthorizeURLBuildsExpectedQuery(t *testing.T) {
	t.Helper()

	task := map[string]any{
		"operation":    "AUTHORIZE_URL",
		"auth_url":     "https://auth.example.com/oauth2/authorize",
		"client_id":    "client-1",
		"redirect_uri": "https://app.local/callback",
		"scopes":       []string{"openid", "email"},
		"state":        "abc123",
	}
	raw, _ := json.Marshal(task)
	result, err := (action{}).Execute(context.Background(), raw, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	value := result.Value.(map[string]any)
	authorizeURL, ok := value["authorize_url"].(string)
	if !ok {
		t.Fatalf("authorize_url type = %T", value["authorize_url"])
	}
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	q := parsed.Query()
	if q.Get("scope") != "openid email" {
		t.Fatalf("scope = %q, want %q", q.Get("scope"), "openid email")
	}
	if q.Get("state") != "abc123" {
		t.Fatalf("state = %q, want abc123", q.Get("state"))
	}
}

func TestFormPayloadsForExchangeAndRefresh(t *testing.T) {
	t.Helper()

	var captured []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		copied := make(url.Values, len(r.PostForm))
		for key, values := range r.PostForm {
			copied[key] = append([]string(nil), values...)
		}
		captured = append(captured, copied)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"abc","refresh_token":"def"}`)
	}))
	defer server.Close()

	cases := []map[string]any{
		{
			"operation":    "EXCHANGE_CODE",
			"token_url":    server.URL,
			"client_id":    "client-1",
			"redirect_uri": "https://app/cb",
			"code":         "code-123",
		},
		{
			"operation":     "REFRESH_TOKEN",
			"token_url":     server.URL,
			"client_id":     "client-1",
			"refresh_token": "refresh-123",
		},
	}

	for _, payload := range cases {
		raw, _ := json.Marshal(payload)
		if _, err := (action{}).Execute(context.Background(), raw, nil); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	}

	if got := captured[0].Get("grant_type"); got != "authorization_code" {
		t.Fatalf("exchange grant_type = %q", got)
	}
	if got := captured[0].Get("code"); got != "code-123" {
		t.Fatalf("exchange code = %q", got)
	}
	if got := captured[1].Get("grant_type"); got != "refresh_token" {
		t.Fatalf("refresh grant_type = %q", got)
	}
	if got := captured[1].Get("refresh_token"); got != "refresh-123" {
		t.Fatalf("refresh token = %q", got)
	}
}

func TestPKCEValidation(t *testing.T) {
	t.Helper()

	t.Run("requires verifier when enabled", func(t *testing.T) {
		payload := map[string]any{
			"operation":    "EXCHANGE_CODE",
			"token_url":    "https://issuer/token",
			"client_id":    "c",
			"redirect_uri": "https://app/cb",
			"code":         "x",
			"pkce": map[string]any{
				"enabled": true,
			},
		}
		raw, _ := json.Marshal(payload)
		_, err := (action{}).Execute(context.Background(), raw, nil)
		if err == nil || !strings.Contains(err.Error(), "pkce.verifier is required") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("challenge method validation", func(t *testing.T) {
		payload := map[string]any{
			"operation":    "EXCHANGE_CODE",
			"token_url":    "https://issuer/token",
			"client_id":    "c",
			"redirect_uri": "https://app/cb",
			"code":         "x",
			"pkce": map[string]any{
				"enabled":          true,
				"verifier":         "abc",
				"challenge_method": "BAD",
			},
		}
		raw, _ := json.Marshal(payload)
		_, err := (action{}).Execute(context.Background(), raw, nil)
		if err == nil || !strings.Contains(err.Error(), "challenge_method") {
			t.Fatalf("err = %v", err)
		}
	})
}
