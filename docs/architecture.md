# Architecture

## Backup Process

1. Connects to the tenant and validates credentials
2. Iterates over the built-in registry of ~99 namespace-scoped resource types
3. Calls the List API for each type in the target namespace (concurrent, limited by `--parallel`)
4. Filters out objects from other namespaces (shared, system) and view-managed children
5. Strips server-managed fields (`system_metadata`, `uid`, `resource_version`, `status`)
6. Scans object references for cross-namespace dependencies on `shared`
7. Writes each object as `{resource-type}/{name}.json`
8. Writes `manifest.json` with metadata, resource counts, and warnings

## Diff and Revert

`diff` compares each backup object against the current live state:

- **Added**: objects in live that aren't in the backup
- **Removed**: objects in the backup that aren't in live
- **Modified**: objects in both but with spec differences (shown as unified diffs)

`revert` runs a diff, then pushes backup state back:

- **Modified** objects are replaced (PUT) in parallel
- **Removed** objects are recreated (POST) in tier order
- **Extra** objects are deleted (DELETE) in reverse tier order (only with `--delete-extra`)

Both commands share the same filtering and sanitization logic as backup.

## Restore Process

1. Reads and validates `manifest.json`
2. Restores objects in dependency order across 5 tiers:
   - **Tier 1**: Standalone primitives (healthchecks, certificates, IP prefix sets)
   - **Tier 2**: Policies that reference tier 1 (origin pools, service policies, network policies)
   - **Tier 3**: Higher-level policies (DNS zones, forward proxies, enhanced firewalls)
   - **Tier 4**: Top-level view objects (HTTP/TCP/UDP/CDN load balancers, virtual-k8s)
   - **Tier 5**: View-managed children (skipped — auto-created by their parent view)
3. Within each tier, creates objects in parallel
4. Handles conflicts per `--on-conflict`: skip (default), overwrite, or fail

## Resource Registry

The registry at `internal/registry/resources.go` defines every namespace-scoped resource type with:

| Field | Description |
|-------|-------------|
| `Kind` | Kebab-case name (e.g., `http-loadbalancer`) |
| `Domain` | API domain (e.g., `config`) |
| `Tier` | Dependency tier (1–5) for ordered restore |
| `ListPath` | API path template with `{namespace}` placeholder |
| `IsView` | Whether this is a view object that auto-creates children |
| `ManagedBy` | If set, this type is always managed by the named view and is skipped entirely |

See [Resource Filtering](resource-filtering.md) for how the registry, object metadata, and namespace checks work together.

## Backup Format

Each backup is a directory:

```
backup-prod-2026-02-25T13-15-00Z/
├── manifest.json
├── healthcheck/
│   ├── hc-web.json
│   └── hc-api.json
├── origin-pool/
│   └── pool-web.json
└── http-loadbalancer/
    └── main-lb.json
```

**`manifest.json`** contains:

```json
{
  "version": "1",
  "tenant": "https://acme.console.ves.volterra.io",
  "namespace": "prod",
  "timestamp": "2026-02-25T13:15:00Z",
  "resource_counts": {
    "healthcheck": 2,
    "origin-pool": 1,
    "http-loadbalancer": 1
  }
}
```

**Object files** contain `metadata` and `spec` only — server-managed fields are stripped so that backup-to-live diffs show only meaningful configuration changes.

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
```
