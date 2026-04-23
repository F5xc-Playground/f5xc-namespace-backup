# Resource Filtering

xcbackup uses three filtering layers to exclude objects that should not be backed up, diffed, or restored.

## 1. Registry-Level Filtering (`ManagedBy`)

Resource types that are **always** view-managed are marked with `ManagedBy` in the registry. These are skipped entirely during backup and diff — no API calls are made.

| Kind | ManagedBy | Notes |
|------|-----------|-------|
| `virtual-host` | `http-loadbalancer` | Always auto-created |
| `cluster` | `origin-pool` | Always auto-created |
| `proxy` | `http-loadbalancer` | Always auto-created |

These live in Tier 5 of the registry. The `ManagedBy` field causes them to be filtered before any API listing.

## 2. Object-Level Filtering (`system_metadata.owner_view`)

Some resource types contain a mix of standalone and view-managed objects. For example, `route` and `advertise-policy` can be created manually, but `http-loadbalancer` also auto-creates instances of these kinds.

The F5 XC API marks auto-created objects with `system_metadata.owner_view`:

```json
{
  "system_metadata": {
    "owner_view": {
      "kind": "http_loadbalancer",
      "name": "my-lb",
      "namespace": "demo-shop",
      "uid": "abc-123-def"
    }
  }
}
```

The `sanitize.IsViewOwned()` function checks for this field at fetch time. Objects with a populated `owner_view` (containing a `kind` string) are skipped in both backup and diff.

### Affected resource types

These kinds have been observed to contain view-owned objects:

- `route` — created by HTTP/TCP load balancers
- `advertise-policy` — created by HTTP/TCP load balancers
- `endpoint` — created by origin pools
- `service-policy-set` — created by load balancers
- `network-policy-set` — created by load balancers

These remain in the registry at their natural tiers (not Tier 5) so that standalone instances are still backed up.

## 3. Namespace-Level Filtering

The F5 XC list API may return objects from shared or system namespaces alongside objects from the target namespace. Each list item includes a `namespace` field at the top level.

Objects whose `namespace` does not match the target namespace are skipped during listing, before any GET call is made.

## Sanitize Fields

After filtering, `sanitize.ForBackup()` strips server-managed fields to produce a clean backup:

- `system_metadata` (entire block, including `owner_view`, timestamps, creator info)
- `resource_version` (top-level)
- `status`
- `referring_objects`
- `deleted_referred_objects`
- `disabled_referred_objects`
- `create_form`
- `replace_form`
- `metadata.uid`
- `metadata.resource_version`

This ensures that backup-to-live diffs only show meaningful configuration changes.
