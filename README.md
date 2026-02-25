# xcbackup

Backup and restore F5 Distributed Cloud (XC) namespace configurations.

`xcbackup` connects to an F5 XC tenant, enumerates every configuration object in a namespace, and exports them as individual JSON files in a directory tree. The same tool restores from a backup, creating objects in the correct dependency order.

## Features

- **Complete namespace backup** — backs up ~99 namespace-scoped resource types across all domains (load balancers, origin pools, WAF policies, DNS zones, certificates, network policies, and more)
- **Smart view handling** — automatically skips auto-managed child objects (e.g., virtual hosts created by HTTP load balancers) to prevent conflicts on restore
- **Dependency-ordered restore** — creates objects in the correct order so that references resolve (healthchecks before origin pools, origin pools before load balancers, etc.)
- **Shared namespace warnings** — detects cross-namespace references to the `shared` namespace and warns you before restore
- **Git-friendly output** — one JSON file per object, organized by resource type, easy to diff and track in version control
- **Flexible auth** — supports both API tokens and mTLS certificates
- **Selective restore** — restore an entire namespace or individual resource types/objects
- **Dry run** — preview what a restore would do without making changes

## Installation

```bash
# From source
go install github.com/your-org/xcbackup/cmd/xcbackup@latest

# Or build locally
git clone <repo-url>
cd xcbackup
make build
```

## Quick Start

### Backup a namespace

```bash
# Using an API token
xcbackup backup \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --token "$XC_API_TOKEN"

# Using mTLS certificate
xcbackup backup \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --cert /path/to/cert.pem \
  --key /path/to/key.pem
```

This creates a timestamped directory with the backup:

```
backup-prod-2026-02-25T13-15-00Z/
├── manifest.json
├── healthcheck/
│   ├── hc-web.json
│   └── hc-api.json
├── origin-pool/
│   ├── pool-web.json
│   └── pool-api.json
├── http-loadbalancer/
│   ├── main-lb.json
│   └── api-lb.json
├── app-firewall/
│   └── default-waf.json
└── ...
```

### Inspect a backup

```bash
xcbackup inspect ./backup-prod-2026-02-25T13-15-00Z/
```

```
Backup: backup-prod-2026-02-25T13-15-00Z
Tenant: acme.console.ves.volterra.io
Namespace: prod
Timestamp: 2026-02-25T13:15:00Z

Resources:
  http-loadbalancer:  2
  origin-pool:        3
  healthcheck:        2
  app-firewall:       1
  service-policy:     4
  dns-zone:           1
  ─────────────────────
  Total:             13

Warnings:
  ⚠ http-loadbalancer/main-lb references shared/app-firewall/default-waf
  ⚠ service-policy/api-policy references shared/ip-prefix-set/office-ips
```

### Restore a namespace

```bash
# Dry run first
xcbackup restore \
  --tenant acme.console.ves.volterra.io \
  --namespace prod-restored \
  --token "$XC_API_TOKEN" \
  --dry-run \
  ./backup-prod-2026-02-25T13-15-00Z/

# Actual restore
xcbackup restore \
  --tenant acme.console.ves.volterra.io \
  --namespace prod-restored \
  --token "$XC_API_TOKEN" \
  ./backup-prod-2026-02-25T13-15-00Z/
```

## Authentication

### API Token

Pass directly or via environment variable:

```bash
# Flag
xcbackup backup --token "your-api-token" ...

# Environment variable
export XC_API_TOKEN="your-api-token"
xcbackup backup ...
```

To create an API token: F5 XC Console > Administration > Personal Management > Credentials > Add Credentials > API Token.

### mTLS Certificate

```bash
xcbackup backup \
  --cert /path/to/cert.pem \
  --key /path/to/key.pem \
  ...
```

To create API certificates: F5 XC Console > Administration > Personal Management > Credentials > Add Credentials > API Certificate.

## Tenant URL

The `--tenant` flag accepts multiple formats:

```bash
--tenant acme                                  # just the tenant name
--tenant acme.console.ves.volterra.io          # full hostname
--tenant https://acme.console.ves.volterra.io  # full URL
```

## Options

### Backup

