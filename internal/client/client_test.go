// internal/client/client_test.go
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_List(t *testing.T) {
	items := []map[string]any{
		{"metadata": map[string]any{"name": "hc1"}},
		{"metadata": map[string]any{"name": "hc2"}},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/namespaces/prod/healthchecks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "APIToken test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
		sem:        make(chan struct{}, 10),
	}

	result, err := c.List("/api/config/namespaces/prod/healthchecks")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("List() returned %d items, want 2", len(result))
	}
}

func TestClient_Get(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{"name": "hc1", "namespace": "prod"},
		"spec":     map[string]any{"timeout": 3},
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/namespaces/prod/healthchecks/hc1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(obj)
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
		sem:        make(chan struct{}, 10),
	}

	result, err := c.Get("/api/config/namespaces/prod/healthchecks/hc1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	md := result["metadata"].(map[string]any)
	if md["name"] != "hc1" {
		t.Errorf("Get() name = %v, want hc1", md["name"])
	}
}

func TestClient_Create(t *testing.T) {
	var received map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(received)
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
		sem:        make(chan struct{}, 10),
	}

	obj := map[string]any{
		"metadata": map[string]any{"name": "hc1"},
		"spec":     map[string]any{"timeout": 3},
	}
	err := c.Create("/api/config/namespaces/prod/healthchecks", obj)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if received["metadata"].(map[string]any)["name"] != "hc1" {
		t.Error("Create() did not send correct body")
	}
}

func TestClient_Replace(t *testing.T) {
	var received map[string]any
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("unexpected method: %s, want PUT", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(received)
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
		sem:        make(chan struct{}, 10),
	}

	obj := map[string]any{
		"metadata": map[string]any{"name": "hc1"},
		"spec":     map[string]any{"timeout": 5},
	}
	err := c.Replace("/api/config/namespaces/prod/healthchecks/hc1", obj)
	if err != nil {
		t.Fatalf("Replace() error: %v", err)
	}
	if received["metadata"].(map[string]any)["name"] != "hc1" {
		t.Error("Replace() did not send correct body")
	}
}

func TestClient_APIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code": 5, "message": "not found"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		token:      "test-token",
		sem:        make(chan struct{}, 10),
	}

	_, err := c.Get("/api/config/namespaces/prod/healthchecks/nonexistent")
	if err == nil {
		t.Fatal("Get() should return error for 404")
	}
}
