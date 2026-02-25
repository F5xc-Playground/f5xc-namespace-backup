# xcbackup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go CLI tool that backs up and restores every object in an F5 XC namespace.

**Architecture:** Single binary with cobra CLI, a static resource registry mapping ~99 resource types to their exact API paths, a generic HTTP client with token/mTLS auth, and backup/restore orchestrators that iterate the registry. Backup produces a directory of JSON files; restore reads them back in dependency-tier order.

**Tech Stack:** Go 1.22+, cobra (CLI), slog (logging), stdlib net/http + crypto/tls (API client). No external HTTP client.

**Key API insight:** API paths are inconsistent — most use `/api/config/namespaces/{ns}/{plural}`, but DNS uses `/api/config/dns/namespaces/{ns}/{plural}`, and pluralization is non-standard (e.g., `service_policys`). The registry stores exact paths.

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/xcbackup/main.go`
- Create: `Makefile`

**Step 1: Initialize Go module**

```bash
cd /Users/kevin/Projects/f5xc-namespace-backup
go mod init github.com/kevingstewart/xcbackup
```

**Step 2: Install cobra**

```bash
go get github.com/spf13/cobra@latest
```

**Step 3: Write main.go with root + 3 subcommands (backup, restore, inspect)**

```go
// cmd/xcbackup/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "xcbackup",
		Short:   "Backup and restore F5 XC namespace configurations",
		Version: version,
	}

	// Persistent flags (shared across subcommands)
	rootCmd.PersistentFlags().String("tenant", "", "F5 XC tenant URL (e.g., acme.console.ves.volterra.io)")
	rootCmd.PersistentFlags().String("namespace", "", "Target namespace")
	rootCmd.PersistentFlags().String("token", "", "API token (or set XC_API_TOKEN env var)")
	rootCmd.PersistentFlags().String("cert", "", "Path to mTLS client certificate")
	rootCmd.PersistentFlags().String("key", "", "Path to mTLS client private key")
	rootCmd.PersistentFlags().Int("parallel", 10, "Max concurrent API calls")

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup all objects in a namespace",
		RunE:  runBackup,
	}
	backupCmd.Flags().String("output-dir", "", "Output directory (default: backup-{ns}-{timestamp})")
	backupCmd.Flags().StringSlice("types", nil, "Only back up these resource types")
	backupCmd.Flags().StringSlice("exclude-types", nil, "Skip these resource types")

	restoreCmd := &cobra.Command{
		Use:   "restore [backup-dir]",
		Short: "Restore objects from a backup",
		Args:  cobra.ExactArgs(1),
		RunE:  runRestore,
	}
	restoreCmd.Flags().Bool("dry-run", false, "Preview without making changes")
	restoreCmd.Flags().String("on-conflict", "skip", "Behavior when object exists: skip, overwrite, fail")
	restoreCmd.Flags().StringSlice("types", nil, "Only restore these resource types")

	inspectCmd := &cobra.Command{
		Use:   "inspect [backup-dir]",
		Short: "Inspect a backup directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runInspect,
	}

	rootCmd.AddCommand(backupCmd, restoreCmd, inspectCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runBackup(cmd *cobra.Command, args []string) error {
	fmt.Println("backup: not yet implemented")
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	fmt.Println("restore: not yet implemented")
	return nil
}

func runInspect(cmd *cobra.Command, args []string) error {
	fmt.Println("inspect: not yet implemented")
	return nil
}
```

**Step 4: Write Makefile**

```makefile
# Makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test clean

build:
	go build $(LDFLAGS) -o bin/xcbackup ./cmd/xcbackup

test:
	go test -v -race ./...

clean:
	rm -rf bin/
```

**Step 5: Build and verify**

```bash
make build
./bin/xcbackup --help
./bin/xcbackup backup --help
./bin/xcbackup restore --help
./bin/xcbackup inspect --help
```

Expected: Help text for all commands with correct flags.

**Step 6: Commit**

```bash
git add go.mod go.sum cmd/ Makefile
git commit -m "feat: project scaffolding with cobra CLI skeleton"
```

---

## Task 2: API Client

**Files:**
- Create: `internal/client/client.go`
- Create: `internal/client/client_test.go`
- Create: `internal/client/tenant.go`
- Create: `internal/client/tenant_test.go`

**Step 1: Write tenant URL normalization tests**

```go
// internal/client/tenant_test.go
package client

import "testing"

func TestNormalizeTenantURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"acme", "https://acme.console.ves.volterra.io"},
		{"acme.console.ves.volterra.io", "https://acme.console.ves.volterra.io"},
		{"https://acme.console.ves.volterra.io", "https://acme.console.ves.volterra.io"},
		{"https://acme.console.ves.volterra.io/", "https://acme.console.ves.volterra.io"},
		{"acme.staging.volterra.us", "https://acme.staging.volterra.us"},
	}
	for _, tt := range tests {
		got := NormalizeTenantURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTenantURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
```

**Step 2: Run test, verify it fails**

```bash
go test ./internal/client/ -run TestNormalizeTenantURL -v
```

Expected: FAIL — package doesn't exist yet.

**Step 3: Implement tenant.go**

```go
// internal/client/tenant.go
package client

import "strings"

// NormalizeTenantURL converts various tenant URL formats to a canonical https:// URL.
// Accepts: "acme", "acme.console.ves.volterra.io", "https://acme.console.ves.volterra.io"
func NormalizeTenantURL(input string) string {
	input = strings.TrimRight(input, "/")

	// Already a full URL
	if strings.HasPrefix(input, "https://") {
		return input
	}

	// Has dots — assume it's a hostname
	if strings.Contains(input, ".") {
		return "https://" + input
	}

	// Just a tenant name
	return "https://" + input + ".console.ves.volterra.io"
}
```

**Step 4: Run test, verify it passes**

```bash
go test ./internal/client/ -run TestNormalizeTenantURL -v
```

**Step 5: Write client tests (using httptest)**

```go
// internal/client/client_test.go
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_List(t *testing.T) {
	items := []map[string]any{
		{"metadata": map[string]any{"name": "hc1"}},
		{"metadata": map[string]any{"name": "hc2"}},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/namespaces/prod/healthchecks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "APIToken test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
	}

	result, err := c.List("/api/config/namespaces/prod/healthchecks")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("List() returned %d items, want 2", len(result))
	}
}

func TestClient_Get(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{"name": "hc1", "namespace": "prod"},
		"spec":     map[string]any{"timeout": 3},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/namespaces/prod/healthchecks/hc1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(obj)
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
	}

	result, err := c.Get("/api/config/namespaces/prod/healthchecks/hc1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	md := result["metadata"].(map[string]any)
	if md["name"] != "hc1" {
		t.Errorf("Get() name = %v, want hc1", md["name"])
	}
}

func TestClient_Create(t *testing.T) {
	var received map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(received)
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
	}

	obj := map[string]any{
		"metadata": map[string]any{"name": "hc1"},
		"spec":     map[string]any{"timeout": 3},
	}
	err := c.Create("/api/config/namespaces/prod/healthchecks", obj)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if received["metadata"].(map[string]any)["name"] != "hc1" {
		t.Error("Create() did not send correct body")
	}
}
```

**Step 6: Run tests, verify they fail**

```bash
go test ./internal/client/ -v
```

**Step 7: Implement client.go**

```go
// internal/client/client.go
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

// Client is an F5 XC API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	sem        chan struct{} // concurrency limiter
	mu         sync.Mutex
}

// Option configures the client.
type Option func(*Client)

// WithToken sets API token auth.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithCert sets mTLS certificate auth.
func WithCert(certFile, keyFile string) Option {
	return func(c *Client) {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			slog.Error("failed to load client certificate", "error", err)
			return
		}
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		c.httpClient = &http.Client{Transport: transport}
	}
}

// WithParallel sets max concurrent requests.
func WithParallel(n int) Option {
	return func(c *Client) { c.sem = make(chan struct{}, n) }
}

