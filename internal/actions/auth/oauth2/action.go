package oauth2

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"flowk/internal/actions/auth/oauth2common"
	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

const ActionName = "OAUTH2"

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string { return ActionName }

func (action) Execute(ctx context.Context, payload json.RawMessage, _ *registry.ExecutionContext) (registry.Result, error) {
	var task taskConfig
	if err := json.Unmarshal(payload, &task); err != nil {
		return registry.Result{}, fmt.Errorf("oauth2: decode payload: %w", err)
	}
	if err := task.validate(); err != nil {
		return registry.Result{}, err
	}

	if task.Operation == "AUTHORIZE_URL" {
		value, err := task.buildAuthorizeURL()
		if err != nil {
			return registry.Result{}, err
		}
		return registry.Result{Value: value, Type: flow.ResultTypeJSON}, nil
	}

	result, err := task.executeHTTP(ctx)
	if err != nil {
		return registry.Result{Value: result, Type: flow.ResultTypeJSON}, err
	}

	return registry.Result{Value: result, Type: flow.ResultTypeJSON}, nil
}

type taskConfig struct {
	Operation           string            `json:"operation"`
	Description         string            `json:"description"`
	AuthURL             string            `json:"auth_url"`
	TokenURL            string            `json:"token_url"`
	DeviceURL           string            `json:"device_url"`
	IntrospectURL       string            `json:"introspect_url"`
	RevokeURL           string            `json:"revoke_url"`
	ClientID            string            `json:"client_id"`
	ClientSecret        string            `json:"client_secret"`
	RedirectURI         string            `json:"redirect_uri"`
	Scopes              any               `json:"scopes"`
	State               string            `json:"state"`
	Audience            string            `json:"audience"`
	Resource            string            `json:"resource"`
	Code                string            `json:"code"`
	RefreshToken        string            `json:"refresh_token"`
	DeviceCode          string            `json:"device_code"`
	Username            string            `json:"username"`
	Password            string            `json:"password"`
	Token               string            `json:"token"`
	ExtraParams         map[string]string `json:"extra_params"`
	Headers             map[string]string `json:"headers"`
	PKCE                pkceConfig        `json:"pkce"`
	TimeoutSeconds      float64           `json:"timeoutSeconds"`
	InsecureSkipVerify  bool              `json:"insecureSkipVerify"`
	ExpectedStatusCodes []int             `json:"expected_status_codes"`
}

type pkceConfig struct {
	Enabled         bool   `json:"enabled"`
	Verifier        string `json:"verifier"`
	Challenge       string `json:"challenge"`
	ChallengeMethod string `json:"challenge_method"`
}

func (t *taskConfig) validate() error {
	t.Operation = strings.ToUpper(strings.TrimSpace(t.Operation))
	if t.Operation == "" {
		return fmt.Errorf("oauth2: operation is required")
	}

	if t.PKCE.Enabled && strings.TrimSpace(t.PKCE.Verifier) == "" {
		return fmt.Errorf("oauth2: pkce.verifier is required when pkce.enabled is true")
	}
	if method := strings.TrimSpace(t.PKCE.ChallengeMethod); method != "" {
		if method != "S256" && method != "plain" {
			return fmt.Errorf("oauth2: pkce.challenge_method must be S256 or plain")
		}
	}
	if t.TimeoutSeconds < 0 {
		return fmt.Errorf("oauth2: timeoutSeconds cannot be negative")
	}

	required := map[string][]string{
		"AUTHORIZE_URL":      {"auth_url", "client_id", "redirect_uri", "scopes"},
		"EXCHANGE_CODE":      {"token_url", "client_id", "redirect_uri", "code"},
		"REFRESH_TOKEN":      {"token_url", "client_id", "refresh_token"},
		"DEVICE_CODE":        {"device_url", "client_id", "scopes"},
		"DEVICE_TOKEN":       {"token_url", "client_id", "device_code"},
		"CLIENT_CREDENTIALS": {"token_url", "client_id", "client_secret"},
		"PASSWORD":           {"token_url", "client_id", "username", "password"},
		"INTROSPECT":         {"introspect_url", "token"},
		"REVOKE":             {"revoke_url", "token"},
	}
	fields, ok := required[t.Operation]
	if !ok {
		return fmt.Errorf("oauth2: unsupported operation %q", t.Operation)
	}
	for _, field := range fields {
		if !t.hasField(field) {
			return fmt.Errorf("oauth2: %s is required for %s", field, t.Operation)
		}
	}

	if _, err := oauth2common.ScopeValue(t.Scopes); err != nil {
		return err
	}

	return nil
}

func (t taskConfig) hasField(name string) bool {
	switch name {
	case "auth_url":
		return strings.TrimSpace(t.AuthURL) != ""
	case "token_url":
		return strings.TrimSpace(t.TokenURL) != ""
	case "device_url":
		return strings.TrimSpace(t.DeviceURL) != ""
	case "introspect_url":
		return strings.TrimSpace(t.IntrospectURL) != ""
	case "revoke_url":
		return strings.TrimSpace(t.RevokeURL) != ""
	case "client_id":
		return strings.TrimSpace(t.ClientID) != ""
	case "redirect_uri":
		return strings.TrimSpace(t.RedirectURI) != ""
	case "scopes":
		s, _ := oauth2common.ScopeValue(t.Scopes)
		return strings.TrimSpace(s) != ""
	case "code":
		return strings.TrimSpace(t.Code) != ""
	case "refresh_token":
		return strings.TrimSpace(t.RefreshToken) != ""
	case "device_code":
		return strings.TrimSpace(t.DeviceCode) != ""
	case "client_secret":
		return strings.TrimSpace(t.ClientSecret) != ""
	case "username":
		return strings.TrimSpace(t.Username) != ""
	case "password":
		return strings.TrimSpace(t.Password) != ""
	case "token":
		return strings.TrimSpace(t.Token) != ""
	default:
		return false
	}
}

