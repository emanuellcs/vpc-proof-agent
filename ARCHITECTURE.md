# VPC Proof Agent — Architecture

This document describes the architectural approach, module responsibilities, and data flow of the VPC Proof Agent.

## Principles

The project follows **Clean Architecture** principles applied to the standard Go project layout:

1. **Separation of concerns** — each package owns a single, well-defined responsibility.
2. **Dependency direction** — `cmd` depends on `internal`; `internal` depends on `pkg` and the standard library. There are no upward dependencies, and no circular imports.
3. **Interface-driven boundaries** — the CLI, API, and probe engines communicate through small, explicit interfaces so implementations can be swapped and unit-tested in isolation.
4. **Portability** — the core logic (probe, diagnostic, report, security) is pure Go and free of AWS SDK coupling at the domain level; AWS interactions are isolated behind the `pkg/metadata` and `pkg/netutil` adapters.

## Layers

```
┌──────────────────────────────────────────────────────────────┐
│                          cmd/vpc-proof                       │
│                 entry point (Cobra CLI + HTTP server boot)   │
└───────────────────────────────┬──────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
┌───────▼───────┐       ┌───────▼───────┐       ┌───────▼───────┐
│  internal/cli │       │  internal/api │       │  internal/config
│  CLI commands │       │  REST API v1  │       │  flags/env/file
└───────┬───────┘       └───────┬───────┘       └───────────────┘
        │                       │
        └───────────┬───────────┘
                    ▼
┌──────────────────────────────────────────────────────────────┐
│                     internal/probe                          │
│        metadata + network probes + consistency checks       │
└───────────────────────┬──────────────────┬──────────────────┘
                        │                  │
                        ▼                  ▼
┌───────────────────────┴──────┐   ┌───────┴──────────────────────┐
│         internal/diagnostic  │   │        internal/report       │
│   failure -> troubleshooting │   │   JSON / Markdown / text     │
└──────────────────────────────┘   └──────────────────────────────┘
```

Cross-cutting concerns — applied everywhere:

```
internal/security        auth, rate limiting, token management
internal/observability   structured logging, request IDs, Prometheus metrics
pkg/metadata             IMDSv2 client
pkg/cidr                 CIDR math / IP ownership
pkg/netutil              routes, DNS, HTTPS connectivity helpers
```

## Module Responsibilities

| Module | Responsibility |
| --- | --- |
| `cmd/vpc-proof` | Thin entry point: wires configuration, logging, and dependencies; boots the CLI or the API. |
| `internal/cli` | Cobra command tree for administration over SSH: deep diagnostics, on-demand probing, report generation. |
| `internal/api` | Versioned HTTP API (v1): routing, handlers, middleware (request ID, auth, rate limit, metrics, logging), graceful shutdown. |
| `internal/probe` | Core engine: IMDSv2 metadata extraction, VPC/subnet CIDR ownership checks, default-route/gateway detection, DNS tests, outbound HTTPS tests, public-IP consistency against an echo service. |
| `internal/diagnostic` | Maps probe failure signals to actionable AWS troubleshooting hints (IGW attachment, route-table association, SG rules, etc.). |
| `internal/report` | Consumes probe + diagnostic results; renders evidence reports in JSON, Markdown, and plain text. |
| `internal/config` | Resolves configuration from flags, environment variables, and config files with a fixed precedence; validates before startup. |
| `internal/security` | Token generation/validation, authentication middleware, and strict per-client rate limiting. Never stores or exposes credentials. |
| `internal/observability` | Structured logging (JSON/Text), request-ID propagation, Prometheus metrics registry. |
| `pkg/cidr` | Reusable CIDR math and IP-ownership helpers. |
| `pkg/metadata` | Hardened IMDSv2 metadata client (TOKEN flow) exposing instance ID, IPs, and AZ. |
| `pkg/netutil` | Default-route/gateway detection, DNS resolution, outbound HTTPS checks, HTTP helpers. |

## Data Flow

### CLI diagnostics flow

```
user (SSH)
   └── internal/cli ──> internal/config (load)
                         internal/observability (logger)
                         internal/probe (run probes)
                                │
                                ├──> pkg/metadata (IMDSv2)
                                ├──> pkg/netutil (routes/DNS/HTTPS)
                                ├──> pkg/cidr (ownership)
                                ▼
                        internal/diagnostic (hints)
                                ▼
                        internal/report (JSON/MD/TXT)
```

### REST API flow

```
public client
   └── internal/api (v1 router)
          ├── security middleware (token auth, rate limit)
          ├── observability middleware (request ID, logging, metrics)
          ├── health / readiness / info handlers
          └── network / probe / echo handlers
                  └──> internal/probe ──> pkg/* adapters
```

## Configuration Strategy

- Precedence (highest to lowest): **flags → environment variables → config file → defaults**.
- All values validated centrally in `internal/config` before the application starts.
- Sensitive values (API token) are loaded from environment or config file and are never logged or exposed via the API.

## Security Model

- **Zero credential exposure**: the agent never reads IAM credentials from the instance role or environment for its own identity; it operates purely as a network observer.
- **API protection**: token authentication + strict rate limiting; heavy probes cached with TTL.
- **LocalStack safety**: provisioning scripts hard-pin dummy credentials and the LocalStack endpoint so they can never affect a real AWS account.

## Extension Points

- New CLI commands → add to `internal/cli`; they consume existing probe/report interfaces.
- New probes → implement the probe interface and register in `internal/probe`; diagnostics and reports pick them up automatically.
- New report formats → add a renderer in `internal/report`.
- New API endpoints → register in the v1 router in `internal/api`.
