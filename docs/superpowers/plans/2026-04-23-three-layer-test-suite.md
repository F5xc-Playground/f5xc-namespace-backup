# Three-Layer Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a reusable FakeXCServer, per-package integration tests, contract tests, and Makefile targets to establish three-layer testing matching the f5xc-k8s-operator pattern.

**Architecture:** FakeXCServer (httptest-based, in-memory CRUD store) lives in `internal/client/testutil/`. Integration tests in each workflow package use it for stateful multi-step testing. Contract tests behind `//go:build contract` verify real API behavior. All tests use the existing `client.NewForTest()` constructor.

**Tech Stack:** Go stdlib (`net/http/httptest`, `testing`, `encoding/json`), existing `client`, `registry`, `sanitize`, `manifest` packages.

---

### Task 1: FakeXCServer Implementation and Smoke Tests

**Files:**
- Create: `internal/client/testutil/fakeserver.go`
- Create: `internal/client/testutil/fakeserver_test.go`

- [ ] **Step 1: Create the testutil directory and write fakeserver.go**

```go
// internal/client/testutil/fakeserver.go
package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

type StoredObject struct {
	Metadata       map[string]interface{}
	SystemMetadata map[string]interface{}
	Spec           map[string]interface{}
}

type RecordedRequest struct {
	Method string
	Path   string
	Body   json.RawMessage
}

type ErrorSpec struct {
	StatusCode int
	Body       string
	Times      int // 0 = forever, >0 = count down
}

type errorEntry struct {
	spec      ErrorSpec
	remaining int // -1 means infinite
}

type FakeXCServer struct {
	Server *httptest.Server

	mu         sync.Mutex
	objects    map[string]StoredObject
	namespaces map[string]bool
	requests   []RecordedRequest
	errors     map[string]*errorEntry
}

func NewFakeXCServer() *FakeXCServer {
	f := &FakeXCServer{
		objects:    make(map[string]StoredObject),
		namespaces: make(map[string]bool),
		errors:     make(map[string]*errorEntry),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *FakeXCServer) Close() {
	f.Server.Close()
}

func (f *FakeXCServer) URL() string {
	return f.Server.URL
}

func (f *FakeXCServer) SeedObject(resource, ns, name string, spec map[string]any) {
	f.SeedObjectWithSystemMetadata(resource, ns, name, spec, nil)
}

func (f *FakeXCServer) SeedObjectWithSystemMetadata(resource, ns, name string, spec, extraSM map[string]any) {
	key := objectKey(resource, ns, name)
	now := time.Now().UTC().Format(time.RFC3339)
	sm := map[string]interface{}{
		"uid":                    fmt.Sprintf("fake-uid-%s-%s-%s", ns, resource, name),
		"creation_timestamp":     now,
		"modification_timestamp": now,
		"tenant":                 "fake-tenant",
	}
	for k, v := range extraSM {
		sm[k] = v
	}
	obj := StoredObject{
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		SystemMetadata: sm,
		Spec:           spec,
	}
	f.mu.Lock()
	f.objects[key] = obj
	f.namespaces[ns] = true
	f.mu.Unlock()
}

func (f *FakeXCServer) InjectError(method, resource, ns, name string, spec ErrorSpec) {
	key := errorKey(method, resource, ns, name)
	entry := &errorEntry{spec: spec}
	if spec.Times == 0 {
		entry.remaining = -1
	} else {
		entry.remaining = spec.Times
	}
	f.mu.Lock()
	f.errors[key] = entry
	f.mu.Unlock()
}

func (f *FakeXCServer) ClearErrors() {
	f.mu.Lock()
	f.errors = make(map[string]*errorEntry)
	f.mu.Unlock()
}

func (f *FakeXCServer) Requests() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]RecordedRequest, len(f.requests))
	copy(cp, f.requests)
	return cp
}

func (f *FakeXCServer) ClearRequests() {
	f.mu.Lock()
	f.requests = nil
	f.mu.Unlock()
}

func errorKey(method, resource, ns, name string) string {
	return fmt.Sprintf("%s %s/%s/%s", strings.ToUpper(method), resource, ns, name)
}

func objectKey(resource, ns, name string) string {
	return fmt.Sprintf("%s/%s/%s", resource, ns, name)
}

func (f *FakeXCServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/web/namespaces" && r.Method == http.MethodGet {
		f.handleListNamespaces(w)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 || parts[1] != "api" || parts[2] != "config" || parts[3] != "namespaces" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	ns := parts[4]
	resource := parts[5]
	name := ""
	if len(parts) >= 7 {
		name = parts[6]
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body: "+err.Error(), http.StatusBadRequest)
		return
	}

	rec := RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
	}
	if len(bodyBytes) > 0 {
		rec.Body = json.RawMessage(bodyBytes)
	}

	f.mu.Lock()
	f.requests = append(f.requests, rec)

	ekey := errorKey(r.Method, resource, ns, name)
	if entry, ok := f.errors[ekey]; ok {
		if entry.remaining > 0 {
			entry.remaining--
			if entry.remaining == 0 {
				delete(f.errors, ekey)
			}
		}
		statusCode := entry.spec.StatusCode
		body := entry.spec.Body
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
		return
	}
	f.mu.Unlock()

	switch r.Method {
	case http.MethodPost:
		f.handleCreate(w, resource, ns, name, bodyBytes)
	case http.MethodGet:
		if name != "" {
			f.handleGet(w, resource, ns, name)
		} else {
			f.handleList(w, resource, ns)
		}
	case http.MethodPut:
		f.handleReplace(w, resource, ns, name, bodyBytes)
	case http.MethodDelete:
		f.handleDelete(w, resource, ns, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *FakeXCServer) handleListNamespaces(w http.ResponseWriter) {
	f.mu.Lock()
	items := make([]map[string]interface{}, 0)
	for ns := range f.namespaces {
		items = append(items, map[string]interface{}{"name": ns})
	}
	f.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (f *FakeXCServer) handleCreate(w http.ResponseWriter, resource, ns, name string, body []byte) {
	var payload map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if name == "" && payload != nil {
		if md, ok := payload["metadata"].(map[string]interface{}); ok {
			if n, ok := md["name"].(string); ok {
				name = n
			}
		}
	}

	key := objectKey(resource, ns, name)

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.objects[key]; exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"object already exists"}`))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	obj := StoredObject{
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		SystemMetadata: map[string]interface{}{
			"uid":                    fmt.Sprintf("fake-uid-%s-%s-%s", ns, resource, name),
			"creation_timestamp":     now,
			"modification_timestamp": now,
			"tenant":                 "fake-tenant",
		},
	}
	if payload != nil {
		if spec, ok := payload["spec"].(map[string]interface{}); ok {
			obj.Spec = spec
		}
	}

	f.objects[key] = obj
	f.namespaces[ns] = true

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metadata":        obj.Metadata,
		"system_metadata": obj.SystemMetadata,
		"spec":            obj.Spec,
	})
}

