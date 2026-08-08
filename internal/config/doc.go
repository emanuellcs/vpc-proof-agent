// Package config centralizes configuration loading and validation for
// the vpc-proof agent.
//
// Configuration is resolved from command-line flags, environment
// variables, and configuration files (in that precedence order), then
// validated before the application starts. The loader and typed options
// are introduced in a later commit.
package config
