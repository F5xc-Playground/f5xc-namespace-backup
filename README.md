# F5 Distributed Cloud Namespace Backup

A CLI tool that backs up, restores, diffs, and reverts [F5 Distributed Cloud](https://www.f5.com/cloud) namespace configurations. Captures every configuration object in a namespace as individual JSON files, organized by resource type, suitable for version control.

## Commands

| Command | Description |
|---------|-------------|
| `backup` | Export all objects in a namespace to a local directory |
| `restore` | Recreate objects from a backup in dependency order |
| `diff` | Compare live namespace state against a backup snapshot |
| `revert` | Push backup state back to the tenant for drifted objects |
| `inspect` | Display backup metadata and resource counts |
| `namespaces` | List available namespaces on the tenant |

## Quick Start

### Prerequisites

- Go 1.25+ (to build from source)
- An F5 XC tenant with an [API token or API certificate](https://docs.cloud.f5.com/docs/how-to/user-mgmt/credentials)

### Install

```bash
# From source
go install github.com/F5xc-Playground/f5xc-namespace-backup/cmd/xcbackup@latest

# Or build locally
git clone https://github.com/F5xc-Playground/f5xc-namespace-backup.git
cd f5xc-namespace-backup
make build
# Binary at ./bin/xcbackup

# Or pull the container
docker pull ghcr.io/f5xc-playground/xcbackup:latest
```

### Backup a namespace

```bash
xcbackup backup \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --token "$XC_API_TOKEN"
```

This creates a timestamped directory:

```
backup-prod-2026-02-25T13-15-00Z/
├── manifest.json
├── healthcheck/
│   ├── hc-web.json
│   └── hc-api.json
├── origin-pool/
│   └── pool-web.json
├── http-loadbalancer/
│   └── main-lb.json
└── ...
```

### Inspect a backup

```bash
xcbackup inspect ./backup-prod-2026-02-25T13-15-00Z/
```

### Detect drift

```bash
xcbackup diff \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --token "$XC_API_TOKEN" \
  ./backup-prod-2026-02-25T13-15-00Z/
```

Shows added, removed, and modified objects with unified diffs.

### Revert drift

```bash
# Preview first
xcbackup revert --dry-run \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --token "$XC_API_TOKEN" \
  ./backup-prod-2026-02-25T13-15-00Z/

# Apply
xcbackup revert \
  --tenant acme.console.ves.volterra.io \
  --namespace prod \
  --token "$XC_API_TOKEN" \
  ./backup-prod-2026-02-25T13-15-00Z/
```

### Restore to a new namespace

```bash
xcbackup restore \
  --tenant acme.console.ves.volterra.io \
  --namespace prod-restored \
  --token "$XC_API_TOKEN" \
  ./backup-prod-2026-02-25T13-15-00Z/
```

## Authentication

**API Token** — pass directly or via environment variable:

```bash
xcbackup backup --token "your-api-token" ...

# Or
export XC_API_TOKEN="your-api-token"
xcbackup backup ...
```

**mTLS Certificate** — pass cert and key paths:

```bash
xcbackup backup --cert /path/to/cert.pem --key /path/to/key.pem ...
```

To create credentials: F5 XC Console → Administration → Personal Management → Credentials.

## Tenant URL

The `--tenant` flag accepts multiple formats:

```bash
--tenant acme                                  # tenant name only
--tenant acme.console.ves.volterra.io          # full hostname
--tenant https://acme.console.ves.volterra.io  # full URL
```

## Command Reference

### backup

```bash
xcbackup backup [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--output-dir` | `backup-{ns}-{timestamp}` | Output directory |
| `--types` | all | Only back up these resource types (comma-separated) |
| `--exclude-types` | none | Skip these resource types (comma-separated) |

### restore

```bash
xcbackup restore [backup-dir] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Preview without making changes |
| `--on-conflict` | `skip` | Behavior when object exists: `skip`, `overwrite`, or `fail` |
| `--types` | all | Only restore these resource types |

### diff

```bash
xcbackup diff [backup-dir] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--types` | all | Only diff these resource types |
| `--exclude-types` | none | Skip these resource types |

### revert

```bash
xcbackup revert [backup-dir] [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | Preview without making changes |
| `--delete-extra` | `false` | Delete objects added since backup |
| `--types` | all | Only revert these resource types |
| `--exclude-types` | none | Skip these resource types |

### Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--tenant` | required | F5 XC tenant URL |
| `--namespace` | required | Target namespace |
| `--token` | `$XC_API_TOKEN` | API token |
| `--cert` | | Path to mTLS certificate |
| `--key` | | Path to mTLS private key |
| `--parallel` | `10` | Max concurrent API calls |

## Key Concepts

**Resource coverage** — backs up ~99 namespace-scoped resource types across all domains: load balancers, origin pools, WAF policies, DNS zones, certificates, network policies, service policies, and more.

**Dependency-ordered restore** — objects are created in tier order so references resolve correctly. Healthchecks before origin pools, origin pools before load balancers, and so on. Deletes happen in reverse tier order.

**View handling** — F5 XC "view" objects (like HTTP Load Balancer) auto-create child objects (like Virtual Host). xcbackup skips these children on backup and lets the system recreate them on restore. See [Resource Filtering](docs/resource-filtering.md).

**Shared namespace** — objects can reference the `shared` namespace, which is visible across all namespaces. xcbackup detects these cross-namespace references and reports them as warnings. Shared objects are not included in the backup.

**Git-friendly output** — one JSON file per object with server-managed fields stripped. Designed for version control and easy diffing between backups.

## Limitations

- **Secrets**: Blindfolded values in certificates and cloud credentials may not be fully restorable — they require re-encryption on the target tenant.
- **View children**: Auto-managed children of view objects are intentionally excluded. Use `--types` to explicitly include them if needed.
- **System namespace**: Infrastructure objects (sites, fleets) in the `system` namespace are not backed up.

## Documentation

- [Architecture and Backup Format](docs/architecture.md)
- [Resource Filtering](docs/resource-filtering.md)
- [Development Guide](docs/development.md)
- [LLM/Agent Operating Guide](llms-full.txt)
- [F5 XC API Reference](https://docs.cloud.f5.com/docs-v2/api)

## License

Apache 2.0
