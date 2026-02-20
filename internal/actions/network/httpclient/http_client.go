package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"flowk/internal/flow"
)

const (
	// ActionName identifies the HTTP client action in the flow definition.
	ActionName = "HTTP_REQUEST"
)

// Response captures the essential information returned by an HTTP request.
type Response struct {
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       any               `json:"body"`
}

// Request captures the normalized request parameters that were sent to the
// remote endpoint. Sensitive information such as Basic Authentication
// passwords and authorization headers are masked to avoid leaking secrets in
// execution logs.
type Request struct {
	Method            string            `json:"method"`
	URL               string            `json:"url"`
	Headers           map[string]string `json:"headers,omitempty"`
	Body              any               `json:"body,omitempty"`
	BasicAuthUser     string            `json:"basic_auth_user,omitempty"`
	BasicAuthPassword string            `json:"basic_auth_password,omitempty"`
}

// Result aggregates the HTTP request metadata with the server response so both
// sides of the exchange can be inspected from execution logs.
type Result struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// Logger defines the minimal interface expected from loggers used by the action.
type Logger interface {
	Printf(format string, v ...interface{})
}

// RequestConfig captures the parameters required to execute the HTTP request.
type RequestConfig struct {
	Protocol            string
	Method              string
	URL                 string
	Headers             map[string]string
	Body                []byte
	CACertPath          string
	ClientCertPath      string
	ClientKeyPath       string
	ClientCertPassword  string
	BasicAuthUser       string
	BasicAuthPassword   string
	Timeout             time.Duration
	InsecureSkipVerify  bool
	AcceptedStatusCodes []int
}

// Execute performs an HTTP request with the provided configuration. It returns the
// response payload alongside the result type so that it can be consumed by
// subsequent tasks.
func Execute(ctx context.Context, cfg RequestConfig, logger Logger) (any, flow.ResultType, error) {
	normalizedProtocol := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	if normalizedProtocol != "http" && normalizedProtocol != "https" {
		return nil, "", fmt.Errorf("http request: unsupported protocol %q", cfg.Protocol)
	}

	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		return nil, "", fmt.Errorf("http request: unsupported method %q", cfg.Method)
	}

	if strings.TrimSpace(cfg.URL) == "" {
		return nil, "", errors.New("http request: url is required")
	}

	parsedURL, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil {
		return nil, "", fmt.Errorf("http request: parsing url: %w", err)
	}

	if parsedURL.Scheme == "" {
		parsedURL.Scheme = normalizedProtocol
	} else if !strings.EqualFold(parsedURL.Scheme, normalizedProtocol) {
		return nil, "", fmt.Errorf("http request: url scheme %q does not match protocol %q", parsedURL.Scheme, cfg.Protocol)
	}

	if parsedURL.Host == "" {
		return nil, "", errors.New("http request: url must include host")
	}

	var transport *http.Transport
	if normalizedProtocol == "https" {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, "", err
		}

		transport = &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsConfig,
		}
	}

	client := &http.Client{}
	if transport != nil {
		client.Transport = transport
	}

	if cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}

	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bytesReader(cfg.Body))
	if err != nil {
		return nil, "", fmt.Errorf("http request: creating request: %w", err)
	}

	for key, value := range cfg.Headers {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		req.Header.Set(trimmedKey, value)
	}

	if cfg.BasicAuthUser != "" {
		req.SetBasicAuth(cfg.BasicAuthUser, cfg.BasicAuthPassword)
	}

	if logger != nil {
		logger.Printf("HTTP %s %s", method, parsedURL.Redacted())

		proxyResolver := http.ProxyFromEnvironment
		if transport != nil && transport.Proxy != nil {
			proxyResolver = transport.Proxy
		}

		if proxyResolver != nil {
			proxyURL, proxyErr := proxyResolver(req)
			switch {
			case proxyErr != nil:
				logger.Printf("HTTP proxy: resolution error: %v", proxyErr)
			case proxyURL != nil:
				logger.Printf("HTTP proxy: %s", proxyURL.Redacted())
			default:
				logger.Printf("HTTP proxy: not configured")
			}
		}
	}

	result := Result{
		Request: Request{
			Method:  method,
			URL:     parsedURL.String(),
			Headers: sanitizeRequestHeaders(req.Header),
		},
	}

	if len(cfg.Body) > 0 {
		result.Request.Body = decodeBody(cfg.Body)
	}

	if cfg.BasicAuthUser != "" {
		result.Request.BasicAuthUser = cfg.BasicAuthUser
		if cfg.BasicAuthPassword != "" {
			result.Request.BasicAuthPassword = "<secret>"
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		execErr := fmt.Errorf("http request: performing request: %w", err)
		logHTTPResult(logger, result, execErr)
		return nil, "", execErr
	}
	defer resp.Body.Close()

	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = strings.Join(values, ", ")
	}

	result.Response = Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		readErr := fmt.Errorf("http request: reading response body: %w", err)
		logHTTPResult(logger, result, readErr)
		return nil, "", readErr
	}

	result.Response.Body = decodeBody(body)

	acceptedCodes := cfg.AcceptedStatusCodes
	if len(acceptedCodes) == 0 {
		acceptedCodes = []int{http.StatusOK, http.StatusCreated}
	}

	for _, code := range acceptedCodes {
		if resp.StatusCode == code {
			logHTTPResult(logger, result, nil)
			return result, flow.ResultTypeJSON, nil
		}
	}

	err = fmt.Errorf(
		"http request: unexpected status code %d (accepted: %v)",
		resp.StatusCode,
		acceptedCodes,
	)
	logHTTPResult(logger, result, err)
	return result, flow.ResultTypeJSON, err
}

