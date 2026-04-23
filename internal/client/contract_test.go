//go:build contract

package client

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func contractClient(t *testing.T) *Client {
	t.Helper()
	tenantURL := os.Getenv("XC_TENANT_URL")
	token := os.Getenv("XC_API_TOKEN")
	if tenantURL == "" || token == "" {
		t.Skip("XC_TENANT_URL and XC_API_TOKEN must be set for contract tests")
	}
	return New(tenantURL, WithToken(token))
}

func contractNamespace(t *testing.T) string {
	t.Helper()
	ns := os.Getenv("XC_TEST_NAMESPACE")
	if ns == "" {
		ns = "backup-test"
	}
	return ns
}

func TestContract_ListNamespaces(t *testing.T) {
	c := contractClient(t)

	items, err := c.List("/api/web/namespaces")
	if err != nil {
		t.Fatalf("List namespaces: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected at least one namespace")
	}
}

func TestContract_CRUD_Healthcheck(t *testing.T) {
	c := contractClient(t)
	ns := contractNamespace(t)
	name := fmt.Sprintf("contract-test-hc-%d", time.Now().UnixMilli())
	basePath := fmt.Sprintf("/api/config/namespaces/%s/healthchecks", ns)
	objPath := basePath + "/" + name

	t.Cleanup(func() { _ = c.Delete(objPath) })
	_ = c.Delete(objPath)

	obj := map[string]any{
		"metadata": map[string]any{"name": name, "namespace": ns},
		"spec": map[string]any{
			"http_health_check":   map[string]any{"path": "/healthz"},
			"timeout":             3,
			"interval":            15,
			"unhealthy_threshold": 3,
			"healthy_threshold":   1,
		},
	}
	if err := c.Create(basePath, obj); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := c.Get(objPath)
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	md, _ := got["metadata"].(map[string]any)
	if md["name"] != name {
		t.Errorf("Get name = %v, want %s", md["name"], name)
	}

	obj["spec"].(map[string]any)["timeout"] = 10
	if err := c.Replace(objPath, obj); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	items, err := c.List(basePath)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, item := range items {
		if n, _ := item["name"].(string); n == name {
			found = true
			break
		}
		if imd, ok := item["metadata"].(map[string]any); ok {
			if n, _ := imd["name"].(string); n == name {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("created object not found in list response")
	}

	if err := c.Delete(objPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = c.Get(objPath)
	if err == nil {
		t.Fatal("Get after delete should return error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("status after delete = %d, want 404", apiErr.StatusCode)
	}
}

func TestContract_ListResources(t *testing.T) {
	c := contractClient(t)
	ns := contractNamespace(t)

	_, err := c.List(fmt.Sprintf("/api/config/namespaces/%s/healthchecks", ns))
	if err != nil {
		t.Fatalf("List healthchecks: %v", err)
	}
}

func TestContract_ErrorCodes(t *testing.T) {
	c := contractClient(t)
	ns := contractNamespace(t)

	_, err := c.Get(fmt.Sprintf("/api/config/namespaces/%s/healthchecks/nonexistent-object-xyz-999", ns))
	if err == nil {
		t.Fatal("Get nonexistent should return error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}
