# Contributing to VPC Proof Agent

Thanks for your interest in contributing. This guide covers code style, testing, and commit conventions.

## Development Setup

1. Install Go 1.26+ and [golangci-lint](https://golangci-lint.run/) v2.x.
2. (Optional) Start LocalStack and provision the lab: `make localstack-setup`.
3. Verify the toolchain works: `make build && make lint && make test`.

## Code Style

- Run `make fmt` before committing; the project uses **gofumpt** and **goimports**.
- Run `make lint` and keep the output clean (golangci-lint v2, configured in `.golangci.yml`).
- Follow [Effective Go](https://go.dev/doc/effective_go) and idiomatic Go conventions.
- **No comments unless they add real value** for readers/maintainers. Prefer self-documenting code; use doc comments on exported identifiers.
- Errors are always handled; never swallow them. Prefer wrapping with context: `fmt.Errorf("...: %w", err)`.
- Keep functions small and focused; name things clearly and unambiguously.

## Testing

- Every new behavior must be covered by unit tests (`*_test.go` next to the code).
- Run the full suite with the race detector: `make test`.
- Use table-driven tests for logic-heavy functions (CIDR math, diagnostics, config validation).
- Network-dependent code (IMDSv2, probes) must be designed against interfaces so tests can inject fakes.

## Architecture Rules

- Business logic lives in `internal/`; reusable low-level helpers live in `pkg/`.
- Dependencies flow downward: `cmd` → `internal` → `pkg`. No upward or circular imports.
- Keep AWS interactions isolated behind adapters in `pkg/`; `internal` modules stay SDK-agnostic.
- Never introduce logic that provisions AWS resources; the agent only validates.

## Commit Conventions

The repository uses [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>[optional scope]: <description>

[optional body]

[optional footer]
```

Common types:

- `feat:` new feature (e.g., `feat(probe): add DNS resolution probe`)
- `fix:` bug fix
- `docs:` documentation only
- `chore:` tooling, dependencies, scaffolding
- `refactor:` code change that neither fixes a bug nor adds a feature
- `test:` adding or correcting tests
- `build:` build system or external dependency changes

Examples:

```
feat(api): add readiness endpoint
fix(cli): resolve report path relative to cwd
docs: expand architecture data-flow diagrams
```

### Pull Request Workflow

1. Create a branch from `main` (`feat/<short-description>` or `fix/<short-description>`).
2. Implement the change with tests.
3. Run `make fmt lint test` and confirm a clean result.
4. Open a PR with a summary and reference to any related issue.
5. Request review; the CI workflow runs lint, build, and tests automatically.
