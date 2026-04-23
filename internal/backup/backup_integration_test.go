package backup

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/client/testutil"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/registry"
)

func TestIntegration_BackupFullWorkflow(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "test-ns", "hc1", map[string]any{"timeout": 3})
	srv.SeedObject("healthchecks", "test-ns", "hc2", map[string]any{"timeout": 5})
	srv.SeedObject("origin_pools", "test-ns", "pool1", map[string]any{"port": 8080})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	outputDir := t.TempDir()
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, err := Run(c, &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.ObjectCount != 3 {
		t.Errorf("ObjectCount = %d, want 3", result.ObjectCount)
	}
	if result.ResourceCounts["healthcheck"] != 2 {
		t.Errorf("healthcheck count = %d, want 2", result.ResourceCounts["healthcheck"])
	}
	if result.ResourceCounts["origin-pool"] != 1 {
		t.Errorf("origin-pool count = %d, want 1", result.ResourceCounts["origin-pool"])
	}

	for _, name := range []string{"hc1", "hc2"} {
		path := filepath.Join(outputDir, "healthcheck", name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s.json not created: %v", name, err)
		}
	}
	poolPath := filepath.Join(outputDir, "origin-pool", "pool1.json")
	if _, err := os.Stat(poolPath); err != nil {
		t.Errorf("pool1.json not created: %v", err)
	}

	m, err := manifest.Read(outputDir)
	if err != nil {
		t.Fatalf("manifest.Read() error: %v", err)
	}
	if m.Namespace != "test-ns" {
		t.Errorf("manifest namespace = %q, want test-ns", m.Namespace)
	}

	hcData, _ := os.ReadFile(filepath.Join(outputDir, "healthcheck", "hc1.json"))
	var hcObj map[string]any
	json.Unmarshal(hcData, &hcObj)
	if _, ok := hcObj["system_metadata"]; ok {
		t.Error("system_metadata should be stripped from backup")
	}
}

func TestIntegration_BackupErrorHandling(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("GET", "healthchecks", "test-ns", "", testutil.ErrorSpec{
		StatusCode: 403,
		Body:       `{"message":"forbidden"}`,
	})
	srv.InjectError("GET", "origin_pools", "test-ns", "", testutil.ErrorSpec{
		StatusCode: 501,
		Body:       `{"message":"not implemented"}`,
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	outputDir := t.TempDir()
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck", "origin-pool"})

	result, err := Run(c, &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() should not return error for 403/501, got: %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Errorf("Warnings = %d, want 2 (got: %v)", len(result.Warnings), result.Warnings)
	}
	if result.ObjectCount != 0 {
		t.Errorf("ObjectCount = %d, want 0", result.ObjectCount)
	}
}

func TestIntegration_BackupAuthFailure(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("GET", "healthchecks", "test-ns", "", testutil.ErrorSpec{
		StatusCode: 401,
		Body:       `{"message":"unauthenticated"}`,
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "bad-token")
	outputDir := t.TempDir()
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	_, err := Run(c, &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	})
	if err == nil {
		t.Fatal("Run() should return error for 401")
	}
}

func TestIntegration_BackupNamespaceFiltering(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "test-ns", "hc1", map[string]any{"timeout": 3})
	srv.SeedObject("healthchecks", "shared", "shared-hc", map[string]any{"timeout": 10})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	outputDir := t.TempDir()
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.ObjectCount != 1 {
		t.Errorf("ObjectCount = %d, want 1 (shared object should be filtered)", result.ObjectCount)
	}

	sharedPath := filepath.Join(outputDir, "healthcheck", "shared-hc.json")
	if _, err := os.Stat(sharedPath); err == nil {
		t.Error("shared-hc.json should not exist in backup")
	}
}

func TestIntegration_BackupViewOwnedFiltering(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "test-ns", "regular-hc", map[string]any{"timeout": 3})
	srv.SeedObjectWithSystemMetadata("healthchecks", "test-ns", "view-owned-hc",
		map[string]any{"timeout": 5},
		map[string]any{
			"owner_view": map[string]any{
				"kind": "http_loadbalancer",
				"name": "my-lb",
			},
		},
	)

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	outputDir := t.TempDir()
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		Namespace: "test-ns",
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.ObjectCount != 1 {
		t.Errorf("ObjectCount = %d, want 1 (view-owned should be filtered)", result.ObjectCount)
	}

	viewPath := filepath.Join(outputDir, "healthcheck", "view-owned-hc.json")
	if _, err := os.Stat(viewPath); err == nil {
		t.Error("view-owned-hc.json should not exist in backup")
	}
}
