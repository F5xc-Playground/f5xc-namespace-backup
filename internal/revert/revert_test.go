package revert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
)

func setupTestBackup(t *testing.T, objects map[string]map[string]map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	for kind, nameMap := range objects {
		typeDir := filepath.Join(dir, kind)
		os.MkdirAll(typeDir, 0755)
		for name, obj := range nameMap {
			data, _ := json.MarshalIndent(obj, "", "  ")
			os.WriteFile(filepath.Join(typeDir, name+".json"), data, 0644)
		}
	}
	return dir
}

func makeObj(name, ns string, spec map[string]any) map[string]any {
	return map[string]any{
		"metadata": map[string]any{"name": name, "namespace": ns},
		"spec":     spec,
	}
}

func TestRun_ReplacesModified(t *testing.T) {
	backupObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	liveObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(10)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": backupObj},
	})

	var mu sync.Mutex
	var methods []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()

		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks/hc1"):
			json.NewEncoder(w).Encode(liveObj)
		case r.Method == "PUT":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1", result.Replaced)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", result.Skipped)
	}

	// Verify a PUT was made
	hasPut := false
	for _, m := range methods {
		if strings.HasPrefix(m, "PUT") {
			hasPut = true
		}
	}
	if !hasPut {
		t.Error("expected a PUT request for replace")
	}
}

func TestRun_CreatesRemoved(t *testing.T) {
	// hc1 is in backup but not live
	backupObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": backupObj},
	})

	var mu sync.Mutex
	var posts []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			// Empty live list
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		case r.Method == "POST":
			mu.Lock()
			posts = append(posts, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if len(posts) != 1 {
		t.Errorf("expected 1 POST, got %d", len(posts))
	}
}

func TestRun_DeletesExtra(t *testing.T) {
	// Backup is empty, live has hc1 → should delete with DeleteExtra
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	liveObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})

	var mu sync.Mutex
	var deletes []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks/hc1"):
			json.NewEncoder(w).Encode(liveObj)
		case r.Method == "DELETE":
			mu.Lock()
			deletes = append(deletes, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
		DeleteExtra:     true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}
	if len(deletes) != 1 {
		t.Errorf("expected 1 DELETE, got %d", len(deletes))
	}
}

func TestRun_WarnsWithoutDeleteExtra(t *testing.T) {
	// Backup is empty, live has hc1 → should warn without DeleteExtra
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	liveObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks/hc1"):
			json.NewEncoder(w).Encode(liveObj)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
		DeleteExtra:     false,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", result.Deleted)
	}
	// Should have a warning about the extra object
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "hc1") && strings.Contains(w, "--delete-extra") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about extra object with --delete-extra hint")
	}
}

func TestRun_DryRun(t *testing.T) {
	backupObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	liveObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(10)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": backupObj},
	})

	var mu sync.Mutex
	var writes int

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" || r.Method == "POST" || r.Method == "DELETE" {
			mu.Lock()
			writes++
			mu.Unlock()
		}
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks/hc1"):
			json.NewEncoder(w).Encode(liveObj)
		default:
			w.WriteHeader(200)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
		DryRun:          true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if writes != 0 {
		t.Errorf("dry run made %d write requests, want 0", writes)
	}
	if result.Replaced != 1 {
		t.Errorf("dry run Replaced = %d, want 1 (should count what would be replaced)", result.Replaced)
	}
}

func TestRun_TierOrderedCreate(t *testing.T) {
	// Both healthcheck (tier 1) and origin-pool (tier 2) are removed
	hcObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	poolObj := makeObj("pool1", "prod", map[string]any{"port": float64(8080)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": hcObj},
		"origin-pool": {"pool1": poolObj},
	})

	var mu sync.Mutex
	var createOrder []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && (strings.HasSuffix(r.URL.Path, "/healthchecks") || strings.HasSuffix(r.URL.Path, "/origin_pools")):
			// Empty live — both are "removed"
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		case r.Method == "POST":
			mu.Lock()
			createOrder = append(createOrder, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}

	// Healthcheck (tier 1) should be created before origin-pool (tier 2)
	if len(createOrder) != 2 {
		t.Fatalf("expected 2 creates, got %d", len(createOrder))
	}
	if !strings.Contains(createOrder[0], "healthchecks") {
		t.Errorf("first create should be healthcheck, got %s", createOrder[0])
	}
	if !strings.Contains(createOrder[1], "origin_pools") {
		t.Errorf("second create should be origin-pool, got %s", createOrder[1])
	}
}

func TestRun_DeleteReverseTierOrder(t *testing.T) {
	// origin-pool (tier 2) and healthcheck (tier 1) added in live, backup empty
	// Should delete tier 2 first, then tier 1
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	hcObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	poolObj := makeObj("pool1", "prod", map[string]any{"port": float64(8080)})

	var mu sync.Mutex
	var deleteOrder []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/origin_pools"):
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "pool1", "namespace": "prod"}},
			})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/healthchecks/hc1"):
			json.NewEncoder(w).Encode(hcObj)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/origin_pools/pool1"):
			json.NewEncoder(w).Encode(poolObj)
		case r.Method == "DELETE":
			mu.Lock()
			deleteOrder = append(deleteOrder, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, _, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "prod",
		Resources:       resources,
		DeleteExtra:     true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Deleted != 2 {
		t.Errorf("Deleted = %d, want 2", result.Deleted)
	}

	// origin-pool (tier 2) should be deleted before healthcheck (tier 1)
	if len(deleteOrder) != 2 {
		t.Fatalf("expected 2 deletes, got %d", len(deleteOrder))
	}
	if !strings.Contains(deleteOrder[0], "origin_pools") {
		t.Errorf("first delete should be origin-pool (tier 2), got %s", deleteOrder[0])
	}
	if !strings.Contains(deleteOrder[1], "healthchecks") {
		t.Errorf("second delete should be healthcheck (tier 1), got %s", deleteOrder[1])
	}
}
