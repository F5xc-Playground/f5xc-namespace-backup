# xcbackup — F5 XC Namespace Backup & Restore Tool

## Design Document

**Date:** 2026-02-25
**Status:** Approved

## Problem

F5 Distributed Cloud (XC) namespaces contain dozens of interrelated configuration objects (load balancers, origin pools, WAF policies, DNS zones, etc.) with no built-in backup/restore mechanism. Losing a namespace's configuration — whether through accidental deletion, misconfiguration, or tenant migration — requires manual recreation of every object in the correct dependency order.

## Solution

A single Go binary (`xcbackup`) that connects to an F5 XC tenant, enumerates every namespace-scoped resource type, and exports each object as a JSON file in a directory tree. The same tool restores from a backup, creating objects in dependency order.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Single binary, no runtime deps, matches F5 tooling (vesctl), excellent concurrency |
| Backup format | Directory tree + JSON | Git-friendly, diffable, supports selective restore |
| Auth | API token + mTLS cert | Covers all user scenarios |
| View handling | Smart mode | Auto-skip view-managed children to prevent restore conflicts |
| CLI framework | Cobra | Standard for Go CLIs |

## Architecture

### Commands

```
xcbackup backup   --tenant <url> --namespace <ns> [auth flags] [--output-dir]
xcbackup restore  --tenant <url> --namespace <ns> [auth flags] [options] <backup-dir>
xcbackup inspect  <backup-dir>
```

### Resource Registry

A static Go map of ~99 namespace-scoped resource types. Each entry contains:

- `Kind`: API resource name (e.g., `http_loadbalancers`)
- `Domain`: Category (e.g., `virtual`, `dns`, `network_security`)
- `Tier`: Dependency tier (1-20) for restore ordering
- `IsView`: Whether this is a view object that auto-creates children
- `ManagedBy`: If non-empty, the view type that manages this resource
- `APIPath`: URL pattern for list/get/create operations

### Backup Format

```
backup-2026-02-25T13-15-00Z/
├── manifest.json
├── healthcheck/
│   └── hc-web.json
├── origin-pool/
│   └── pool-web.json
├── http-loadbalancer/
│   └── main-lb.json
└── ...
```

### Manifest Schema

```json
{
  "version": "1",
  "tool_version": "0.1.0",
  "tenant": "acme.console.ves.volterra.io",
  "namespace": "prod",
  "timestamp": "2026-02-25T13:15:00Z",
  "resource_counts": {},
  "skipped_view_children": [],
  "shared_references": [],
  "warnings": [],
  "errors": []
}
```

### Object Sanitization

Before writing each object to disk, strip:
- `system_metadata` (auto-generated on create)
- `metadata.uid` (auto-assigned)
- `metadata.resource_version` (stale on restore)

Preserve:
- `metadata.name`, `metadata.namespace`, `metadata.labels`, `metadata.annotations`
- `spec` (entire desired state)

### Shared Namespace Reference Detection

Walk every object's JSON tree looking for `ObjectRefType` patterns (objects with `name` + `namespace` fields). Flag any reference where `namespace == "shared"` in the manifest warnings.

### Restore Strategy

1. Parse manifest, validate backup integrity
2. Check that shared namespace references still exist on the target tenant
3. Restore objects tier by tier (lowest tier first = leaf dependencies first)
4. Within a tier, create objects in parallel
5. Support `--dry-run`, and conflict modes: `skip` (default), `overwrite`, `fail`

### Concurrency

- Parallel API calls with configurable limit (`--parallel`, default 10)
- Rate limiting to respect API throttling

## Project Structure

```
xcbackup/
├── cmd/
│   └── xcbackup/
│       └── main.go
├── internal/
│   ├── client/          # F5 XC API client (auth, HTTP, rate limiting)
│   ├── registry/        # Resource type registry
│   ├── backup/          # Backup orchestration
│   ├── restore/         # Restore orchestration (dependency ordering)
│   ├── manifest/        # Manifest read/write
│   └── refs/            # Shared reference detection/walking
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Dependencies

- `github.com/spf13/cobra` — CLI framework
- `log/slog` — structured logging (stdlib, Go 1.21+)
- `net/http` + `crypto/tls` — API client (stdlib)
- No external HTTP client libraries needed
