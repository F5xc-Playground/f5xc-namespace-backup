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
