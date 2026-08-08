package api

import _ "embed"

// openAPIDocument is the embedded OpenAPI 3.0 specification describing every
// endpoint of the API. It is served at GET /api/v1/openapi.json.
//
//go:embed openapi.json
var openAPIDocument []byte
