package diff

import (
	"net/http"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client/testutil"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
)

func TestIntegration_DiffNoDrift(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(3)})

	backupObj := makeObj("hc1", "prod", map[string]any{"timeout": float64(3)})
	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {"hc1": backupObj},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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

func TestIntegration_DiffAllCategories(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(99)})
	srv.SeedObject("healthchecks", "prod", "hc3", map[string]any{"timeout": float64(7)})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": makeObj("hc1", "prod", map[string]any{"timeout": float64(3)}),
			"hc2": makeObj("hc2", "prod", map[string]any{"timeout": float64(5)}),
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
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
		t.Errorf("Added = %d, want 1", len(report.Added))
	} else if report.Added[0].Name != "hc3" {
		t.Errorf("Added[0].Name = %q, want hc3", report.Added[0].Name)
	}
	if len(report.Removed) != 1 {
		t.Errorf("Removed = %d, want 1", len(report.Removed))
	} else if report.Removed[0].Name != "hc2" {
		t.Errorf("Removed[0].Name = %q, want hc2", report.Removed[0].Name)
	}
	if len(report.Modified) != 1 {
		t.Errorf("Modified = %d, want 1", len(report.Modified))
	} else {
		if report.Modified[0].Name != "hc1" {
			t.Errorf("Modified[0].Name = %q, want hc1", report.Modified[0].Name)
		}
		if report.Modified[0].UnifiedDiff == "" {
			t.Error("Modified[0].UnifiedDiff should not be empty")
		}
	}
}

func TestIntegration_DiffErrorInjection(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("GET", "healthchecks", "prod", "", testutil.ErrorSpec{
		StatusCode: 403,
		Body:       `{"message":"forbidden"}`,
	})

	backupDir := setupTestBackup(t, map[string]map[string]map[string]any{
		"healthcheck": {
			"hc1": makeObj("hc1", "prod", map[string]any{"timeout": float64(3)}),
		},
	})

	c := client.NewForTest(srv.URL(), &http.Client{}, "test-token")
	resources := registry.FilterByKinds(registry.All(), []string{"healthcheck"})

	report, err := Run(c, &Options{
		BackupDir: backupDir,
		Namespace: "prod",
		Resources: resources,
	})
	if err != nil {
		t.Fatalf("Run() should not return hard error for 403, got: %v", err)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("Warnings = %d, want 1", len(report.Warnings))
	}
}
