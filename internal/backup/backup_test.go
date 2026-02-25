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
			json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		}
	}))
	defer server.Close()

	c := client.NewForTest(server.URL, server.Client(), "test-token")
	outputDir := t.TempDir()
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

	// Verify sanitization
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