func (f *FakeXCServer) handleGet(w http.ResponseWriter, resource, ns, name string) {
	key := objectKey(resource, ns, name)

	f.mu.Lock()
	obj, exists := f.objects[key]
	f.mu.Unlock()

	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metadata":        obj.Metadata,
		"system_metadata": obj.SystemMetadata,
		"spec":            obj.Spec,
	})
}

func (f *FakeXCServer) handleList(w http.ResponseWriter, resource, ns string) {
	prefix := fmt.Sprintf("%s/%s/", resource, ns)
	sharedPrefix := fmt.Sprintf("%s/shared/", resource)

	f.mu.Lock()
	items := make([]map[string]interface{}, 0)
	for k, v := range f.objects {
		if strings.HasPrefix(k, prefix) || (ns != "shared" && strings.HasPrefix(k, sharedPrefix)) {
			item := map[string]interface{}{
				"name":            v.Metadata["name"],
				"namespace":       v.Metadata["namespace"],
				"metadata":        v.Metadata,
				"system_metadata": v.SystemMetadata,
				"spec":            v.Spec,
			}
			items = append(items, item)
		}
	}
	f.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (f *FakeXCServer) handleReplace(w http.ResponseWriter, resource, ns, name string, body []byte) {
	key := objectKey(resource, ns, name)

	var payload map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.objects[key]; !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	obj := StoredObject{
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		SystemMetadata: map[string]interface{}{
			"uid":                    fmt.Sprintf("fake-uid-%s-%s-%s", ns, resource, name),
			"modification_timestamp": now,
			"tenant":                 "fake-tenant",
		},
	}
	if payload != nil {
		if spec, ok := payload["spec"].(map[string]interface{}); ok {
			obj.Spec = spec
		}
	}

	f.objects[key] = obj

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metadata":        obj.Metadata,
		"system_metadata": obj.SystemMetadata,
		"spec":            obj.Spec,
	})
}

