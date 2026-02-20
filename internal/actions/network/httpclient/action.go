package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"flowk/internal/actions/registry"
	"flowk/internal/shared/expansion"
)

type taskConfig struct {
	Protocol                 string            `json:"protocol"`
	Method                   string            `json:"method"`
	URL                      string            `json:"url"`
	Headers                  map[string]string `json:"headers"`
	Body                     string            `json:"body"`
	BodyFile                 string            `json:"body_file"`
	BodyFileLegacy           string            `json:"bodyFile"`
	CACert                   string            `json:"cacert"`
	Cert                     string            `json:"cert"`
	CertPassword             string            `json:"cert_password"`
	CertPasswordLegacy       string            `json:"certPassword"`
	Key                      string            `json:"key"`
	User                     string            `json:"user"`
	Password                 string            `json:"password"`
	TimeoutSeconds           *float64          `json:"timeout_seconds"`
	TimeoutSecondsAlt        *float64          `json:"timeoutSeconds"`
	InsecureSkipVerify       bool              `json:"insecure_skip_verify"`
	InsecureSkipVerifyLegacy bool              `json:"insecureSkipVerify"`
	AcceptedStatusCodes      []int             `json:"accepted_status_codes"`
}

func (c *taskConfig) Validate() error {
	if strings.TrimSpace(c.Protocol) == "" {
		return fmt.Errorf("http task: protocol is required")
	}
	if strings.TrimSpace(c.Method) == "" {
		return fmt.Errorf("http task: method is required")
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("http task: url is required")
	}
	if strings.TrimSpace(c.Body) != "" && strings.TrimSpace(c.BodyFile) != "" {
		return fmt.Errorf("http task: body and body_file cannot be used together")
	}
	if c.TimeoutSeconds != nil && *c.TimeoutSeconds < 0 {
		return fmt.Errorf("http task: timeout_seconds must be non-negative")
	}
	return nil
}

func decodeTask(data json.RawMessage, vars map[string]expansion.Variable) (RequestConfig, error) {
	var cfg taskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return RequestConfig{}, fmt.Errorf("decoding http task payload: %w", err)
	}

	if cfg.BodyFile == "" && cfg.BodyFileLegacy != "" {
		cfg.BodyFile = cfg.BodyFileLegacy
	}
	if cfg.CertPassword == "" && cfg.CertPasswordLegacy != "" {
		cfg.CertPassword = cfg.CertPasswordLegacy
	}
	if cfg.TimeoutSeconds == nil && cfg.TimeoutSecondsAlt != nil {
		cfg.TimeoutSeconds = cfg.TimeoutSecondsAlt
	}
	if !cfg.InsecureSkipVerify && cfg.InsecureSkipVerifyLegacy {
		cfg.InsecureSkipVerify = true
	}

	if err := cfg.Validate(); err != nil {
		return RequestConfig{}, err
	}

	var body []byte
	switch {
	case strings.TrimSpace(cfg.BodyFile) != "":
		data, err := os.ReadFile(cfg.BodyFile)
		if err != nil {
			return RequestConfig{}, fmt.Errorf("http task: reading body_file %q: %w", cfg.BodyFile, err)
		}

		expanded, err := expansion.ExpandString(string(data), vars)
		if err != nil {
			return RequestConfig{}, fmt.Errorf("http task: expanding body_file %q: %w", cfg.BodyFile, err)
		}

		body = []byte(expanded)
	case strings.TrimSpace(cfg.Body) != "":
		bodyValue := cfg.Body
		trimmedBody := strings.TrimSpace(bodyValue)
		if strings.HasPrefix(trimmedBody, "@") {
			bodyPath := strings.TrimSpace(strings.TrimPrefix(trimmedBody, "@"))
			if bodyPath == "" {
				return RequestConfig{}, fmt.Errorf("http task: body path is empty")
			}

			data, err := os.ReadFile(bodyPath)
			if err != nil {
				return RequestConfig{}, fmt.Errorf("http task: reading body %q: %w", bodyPath, err)
			}

			expanded, err := expansion.ExpandString(string(data), vars)
			if err != nil {
				return RequestConfig{}, fmt.Errorf("http task: expanding body %q: %w", bodyPath, err)
			}

			body = []byte(expanded)
		} else {
			body = []byte(bodyValue)
		}
	}

	var timeout time.Duration
	if cfg.TimeoutSeconds != nil {
		seconds := *cfg.TimeoutSeconds
		if seconds < 0 {
			return RequestConfig{}, fmt.Errorf("http task: timeout_seconds must be non-negative")
		}
		timeout = time.Duration(seconds * float64(time.Second))
	}

	return RequestConfig{
		Protocol:            cfg.Protocol,
		Method:              cfg.Method,
		URL:                 cfg.URL,
		Headers:             cfg.Headers,
		Body:                body,
		CACertPath:          cfg.CACert,
		ClientCertPath:      cfg.Cert,
		ClientKeyPath:       cfg.Key,
		ClientCertPassword:  cfg.CertPassword,
		BasicAuthUser:       cfg.User,
		BasicAuthPassword:   cfg.Password,
		Timeout:             timeout,
		InsecureSkipVerify:  cfg.InsecureSkipVerify,
		AcceptedStatusCodes: cfg.AcceptedStatusCodes,
	}, nil
}

type action struct{}

func init() {
	registry.Register(action{})
}

func (action) Name() string {
	return ActionName
}

func (action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	cfg, err := decodeTask(payload, cloneVariables(execCtx))
	if err != nil {
		return registry.Result{}, err
	}

	value, resultType, err := Execute(ctx, cfg, execCtx.Logger)
	if err != nil {
		return registry.Result{}, err
	}
	return registry.Result{Value: value, Type: resultType}, nil
}

func cloneVariables(execCtx *registry.ExecutionContext) map[string]expansion.Variable {
	if execCtx == nil || execCtx.Variables == nil {
		return nil
	}

	vars := make(map[string]expansion.Variable, len(execCtx.Variables))
	for name, variable := range execCtx.Variables {
		vars[name] = expansion.Variable{
			Name:   variable.Name,
			Type:   variable.Type,
			Value:  variable.Value,
			Secret: variable.Secret,
		}
	}
	return vars
}
