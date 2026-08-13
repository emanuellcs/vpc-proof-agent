# Changelog

All notable changes to the VPC Proof Agent are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Time zone:** All dates in this changelog follow UTC-0 (Greenwich Meridian) for the global community.

## [Unreleased]

## [1.0.0] - 2026-08-13

The initial production release of the VPC Proof Agent: a diagnostic and evidence-gathering tool that validates a manually provisioned AWS networking environment on an EC2 instance.

### Added

- **Configuration engine** (`internal/config`): multi-source loading with a strict precedence order (flags > environment variables prefixed `VPC_PROOF_` > YAML file > defaults), a YAML-aware duration type, and consolidated validation with field-prefixed error messages.
- **Structured logging** (`internal/observability`): JSON and console formats on a hardened logger, child loggers with context fields, and automatic redaction of sensitive keys (tokens, passwords, credentials).
- **IMDSv2 metadata client** (`pkg/metadata`): strict token handshake with cached session-token refresh, configurable endpoint, and context-aware requests.
- **Network utilities** (`pkg/cidr`, `pkg/netutil`): CIDR math on `net/netip`, a pure `/proc/net/route` parser, injectable route-table and interface abstractions, DNS resolution, and retrying HTTP helpers.
- **Probe engine** (`internal/probe`): nine probes covering IMDSv2 metadata (`metadata`), VPC ownership (`vpc_ownership`), subnet ownership (`subnet_ownership`), default route (`default_route`), DNS resolution (`dns`), outbound HTTPS to the internet (`internet_https`), public-IP consistency (`public_ip_consistency`), system resources (`system_resources`), and clock skew (`clock_skew`); a runner with per-probe and global timeouts.
- **Diagnostic engine** (`internal/diagnostic`): a rule-based matrix that translates probe failures into actionable AWS troubleshooting hints.
- **Report engine** (`internal/report`): JSON, Markdown, and plain-text evidence reports with a SHA-256 integrity hash over the canonical representation.
- **REST API** (`internal/api`): a versioned v1 API built on `net/http` method routing, with request IDs, recovery, security headers, request logging, bearer-token authentication, per-IP rate limiting, and a cached probe report.
- **History tracking** (`internal/history`): a thread-safe capped store of run summaries with optional atomic disk persistence.
- **Transparency endpoints**: configuration exposure with token redaction and an embedded OpenAPI 3.0 specification.
- **CLI suite** (`internal/cli`): `status`, `check` (CI/CD gateway exit codes), `diagnose`, `report`, `watch`, `echo-client`, `completions`, `serve`, and `validate-config`.
- **Metrics**: dependency-free Prometheus-compatible exposition (request counters, latency histogram, probe-status gauges).
- **Optional TLS**: HTTPS serving when a certificate and key are configured.
- **End-to-end tests** (`test/e2e`): exercises the compiled binary against mock IMDS and echo servers, asserting CLI exit codes, report integrity, and the HTTP server lifecycle with graceful shutdown.
- **Tooling and operations**: a Makefile (build, test, lint, e2e, LocalStack targets), strict golangci-lint configuration, GitHub Actions CI, a GoReleaser release pipeline that cross-compiles static binaries for Amazon Linux 2023 (`linux/amd64` and `linux/arm64`) and publishes GitHub Releases, an idempotent LocalStack provisioning script, and a hardened systemd unit.

### Fixed

- **systemd unit** (`deploy/systemd/vpc-proof.service`): removed `ProcSubset=pid`, which hid the host-wide `/proc` files required by the `default_route` and `system_resources` probes.

[Unreleased]: https://github.com/emanuellcs/vpc-proof-agent/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/emanuellcs/vpc-proof-agent/releases/tag/v1.0.0
