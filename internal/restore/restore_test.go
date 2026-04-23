package restore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/manifest"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
)

func setupTestBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

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

	os.MkdirAll(filepath.Join(dir, "healthcheck"), 0755)
	hc := map[string]any{
		"metadata": map[string]any{"name": "hc1", "namespace": "prod"},
		"spec":     map[string]any{"timeout": 3},
	}
	data, _ := json.MarshalIndent(hc, "", "  ")
	os.WriteFile(filepath.Join(dir, "healthcheck", "hc1.json"), data, 0644)

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
		// GET for conflict check — return 404
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

	// healthcheck (tier 1) should be before origin-pool (tier 2)
	if len(createOrder) != 2 {
		t.Fatalf("expected 2 creates, got %d", len(createOrder))
	}
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
