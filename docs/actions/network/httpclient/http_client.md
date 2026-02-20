# Functional Overview

`http_client.go` implements the **HTTP_REQUEST** action. It constructs and executes HTTP or HTTPS requests using parameters loaded from a flow definition, and returns a structured representation of the response so later tasks can inspect status codes, headers, and bodies.

# Technical Implementation Details

* **Configuration structures:**
  * `RequestConfig` gathers protocol, method, URL, headers, payload, TLS files, basic authentication credentials, timeout, TLS verification options, and the list of accepted HTTP status codes (defaulting to 200/201 when omitted).
  * `Request` stores the normalized request metadata that will be persisted in logs (method, URL, headers, body, and masked basic authentication fields).
  * `Response` captures the response status code, textual status, headers, and decoded body.
  * `Result` groups the sanitised `Request` and `Response` so execution logs expose both sides of the exchange.
  * `Logger` defines a minimal `Printf` interface, keeping the action agnostic of concrete logging implementations.
  * Task payloads may contain `${var}` placeholders; these are resolved by the application runner before the config reaches the action so the HTTP client always receives concrete values.
* **Input validation:**
  * Only `http` and `https` protocols are accepted. Methods are restricted to GET, POST, PUT, and DELETE.
  * URLs are parsed with `net/url`. If the scheme is missing, it is filled from the protocol; mismatches between the two cause an error. Hosts must be present. Note that `api.example.com/path` is treated as a path (host empty). Use `https://api.example.com/path` or `//api.example.com/path` when you rely on the `protocol` field.
* **TLS support:**
  * When using HTTPS, `buildTLSConfig` constructs a `*tls.Config` honouring the `InsecureSkipVerify` flag.
  * `loadCACert` loads custom root CA files into a new `x509.CertPool`.
  * `loadClientCertificate` supports both PEM key pairs (`cert` + `key`) and password-protected PKCS#12 bundles using `go-pkcs12`.
* **HTTP client assembly:**
  * A custom `http.Transport` is attached when TLS settings are supplied.
  * The `http.Client` respects the configured timeout. Zero values fall back to the default.
  * Requests are created with `http.NewRequestWithContext`, ensuring they cancel with the parent context.
  * Headers are normalised by trimming spaces around names before calling `req.Header.Set`.
  * Basic authentication is configured via `req.SetBasicAuth` when credentials are provided.
* **Execution and logging:** The logger records the method and redacted URL. Errors during request creation, execution, or body reading wrap the underlying issue with context-specific prefixes (`"http request: ..."`).
* **Response processing:**
  * The body is read fully with `io.ReadAll` and passed to `decodeBody` which attempts to decode JSON into Go types (maps, slices, primitives). Non-JSON responses are returned as plain strings, and empty responses yield an empty string.
  * Headers are flattened into a `map[string]string` by joining multi-value entries with ", ".
  * Request headers are sanitised through `sanitizeRequestHeaders`, masking sensitive values such as `Authorization`, `Cookie`, and `Proxy-Authorization` before persisting them.
  * Once the response is assembled, the status code is validated against the accepted list. Codes outside the list trigger an error so the task is marked as failed.
  * The final result is a `Result` struct and the declared type `flow.ResultTypeJSON` so downstream steps know they can inspect structured data.
* **Utility helpers:**
  * `bytesReader` returns `nil` when the body is empty, letting `http.NewRequestWithContext` use `nil` for GET requests.
  * `normalize` helper functions convert optional legacy formats, while TLS helpers encapsulate file IO and parsing logic.
