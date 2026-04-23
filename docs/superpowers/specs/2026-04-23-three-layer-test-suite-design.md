# Three-Layer Test Suite Design

Adapt the three-layer testing architecture from `f5xc-k8s-operator` for `f5xc-namespace-backup`. Adds a reusable FakeXCServer, per-package integration tests, and contract tests against real F5 XC API.

## Context

The project has 57 unit tests across 10 packages with varying coverage (48%-100%). Tests use ad-hoc `httptest.NewTLSServer` handlers — no reusable fake server, no stateful workflow testing, no contract tests. The k8s operator project has a proven three-layer pattern (unit / integration with FakeXCServer / contract with real API) that maps directly to this project since both hit the same F5 XC REST API.

## Test Layers

### Layer 1: Unit Tests (existing, no changes)

Pure logic tests. Already cover sanitize (100%), refs (100%), diff algorithm (77%), registry (68%), manifest (77%). Use table-driven tests with ad-hoc httptest servers for client/workflow tests. No changes needed — these stay as-is.

### Layer 2: Integration Tests (new)

Full workflow tests against a stateful FakeXCServer. The key difference from unit tests: the fake server holds state across calls, enabling multi-step sequences (seed data -> run workflow -> verify server state). Run as part of `make test` — no build tags, no external dependencies.

### Layer 3: Contract Tests (new)

Tests against real F5 XC API behind `//go:build contract`. Verify API paths, response shapes, and error codes. Small scope — contract tests are slow and hit real infra.

## Component 1: FakeXCServer

**Location**: `internal/client/testutil/fakeserver.go`

Adapted from `f5xc-k8s-operator/internal/xcclient/testutil/fakeserver.go`. Same core design:

- In-memory object store keyed by `resource/namespace/name`
- CRUD handlers for the F5 XC REST API pattern
- Error injection via `InjectError(method, resource, ns, name, ErrorSpec)`
- Request recording via `Requests()` for asserting call patterns
- Thread-safe with `sync.Mutex`

### API Routes

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/config/namespaces/{ns}/{resource}` | List objects |
| GET | `/api/config/namespaces/{ns}/{resource}/{name}` | Get object |
| POST | `/api/config/namespaces/{ns}/{resource}` | Create object |
| PUT | `/api/config/namespaces/{ns}/{resource}/{name}` | Replace object |
| DELETE | `/api/config/namespaces/{ns}/{resource}/{name}` | Delete object |
| GET | `/api/web/namespaces` | List namespaces |

### Adaptations from k8s operator version

1. **Top-level list fields**: List response items include top-level `name` and `namespace` fields alongside the nested `metadata` fields. The backup and diff code checks `item["name"]` first, then falls back to `item["metadata"]["name"]`. The namespace-filtering logic (`item["namespace"]`) requires top-level namespace fields.

2. **SeedObject helper**: `SeedObject(resource, ns, name string, spec map[string]any)` for pre-populating server state without going through the HTTP create path. Simplifies integration test setup.

3. **Namespace list endpoint**: `GET /api/web/namespaces` returns seeded namespaces. The `namespaces` command uses this.

### Types

```go
// StoredObject is the internal representation. Not serialized directly — the
// handlers build the appropriate JSON shape for list vs. get responses.
type StoredObject struct {
    Metadata       map[string]interface{}
    SystemMetadata map[string]interface{}
    Spec           map[string]interface{}
}

type RecordedRequest struct {
    Method string
    Path   string
    Body   json.RawMessage
}

