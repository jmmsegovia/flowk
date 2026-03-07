package ui

import _ "embed"

//go:generate go run ./openapi_gen

// OpenAPISpecJSON exposes the OpenAPI contract consumed by the UI and external clients.
//
//go:embed openapi.json
var OpenAPISpecJSON []byte