func (t taskConfig) buildAuthorizeURL() (map[string]any, error) {
	scope, _ := oauth2common.ScopeValue(t.Scopes)
	parsed, err := url.Parse(t.AuthURL)
	if err != nil {
		return nil, fmt.Errorf("oauth2: parse auth_url: %w", err)
	}

	q := parsed.Query()
	q.Set("response_type", "code")
	q.Set("client_id", t.ClientID)
	q.Set("redirect_uri", t.RedirectURI)
	q.Set("scope", scope)
	if strings.TrimSpace(t.State) != "" {
		q.Set("state", t.State)
	}
	if strings.TrimSpace(t.Audience) != "" {
		q.Set("audience", t.Audience)
	}
	if strings.TrimSpace(t.Resource) != "" {
		q.Set("resource", t.Resource)
	}
	if t.PKCE.Enabled {
		challengeMethod := strings.TrimSpace(t.PKCE.ChallengeMethod)
		if challengeMethod == "" {
			challengeMethod = "S256"
		}
		challenge := strings.TrimSpace(t.PKCE.Challenge)
		if challenge == "" {
			if challengeMethod == "plain" {
				challenge = t.PKCE.Verifier
			} else {
				hash := sha256.Sum256([]byte(t.PKCE.Verifier))
				challenge = base64.RawURLEncoding.EncodeToString(hash[:])
			}
		}
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", challengeMethod)
	}
	for key, value := range t.ExtraParams {
		if strings.TrimSpace(key) == "" {
			continue
		}
		q.Set(key, value)
	}
	parsed.RawQuery = q.Encode()

	return map[string]any{"authorize_url": parsed.String()}, nil
}

func (t taskConfig) executeHTTP(ctx context.Context) (oauth2common.HTTPExchangeResult, error) {
	endpoint, form, err := t.endpointAndForm()
	if err != nil {
		return oauth2common.HTTPExchangeResult{}, err
	}
	return oauth2common.ExecuteFormRequest(ctx, http.MethodPost, endpoint, form, oauth2common.HTTPOptions{
		Headers:             t.Headers,
		TimeoutSeconds:      t.TimeoutSeconds,
		InsecureSkipVerify:  t.InsecureSkipVerify,
		ExpectedStatusCodes: t.ExpectedStatusCodes,
	})
}

func (t taskConfig) endpointAndForm() (string, url.Values, error) {
	scope, _ := oauth2common.ScopeValue(t.Scopes)
	v := url.Values{}
	switch t.Operation {
	case "EXCHANGE_CODE":
		v.Set("grant_type", "authorization_code")
		v.Set("client_id", t.ClientID)
		v.Set("redirect_uri", t.RedirectURI)
		v.Set("code", t.Code)
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		if t.PKCE.Enabled {
			v.Set("code_verifier", t.PKCE.Verifier)
		}
		return t.TokenURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "REFRESH_TOKEN":
		v.Set("grant_type", "refresh_token")
		v.Set("client_id", t.ClientID)
		v.Set("refresh_token", t.RefreshToken)
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		if scope != "" {
			v.Set("scope", scope)
		}
		return t.TokenURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "DEVICE_CODE":
		v.Set("client_id", t.ClientID)
		v.Set("scope", scope)
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		if t.Audience != "" {
			v.Set("audience", t.Audience)
		}
		if t.Resource != "" {
			v.Set("resource", t.Resource)
		}
		return t.DeviceURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "DEVICE_TOKEN":
		v.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		v.Set("client_id", t.ClientID)
		v.Set("device_code", t.DeviceCode)
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		return t.TokenURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "CLIENT_CREDENTIALS":
		v.Set("grant_type", "client_credentials")
		v.Set("client_id", t.ClientID)
		v.Set("client_secret", t.ClientSecret)
		if scope != "" {
			v.Set("scope", scope)
		}
		if t.Audience != "" {
			v.Set("audience", t.Audience)
		}
		if t.Resource != "" {
			v.Set("resource", t.Resource)
		}
		return t.TokenURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "PASSWORD":
		v.Set("grant_type", "password")
		v.Set("client_id", t.ClientID)
		v.Set("username", t.Username)
		v.Set("password", t.Password)
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		if scope != "" {
			v.Set("scope", scope)
		}
		return t.TokenURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "INTROSPECT":
		v.Set("token", t.Token)
		if t.ClientID != "" {
			v.Set("client_id", t.ClientID)
		}
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		return t.IntrospectURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	case "REVOKE":
		v.Set("token", t.Token)
		if t.ClientID != "" {
			v.Set("client_id", t.ClientID)
		}
		if t.ClientSecret != "" {
			v.Set("client_secret", t.ClientSecret)
		}
		return t.RevokeURL, oauth2common.WithExtras(v, t.ExtraParams), nil
	default:
		return "", nil, fmt.Errorf("oauth2: unsupported operation %q", t.Operation)
	}
}
