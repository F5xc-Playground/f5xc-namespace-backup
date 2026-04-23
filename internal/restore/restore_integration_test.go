package restore

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/client/testutil"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/registry"
)

func writeBackupDir(t *testing.T, objects map[string]map[string]map[string]any) string {
	t.Helper()
	dir := t.TempDir()

	m := &manifest.Manifest{
		Version:        "1",
		Tenant:         "https://test.console.ves.volterra.io",
		Namespace:      "test-ns",
		Timestamp:      "2026-04-23T00:00:00Z",
		ResourceCounts: make(map[string]int),
	}

	for kind, nameMap := range objects {
		typeDir := filepath.Join(dir, kind)
		os.MkdirAll(typeDir, 0755)
		m.ResourceCounts[kind] = len(nameMap)
		for name, obj := range nameMap {
			data, _ := json.MarshalIndent(obj, "", "  ")
			os.WriteFile(filepath.Join(typeDir, name+".json"), data, 0644)
		}
	}

	manifest.Write(dir, m)
	return dir
}

func TestIntegration_RestoreCreatesObjects(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
		"origin-pool": {
			"pool1": {"metadata": map[string]any{"name": "pool1", "namespace": "test-ns"}, "spec": map[string]any{"port": float64(8080)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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

	reqs := srv.Requests()
	posts := 0
	for _, r := range reqs {
		if r.Method == "POST" {
			posts++
		}
	}
	if posts != 2 {
		t.Errorf("POST requests = %d, want 2", posts)
	}
}

func TestIntegration_RestoreTierOrder(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
		"origin-pool": {
			"pool1": {"metadata": map[string]any{"name": "pool1", "namespace": "test-ns"}, "spec": map[string]any{"port": float64(8080)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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
		t.Fatalf("Created = %d, want 2", result.Created)
	}

	var postPaths []string
	for _, r := range srv.Requests() {
		if r.Method == "POST" {
			postPaths = append(postPaths, r.Path)
		}
	}
	if len(postPaths) != 2 {
		t.Fatalf("POST requests = %d, want 2", len(postPaths))
	}
	if !strings.Contains(postPaths[0], "healthchecks") {
		t.Errorf("first POST should be healthcheck (tier 1), got %s", postPaths[0])
	}
	if !strings.Contains(postPaths[1], "origin_pools") {
		t.Errorf("second POST should be origin-pool (tier 2), got %s", postPaths[1])
	}
}

func TestIntegration_RestoreConflictSkip(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "restored-ns", "hc1", map[string]any{"timeout": 99})

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "skip",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Created != 0 {
		t.Errorf("Created = %d, want 0", result.Created)
	}

	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			t.Error("skip mode should not make PUT requests")
		}
	}
}

func TestIntegration_RestoreConflictOverwrite(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "restored-ns", "hc1", map[string]any{"timeout": 99})

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "overwrite",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}

	puts := 0
	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			puts++
		}
	}
	if puts != 1 {
		t.Errorf("PUT requests = %d, want 1", puts)
	}
}

func TestIntegration_RestoreConflictFail(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "restored-ns", "hc1", map[string]any{"timeout": 99})

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "fail",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
}

func TestIntegration_RestoreErrorInjection(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("POST", "healthchecks", "restored-ns", "", testutil.ErrorSpec{
		StatusCode: 500,
		Body:       `{"message":"internal server error"}`,
	})

	backupDir := writeBackupDir(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": {"metadata": map[string]any{"name": "hc1", "namespace": "test-ns"}, "spec": map[string]any{"timeout": float64(3)}},
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	result, err := Run(c, &Options{
		BackupDir:       backupDir,
		TargetNamespace: "restored-ns",
		Resources:       resources,
		OnConflict:      "skip",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error message")
	}
}
