# Development

## Prerequisites

- Go 1.25+

No other tools required. The project uses only the Go standard library plus [cobra](https://github.com/spf13/cobra) for CLI.

## Make Targets

```bash
make build           # Build binary to ./bin/xcbackup
make test            # Unit and integration tests with -race
make test-contract   # Contract tests against a live F5 XC tenant
make clean           # Remove ./bin/
```

## Running Tests

### Unit and Integration Tests

```bash
make test
```

The test suite has two layers that run together:

- **Unit tests** — per-package tests with ad-hoc httptest servers. Cover sanitize, refs, diff algorithm, registry, manifest, and client logic.
- **Integration tests** — full workflow tests against a stateful `FakeXCServer` (in-memory httptest server that mimics the F5 XC REST API). Cover multi-step sequences: seed data → run workflow → verify server state. Files named `*_integration_test.go` in the backup, restore, diff, and revert packages.

Both layers are fast (seconds) with no external dependencies.

### Contract Tests

Contract tests run against a real F5 XC tenant to verify API paths, response shapes, and error codes. They're behind `//go:build contract` and excluded from normal test runs.

```bash
export XC_TENANT_URL=https://your-tenant.console.ves.volterra.io
export XC_API_TOKEN=your-api-token
export XC_TEST_NAMESPACE=your-xc-namespace   # optional, defaults to "backup-test"

make test-contract
```

### FakeXCServer

The integration tests use a shared fake at `internal/client/testutil/fakeserver.go`. It provides:

- In-memory CRUD store keyed by `resource/namespace/name`
- Handlers for all F5 XC REST API patterns (List, Get, Create, Replace, Delete)
- Error injection via `InjectError(method, resource, ns, name, ErrorSpec)`
- Request recording via `Requests()` for asserting on call patterns
- Cross-namespace list behavior (shared namespace objects appear in other namespace listings)
- `SeedObject()` and `SeedObjectWithSystemMetadata()` for test setup

## Project Structure

```
cmd/xcbackup/           CLI entry point (Cobra commands)
internal/
  backup/               Backup orchestration
  restore/              Restore orchestration with tier ordering
  diff/                 Drift detection with unified diff output
  revert/               Smart rollback (replace, recreate, delete)
  client/               F5 XC REST API client (token + mTLS auth)
  client/testutil/      FakeXCServer for integration testing
  registry/             Resource type registry (~99 types, 5 tiers)
  manifest/             Manifest read/write
  refs/                 Shared namespace reference detection
  inspect/              Backup inspection and reporting
  sanitize/             Field stripping for backup and restore
docs/                   User and developer documentation
```

## Releases

Binary releases are built with [GoReleaser](https://goreleaser.com/). The configuration at `.goreleaser.yml` produces binaries for Linux, macOS, and Windows on amd64 and arm64.

```bash
# Tag and release
git tag v1.0.0
goreleaser release
```
