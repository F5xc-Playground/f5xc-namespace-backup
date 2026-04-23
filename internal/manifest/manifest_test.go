package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/refs"
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
