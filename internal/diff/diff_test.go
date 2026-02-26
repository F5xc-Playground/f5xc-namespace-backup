package diff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
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

func TestRun_DetectsUnchanged(t *testing.T) {
	obj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": obj},
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/prod/healthchecks":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case "/api/config/namespaces/prod/healthchecks/hc1":
			json.NewEncoder(w).Encode(obj)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if report.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", report.Unchanged)
	}
	if len(report.Added) != 0 {
		t.Errorf("Added = %d, want 0", len(report.Added))
	}
	if len(report.Removed) != 0 {
		t.Errorf("Removed = %d, want 0", len(report.Removed))
	}
	if len(report.Modified) != 0 {
		t.Errorf("Modified = %d, want 0", len(report.Modified))
	}
}

func TestRun_DetectsAdded(t *testing.T) {
	// Backup has hc1, live has hc1 + hc2
	obj1 := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": obj1},
	})

	obj2 := makeObj("hc2", "prod", map[string]any{"timeout": float64(5)})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/prod/healthchecks":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"name": "hc1", "namespace": "prod"},
					{"name": "hc2", "namespace": "prod"},
				},
			})
		case "/api/config/namespaces/prod/healthchecks/hc1":
			json.NewEncoder(w).Encode(obj1)
		case "/api/config/namespaces/prod/healthchecks/hc2":
			json.NewEncoder(w).Encode(obj2)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Added) != 1 {
		t.Fatalf("Added = %d, want 1", len(report.Added))
	}
	if report.Added[0].Name != "hc2" {
		t.Errorf("Added[0].Name = %q, want hc2", report.Added[0].Name)
	}
	if report.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", report.Unchanged)
	}
}

func TestRun_DetectsRemoved(t *testing.T) {
	// Backup has hc1 + hc2, live has hc1 only
	obj1 := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	obj2 := makeObj("hc2", "prod", map[string]any{"timeout": float64(5)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": obj1, "hc2": obj2},
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/prod/healthchecks":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case "/api/config/namespaces/prod/healthchecks/hc1":
			json.NewEncoder(w).Encode(obj1)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Removed) != 1 {
		t.Fatalf("Removed = %d, want 1", len(report.Removed))
	}
	if report.Removed[0].Name != "hc2" {
		t.Errorf("Removed[0].Name = %q, want hc2", report.Removed[0].Name)
	}
}

func TestRun_DetectsModified(t *testing.T) {
	backupObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": backupObj},
	})

	liveObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(10)})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/prod/healthchecks":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"name": "hc1", "namespace": "prod"}},
			})
		case "/api/config/namespaces/prod/healthchecks/hc1":
			json.NewEncoder(w).Encode(liveObj)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Modified) != 1 {
		t.Fatalf("Modified = %d, want 1", len(report.Modified))
	}
	if report.Modified[0].Name != "hc1" {
		t.Errorf("Modified[0].Name = %q, want hc1", report.Modified[0].Name)
	}
	if report.Modified[0].UnifiedDiff == "" {
		t.Error("Modified[0].UnifiedDiff should not be empty")
	}
}

func TestRun_FiltersOtherNamespace(t *testing.T) {
	obj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": obj},
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config/namespaces/prod/healthchecks":
			// Return items from both prod and shared namespaces
			json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"name": "hc1", "namespace": "prod"},
					{"name": "shared-hc", "namespace": "shared"},
				},
			})
		case "/api/config/namespaces/prod/healthchecks/hc1":
			json.NewEncoder(w).Encode(obj)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// shared-hc should be filtered out, not counted as Added
	if len(report.Added) != 0 {
		t.Errorf("Added = %d, want 0 (shared namespace objects should be filtered)", len(report.Added))
	}
	if report.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", report.Unchanged)
	}
}

func TestRun_SkipsManagedResources(t *testing.T) {
	// Include a managed resource — it should be filtered out
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(404)
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := []registry.Resource{
		{Kind: "virtual-host", Tier: 5, ManagedBy: "http-loadbalancer", ListPath: "/api/config/namespaces/{namespace}/virtual_hosts"},
	}

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("should not make API calls for managed resources, got %d", requestCount)
	}
	if len(report.Added) != 0 && len(report.Removed) != 0 {
		t.Error("should have no diffs for managed resources")
	}
}

func TestRun_HandlesInaccessible(t *testing.T) {
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"code": 7, "message": "forbidden"}`))
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("Warnings = %d, want 1", len(report.Warnings))
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := map[string]any{"spec": map[string]any{"timeout": float64(3)}}
	b := map[string]any{"spec": map[string]any{"timeout": float64(10)}}

	result := unifiedDiff("a.json", "b.json", a, b)
	if result == "" {
		t.Error("unifiedDiff should produce output for different objects")
	}
	if !contains(result, "--- a.json") {
		t.Error("diff should contain --- header")
	}
	if !contains(result, "+++ b.json") {
		t.Error("diff should contain +++ header")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