// New creates a new F5 XC API client.
func New(tenantURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    NormalizeTenantURL(tenantURL),
		httpClient: &http.Client{},
		sem:        make(chan struct{}, 10),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// BaseURL returns the normalized tenant base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) do(method, path string, body io.Reader) ([]byte, int, error) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "APIToken "+c.token)
	}

	slog.Debug("API request", "method", method, "path", path)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return data, resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, resp.StatusCode, nil
}

// List returns all items at the given API path.
// The API returns {"items": [...]}.
func (c *Client) List(path string) ([]map[string]any, error) {
	data, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decoding list response: %w", err)
	}

	return resp.Items, nil
}

// Get returns a single object at the given API path.
func (c *Client) Get(path string) (map[string]any, error) {
	data, _, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decoding get response: %w", err)
	}

	return obj, nil
}

// Create posts an object to the given API path.
func (c *Client) Create(path string, obj map[string]any) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encoding object: %w", err)
	}

	_, _, err = c.do("POST", path, bytes.NewReader(body))
	return err
}

// Replace does a full PUT replace of an object at the given API path.
func (c *Client) Replace(path string, obj map[string]any) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("encoding object: %w", err)
	}

	_, _, err = c.do("PUT", path, bytes.NewReader(body))
	return err
}
```

**Step 8: Run tests, verify they pass**

```bash
go test ./internal/client/ -v
```

**Step 9: Commit**

```bash
git add internal/client/
git commit -m "feat: API client with token/mTLS auth and concurrency limiting"
```

---

## Task 3: Resource Registry

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/registry_test.go`
- Create: `internal/registry/resources.go` (the big data file)

**Step 1: Write registry tests**

```go
// internal/registry/registry_test.go
package registry

import "testing"

func TestAllResources_NotEmpty(t *testing.T) {
	resources := All()
	if len(resources) == 0 {
		t.Fatal("All() returned empty registry")
	}
}

func TestAllResources_HaveRequiredFields(t *testing.T) {
	for _, r := range All() {
		if r.Kind == "" {
			t.Errorf("resource with empty Kind")
		}
		if r.ListPath == "" {
			t.Errorf("resource %q has empty ListPath", r.Kind)
		}
		if r.Tier < 1 || r.Tier > 20 {
			t.Errorf("resource %q has invalid Tier %d", r.Kind, r.Tier)
		}
	}
}

func TestAllResources_UniqueKinds(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range All() {
		if seen[r.Kind] {
			t.Errorf("duplicate Kind: %q", r.Kind)
		}
		seen[r.Kind] = true
	}
}

func TestFilterByTier(t *testing.T) {
	tier1 := FilterByTier(All(), 1)
	for _, r := range tier1 {
		if r.Tier != 1 {
			t.Errorf("FilterByTier(1) returned resource %q with Tier %d", r.Kind, r.Tier)
		}
	}
}

func TestFilterByKinds(t *testing.T) {
	result := FilterByKinds(All(), []string{"healthcheck", "origin-pool"})
	if len(result) != 2 {
		t.Errorf("FilterByKinds returned %d items, want 2", len(result))
	}
}

func TestExcludeKinds(t *testing.T) {
	all := All()
	excluded := ExcludeKinds(all, []string{"healthcheck"})
	if len(excluded) != len(all)-1 {
		t.Errorf("ExcludeKinds removed %d items, want 1", len(all)-len(excluded))
	}
}

func TestTiers(t *testing.T) {
	tiers := Tiers(All())
	if len(tiers) == 0 {
		t.Fatal("Tiers() returned empty")
	}
	// Tiers should be sorted ascending
	for i := 1; i < len(tiers); i++ {
		if tiers[i] <= tiers[i-1] {
			t.Errorf("Tiers not sorted: %v", tiers)
		}
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/registry/ -v
```

**Step 3: Implement registry.go (types + helpers)**

```go
// internal/registry/registry.go
package registry

import "sort"

// Resource describes a namespace-scoped F5 XC resource type.
type Resource struct {
	// Kind is the canonical resource name, e.g., "healthcheck", "http-loadbalancer".
	Kind string

	// Domain is the API domain, e.g., "virtual", "dns", "network_security".
	Domain string

	// Tier is the dependency tier for restore ordering (1 = no deps, higher = more deps).
	Tier int

	// ListPath is the API path template for listing objects.
	// Use {namespace} as the placeholder.
	// e.g., "/api/config/namespaces/{namespace}/healthchecks"
	ListPath string

	// ObjectPath is the API path template for get/create.
	// e.g., "/api/config/namespaces/{namespace}/healthchecks/{name}"
	ObjectPath string

	// IsView indicates this is a "view" object that auto-creates child resources.
	IsView bool

	// ManagedBy is the Kind of the view that manages this resource (empty if standalone).
	ManagedBy string
}

// All returns the complete resource registry.
func All() []Resource {
	return allResources
}

// FilterByTier returns resources matching the given tier.
func FilterByTier(resources []Resource, tier int) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.Tier == tier {
			result = append(result, r)
		}
	}
	return result
}

// FilterByKinds returns only resources whose Kind is in the given list.
func FilterByKinds(resources []Resource, kinds []string) []Resource {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	var result []Resource
	for _, r := range resources {
		if set[r.Kind] {
			result = append(result, r)
		}
	}
	return result
}

// ExcludeKinds returns resources whose Kind is NOT in the given list.
func ExcludeKinds(resources []Resource, kinds []string) []Resource {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	var result []Resource
	for _, r := range resources {
		if !set[r.Kind] {
			result = append(result, r)
		}
	}
	return result
}

// Tiers returns sorted unique tier numbers present in the given resources.
func Tiers(resources []Resource) []int {
	set := make(map[int]bool)
	for _, r := range resources {
		set[r.Tier] = true
	}
	tiers := make([]int, 0, len(set))
	for t := range set {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	return tiers
}

// ByKind returns a map from Kind to Resource for quick lookup.
func ByKind(resources []Resource) map[string]Resource {
	m := make(map[string]Resource, len(resources))
	for _, r := range resources {
		m[r.Kind] = r
	}
	return m
}

// ViewManaged returns only resources that are managed by a view object.
func ViewManaged(resources []Resource) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.ManagedBy != "" {
			result = append(result, r)
		}
	}
	return result
}

// Standalone returns resources that are NOT managed by a view.
func Standalone(resources []Resource) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.ManagedBy == "" {
			result = append(result, r)
		}
	}
	return result
}
```

**Step 4: Implement resources.go (the data file)**

This file contains all ~99 resource entries. Note: the `ListPath` uses `{namespace}` placeholder. The `ObjectPath` appends `/{name}`.

The path pattern is: List = `ListPath`, Get = `ListPath + "/" + name`, Create = POST to `ListPath`.

