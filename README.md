# VPC Proof Agent

A comprehensive **diagnostic and evidence-gathering tool** written in Go that technically validates and proves that a manually provisioned AWS networking environment is functioning correctly.

The agent runs on an Amazon EC2 instance (Amazon Linux 2) and validates a target environment that consists of a VPC (`10.0.0.0/16`), a public subnet (`10.0.1.0/24` with auto-assign public IP), an Internet Gateway, a Route Table with a default route to the IGW, a Security Group, and an EC2 instance (`t2.micro`).

> **The agent does NOT provision AWS resources.** It assumes the environment exists and validates it, producing reproducible evidence.

- **CLI** — administration, deep diagnostics, and report generation, used locally over SSH.
- **REST API (v1)** — publicly exposed to prove internet reachability, external routing, and Security Group configuration.

---

## Table of Contents

- [Vision](#vision)
- [Features](#features)
- [Target AWS Environment](#target-aws-environment)
- [Architecture](#architecture)
- [Repository Layout](#repository-layout)
- [Requirements](#requirements)
- [Getting Started](#getting-started)
  - [1. Start LocalStack](#1-start-localstack)
  - [2. Provision the Lab](#2-provision-the-lab)
  - [3. Build and Test](#3-build-and-test)
- [Tooling](#tooling)
- [Security](#security)
- [Documentation](#documentation)
- [License](#license)

---

## Vision

The VPC Proof Agent was created to answer one question with hard evidence:

> *"Is this AWS networking lab really wired up correctly?"*

It fetches EC2 metadata (via IMDSv2), runs network probes, cross-checks the AWS-reported public IP against an external echo service, translates any failure into actionable AWS troubleshooting hints, and renders the whole picture into JSON, Markdown, or plain-text reports.

Read more in [ARCHITECTURE.md](./ARCHITECTURE.md). A Portuguese version of this README, tailored for academic evaluation, is available in [README_pt-BR.md](./README_pt-BR.md).

## Features

| Area | Capability |
| --- | --- |
| Metadata | Secure EC2 metadata extraction using IMDSv2 (Instance ID, Private/Public IP, AZ) |
| Network probes | IP ownership (VPC/Subnet CIDR matching), default-route/gateway detection, DNS resolution, outbound HTTPS connectivity |
| Consistency | Cross-check of AWS-reported public IP against an external echo service |
| Diagnostics | Probe failures translated into actionable AWS troubleshooting hints |
| Reporting | Evidence reports in JSON, Markdown, and plain text |
| REST API | Versioned `v1` API: health, readiness, info, network details, active probing, external request echo, Prometheus metrics |
| Security | Token-based API authentication, strict rate limiting, caching of heavy probes, zero exposure of credentials |
| Observability | Structured logging (JSON/Text), request IDs, Prometheus-compatible metrics |
| Operations | systemd service definition, graceful shutdown, config via flags/env/files |

> Status: the repository is currently at **Commit 4**. The configuration
> system, structured logging, the full CLI (status/check/diagnose/report), the
> probe engine, the diagnostic rule matrix, and the report engine are
> implemented. The REST API is intentionally not implemented yet.

## Target AWS Environment

The environment that the agent validates:

```
VPC           10.0.0.0/16
├── Subnet         10.0.1.0/24  (auto-assign public IP enabled)
├── Internet GW     igw attached to the VPC
├── Route Table     0.0.0.0/0 -> IGW, associated with the subnet
├── Security Group  SSH (22) from a specific IP; HTTP (8080) for the API
└── EC2 instance    t2.micro running the agent
```

## Architecture

The project follows **Clean Architecture** principles with the standard Go project layout:

```
cmd/       entry points            (thin)
internal/  application modules     (business logic)
pkg/       shared, reusable libs   (CIDR math, IMDSv2 client, net utils)
deploy/    systemd units & scripts
scripts/   automation (LocalStack provisioning)
docs/      extended documentation
```

Internal modules and their responsibilities are described in detail in [ARCHITECTURE.md](./ARCHITECTURE.md).

## Repository Layout

```
.
├── cmd/vpc-proof/            # main entry point
├── internal/
│   ├── cli/                  # Cobra CLI command definitions
│   ├── api/                  # HTTP server, routers, handlers, middleware
│   ├── probe/                # core network & metadata validation
│   ├── diagnostic/           # troubleshooting-hint engine
│   ├── report/               # report generation & formatting
│   ├── config/               # config loading & validation
│   ├── security/             # auth, rate limiting, token management
│   └── observability/        # logging & metrics
├── pkg/
│   ├── cidr/                 # CIDR math
│   ├── metadata/             # AWS metadata (IMDSv2) client
│   └── netutil/              # network utilities
├── deploy/
│   └── systemd/              # vpc-proof.service unit
├── scripts/                  # LocalStack setup & teardown
└── docs/                     # extended documentation
```

## Requirements

- Go **1.26** or newer
- [LocalStack](https://docs.localstack.cloud/) running locally (port `4566`)
- AWS CLI v2 (`aws`)
- GNU Make
- [golangci-lint](https://golangci-lint.run/) **v2.x** (for `make lint`)

## Getting Started

### 1. Start LocalStack

```bash
lstk start   # or: docker run ... localstack/localstack
```

### 2. Provision the Lab

The script provisions the exact AWS lab inside LocalStack. It is **idempotent**, so it can be run repeatedly.

```bash
make localstack-setup
```

It creates the VPC, subnet, Internet Gateway, Route Table (default route to the IGW), and Security Group (SSH/22 + HTTP/8080), prints the generated resource IDs, and writes them to `scripts/.localstack-resources.env`.

To clean up afterwards:

```bash
make localstack-teardown
```

> **Safety:** both scripts force LocalStack-only credentials and endpoints, so they can never touch a real AWS account — even if real credentials exist on the machine.

### 3. Build and Test

```bash
make build          # builds bin/vpc-proof
make test           # go test -race ./...
make lint           # golangci-lint run
make fmt            # gofumpt + goimports formatting
make run            # runs the scaffold binary
```

## Tooling

| Command | Description |
| --- | --- |
| `make help` | List all targets |
| `make build` | Build the binary into `bin/vpc-proof` |
| `make run` | Run the scaffold binary |
| `make test` | Run tests with the race detector |
| `make vet` | Run `go vet` |
| `make fmt` | Format code (gofumpt + goimports) |
| `make lint` | Run golangci-lint |
| `make tidy` | Tidy modules |
| `make tools` | Install development tools (mockgen) |
| `make mocks` | Generate mocks (`go generate ./...`) |
| `make run-status` | Quick status against the current environment (graceful under LocalStack) |
| `make run-check` | Full probe suite; the exit code doubles as a CI gateway |
| `make run-report` | Generate a Markdown evidence report to stdout |
| `make localstack-setup` | Provision the lab in LocalStack |
| `make localstack-teardown` | Tear down the lab |
| `make clean` | Remove build artifacts |

## Configuration

The agent is configured through up to four sources, applied in precedence
order (highest first):

1. **Command-line flags** — `--config`, `--log-level`, `--log-format`.
2. **Environment variables** — prefixed with `VPC_PROOF_` (see
   [.env.example](./.env.example)).
3. **A YAML config file** — passed via `--config`, the `VPC_PROOF_CONFIG`
   environment variable, or discovered at `./vpc-proof.yaml`,
   `$XDG_CONFIG_HOME/vpc-proof/config.yaml`, and
   `/etc/vpc-proof/config.yaml`. See [config.example.yaml](./config.example.yaml).
4. **Built-in defaults**.

Validate a configuration without running anything:

```bash
vpc-proof validate-config
vpc-proof validate-config --config config.example.yaml
```

`validate-config` prints a success message or a detailed list of errors, each
prefixed with the offending field (for example `server.port: must be between
1 and 65535, got 70000`).

## CLI Commands

| Command | Description |
| --- | --- |
| `vpc-proof version` | Print version, commit, build date, Go version, and platform |
| `vpc-proof status` | Quick summary of the instance (metadata + default route) |
| `vpc-proof check` | Run the full probe suite; CI/CD gateway exit code |
| `vpc-proof diagnose` | Run probes and output troubleshooting hints |
| `vpc-proof report` | Generate an evidence report; `--format json|markdown|text`, `--output <path|->` |
| `vpc-proof serve` | Start the REST API; `--addr`, `--port` (stub) |
| `vpc-proof validate-config` | Load and validate the configuration |

The root command loads and validates the configuration, initializes structured
logging (JSON or console), and builds the application container (metadata
client, probe runner, diagnostic engine) before any subcommand runs; failures
abort with a non-zero exit code.

## Reports & Exit Codes

### Reports

`vpc-proof report` runs the probe suite, derives troubleshooting hints, and
renders a professional evidence document in three formats:

```bash
vpc-proof report --format json                                # machine-readable
vpc-proof report --format markdown --output evidence.md       # for the academic PDF
vpc-proof report --format text                                # console-friendly
```

The Markdown report contains sections for instance metadata, a network
summary, aggregated results, a probe-results table, and the diagnostic hints,
so it can be pasted directly into a report or PDF. When a field cannot be
retrieved (for example against LocalStack, which has no real EC2 metadata) it
is rendered as `N/A` instead of failing.

### Exit codes

`vpc-proof check` is designed for CI/CD pipelines and shell scripts. It maps
the overall probe status to the process exit code:

| Exit code | Meaning |
| --- | --- |
| `0` | Overall status is **pass** or **skip** |
| `1` | At least one probe **failed** |
| `2` | No failures, but at least one probe **warned** |

`status` and `report` always exit `0` on success (they are informational),
while `validate-config` exits non-zero when the configuration is invalid.

## Security

- The agent **never** provisions resources and **never** reads or exposes AWS credentials.
- The REST API is protected by token-based authentication and strict rate limiting.
- Heavy probes are cached; sensitive data is excluded from logs, responses, and reports.
- LocalStack scripts pin dummy credentials and a dedicated endpoint as a hard safety guarantee against accidental access to real AWS infrastructure.

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) — Clean Architecture, module responsibilities, data flow.
- [CONTRIBUTING.md](./CONTRIBUTING.md) — code style, testing, commit conventions.
- [README_pt-BR.md](./README_pt-BR.md) — Portuguese, tailored for academic evaluators.
- [docs/](./docs/) — extended documentation index.

## License

This project is licensed under the [MIT License](./LICENSE).