type ErrorSpec struct {
    StatusCode int
    Body       string
    Times      int // 0 = forever, >0 = count down
}
```

### Response Shapes

**List** (`GET /api/config/namespaces/{ns}/{resource}`): Returns `{"items": [...]}` where each item has top-level `name` and `namespace` fields plus `metadata`, `system_metadata`, and `spec`. Matches real API list behavior.

**Get** (`GET /api/config/namespaces/{ns}/{resource}/{name}`): Returns `{"metadata": {...}, "system_metadata": {...}, "spec": {...}}` without top-level name/namespace. Matches real API get behavior.

### Public API

```go
func NewFakeXCServer() *FakeXCServer
func (f *FakeXCServer) Close()
func (f *FakeXCServer) URL() string
func (f *FakeXCServer) SeedObject(resource, ns, name string, spec map[string]any)
func (f *FakeXCServer) InjectError(method, resource, ns, name string, spec ErrorSpec)
func (f *FakeXCServer) ClearErrors()
func (f *FakeXCServer) Requests() []RecordedRequest
func (f *FakeXCServer) ClearRequests()
```

## Component 2: Integration Tests

Per-package `*_integration_test.go` files. Each uses `testutil.NewFakeXCServer()` and `client.NewForTest()`.

### `internal/backup/backup_integration_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestIntegration_BackupFullWorkflow` | Seed multiple resource types, run backup, verify files written and manifest correct |
| `TestIntegration_BackupErrorHandling` | Inject 403/404/501 errors, verify warnings vs. skips (not hard failures) |
| `TestIntegration_BackupAuthFailure` | Inject 401, verify auth error returned |
| `TestIntegration_BackupNamespaceFiltering` | Seed objects in target + shared namespaces, verify only target backed up |
| `TestIntegration_BackupViewOwnedFiltering` | Seed objects with `system_metadata.owner_view`, verify skipped |

### `internal/restore/restore_integration_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestIntegration_RestoreCreatesObjects` | Restore from backup dir, verify objects created in FakeXCServer |
| `TestIntegration_RestoreTierOrder` | Restore healthcheck (tier 1) + origin-pool (tier 2), verify order via request recording |
| `TestIntegration_RestoreConflictSkip` | Pre-seed objects, restore with `OnConflict: "skip"`, verify no overwrites |
| `TestIntegration_RestoreConflictOverwrite` | Pre-seed objects, restore with `OnConflict: "overwrite"`, verify PUT requests |
| `TestIntegration_RestoreConflictFail` | Pre-seed objects, restore with `OnConflict: "fail"`, verify errors |
| `TestIntegration_RestoreErrorInjection` | Inject 409 on create, 500 on replace, verify error handling |

### `internal/diff/diff_integration_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestIntegration_DiffNoDrift` | Backup matches server state, verify all unchanged |
| `TestIntegration_DiffAllCategories` | Seed added/removed/modified objects, verify report has all categories |
| `TestIntegration_DiffErrorInjection` | Inject 403 on list, verify warnings not hard errors |

### `internal/revert/revert_integration_test.go`

| Test | What it verifies |
|------|-----------------|
| `TestIntegration_RevertModified` | Modify objects in server vs backup, revert, verify PUT requests |
| `TestIntegration_RevertRecreatesRemoved` | Remove objects from server, revert, verify POST requests with tier ordering |
| `TestIntegration_RevertDeleteExtra` | Add objects to server not in backup, revert with `DeleteExtra`, verify DELETE in reverse tier order |
| `TestIntegration_RevertDryRun` | Verify no mutating requests recorded |

## Component 3: Contract Tests

**Location**: `internal/client/contract_test.go`

Build tag: `//go:build contract`

Environment variables (matching k8s operator):
- `XC_TENANT_URL` — tenant URL (e.g., `https://acme.console.ves.volterra.io`)
- `XC_API_TOKEN` — API token
- `XC_TEST_NAMESPACE` — test namespace (defaults to `backup-test`)

### Helpers

```go
func contractClient(t *testing.T) *client.Client   // t.Skip() if env not set
func contractNamespace(t *testing.T) string          // defaults to "backup-test"
```

### Tests

| Test | What it verifies |
|------|-----------------|
| `TestContract_ListNamespaces` | `/api/web/namespaces` returns results |
| `TestContract_CRUD_Healthcheck` | Create -> get -> replace -> list (verify present) -> delete -> get (verify 404). Uses unique name `contract-test-hc-{timestamp}`. Cleanup via `t.Cleanup`. |
| `TestContract_ListResources` | List a common resource type, verify response has items array |
| `TestContract_ErrorCodes` | Get nonexistent object, verify 404 as `*APIError` |

## Component 4: Makefile

```makefile
test:
    go test -v -race ./...

test-contract:
    go test -v -race -count=1 -tags=contract ./...
```

`-count=1` on contract tests to prevent caching. Integration tests run with `make test` since they use in-process FakeXCServer and are fast.

## File Summary

| File | Type | New/Modified |
|------|------|-------------|
| `internal/client/testutil/fakeserver.go` | FakeXCServer | New |
| `internal/backup/backup_integration_test.go` | Integration tests | New |
| `internal/restore/restore_integration_test.go` | Integration tests | New |
| `internal/diff/diff_integration_test.go` | Integration tests | New |
| `internal/revert/revert_integration_test.go` | Integration tests | New |
| `internal/client/contract_test.go` | Contract tests | New |
| `Makefile` | Build targets | Modified |