```go
// internal/registry/resources.go
package registry

// allResources is the complete registry of namespace-scoped F5 XC resource types.
// Paths verified against the F5 XC OpenAPI specification.
//
// Tier guide:
//   1 - Leaf primitives (no deps on other ns objects)
//   2 - References tier-1 objects
//   3 - References tier-1 or tier-2 objects
//   4 - Complex view objects (LBs, etc.)
//   5 - Objects that reference views or complex policies
//
// Note: F5 XC uses inconsistent pluralization. Paths are exact from the OAS.
// e.g., "service_policys" NOT "service_policies"
var allResources = []Resource{
	// ── Tier 1: Standalone Primitives ─────────────────────────────────────────

	// Certificates
	{Kind: "certificate", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/certificates"},
	{Kind: "certificate-chain", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/certificate_chains"},
	{Kind: "trusted-ca-list", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/trusted_ca_lists"},
	{Kind: "crl", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/crls"},

	// Network primitives
	{Kind: "ip-prefix-set", Domain: "network", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/ip_prefix_sets"},
	{Kind: "geo-location-set", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/geo_location_sets"},

	// Healthchecks
	{Kind: "healthcheck", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/healthchecks"},

	// Rate limiting primitives
	{Kind: "rate-limiter", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/rate_limiters"},
	{Kind: "policer", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/policers"},
	{Kind: "protocol-policer", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/protocol_policers"},

	// User identification
	{Kind: "user-identification", Domain: "tenant_and_identity", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/user_identifications"},

	// App firewall (WAF)
	{Kind: "app-firewall", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_firewalls"},

	// Data types
	{Kind: "data-type", Domain: "data_and_privacy_security", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/data_types"},

	// BIG-IP
	{Kind: "bigip-irule", Domain: "bigip", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/bigip_irules"},
	{Kind: "data-group", Domain: "bigip", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/data_groups"},

	// Virtual site
	{Kind: "virtual-site", Domain: "sites", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/virtual_sites"},

	// Virtual network
	{Kind: "virtual-network", Domain: "network", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/virtual_networks"},

	// Cloud credentials
	{Kind: "cloud-credentials", Domain: "cloud_infrastructure", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/cloud_credentialss"},

	// Secrets & blindfold
	{Kind: "secret-policy", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_policys"},
	{Kind: "secret-policy-rule", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_policy_rules"},
	{Kind: "secret-management-access", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_management_accesss"},
	{Kind: "voltshare-admin-policy", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/voltshare_admin_policys"},

	// Observability
	{Kind: "alert-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/alert_receivers"},
	{Kind: "alert-policy", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/alert_policys"},
	{Kind: "global-log-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/global_log_receivers"},
	{Kind: "log-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/log_receivers"},

	// Malicious user mitigation
	{Kind: "malicious-user-mitigation", Domain: "secops_and_incident_response", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/malicious_user_mitigations"},

	// Protocol inspection
	{Kind: "protocol-inspection", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/protocol_inspections"},

	// Workload flavors
	{Kind: "workload-flavor", Domain: "container_services", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/workload_flavors"},

	// K8s policies
	{Kind: "k8s-cluster-role", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_cluster_roles"},
	{Kind: "k8s-pod-security-admission", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_pod_security_admissions"},
	{Kind: "k8s-pod-security-policy", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_pod_security_policys"},

	// DDoS
	{Kind: "infraprotect-asn-prefix", Domain: "ddos", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_asn_prefixs"},
	{Kind: "infraprotect-firewall-rule", Domain: "ddos", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_firewall_rules"},

	// App setting
	{Kind: "app-setting", Domain: "service_mesh", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_settings"},
	{Kind: "app-type", Domain: "service_mesh", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_types"},

	// NGINX
	{Kind: "nginx-service-discovery", Domain: "nginx_one", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/nginx_service_discoverys"},

	// ── Tier 2: Reference tier-1 objects ──────────────────────────────────────

	// Origin pool (references healthcheck)
	{Kind: "origin-pool", Domain: "virtual", Tier: 2, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/origin_pools"},

	// Security policies
	{Kind: "service-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/service_policys"},
	{Kind: "service-policy-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/service_policy_rules"},
	{Kind: "network-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_policys"},
	{Kind: "network-policy-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_rules"},
	{Kind: "network-firewall", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_firewalls"},
	{Kind: "fast-acl-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/fast_acl_rules"},
	{Kind: "filter-set", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/filter_sets"},
	{Kind: "segment", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/segments"},
	{Kind: "nat-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/nat_policys"},

	// Rate limiter policy
	{Kind: "rate-limiter-policy", Domain: "virtual", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/rate_limiter_policys"},

	// WAF exclusion policy
	{Kind: "waf-exclusion-policy", Domain: "virtual", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/waf_exclusion_policys"},

	// Sensitive data policy
	{Kind: "sensitive-data-policy", Domain: "data_and_privacy_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/sensitive_data_policys"},

	// Network routing
	{Kind: "route", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/routes"},
	{Kind: "network-connector", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_connectors"},
	{Kind: "bgp-routing-policy", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/bgp_routing_policys"},
	{Kind: "advertise-policy", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/advertise_policys"},
	{Kind: "srv6-network-slice", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/srv6_network_slices"},
	{Kind: "dc-cluster-group", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/dc_cluster_groups"},
	{Kind: "policy-based-routing", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/policy_based_routings"},

	// K8s cluster role binding (refs cluster role)
	{Kind: "k8s-cluster-role-binding", Domain: "managed_kubernetes", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/k8s_cluster_role_bindings"},

	// DDoS groups
	{Kind: "infraprotect-firewall-rule-group", Domain: "ddos", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_firewall_rule_groups"},
	{Kind: "infraprotect-internet-prefix-advertisement", Domain: "ddos", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_internet_prefix_advertisements"},

	// API definition
	{Kind: "api-definition", Domain: "api", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/api_definitions"},
	{Kind: "app-api-group", Domain: "api", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/app_api_groups"},

	// Marketplace
	{Kind: "external-connector", Domain: "marketplace", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/external_connectors"},
	{Kind: "addon-subscription", Domain: "marketplace", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/addon_subscriptions"},

	// ── Tier 3: Higher-level policy views ─────────────────────────────────────

	// Network policy view (creates network_policy + rules)
	{Kind: "network-policy-view", Domain: "network_security", Tier: 3, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_views"},

	// Forward proxy policy (creates service_policy + rules)
	{Kind: "forward-proxy-policy", Domain: "network_security", Tier: 3, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/forward_proxy_policys"},

	// Fast ACL (refs fast-acl-rule)
	{Kind: "fast-acl", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/fast_acls"},

	// Enhanced firewall policy
	{Kind: "enhanced-firewall-policy", Domain: "virtual", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/enhanced_firewall_policys"},

	// Service policy set (read-only computed — skip on restore)
	{Kind: "service-policy-set", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/service_policy_sets"},
	{Kind: "network-policy-set", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_sets"},

	// DNS resources (note: different API prefix!)
	{Kind: "dns-zone", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_zones"},
	{Kind: "dns-domain", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_domains"},
	{Kind: "dns-lb-health-check", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_lb_health_checks"},
	{Kind: "dns-lb-pool", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_lb_pools"},
	{Kind: "dns-load-balancer", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_load_balancers"},
	{Kind: "dns-compliance-checks", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_compliance_checkss"},

	// API security
	{Kind: "api-discovery", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_discoverys"},
	{Kind: "api-crawler", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_crawlers"},
	{Kind: "api-testing", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_testings"},
	{Kind: "discovery", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/discoverys"},

	// Bot defense
	{Kind: "bot-defense-app-infrastructure", Domain: "bot_and_threat_defense", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/bot_defense_app_infrastructures"},
	{Kind: "protected-application", Domain: "shape", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/protected_applications"},

	// Service mesh
	{Kind: "site-mesh-group", Domain: "service_mesh", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/site_mesh_groups"},
	{Kind: "endpoint", Domain: "service_mesh", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/endpoints"},

	// Cloud
	{Kind: "cloud-connect", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_connects"},
	{Kind: "cloud-elastic-ip", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_elastic_ips"},
	{Kind: "cloud-link", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_links"},

	// CE management
	{Kind: "network-interface", Domain: "ce_management", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/network_interfaces"},
	{Kind: "usb-policy", Domain: "ce_management", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/usb_policys"},

	// ── Tier 4: Top-level view objects (LBs, etc.) ────────────────────────────

	// Load balancers (reference origin pools, WAF, policies, etc.)
	{Kind: "http-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/http_loadbalancers"},
	{Kind: "tcp-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/tcp_loadbalancers"},
	{Kind: "udp-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/udp_loadbalancers"},
	{Kind: "cdn-loadbalancer", Domain: "cdn", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/cdn_loadbalancers"},

	// Container services
	{Kind: "virtual-k8s", Domain: "container_services", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/virtual_k8ss"},
	{Kind: "workload", Domain: "container_services", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/workloads"},
	{Kind: "k8s-cluster", Domain: "sites", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/k8s_clusters"},

	// ── Tier 5: View-managed children (skipped in smart mode) ─────────────────

	// Auto-created by http/tcp/udp load balancer views
	{Kind: "virtual-host", Domain: "virtual", Tier: 5, ManagedBy: "http-loadbalancer",
		ListPath: "/api/config/namespaces/{namespace}/virtual_hosts"},
	{Kind: "cluster", Domain: "virtual", Tier: 5, ManagedBy: "origin-pool",
		ListPath: "/api/config/namespaces/{namespace}/clusters"},
	{Kind: "proxy", Domain: "virtual", Tier: 5, ManagedBy: "http-loadbalancer",
		ListPath: "/api/config/namespaces/{namespace}/proxys"},
}
```

