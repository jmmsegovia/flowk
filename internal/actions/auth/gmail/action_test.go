package gmail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowk/internal/actions/auth/oauth2common"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		wantErr string
	}{
		{name: "missing operation", payload: map[string]any{}, wantErr: "operation is required"},
		{name: "unsupported operation", payload: map[string]any{"operation": "X"}, wantErr: "unsupported operation"},
		{name: "missing access token", payload: map[string]any{"operation": "SEND_MESSAGE", "raw_message": "abc"}, wantErr: "access_token is required"},
		{name: "missing raw message", payload: map[string]any{"operation": "SEND_MESSAGE", "access_token": "token"}, wantErr: "raw_message is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			_, err := (action{}).Execute(context.Background(), raw, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestSendMessageRequest(t *testing.T) {
	var (
		gotAuth string
		gotBody string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg-123","threadId":"thr-1"}`)
	}))
	defer server.Close()

	payload := map[string]any{
		"operation":      "SEND_MESSAGE",
		"access_token":   "token-1",
		"raw_message":    "cmF3",
		"api_base_url":   server.URL,
		"user_id":        "me",
		"timeoutSeconds": 1,
	}
	raw, _ := json.Marshal(payload)

	res, err := (action{}).Execute(context.Background(), raw, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody != `{"raw":"cmF3"}` {
		t.Fatalf("body = %s", gotBody)
	}

	value := res.Value.(oauth2common.HTTPExchangeResult)
	if value.Response.StatusCode != 200 {
		t.Fatalf("status_code = %v", value.Response.StatusCode)
	}
}
