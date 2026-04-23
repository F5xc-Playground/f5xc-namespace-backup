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
		Times:      0,
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