**Step 5: Run tests, verify they pass**

```bash
go test ./internal/registry/ -v
```

**Step 6: Commit**

```bash
git add internal/registry/
git commit -m "feat: resource registry with ~99 namespace-scoped XC resource types"
```

---

## Task 4: Object Sanitization

**Files:**
- Create: `internal/sanitize/sanitize.go`
- Create: `internal/sanitize/sanitize_test.go`

**Step 1: Write tests**

```go
// internal/sanitize/sanitize_test.go
package sanitize

import (
	"encoding/json"
	"testing"
)

func TestForBackup(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":             "hc1",
			"namespace":        "prod",
			"uid":              "abc-123",
			"resource_version":  "rv-456",
			"labels":           map[string]any{"app": "web"},
			"annotations":      map[string]any{"note": "test"},
		},
		"system_metadata": map[string]any{
			"uid":                  "sys-abc",
			"creation_timestamp":   "2026-01-01T00:00:00Z",
			"creator_id":          "user@example.com",
		},
		"spec": map[string]any{
			"timeout": float64(3),
		},
	}

	result := ForBackup(obj)

	// system_metadata should be removed
	if _, ok := result["system_metadata"]; ok {
		t.Error("system_metadata should be removed")
	}

	// metadata.uid should be removed
	md := result["metadata"].(map[string]any)
	if _, ok := md["uid"]; ok {
		t.Error("metadata.uid should be removed")
	}

	// metadata.resource_version should be removed
	if _, ok := md["resource_version"]; ok {
		t.Error("metadata.resource_version should be removed")
	}

	// metadata.name, namespace, labels, annotations should remain
	if md["name"] != "hc1" {
		t.Error("metadata.name should remain")
	}
	if md["namespace"] != "prod" {
		t.Error("metadata.namespace should remain")
	}
	if md["labels"] == nil {
		t.Error("metadata.labels should remain")
	}

	// spec should remain
	if result["spec"] == nil {
		t.Error("spec should remain")
	}
}

func TestForBackup_DoesNotMutateOriginal(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name": "hc1",
			"uid":  "abc-123",
		},
		"system_metadata": map[string]any{"uid": "sys-abc"},
		"spec":            map[string]any{"timeout": float64(3)},
	}

	// Deep copy via JSON round-trip to get original state
	origJSON, _ := json.Marshal(obj)

	_ = ForBackup(obj)

	afterJSON, _ := json.Marshal(obj)
	if string(origJSON) != string(afterJSON) {
		t.Error("ForBackup should not mutate the original object")
	}
}

func TestForRestore(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":      "hc1",
			"namespace": "prod",
			"labels":    map[string]any{"app": "web"},
		},
		"spec": map[string]any{"timeout": float64(3)},
	}

	result := ForRestore(obj, "staging")

	md := result["metadata"].(map[string]any)
	if md["namespace"] != "staging" {
		t.Errorf("ForRestore should set namespace to target, got %v", md["namespace"])
	}
	if md["name"] != "hc1" {
		t.Error("ForRestore should preserve name")
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/sanitize/ -v
```

**Step 3: Implement sanitize.go**

```go
// internal/sanitize/sanitize.go
package sanitize

import "encoding/json"

// ForBackup returns a sanitized copy of an API object for writing to disk.
// Strips system_metadata, metadata.uid, and metadata.resource_version.
func ForBackup(obj map[string]any) map[string]any {
	result := deepCopy(obj)

	delete(result, "system_metadata")

	if md, ok := result["metadata"].(map[string]any); ok {
		delete(md, "uid")
		delete(md, "resource_version")
	}

	return result
}

// ForRestore returns a copy of a backed-up object ready for POST to the API.
// Sets metadata.namespace to the target namespace.
func ForRestore(obj map[string]any, targetNamespace string) map[string]any {
	result := deepCopy(obj)

	if md, ok := result["metadata"].(map[string]any); ok {
		md["namespace"] = targetNamespace
	}

	return result
}

func deepCopy(obj map[string]any) map[string]any {
	data, _ := json.Marshal(obj)
	var result map[string]any
	json.Unmarshal(data, &result)
	return result
}
```

**Step 4: Run tests, verify they pass**

```bash
go test ./internal/sanitize/ -v
```

**Step 5: Commit**

```bash
git add internal/sanitize/
git commit -m "feat: object sanitization for backup and restore"
```

---

## Task 5: Shared Reference Detection

**Files:**
- Create: `internal/refs/refs.go`
- Create: `internal/refs/refs_test.go`

**Step 1: Write tests**

```go
// internal/refs/refs_test.go
package refs

import "testing"

func TestFindSharedRefs(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":      "main-lb",
			"namespace": "prod",
		},
		"spec": map[string]any{
			"app_firewall": map[string]any{
				"name":      "default-waf",
				"namespace": "shared",
				"tenant":    "acme",
			},
			"origin_pools": []any{
				map[string]any{
					"pool": map[string]any{
						"name":      "local-pool",
						"namespace": "prod",
					},
				},
				map[string]any{
					"pool": map[string]any{
						"name":      "shared-pool",
						"namespace": "shared",
					},
				},
			},
		},
	}

	refs := FindSharedRefs("http-loadbalancer", "main-lb", obj)

	if len(refs) != 2 {
		t.Fatalf("FindSharedRefs returned %d refs, want 2", len(refs))
	}

	// Check that both shared refs are found
	foundWAF := false
	foundPool := false
	for _, ref := range refs {
		if ref.TargetName == "default-waf" {
			foundWAF = true
		}
		if ref.TargetName == "shared-pool" {
			foundPool = true
		}
	}
	if !foundWAF {
		t.Error("missing shared ref to default-waf")
	}
	if !foundPool {
		t.Error("missing shared ref to shared-pool")
	}
}

func TestFindSharedRefs_NoSharedRefs(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"pool": map[string]any{
				"name":      "local-pool",
				"namespace": "prod",
			},
		},
	}

	refs := FindSharedRefs("origin-pool", "pool1", obj)
	if len(refs) != 0 {
		t.Errorf("FindSharedRefs returned %d refs, want 0", len(refs))
	}
}

func TestFindSharedRefs_OmittedNamespace(t *testing.T) {
	// When namespace is omitted, it defaults to the parent object's namespace — NOT shared
	obj := map[string]any{
		"spec": map[string]any{
			"ref": map[string]any{
				"name": "some-object",
				// no namespace field
			},
		},
	}

	refs := FindSharedRefs("http-loadbalancer", "lb1", obj)
	if len(refs) != 0 {
		t.Errorf("omitted namespace should not be treated as shared, got %d refs", len(refs))
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/refs/ -v
```

**Step 3: Implement refs.go**

```go
// internal/refs/refs.go
package refs

// SharedRef represents a cross-namespace reference to the "shared" namespace.
type SharedRef struct {
	SourceKind string // e.g., "http-loadbalancer"
	SourceName string // e.g., "main-lb"
	TargetName string // e.g., "default-waf"
	FieldPath  string // e.g., "spec.app_firewall"
}

// FindSharedRefs walks an object's JSON tree and returns all references
// to the "shared" namespace. It detects ObjectRefType patterns: maps
// containing both "name" and "namespace" keys where namespace == "shared".
func FindSharedRefs(sourceKind, sourceName string, obj map[string]any) []SharedRef {
	var refs []SharedRef
	walkJSON(obj, "", func(path string, v map[string]any) {
		ns, hasNS := v["namespace"]
		name, hasName := v["name"]
		if hasNS && hasName {
			if nsStr, ok := ns.(string); ok && nsStr == "shared" {
				if nameStr, ok := name.(string); ok {
					refs = append(refs, SharedRef{
						SourceKind: sourceKind,
						SourceName: sourceName,
						TargetName: nameStr,
						FieldPath:  path,
					})
				}
			}
		}
	})
	return refs
}

// walkJSON recursively walks a JSON structure, calling fn for every map encountered.
func walkJSON(v any, path string, fn func(string, map[string]any)) {
	switch val := v.(type) {
	case map[string]any:
		fn(path, val)
		for k, child := range val {
			childPath := path
			if childPath != "" {
				childPath += "."
			}
			childPath += k
			walkJSON(child, childPath, fn)
		}
	case []any:
		for i, child := range val {
			_ = i
			walkJSON(child, path, fn)
		}
	}
}
```

