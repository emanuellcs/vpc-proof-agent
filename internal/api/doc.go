// Package api implements the public, versioned REST API (v1) for the
// vpc-proof agent.
//
// It owns the HTTP server, routing, request handlers, and the middleware
// stack (request IDs, structured logging, token authentication, rate
// limiting, and Prometheus metrics). Handlers delegate to the probe,
// diagnostic, and report packages and never access AWS credentials.
package api