func logHTTPResult(logger Logger, result Result, execErr error) {
	if logger == nil {
		return
	}

	payload := map[string]any{
		"request":  result.Request,
		"response": result.Response,
	}

	if execErr != nil {
		payload["error"] = execErr.Error()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		if execErr != nil {
			logger.Printf("HTTP result: request=%+v response=%+v error=%v (marshal error: %v)", result.Request, result.Response, execErr, err)
			return
		}

		logger.Printf("HTTP result: request=%+v response=%+v (marshal error: %v)", result.Request, result.Response, err)
		return
	}

	logger.Printf("HTTP result: %s", data)
}

func decodeBody(data []byte) any {
	if len(data) == 0 {
		return ""
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ""
	}

	var parsed any
	if err := json.Unmarshal(trimmed, &parsed); err == nil {
		return parsed
	}

	return string(data)
}

func bytesReader(data []byte) io.Reader {
	if len(data) == 0 {
		return nil
	}
	return bytes.NewReader(data)
}

func sanitizeRequestHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	sanitized := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			sanitized[key] = ""
			continue
		}

		joined := strings.Join(values, ", ")
		switch strings.ToLower(key) {
		case "authorization", "proxy-authorization", "set-cookie", "cookie":
			sanitized[key] = "<secret>"
		default:
			sanitized[key] = joined
		}
	}

	if len(sanitized) == 0 {
		return nil
	}

	return sanitized
}

func buildTLSConfig(cfg RequestConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec

	if strings.TrimSpace(cfg.CACertPath) != "" {
		caPool, err := loadCACert(cfg.CACertPath)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = caPool
	}

	if strings.TrimSpace(cfg.ClientCertPath) != "" {
		certificate, err := loadClientCertificate(cfg)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	return tlsConfig, nil
}

func loadCACert(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("http request: reading CA certificate %q: %w", path, err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(data); ok {
		return pool, nil
	}

	return nil, fmt.Errorf("http request: parsing CA certificate %q: failed to append certificate", path)
}

func loadClientCertificate(cfg RequestConfig) (tls.Certificate, error) {
	certPath := strings.TrimSpace(cfg.ClientCertPath)
	keyPath := strings.TrimSpace(cfg.ClientKeyPath)

	if keyPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("http request: loading client certificate %q and key %q: %w", certPath, keyPath, err)
		}
		return cert, nil
	}

	if strings.TrimSpace(cfg.ClientCertPassword) == "" {
		return tls.Certificate{}, fmt.Errorf("http request: client certificate password is required when key is not provided")
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("http request: reading client certificate %q: %w", certPath, err)
	}

	privateKey, cert, caCerts, err := pkcs12.DecodeChain(data, cfg.ClientCertPassword)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("http request: decoding PKCS#12 certificate %q: %w", certPath, err)
	}

	certificate := tls.Certificate{
		Certificate: make([][]byte, 0, 1+len(caCerts)),
		PrivateKey:  privateKey,
		Leaf:        cert,
	}
	certificate.Certificate = append(certificate.Certificate, cert.Raw)
	for _, ca := range caCerts {
		certificate.Certificate = append(certificate.Certificate, ca.Raw)
	}

	return certificate, nil
}