**Step 4: Run tests, verify they pass**

```bash
go test ./internal/refs/ -v
```

**Step 5: Commit**

```bash
git add internal/refs/
git commit -m "feat: shared namespace reference detection"
```

---

## Task 6: Manifest Types

**Files:**
- Create: `internal/manifest/manifest.go`
- Create: `internal/manifest/manifest_test.go`

**Step 1: Write tests**

```go
// internal/manifest/manifest_test.go
package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/refs"
)

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()

	m := &Manifest{
		Version:     "1",
		ToolVersion: "0.1.0",
		Tenant:      "acme.console.ves.volterra.io",
		Namespace:   "prod",
		Timestamp:   "2026-02-25T13:15:00Z",
		ResourceCounts: map[string]int{
			"healthcheck":       2,
			"http-loadbalancer": 1,
		},
		SkippedViewChildren: []string{"virtual-host/ves-io-http-main-lb"},
		SharedReferences: []refs.SharedRef{
			{
				SourceKind: "http-loadbalancer",
				SourceName: "main-lb",
				TargetName: "default-waf",
				FieldPath:  "spec.app_firewall",
			},
		},
		Warnings: []string{"http-loadbalancer/main-lb references shared/app-firewall/default-waf"},
		Errors:   nil,
	}

	if err := Write(dir, m); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not created: %v", err)
	}

	loaded, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if loaded.Tenant != m.Tenant {
		t.Errorf("Tenant = %q, want %q", loaded.Tenant, m.Tenant)
	}
	if loaded.Namespace != m.Namespace {
		t.Errorf("Namespace = %q, want %q", loaded.Namespace, m.Namespace)
	}
	if loaded.ResourceCounts["healthcheck"] != 2 {
		t.Error("ResourceCounts not preserved")
	}
	if len(loaded.SharedReferences) != 1 {
		t.Error("SharedReferences not preserved")
	}
	if len(loaded.SkippedViewChildren) != 1 {
		t.Error("SkippedViewChildren not preserved")
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/manifest/ -v
```

**Step 3: Implement manifest.go**

```go
// internal/manifest/manifest.go
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevingstewart/xcbackup/internal/refs"
)

// Manifest describes a backup's metadata.
type Manifest struct {
	Version             string            `json:"version"`
	ToolVersion         string            `json:"tool_version"`
	Tenant              string            `json:"tenant"`
	Namespace           string            `json:"namespace"`
	Timestamp           string            `json:"timestamp"`
	ResourceCounts      map[string]int    `json:"resource_counts"`
	SkippedViewChildren []string          `json:"skipped_view_children"`
	SharedReferences    []refs.SharedRef  `json:"shared_references"`
	Warnings            []string          `json:"warnings"`
	Errors              []string          `json:"errors"`
}

// Write serializes the manifest to manifest.json in the given directory.
func Write(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	path := filepath.Join(dir, "manifest.json")
	return os.WriteFile(path, data, 0644)
}

// Read deserializes manifest.json from the given directory.
func Read(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}
```

**Step 4: Run tests, verify they pass**

```bash
go test ./internal/manifest/ -v
```

**Step 5: Commit**

```bash
git add internal/manifest/
git commit -m "feat: manifest read/write with shared ref and view child tracking"
```

---

## Task 7: Backup Command

**Files:**
- Create: `internal/backup/backup.go`
- Create: `internal/backup/backup_test.go`
- Modify: `cmd/xcbackup/main.go` — wire up `runBackup`

**Step 1: Write backup orchestration tests**

```go
// internal/backup/backup_test.go
package backup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/registry"
)

func TestRun_BacksUpObjects(t *testing.T) {
	// Mock API: return 2 healthchecks, 1 origin pool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/test-ns/healthchecks":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"metadata":        map[string]any{"name": "hc1", "namespace": "test-ns"},
						"system_metadata": map[string]any{"uid": "sys1"},
						"spec":            map[string]any{"timeout": 3},
					},
					{
						"metadata":        map[string]any{"name": "hc2", "namespace": "test-ns"},
						"system_metadata": map[string]any{"uid": "sys2"},
						"spec":            map[string]any{"timeout": 5},
					},
				},
			})
		case "/api/config/namespaces/test-ns/healthchecks/hc1":
			json.NewEncoder(w).Encode(map[string]any{
				"metadata":        map[string]any{"name": "hc1", "namespace": "test-ns", "uid": "u1", "resource_version": "rv1"},
				"system_metadata": map[string]any{"uid": "sys1"},
				"spec":            map[string]any{"timeout": 3},
			})
		case "/api/config/namespaces/test-ns/healthchecks/hc2":
			json.NewEncoder(w).Encode(map[string]any{
				"metadata":        map[string]any{"name": "hc2", "namespace": "test-ns", "uid": "u2", "resource_version": "rv2"},
				"system_metadata": map[string]any{"uid": "sys2"},
				"spec":            map[string]any{"timeout": 5},
			})
		case "/api/config/namespaces/test-ns/origin_pools":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"metadata": map[string]any{"name": "pool1", "namespace": "test-ns"},
						"spec": map[string]any{
							"healthcheck": map[string]any{"name": "hc1", "namespace": "shared"},
						},
					},
				},
			})
		case "/api/config/namespaces/test-ns/origin_pools/pool1":
			json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "pool1", "namespace": "test-ns", "uid": "u3", "resource_version": "rv3"},
				"spec": map[string]any{
					"healthcheck": map[string]any{"name": "hc1", "namespace": "shared"},
				},
			})
		default:
			// Return empty list for all other resource types
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		}
	}))
	defer server.Close()

	c := &client.Client{}
	// Use the test helper to create a client pointing at our test server
	c = client.NewForTest(server.URL, server.Client(), "test-token")

	outputDir := t.TempDir()

	// Only back up healthcheck + origin-pool for this test
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	opts := &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	}

	result, err := Run(c, opts)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify files created
	hc1Path := filepath.Join(outputDir, "healthcheck", "hc1.json")
	if _, err := os.Stat(hc1Path); err != nil {
		t.Errorf("hc1.json not created: %v", err)
	}

	hc2Path := filepath.Join(outputDir, "healthcheck", "hc2.json")
	if _, err := os.Stat(hc2Path); err != nil {
		t.Errorf("hc2.json not created: %v", err)
	}

	poolPath := filepath.Join(outputDir, "origin-pool", "pool1.json")
	if _, err := os.Stat(poolPath); err != nil {
		t.Errorf("pool1.json not created: %v", err)
	}

	// Verify manifest
	m, err := manifest.Read(outputDir)
	if err != nil {
		t.Fatalf("manifest.Read() error: %v", err)
	}
	if m.ResourceCounts["healthcheck"] != 2 {
		t.Errorf("manifest healthcheck count = %d, want 2", m.ResourceCounts["healthcheck"])
	}
	if m.ResourceCounts["origin-pool"] != 1 {
		t.Errorf("manifest origin-pool count = %d, want 1", m.ResourceCounts["origin-pool"])
	}

	// Verify shared ref detected
	if len(result.SharedRefs) == 0 {
		t.Error("shared reference to hc1 in shared namespace not detected")
	}

	// Verify sanitization (read hc1.json and check no system_metadata)
	hc1Data, _ := os.ReadFile(hc1Path)
	var hc1Obj map[string]any
	json.Unmarshal(hc1Data, &hc1Obj)
	if _, ok := hc1Obj["system_metadata"]; ok {
		t.Error("system_metadata should be stripped from backup")
	}
	if md, ok := hc1Obj["metadata"].(map[string]any); ok {
		if _, ok := md["uid"]; ok {
			t.Error("metadata.uid should be stripped from backup")
		}
	}
}
```