func (f *FakeXCServer) handleDelete(w http.ResponseWriter, resource, ns, name string) {
	key := objectKey(resource, ns, name)

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.objects[key]; !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
		return
	}

	delete(f.objects, key)
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "marshalling response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
```

- [ ] **Step 2: Write fakeserver_test.go smoke tests**

```go
// internal/client/testutil/fakeserver_test.go
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFakeXCServer_SeedAndList(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})

	resp, err := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	defer resp.Body.Close()

	var listResp struct {
		Items []map[string]any `json:"items"`
	}
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &listResp)

	if len(listResp.Items) != 1 {
		t.Fatalf("list returned %d items, want 1", len(listResp.Items))
	}
	if listResp.Items[0]["name"] != "hc1" {
		t.Errorf("list item name = %v, want hc1", listResp.Items[0]["name"])
	}
	if listResp.Items[0]["namespace"] != "prod" {
		t.Errorf("list item namespace = %v, want prod", listResp.Items[0]["namespace"])
	}
}

func TestFakeXCServer_Get(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})

	resp, err := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/hc1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var obj map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &obj)

	md := obj["metadata"].(map[string]any)
	if md["name"] != "hc1" {
		t.Errorf("metadata.name = %v, want hc1", md["name"])
	}
	if _, ok := obj["name"]; ok {
		t.Error("get response should not have top-level name field")
	}
	if obj["system_metadata"] == nil {
		t.Error("get response should include system_metadata")
	}
}

func TestFakeXCServer_Get_NotFound(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/nonexistent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestFakeXCServer_CreateAndGet(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	body := `{"metadata":{"name":"hc1","namespace":"prod"},"spec":{"timeout":3}}`
	resp, err := http.Post(
		srv.URL()+"/api/config/namespaces/prod/healthchecks",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("POST status = %d, want 200", resp.StatusCode)
	}

	resp2, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/hc1")
	defer resp2.Body.Close()
	data, _ := io.ReadAll(resp2.Body)

	var obj map[string]any
	json.Unmarshal(data, &obj)

	md := obj["metadata"].(map[string]any)
	if md["name"] != "hc1" {
		t.Errorf("name = %v, want hc1", md["name"])
	}
}

func TestFakeXCServer_CreateConflict(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})

	body := `{"metadata":{"name":"hc1"},"spec":{"timeout":5}}`
	resp, err := http.Post(
		srv.URL()+"/api/config/namespaces/prod/healthchecks",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 409 {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
}

func TestFakeXCServer_Delete(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})

	req, _ := http.NewRequest("DELETE", srv.URL()+"/api/config/namespaces/prod/healthchecks/hc1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("DELETE status = %d, want 200", resp.StatusCode)
	}

	resp2, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/hc1")
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Errorf("GET after DELETE status = %d, want 404", resp2.StatusCode)
	}
}

func TestFakeXCServer_ErrorInjection(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("GET", "healthchecks", "prod", "", ErrorSpec{
		StatusCode: 403,
		Body:       `{"message":"forbidden"}`,
		Times:      1,
	})

	resp, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("first request status = %d, want 403", resp.StatusCode)
	}

	resp2, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("second request status = %d, want 200 (error should have expired)", resp2.StatusCode)
	}
}

func TestFakeXCServer_ErrorInjectionForever(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.InjectError("GET", "healthchecks", "prod", "", ErrorSpec{
		StatusCode: 401,
		Body:       `{"message":"unauthenticated"}`,
		Times:      0, // forever
	})

	for i := 0; i < 3; i++ {
		resp, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Errorf("request %d status = %d, want 401", i, resp.StatusCode)
		}
	}
}

func TestFakeXCServer_RequestRecording(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
	http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/hc1")

	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("recorded %d requests, want 2", len(reqs))
	}
	if reqs[0].Method != "GET" {
		t.Errorf("request[0].Method = %q, want GET", reqs[0].Method)
	}
	if reqs[1].Path != "/api/config/namespaces/prod/healthchecks/hc1" {
		t.Errorf("request[1].Path = %q", reqs[1].Path)
	}

	srv.ClearRequests()
	if len(srv.Requests()) != 0 {
		t.Error("ClearRequests did not clear")
	}
}

func TestFakeXCServer_ListNamespaces(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})
	srv.SeedObject("healthchecks", "staging", "hc2", map[string]any{"timeout": 5})

	resp, _ := http.Get(srv.URL() + "/api/web/namespaces")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var listResp struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(data, &listResp)

	if len(listResp.Items) < 2 {
		t.Errorf("namespace list returned %d items, want >= 2", len(listResp.Items))
	}
}

