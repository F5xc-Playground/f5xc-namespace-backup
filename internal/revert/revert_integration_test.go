package revert

import (
	"net/http"
	"strings"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client/testutil"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
)

func TestIntegration_RevertModified(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(99)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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

func TestIntegration_RevertRecreatesRemoved(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})},
		"origin-pool": {"pool1": makeObj("pool1", "prod", map[string]any{"port": float64(8080)})},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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

func TestIntegration_RevertDeleteExtra(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(3)})
	srv.SeedObject("origin_pools", "prod", "pool1", map[string]any{"port": float64(8080)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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

	var deletePaths []string
	for _, r := range srv.Requests() {
		if r.Method == "DELETE" {
			deletePaths = append(deletePaths, r.Path)
		}
	}
	if len(deletePaths) != 2 {
		t.Fatalf("DELETE requests = %d, want 2", len(deletePaths))
	}
	if !strings.Contains(deletePaths[0], "origin_pools") {
		t.Errorf("first DELETE should be origin-pool (tier 2), got %s", deletePaths[0])
	}
	if !strings.Contains(deletePaths[1], "healthchecks") {
		t.Errorf("second DELETE should be healthcheck (tier 1), got %s", deletePaths[1])
	}
}

func TestIntegration_RevertDryRun(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(99)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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
	if result.Replaced != 1 {
		t.Errorf("Replaced = %d, want 1 (dry run should count what would be replaced)", result.Replaced)
	}

	for _, r := range srv.Requests() {
		if r.Method == "PUT" || r.Method == "POST" || r.Method == "DELETE" {
			t.Errorf("dry run should not make %s requests", r.Method)
		}
	}
}