| Flag | Default | Description |
|------|---------|-------------|
| `--tenant` | (required) | F5 XC tenant URL |
| `--namespace` | (required) | Namespace to back up |
| `--token` | `$XC_API_TOKEN` | API token |
| `--cert` | | Path to mTLS certificate |
| `--key` | | Path to mTLS private key |
| `--output-dir` | `./backup-{ns}-{timestamp}/` | Output directory |
| `--parallel` | `10` | Max concurrent API calls |
| `--types` | (all) | Comma-separated list of resource types to back up |
| `--exclude-types` | | Comma-separated list of resource types to skip |

### Restore

| Flag | Default | Description |
|------|---------|-------------|
| `--tenant` | (required) | Target F5 XC tenant URL |
| `--namespace` | (required) | Target namespace |
| `--token` | `$XC_API_TOKEN` | API token |
| `--cert` | | Path to mTLS certificate |
| `--key` | | Path to mTLS private key |
| `--dry-run` | `false` | Preview without making changes |
| `--on-conflict` | `skip` | Behavior when object exists: `skip`, `overwrite`, or `fail` |
| `--parallel` | `10` | Max concurrent API calls |
| `--types` | (all) | Comma-separated list of resource types to restore |

### Inspect

No additional flags — just pass the backup directory path.

## How It Works

### Backup Process

1. Connects to the tenant and validates credentials
2. Iterates over the built-in registry of ~99 namespace-scoped resource types
3. Calls the List API for each type in the target namespace
4. For each object found, calls the Get API to retrieve the full object
5. Strips system-managed fields (`system_metadata`, `uid`, `resource_version`)
6. Detects view-managed child objects and skips them (they'll be auto-recreated on restore)
7. Scans all object references for cross-namespace dependencies on `shared`
8. Writes each object as `{resource-type}/{name}.json`
9. Writes `manifest.json` with metadata, resource counts, and warnings

### Restore Process

1. Reads and validates `manifest.json`
2. Checks that any referenced `shared` namespace objects exist on the target tenant
3. Restores objects in dependency order (20 tiers, leaf objects first):
   - Tier 1: Standalone primitives (healthchecks, IP prefix sets, certificates)
   - Tier 2: Security policies (WAF, service policies)
   - Tier 3: Pools and intermediate objects (origin pools)
   - Tier 4+: Composite/view objects (load balancers, DNS zones)
4. Within each tier, creates objects in parallel
5. Reports results: created, skipped (already exists), failed

### View Object Handling

F5 XC "view" objects (like HTTP Load Balancer) automatically create and manage child objects (like Virtual Host, Cluster). `xcbackup` handles this by:

- **On backup:** Detecting and skipping auto-managed children. These are recorded in `manifest.json` under `skipped_view_children`.
- **On restore:** Only creating the view object. The system automatically recreates the children.

This prevents the "duplicate object" errors that would occur if both a view and its children were restored.

### Shared Namespace References

Objects in your namespace can reference objects in the `shared` namespace (which is visible across all namespaces). `xcbackup` detects these references and:

- **On backup:** Lists them as warnings in `manifest.json` and in the terminal output
- **On restore:** Verifies the referenced shared objects exist on the target tenant before proceeding. Missing shared references are reported as errors.

The `shared` namespace objects themselves are **not** included in the backup — they're managed separately and may be shared across many namespaces.

## Backup Format

Each backup is a directory containing:

- **`manifest.json`** — backup metadata, resource counts, shared references, warnings
- **`{resource-type}/{name}.json`** — one file per object, containing `metadata` and `spec`

This format is designed to be checked into git for version tracking and easy diffing between backups.

## Limitations

- **Secrets**: Objects containing secrets (e.g., blindfolded values in certificates, cloud credentials) may not be fully restorable. The backup captures the object structure but encrypted/blindfolded values require re-encryption on the target tenant.
- **Resource versions**: The F5 XC API evolves over time. A backup taken on one API version may need adjustment to restore on a newer version.
- **View children**: Auto-managed children of view objects are intentionally excluded. If you need to back up non-view child objects, use `--types` to explicitly include them.
- **System namespace**: Objects in the `system` namespace (sites, fleets) are not backed up — these are infrastructure objects, not application configuration.

## License

[TBD]