**Step 2: Add test helper to client package**

Add to `internal/client/client.go`:

```go
// NewForTest creates a client for testing with a custom HTTP client.
func NewForTest(baseURL string, httpClient *http.Client, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		token:      token,
		sem:        make(chan struct{}, 10),
	}
}
```

**Step 3: Run tests, verify they fail**

```bash
go test ./internal/backup/ -v
```

**Step 4: Implement backup.go**

```go
// internal/backup/backup.go
package backup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/refs"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/sanitize"
)

// Options configures a backup run.
type Options struct {
	Namespace string
	OutputDir string
	Resources []registry.Resource
}

// Result holds backup results.
type Result struct {
	ObjectCount     int
	ResourceCounts  map[string]int
	SharedRefs      []refs.SharedRef
	SkippedChildren []string
	Warnings        []string
	Errors          []string
}

// Run executes a namespace backup.
func Run(c *client.Client, opts *Options) (*Result, error) {
	result := &Result{
		ResourceCounts: make(map[string]int),
	}

	// Build set of view-managed kinds for smart skip
	managedKinds := make(map[string]bool)
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			managedKinds[r.Kind] = true
		}
	}

	// Filter out view-managed resources
	var resources []registry.Resource
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			slog.Info("skipping view-managed resource", "kind", r.Kind, "managed_by", r.ManagedBy)
			continue
		}
		resources = append(resources, r)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, res := range resources {
		wg.Add(1)
		go func(res registry.Resource) {
			defer wg.Done()

			listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.Namespace)
			slog.Info("listing resources", "kind", res.Kind, "path", listPath)

			items, err := c.List(listPath)
			if err != nil {
				slog.Warn("failed to list", "kind", res.Kind, "error", err)
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("list %s: %v", res.Kind, err))
				mu.Unlock()
				return
			}

			if len(items) == 0 {
				return
			}

			// Create directory for this resource type
			typeDir := filepath.Join(opts.OutputDir, res.Kind)
			if err := os.MkdirAll(typeDir, 0755); err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("mkdir %s: %v", res.Kind, err))
				mu.Unlock()
				return
			}

			for _, item := range items {
				md, _ := item["metadata"].(map[string]any)
				name, _ := md["name"].(string)
				if name == "" {
					continue
				}

				// Get full object
				getPath := listPath + "/" + name
				obj, err := c.Get(getPath)
				if err != nil {
					slog.Warn("failed to get", "kind", res.Kind, "name", name, "error", err)
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("get %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				// Detect shared refs
				sharedRefs := refs.FindSharedRefs(res.Kind, name, obj)

				// Sanitize for backup
				clean := sanitize.ForBackup(obj)

				// Write to file
				data, err := json.MarshalIndent(clean, "", "  ")
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("marshal %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				filePath := filepath.Join(typeDir, name+".json")
				if err := os.WriteFile(filePath, data, 0644); err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("write %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				mu.Lock()
				result.ObjectCount++
				result.ResourceCounts[res.Kind]++
				result.SharedRefs = append(result.SharedRefs, sharedRefs...)
				for _, ref := range sharedRefs {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s/%s references shared/%s (%s)", ref.SourceKind, ref.SourceName, ref.TargetName, ref.FieldPath))
				}
				mu.Unlock()

				slog.Debug("backed up", "kind", res.Kind, "name", name)
			}
		}(res)
	}

	wg.Wait()

	// Write manifest
	m := &manifest.Manifest{
		Version:             "1",
		ToolVersion:         "0.1.0",
		Tenant:              c.BaseURL(),
		Namespace:           opts.Namespace,
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
		ResourceCounts:      result.ResourceCounts,
		SkippedViewChildren: result.SkippedChildren,
		SharedReferences:    result.SharedRefs,
		Warnings:            result.Warnings,
		Errors:              result.Errors,
	}

	if err := manifest.Write(opts.OutputDir, m); err != nil {
		return result, fmt.Errorf("writing manifest: %w", err)
	}

	return result, nil
}
```

**Step 5: Run tests, verify they pass**

```bash
go test ./internal/backup/ -v
```

**Step 6: Wire up runBackup in main.go**

Update the `runBackup` function in `cmd/xcbackup/main.go` to parse flags, create the client, resolve the resource list, create the output directory, call `backup.Run`, and print results. This connects all the pieces.

```go
func runBackup(cmd *cobra.Command, args []string) error {
	tenant, _ := cmd.Flags().GetString("tenant")
	namespace, _ := cmd.Flags().GetString("namespace")
	token, _ := cmd.Flags().GetString("token")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	parallel, _ := cmd.Flags().GetInt("parallel")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	types, _ := cmd.Flags().GetStringSlice("types")
	excludeTypes, _ := cmd.Flags().GetStringSlice("exclude-types")

	if tenant == "" || namespace == "" {
		return fmt.Errorf("--tenant and --namespace are required")
	}

	// Resolve token from env if not set
	if token == "" {
		token = os.Getenv("XC_API_TOKEN")
	}
	if token == "" && certFile == "" {
		return fmt.Errorf("provide --token (or XC_API_TOKEN) or --cert/--key")
	}

	// Build client
	var opts []client.Option
	if token != "" {
		opts = append(opts, client.WithToken(token))
	}
	if certFile != "" && keyFile != "" {
		opts = append(opts, client.WithCert(certFile, keyFile))
	}
	opts = append(opts, client.WithParallel(parallel))
	c := client.New(tenant, opts...)

	// Resolve resources
	resources := registry.All()
	if len(types) > 0 {
		resources = registry.FilterByKinds(resources, types)
	}
	if len(excludeTypes) > 0 {
		resources = registry.ExcludeKinds(resources, excludeTypes)
	}

	// Default output dir
	if outputDir == "" {
		outputDir = fmt.Sprintf("backup-%s-%s", namespace, time.Now().UTC().Format("2006-01-02T15-04-05Z"))
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	fmt.Printf("Backing up namespace %q from %s\n", namespace, c.BaseURL())
	fmt.Printf("Output: %s\n\n", outputDir)

	result, err := backup.Run(c, &backup.Options{
		Namespace: namespace,
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		return err
	}

	// Print results
	fmt.Printf("\nBackup complete: %d objects\n", result.ObjectCount)
	for kind, count := range result.ResourceCounts {
		fmt.Printf("  %-30s %d\n", kind, count)
	}
	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
	}

	return nil
}
```

Don't forget to add the necessary imports to main.go:
```go
import (
	"fmt"
	"os"
	"time"

	"github.com/kevingstewart/xcbackup/internal/backup"
	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/spf13/cobra"
)
```

**Step 7: Build and verify**

```bash
make build
./bin/xcbackup backup --help
```

**Step 8: Commit**

```bash
git add internal/backup/ cmd/xcbackup/main.go
git commit -m "feat: backup command with parallel API fetching, sanitization, and shared ref detection"
```

---

## Task 8: Inspect Command

**Files:**
- Create: `internal/inspect/inspect.go`
- Create: `internal/inspect/inspect_test.go`
- Modify: `cmd/xcbackup/main.go` — wire up `runInspect`

**Step 1: Write tests**