func TestFakeXCServer_ListIncludesSharedNamespace(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": 3})
	srv.SeedObject("healthchecks", "shared", "shared-hc", map[string]any{"timeout": 10})

	resp, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var listResp struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(data, &listResp)

	if len(listResp.Items) != 2 {
		t.Fatalf("list returned %d items, want 2 (prod + shared)", len(listResp.Items))
	}

	namespaces := make(map[string]bool)
	for _, item := range listResp.Items {
		ns, _ := item["namespace"].(string)
		namespaces[ns] = true
	}
	if !namespaces["prod"] || !namespaces["shared"] {
		t.Errorf("expected items from prod and shared, got namespaces: %v", namespaces)
	}
}

func TestFakeXCServer_SeedObjectWithSystemMetadata(t *testing.T) {
	srv := NewFakeXCServer()
	defer srv.Close()

	srv.SeedObjectWithSystemMetadata("healthchecks", "prod", "hc1",
		map[string]any{"timeout": 3},
		map[string]any{
			"owner_view": map[string]any{
				"kind": "http_loadbalancer",
				"name": "my-lb",
			},
		},
	)

	resp, _ := http.Get(srv.URL() + "/api/config/namespaces/prod/healthchecks/hc1")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var obj map[string]any
	json.Unmarshal(data, &obj)

	sm := obj["system_metadata"].(map[string]any)
	ov, ok := sm["owner_view"].(map[string]any)
	if !ok {
		t.Fatal("system_metadata.owner_view not set")
	}
	if ov["kind"] != "http_loadbalancer" {
		t.Errorf("owner_view.kind = %v, want http_loadbalancer", ov["kind"])
	}
	if sm["uid"] == nil {
		t.Error("default uid should still be present")
	}
}
```

- [ ] **Step 3: Run the FakeXCServer tests**

Run: `go test -v -race ./internal/client/testutil/`

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/client/testutil/fakeserver.go internal/client/testutil/fakeserver_test.go
git commit -m "feat: add FakeXCServer for integration testing

Reusable httptest-based fake that mimics the F5 XC REST API.
Supports CRUD operations, error injection, request recording,
cross-namespace list behavior, and custom system_metadata seeding."
```

---

### Task 2: Backup Integration Tests

**Files:**
- Create: `internal/backup/backup_integration_test.go`

- [ ] **Step 1: Write backup_integration_test.go**

```go
// internal/backup/backup_integration_test.go
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
```

- [ ] **Step 2: Run the backup integration tests**

Run: `go test -v -race ./internal/backup/ -run TestIntegration`

Expected: All 5 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/backup/backup_integration_test.go
git commit -m "test: add backup integration tests with FakeXCServer

Tests full workflow, error handling (403/404/501), auth failure,
namespace filtering, and view-owned object filtering."
```

---

### Task 3: Restore Integration Tests

**Files:**
- Create: `internal/restore/restore_integration_test.go`

- [ ] **Step 1: Write restore_integration_test.go**

Note: The existing `restore_test.go` has a `setupTestBackup(t)` that returns a fixed backup dir. The integration tests need a flexible version, so we use a differently-named helper `writeBackupDir`.

```go
// internal/restore/restore_integration_test.go
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
```

- [ ] **Step 2: Run the restore integration tests**

Run: `go test -v -race ./internal/restore/ -run TestIntegration`

Expected: All 6 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/restore/restore_integration_test.go
git commit -m "test: add restore integration tests with FakeXCServer

Tests object creation, tier ordering, all three conflict modes
(skip/overwrite/fail), and error injection scenarios."
```

---

### Task 4: Diff Integration Tests

**Files:**
- Create: `internal/diff/diff_integration_test.go`

- [ ] **Step 1: Write diff_integration_test.go**

Note: This file reuses `setupTestBackup` and `makeObj` from the existing `diff_test.go` (same package, accessible across test files).

```go
// internal/diff/diff_integration_test.go
package diff

import (
	"net/http"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/client/testutil"
	"github.com/kevingstewart/xcbackup/internal/registry"
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

	// Live state: hc1 (modified), hc3 (added — not in backup)
	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(99)})
	srv.SeedObject("healthchecks", "prod", "hc3", map[string]any{"timeout": float64(7)})

	// Backup state: hc1 (original), hc2 (removed — not in live)
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
```

- [ ] **Step 2: Run the diff integration tests**

