// internal/client/client_test.go
package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	tests := []struct {
		name           string
		statusCode     int
		body           string
		wantStatusCode int
		wantMessage    string
		wantContains   string // substring expected in Error()
	}{
		{
			name:           "401 with JSON message",
			statusCode:     401,
			body:           `{"code": 16, "message": "unauthenticated"}`,
			wantStatusCode: 401,
			wantMessage:    "unauthenticated",
			wantContains:   "authentication failed",
		},
		{
			name:           "403 permission denied",
			statusCode:     403,
			body:           `{"code": 7, "message": "forbidden"}`,
			wantStatusCode: 403,
			wantMessage:    "forbidden",
			wantContains:   "permission denied",
		},
		{
			name:           "404 not found",
			statusCode:     404,
			body:           `{"code": 5, "message": "not found"}`,
			wantStatusCode: 404,
			wantMessage:    "not found",
			wantContains:   "not found",
		},
		{
			name:           "409 conflict",
			statusCode:     409,
			body:           `{"code": 6, "message": "already exists"}`,
			wantStatusCode: 409,
			wantMessage:    "already exists",
			wantContains:   "conflict",
		},
		{
			name:           "429 rate limited",
			statusCode:     429,
			body:           `{"error": "too many requests"}`,
			wantStatusCode: 429,
			wantMessage:    "too many requests",
			wantContains:   "rate limited",
		},
		{
			name:           "500 server error",
			statusCode:     500,
			body:           `{"message": "internal error"}`,
			wantStatusCode: 500,
			wantMessage:    "internal error",
			wantContains:   "server error",
		},
		{
			name:           "non-JSON body",
			statusCode:     502,
			body:           `Bad Gateway`,
			wantStatusCode: 502,
			wantMessage:    "",
			wantContains:   "server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
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
				t.Fatal("Get() should return error")
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error should be *APIError, got %T: %v", err, err)
			}

			if apiErr.StatusCode != tt.wantStatusCode {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.wantStatusCode)
			}

			if apiErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMessage)
			}

			if !strings.Contains(apiErr.Error(), tt.wantContains) {
				t.Errorf("Error() = %q, want it to contain %q", apiErr.Error(), tt.wantContains)
			}
		})
	}
}