```go
// internal/inspect/inspect_test.go
package inspect

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/refs"
)

func TestRun_PrintsManifestSummary(t *testing.T) {
	dir := t.TempDir()

	m := &manifest.Manifest{
		Version:     "1",
		ToolVersion: "0.1.0",
		Tenant:      "acme.console.ves.volterra.io",
		Namespace:   "prod",
		Timestamp:   "2026-02-25T13:15:00Z",
		ResourceCounts: map[string]int{
			"healthcheck":       2,
			"http-loadbalancer": 1,
		},
		SharedReferences: []refs.SharedRef{
			{SourceKind: "http-loadbalancer", SourceName: "main-lb", TargetName: "default-waf", FieldPath: "spec.app_firewall"},
		},
		Warnings: []string{"http-loadbalancer/main-lb references shared/app-firewall/default-waf"},
	}
	manifest.Write(dir, m)

	// Create the resource directories so inspect can count files
	os.MkdirAll(filepath.Join(dir, "healthcheck"), 0755)
	os.WriteFile(filepath.Join(dir, "healthcheck", "hc1.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "healthcheck", "hc2.json"), []byte("{}"), 0644)
	os.MkdirAll(filepath.Join(dir, "http-loadbalancer"), 0755)
	os.WriteFile(filepath.Join(dir, "http-loadbalancer", "main-lb.json"), []byte("{}"), 0644)

	var buf bytes.Buffer
	err := Run(dir, &buf)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("acme.console.ves.volterra.io")) {
		t.Error("output should contain tenant URL")
	}
	if !bytes.Contains([]byte(output), []byte("prod")) {
		t.Error("output should contain namespace")
	}
	if !bytes.Contains([]byte(output), []byte("healthcheck")) {
		t.Error("output should contain resource types")
	}
	if !bytes.Contains([]byte(output), []byte("shared")) {
		t.Error("output should contain shared reference warnings")
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/inspect/ -v
```

**Step 3: Implement inspect.go**

```go
// internal/inspect/inspect.go
package inspect

import (
	"fmt"
	"io"
	"sort"

	"github.com/kevingstewart/xcbackup/internal/manifest"
)

// Run reads a backup directory and prints a summary.
func Run(dir string, w io.Writer) error {
	m, err := manifest.Read(dir)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Backup: %s\n", dir)
	fmt.Fprintf(w, "Tenant: %s\n", m.Tenant)
	fmt.Fprintf(w, "Namespace: %s\n", m.Namespace)
	fmt.Fprintf(w, "Timestamp: %s\n", m.Timestamp)
	fmt.Fprintf(w, "Tool Version: %s\n\n", m.ToolVersion)

	// Sort resource types for consistent output
	kinds := make([]string, 0, len(m.ResourceCounts))
	for k := range m.ResourceCounts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	total := 0
	fmt.Fprintf(w, "Resources:\n")
	for _, k := range kinds {
		count := m.ResourceCounts[k]
		total += count
		fmt.Fprintf(w, "  %-35s %d\n", k, count)
	}
	fmt.Fprintf(w, "  %-35s ──\n", "")
	fmt.Fprintf(w, "  %-35s %d\n\n", "Total", total)

	if len(m.SkippedViewChildren) > 0 {
		fmt.Fprintf(w, "Skipped view-managed children:\n")
		for _, s := range m.SkippedViewChildren {
			fmt.Fprintf(w, "  - %s\n", s)
		}
		fmt.Fprintln(w)
	}

	if len(m.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for _, w2 := range m.Warnings {
			fmt.Fprintf(w, "  ⚠ %s\n", w2)
		}
		fmt.Fprintln(w)
	}

	if len(m.Errors) > 0 {
		fmt.Fprintf(w, "Errors:\n")
		for _, e := range m.Errors {
			fmt.Fprintf(w, "  ✗ %s\n", e)
		}
		fmt.Fprintln(w)
	}

	return nil
}
```

**Step 4: Run tests, verify they pass**

```bash
go test ./internal/inspect/ -v
```

**Step 5: Wire up runInspect in main.go**

```go
func runInspect(cmd *cobra.Command, args []string) error {
	return inspect.Run(args[0], os.Stdout)
}
```

Add `"github.com/kevingstewart/xcbackup/internal/inspect"` to imports.

**Step 6: Build and verify**

```bash
make build
./bin/xcbackup inspect --help
```

**Step 7: Commit**

```bash
git add internal/inspect/ cmd/xcbackup/main.go
git commit -m "feat: inspect command to display backup contents and warnings"
```

---

## Task 9: Restore Command

**Files:**
- Create: `internal/restore/restore.go`
- Create: `internal/restore/restore_test.go`
- Modify: `cmd/xcbackup/main.go` — wire up `runRestore`

**Step 1: Write tests**

```go
// internal/restore/restore_test.go
package restore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/registry"
)

func setupTestBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Write manifest
	m := &manifest.Manifest{
		Version:   "1",
		Tenant:    "https://test.console.ves.volterra.io",
		Namespace: "prod",
		Timestamp: "2026-02-25T13:15:00Z",
		ResourceCounts: map[string]int{
			"healthcheck": 1,
			"origin-pool": 1,
		},
	}
	manifest.Write(dir, m)

	// Write healthcheck
	os.MkdirAll(filepath.Join(dir, "healthcheck"), 0755)
	hc := map[string]any{
		"metadata": map[string]any{"name": "hc1", "namespace": "prod"},
		"spec":     map[string]any{"timeout": 3},
	}
	data, _ := json.MarshalIndent(hc, "", "  ")
	os.WriteFile(filepath.Join(dir, "healthcheck", "hc1.json"), data, 0644)

	// Write origin pool
	os.MkdirAll(filepath.Join(dir, "origin-pool"), 0755)
	pool := map[string]any{
		"metadata": map[string]any{"name": "pool1", "namespace": "prod"},
		"spec":     map[string]any{"port": 8080},
	}
	data, _ = json.MarshalIndent(pool, "", "  ")
	os.WriteFile(filepath.Join(dir, "origin-pool", "pool1.json"), data, 0644)

	return dir
}

func TestRun_RestoresInTierOrder(t *testing.T) {
	backupDir := setupTestBackup(t)

	var mu sync.Mutex
	var createOrder []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			mu.Lock()
			createOrder = append(createOrder, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		// GET for conflict check — return 404 (object doesn't exist)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"code": 5})
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")

	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "skip",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}

	// healthcheck (tier 1) should be created before origin-pool (tier 2)
	if len(createOrder) != 2 {
		t.Fatalf("expected 2 creates, got %d", len(createOrder))
	}
	// First create should be healthcheck path
	if createOrder[0] != "/api/config/namespaces/restored-ns/healthchecks" {
		t.Errorf("first create should be healthcheck, got %s", createOrder[0])
	}
}

func TestRun_DryRun(t *testing.T) {
	backupDir := setupTestBackup(t)

	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		DryRun:          true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if requestCount != 0 {
		t.Errorf("dry run should make 0 API requests, got %d", requestCount)
	}
	if result.Created != 0 {
		t.Error("dry run should not create objects")
	}
}

func TestRun_SkipsExisting(t *testing.T) {
	backupDir := setupTestBackup(t)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET returns 200 = object exists
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "existing"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "skip",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
}
```

**Step 2: Run tests, verify they fail**

```bash
go test ./internal/restore/ -v
```

**Step 3: Implement restore.go**