Run: `go test -v -race ./internal/diff/ -run TestIntegration`

Expected: All 3 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/diff/diff_integration_test.go
git commit -m "test: add diff integration tests with FakeXCServer

Tests no-drift detection, all drift categories (added/removed/modified),
and error injection for inaccessible resources."
```

---

### Task 5: Revert Integration Tests

**Files:**
- Create: `internal/revert/revert_integration_test.go`

- [ ] **Step 1: Write revert_integration_test.go**

Note: Reuses `setupTestBackup` and `makeObj` from the existing `revert_test.go`.

```go
// internal/revert/revert_integration_test.go
package revert

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/client/testutil"
	"github.com/kevingstewart/xcbackup/internal/registry"
)

func TestIntegration_RevertModified(t *testing.T) {
	srv := testutil.NewFakeXCServer()
	defer srv.Close()

	// Live: hc1 with modified spec
	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(99)})

	// Backup: hc1 with original spec
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

	// Live: empty (hc1 and pool1 are "removed")
	// Backup: hc1 (tier 1) and pool1 (tier 2)
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

	// Live: hc1 (tier 1) and pool1 (tier 2) — both extra (not in backup)
	srv.SeedObject("healthchecks", "prod", "hc1", map[string]any{"timeout": float64(3)})
	srv.SeedObject("origin_pools", "prod", "pool1", map[string]any{"port": float64(8080)})

	// Backup: empty
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
	// Reverse tier order: origin-pool (tier 2) deleted before healthcheck (tier 1)
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
```

- [ ] **Step 2: Run the revert integration tests**

Run: `go test -v -race ./internal/revert/ -run TestIntegration`

Expected: All 4 tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/revert/revert_integration_test.go
git commit -m "test: add revert integration tests with FakeXCServer

Tests replace modified, recreate removed (tier-ordered), delete extra
(reverse tier order), and dry run mode."
```

---

### Task 6: Contract Tests

**Files:**
- Create: `internal/client/contract_test.go`

- [ ] **Step 1: Write contract_test.go**

```go
// internal/client/contract_test.go
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

	// Create
	obj := map[string]any{
		"metadata": map[string]any{"name": name, "namespace": ns},
		"spec": map[string]any{
			"http_health_check":   map[string]any{},
			"timeout":             3,
			"interval":            15,
			"unhealthy_threshold": 3,
			"healthy_threshold":   1,
		},
	}
	if err := c.Create(basePath, obj); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get
	got, err := c.Get(objPath)
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	md, _ := got["metadata"].(map[string]any)
	if md["name"] != name {
		t.Errorf("Get name = %v, want %s", md["name"], name)
	}

	// Replace
	obj["spec"].(map[string]any)["timeout"] = 10
	if err := c.Replace(objPath, obj); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// List and verify present
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

	// Delete
	if err := c.Delete(objPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify gone
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
```

- [ ] **Step 2: Verify contract tests build (without running against real API)**

Run: `go build -tags=contract ./internal/client/`

Expected: Compiles with no errors. (We do NOT run the tests here since they require real credentials.)

- [ ] **Step 3: Verify contract tests are excluded from normal test runs**

Run: `go test -v -race ./internal/client/ -run TestContract`

Expected: No contract tests run (they're behind the build tag).

- [ ] **Step 4: Commit**

```bash
git add internal/client/contract_test.go
git commit -m "test: add contract tests for real F5 XC API

Behind //go:build contract tag. Tests CRUD lifecycle, list namespaces,
list resources, and error codes. Uses XC_TENANT_URL, XC_API_TOKEN,
XC_TEST_NAMESPACE env vars, matching the k8s operator pattern."
```

---

### Task 7: Makefile Update and Final Verification

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add test-contract target to Makefile**

Add `test-contract` to `.PHONY` and add the target:

```makefile
.PHONY: build test test-contract clean
```

Add after the `test:` target:

```makefile
test-contract:
	go test -v -race -count=1 -tags=contract ./...
```

- [ ] **Step 2: Run the full test suite**

Run: `make test`

Expected: All existing unit tests AND all new integration tests pass. No contract tests run.

- [ ] **Step 3: Verify test-contract target compiles**

Run: `go test -v -race -count=1 -tags=contract -list '.*' ./internal/client/ 2>&1 | head -20`

Expected: Lists contract test names without running them.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "build: add test-contract Makefile target

make test runs unit + integration tests (fast, no external deps).
make test-contract runs contract tests against real F5 XC API."
```
