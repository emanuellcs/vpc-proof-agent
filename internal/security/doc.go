// Package security implements authentication, rate limiting, and token
// management for the vpc-proof agent.
//
// It enforces token-based API authentication and strict per-client rate
// limiting. The agent never stores or exposes IAM credentials or
// environment secrets; this package guarantees zero leakage of sensitive
// data through logs, API responses, or reports.
package security
