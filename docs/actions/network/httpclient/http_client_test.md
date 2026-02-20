# Functional Overview

`http_client_test.go` exercises the HTTP_REQUEST action end-to-end. It spins up in-memory HTTP/HTTPS servers to verify that requests are constructed properly, responses are decoded, TLS settings are honoured, and validation failures produce errors.

# Technical Implementation Details

* **Recording logger:** A lightweight `recordingLogger` collects formatted messages so tests can assert that the action logs the HTTP verb and redacted URL.
* **HTTP interactions:**
  * `TestExecuteHTTP` uses `httptest.NewServer` to assert that headers, basic authentication, and JSON bodies are transmitted, and that the returned `Result` captures both the sanitised request metadata and the decoded response payload.
  * `TestExecuteHTTPParsesJSONArray` ensures JSON arrays are decoded into `[]any` values accessible to later tasks or VARIABLES placeholders that inspect previous HTTP responses.
* **HTTPS and custom CA certificates:** `TestExecuteHTTPSWithCustomCA` provisions an HTTPS server with a self-signed certificate, writes the certificate to a temporary directory, and confirms that providing `CACertPath` enables successful TLS validation.
* **Validation checks:** Additional tests (`TestExecuteRejectsInvalidProtocol`, `TestExecuteRejectsInvalidMethod`, `TestExecuteRequiresHost`) verify that improper configuration yields immediate errors.
* **Utilities:** The tests use Go's standard `testing`, `net/http/httptest`, and temporary directory helpers to isolate network interactions and file-system state.
