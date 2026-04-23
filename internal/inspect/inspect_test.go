package inspect

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/manifest"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/refs"
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
