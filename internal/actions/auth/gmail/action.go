package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"flowk/internal/actions/auth/oauth2common"
	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

const ActionName = "GMAIL"

const OperationSendMessage = "SEND_MESSAGE"

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string { return ActionName }

func (action) Execute(ctx context.Context, payload json.RawMessage, _ *registry.ExecutionContext) (registry.Result, error) {
	var task taskConfig
	if err := json.Unmarshal(payload, &task); err != nil {
		return registry.Result{}, fmt.Errorf("gmail: decode payload: %w", err)
	}
	if err := task.validate(); err != nil {
		return registry.Result{}, err
	}

	result, err := task.sendMessage(ctx)
	if err != nil {
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, err
	}
	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}

type taskConfig struct {
	Operation           string            `json:"operation"`
	AccessToken         string            `json:"access_token"`
	UserID              string            `json:"user_id"`
	RawMessage          string            `json:"raw_message"`
	APIBaseURL          string            `json:"api_base_url"`
	TimeoutSeconds      float64           `json:"timeoutSeconds"`
	InsecureSkipVerify  bool              `json:"insecureSkipVerify"`
	ExpectedStatusCodes []int             `json:"expected_status_codes"`
	Headers             map[string]string `json:"headers"`
}

func (t *taskConfig) validate() error {
	t.Operation = strings.ToUpper(strings.TrimSpace(t.Operation))
	if t.Operation == "" {
		return fmt.Errorf("gmail: operation is required")
	}
	if t.Operation != OperationSendMessage {
		return fmt.Errorf("gmail: unsupported operation %q", t.Operation)
	}
	if strings.TrimSpace(t.AccessToken) == "" {
		return fmt.Errorf("gmail: access_token is required for %s", t.Operation)
	}
	if strings.TrimSpace(t.RawMessage) == "" {
		return fmt.Errorf("gmail: raw_message is required for %s", t.Operation)
	}
	if t.TimeoutSeconds < 0 {
		return fmt.Errorf("gmail: timeoutSeconds cannot be negative")
	}
	if strings.TrimSpace(t.UserID) == "" {
		t.UserID = "me"
	}
	return nil
}

func (t taskConfig) sendMessage(ctx context.Context) (oauth2common.HTTPExchangeResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(t.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://gmail.googleapis.com"
	}
	endpoint := fmt.Sprintf("%s/gmail/v1/users/%s/messages/send", baseURL, t.UserID)
	headers := map[string]string{"Authorization": "Bearer " + t.AccessToken}
	for key, value := range t.Headers {
		headers[key] = value
	}
	return oauth2common.ExecuteJSONRequest(ctx, http.MethodPost, endpoint, map[string]any{"raw": t.RawMessage}, oauth2common.HTTPOptions{
		Headers:             headers,
		TimeoutSeconds:      t.TimeoutSeconds,
		InsecureSkipVerify:  t.InsecureSkipVerify,
		ExpectedStatusCodes: t.expectedStatusCodes(),
	})
}

func (t taskConfig) expectedStatusCodes() []int {
	if len(t.ExpectedStatusCodes) == 0 {
		return []int{http.StatusOK}
	}
	return t.ExpectedStatusCodes
}
