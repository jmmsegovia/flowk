package oauth2common

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type HTTPOptions struct {
	Headers             map[string]string
	TimeoutSeconds      float64
	InsecureSkipVerify  bool
	ExpectedStatusCodes []int
}

type HTTPExchangeResult struct {
	Request  HTTPRequest  `json:"request"`
	Response HTTPResponse `json:"response"`
}

type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       any               `json:"body"`
}

func ScopeValue(scopes any) (string, error) {
	switch v := scopes.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(v), nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("oauth2: scopes array must contain only strings")
			}
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.Join(parts, " "), nil
	default:
		return "", fmt.Errorf("oauth2: scopes must be string or array")
	}
}

func ExecuteFormRequest(ctx context.Context, method, endpoint string, form url.Values, opts HTTPOptions) (HTTPExchangeResult, error) {
	body := form.Encode()
	result := HTTPExchangeResult{Request: HTTPRequest{Method: method, URL: endpoint, Body: RedactMap(flattenValues(form))}}
	return executeRequest(ctx, method, endpoint, strings.NewReader(body), "application/x-www-form-urlencoded", result, opts)
}

func ExecuteJSONRequest(ctx context.Context, method, endpoint string, payload any, opts HTTPOptions) (HTTPExchangeResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return HTTPExchangeResult{}, fmt.Errorf("oauth2: encode json payload: %w", err)
	}
	result := HTTPExchangeResult{Request: HTTPRequest{Method: method, URL: endpoint, Body: RedactAny(payload)}}
	return executeRequest(ctx, method, endpoint, strings.NewReader(string(data)), "application/json", result, opts)
}

func executeRequest(ctx context.Context, method, endpoint string, body io.Reader, contentType string, result HTTPExchangeResult, opts HTTPOptions) (HTTPExchangeResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return result, fmt.Errorf("oauth2: create request: %w", err)
	}

	headers := map[string]string{"Content-Type": contentType, "Accept": "application/json"}
	for k, v := range opts.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headers[k] = v
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	result.Request.Headers = sanitizeHeaders(req.Header)

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerify} //nolint:gosec
	}
	client := &http.Client{Transport: transport}
	if opts.TimeoutSeconds > 0 {
		client.Timeout = time.Duration(opts.TimeoutSeconds * float64(time.Second))
	}

	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("oauth2: perform request: %w", err)
	}
	defer resp.Body.Close()

	result.Response.StatusCode = resp.StatusCode
	result.Response.Status = resp.Status
	result.Response.Headers = flattenHeader(resp.Header)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return result, fmt.Errorf("oauth2: read response body: %w", err)
	}
	result.Response.Body = RedactAny(decodeBody(bodyBytes))

	codes := opts.ExpectedStatusCodes
	if len(codes) == 0 {
		codes = []int{http.StatusOK}
	}
	for _, code := range codes {
		if resp.StatusCode == code {
			return result, nil
		}
	}
	return result, fmt.Errorf("oauth2: unexpected status code %d (expected: %v)", resp.StatusCode, codes)
}

var secretKeys = map[string]struct{}{
	"client_secret": {}, "password": {}, "refresh_token": {}, "code": {}, "device_code": {}, "token": {}, "access_token": {}, "id_token": {}, "authorization": {},
}

func RedactMap(values map[string]string) map[string]string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(values))
	for _, key := range keys {
		out[key] = redactByKey(key, values[key])
	}
	return out
}

func RedactAny(input any) any {
	switch v := input.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			if _, ok := secretKeys[strings.ToLower(key)]; ok {
				result[key] = "<secret>"
				continue
			}
			result[key] = RedactAny(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = RedactAny(item)
		}
		return result
	default:
		return v
	}
}

func WithExtras(values url.Values, extra map[string]string) url.Values {
	for key, value := range extra {
		if strings.TrimSpace(key) == "" {
			continue
		}
		values.Set(key, value)
	}
	return values
}

func flattenValues(values url.Values) map[string]string {
	out := make(map[string]string, len(values))
	for key, list := range values {
		out[key] = strings.Join(list, " ")
	}
	return out
}

func flattenHeader(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, list := range headers {
		out[key] = strings.Join(list, ", ")
	}
	return out
}

func sanitizeHeaders(headers http.Header) map[string]string {
	out := make(map[string]string, len(headers))
	for key, list := range headers {
		value := strings.Join(list, ", ")
		switch strings.ToLower(key) {
		case "authorization", "proxy-authorization", "cookie", "set-cookie":
			out[key] = "<secret>"
		default:
			out[key] = value
		}
	}
	return out
}

func decodeBody(data []byte) any {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}
	return string(data)
}

func redactByKey(key, value string) string {
	if _, ok := secretKeys[strings.ToLower(strings.TrimSpace(key))]; ok {
		return "<secret>"
	}
	return value
}