```go
// internal/restore/restore.go
package restore

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/sanitize"
)

// Options configures a restore run.
type Options struct {
	BackupDir       string
	TargetNamespace string
	Resources       []registry.Resource
	DryRun          bool
	OnConflict      string // "skip", "overwrite", "fail"
}

// Result holds restore results.
type Result struct {
	Created int
	Skipped int
	Updated int
	Failed  int
	Errors  []string
}

// Run executes a namespace restore from a backup directory.
func Run(c *client.Client, opts *Options) (*Result, error) {
	result := &Result{}

	// Filter to only standalone (non-view-managed) resources that have backup files
	var resources []registry.Resource
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			continue
		}
		typeDir := filepath.Join(opts.BackupDir, r.Kind)
		if _, err := os.Stat(typeDir); err == nil {
			resources = append(resources, r)
		}
	}

	// Group by tier, restore tier by tier
	tiers := registry.Tiers(resources)

	for _, tier := range tiers {
		tierResources := registry.FilterByTier(resources, tier)
		slog.Info("restoring tier", "tier", tier, "resource_types", len(tierResources))

		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, res := range tierResources {
			wg.Add(1)
			go func(res registry.Resource) {
				defer wg.Done()

				typeDir := filepath.Join(opts.BackupDir, res.Kind)
				entries, err := os.ReadDir(typeDir)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("read dir %s: %v", res.Kind, err))
					result.Failed++
					mu.Unlock()
					return
				}

				for _, entry := range entries {
					if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
						continue
					}

					data, err := os.ReadFile(filepath.Join(typeDir, entry.Name()))
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("read %s/%s: %v", res.Kind, entry.Name(), err))
						result.Failed++
						mu.Unlock()
						continue
					}

					var obj map[string]any
					if err := json.Unmarshal(data, &obj); err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("parse %s/%s: %v", res.Kind, entry.Name(), err))
						result.Failed++
						mu.Unlock()
						continue
					}

					// Get the object name
					md, _ := obj["metadata"].(map[string]any)
					name, _ := md["name"].(string)

					if opts.DryRun {
						slog.Info("dry run: would create", "kind", res.Kind, "name", name)
						continue
					}

					// Prepare for restore (set target namespace)
					restored := sanitize.ForRestore(obj, opts.TargetNamespace)

					listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.TargetNamespace)
					getPath := listPath + "/" + name

					// Check if object already exists
					_, err = c.Get(getPath)
					exists := err == nil

					if exists {
						switch opts.OnConflict {
						case "skip":
							slog.Info("skipping existing", "kind", res.Kind, "name", name)
							mu.Lock()
							result.Skipped++
							mu.Unlock()
							continue
						case "overwrite":
							if err := c.Replace(getPath, restored); err != nil {
								mu.Lock()
								result.Errors = append(result.Errors, fmt.Sprintf("replace %s/%s: %v", res.Kind, name, err))
								result.Failed++
								mu.Unlock()
								continue
							}
							mu.Lock()
							result.Updated++
							mu.Unlock()
							slog.Info("updated", "kind", res.Kind, "name", name)
							continue
						case "fail":
							mu.Lock()
							result.Errors = append(result.Errors, fmt.Sprintf("%s/%s already exists", res.Kind, name))
							result.Failed++
							mu.Unlock()
							continue
						}
					}

					// Create the object
					if err := c.Create(listPath, restored); err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("create %s/%s: %v", res.Kind, name, err))
						result.Failed++
						mu.Unlock()
						continue
					}

					mu.Lock()
					result.Created++
					mu.Unlock()
					slog.Info("created", "kind", res.Kind, "name", name)
				}
			}(res)
		}

		wg.Wait()
	}

	return result, nil
}
```

**Step 4: Run tests, verify they pass**

```bash
go test ./internal/restore/ -v
```

**Step 5: Wire up runRestore in main.go**

```go
func runRestore(cmd *cobra.Command, args []string) error {
	tenant, _ := cmd.Flags().GetString("tenant")
	namespace, _ := cmd.Flags().GetString("namespace")
	token, _ := cmd.Flags().GetString("token")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	parallel, _ := cmd.Flags().GetInt("parallel")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	onConflict, _ := cmd.Flags().GetString("on-conflict")
	types, _ := cmd.Flags().GetStringSlice("types")

	if tenant == "" || namespace == "" {
		return fmt.Errorf("--tenant and --namespace are required")
	}

	if token == "" {
		token = os.Getenv("XC_API_TOKEN")
	}
	if token == "" && certFile == "" {
		return fmt.Errorf("provide --token (or XC_API_TOKEN) or --cert/--key")
	}

	var opts []client.Option
	if token != "" {
		opts = append(opts, client.WithToken(token))
	}
	if certFile != "" && keyFile != "" {
		opts = append(opts, client.WithCert(certFile, keyFile))
	}
	opts = append(opts, client.WithParallel(parallel))
	c := client.New(tenant, opts...)

	resources := registry.All()
	if len(types) > 0 {
		resources = registry.FilterByKinds(resources, types)
	}

	backupDir := args[0]

	if dryRun {
		fmt.Println("DRY RUN — no changes will be made\n")
	}

	fmt.Printf("Restoring to namespace %q on %s\n", namespace, c.BaseURL())
	fmt.Printf("From backup: %s\n\n", backupDir)

	result, err := restore.Run(c, &restore.Options{
		BackupDir:       backupDir,
		TargetNamespace: namespace,
		Resources:       resources,
		DryRun:          dryRun,
		OnConflict:      onConflict,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nRestore complete:\n")
	fmt.Printf("  Created:  %d\n", result.Created)
	fmt.Printf("  Updated:  %d\n", result.Updated)
	fmt.Printf("  Skipped:  %d\n", result.Skipped)
	fmt.Printf("  Failed:   %d\n", result.Failed)

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
	}

	return nil
}
```

Add `"github.com/kevingstewart/xcbackup/internal/restore"` to imports.

**Step 6: Build and verify**

```bash
make build
./bin/xcbackup restore --help
```

**Step 7: Commit**

```bash
git add internal/restore/ cmd/xcbackup/main.go
git commit -m "feat: restore command with tier-ordered creation, dry-run, and conflict handling"
```

---

## Task 10: Verify API Path Accuracy

**Files:**
- Modify: `internal/registry/resources.go` — fix any incorrect pluralizations

The resource registry's API paths are critical. Many are based on research, but some pluralizations may be wrong (F5 XC is notoriously inconsistent: `service_policys`, `cloud_credentialss`, etc.).

**Step 1: Write a validation test that checks path patterns**

```go
// internal/registry/registry_test.go (add to existing file)

func TestAllResources_PathsHaveNamespacePlaceholder(t *testing.T) {
	for _, r := range All() {
		if !strings.Contains(r.ListPath, "{namespace}") {
			t.Errorf("resource %q ListPath missing {namespace}: %s", r.Kind, r.ListPath)
		}
	}
}

func TestAllResources_PathsStartWithAPI(t *testing.T) {
	for _, r := range All() {
		if !strings.HasPrefix(r.ListPath, "/api/") {
			t.Errorf("resource %q ListPath doesn't start with /api/: %s", r.Kind, r.ListPath)
		}
	}
}
```

**Step 2: Run all tests**

```bash
go test ./... -v
```

**Step 3: If any path is discovered to be wrong during real testing, update resources.go**

This task is inherently iterative — the exact plural forms can only be fully validated against a live tenant or the OAS. The registry is structured to make these corrections trivial (just change the string).

**Step 4: Commit any fixes**

```bash
git add internal/registry/
git commit -m "fix: verify and correct API path pluralizations in registry"
```

---

## Task 11: Final Integration — Build, Test, Tidy

**Step 1: Run go mod tidy**

```bash
go mod tidy
```

**Step 2: Run all tests**

```bash
make test
```

Expected: All tests pass.

**Step 3: Build final binary**

```bash
make build
./bin/xcbackup --version
```

**Step 4: Add .goreleaser.yml for future releases (optional but lightweight)**

```yaml
# .goreleaser.yml
version: 2
builds:
  - main: ./cmd/xcbackup
    binary: xcbackup
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - formats: ['tar.gz']
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
```

**Step 5: Final commit**

```bash
git add go.mod go.sum .goreleaser.yml
git commit -m "chore: tidy dependencies and add goreleaser config"
```

---

## Summary of Commits

| # | Commit Message | What it adds |
|---|----------------|-------------|
| 1 | `feat: project scaffolding with cobra CLI skeleton` | go.mod, main.go, Makefile |
| 2 | `feat: API client with token/mTLS auth and concurrency limiting` | internal/client/ |
| 3 | `feat: resource registry with ~99 namespace-scoped XC resource types` | internal/registry/ |
| 4 | `feat: object sanitization for backup and restore` | internal/sanitize/ |
| 5 | `feat: shared namespace reference detection` | internal/refs/ |
| 6 | `feat: manifest read/write with shared ref and view child tracking` | internal/manifest/ |
| 7 | `feat: backup command with parallel API fetching` | internal/backup/ + main.go wiring |
| 8 | `feat: inspect command to display backup contents` | internal/inspect/ + main.go wiring |
| 9 | `feat: restore command with tier-ordered creation` | internal/restore/ + main.go wiring |
| 10 | `fix: verify and correct API path pluralizations` | registry fixes |
| 11 | `chore: tidy dependencies and add goreleaser config` | go.mod, .goreleaser.yml |
