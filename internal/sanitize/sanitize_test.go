package sanitize

import (
	"encoding/json"
	"testing"
)

func TestForBackup(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":             "hc1",
			"namespace":        "prod",
			"uid":              "abc-123",
			"resource_version": "rv-456",
			"labels":           map[string]any{"app": "web"},
			"annotations":      map[string]any{"note": "test"},
		},
		"system_metadata": map[string]any{
			"uid":                "sys-abc",
			"creation_timestamp": "2026-01-01T00:00:00Z",
			"creator_id":        "user@example.com",
		},
		"spec": map[string]any{
			"timeout": float64(3),
		},
	}

	result := ForBackup(obj)

	if _, ok := result["system_metadata"]; ok {
		t.Error("system_metadata should be removed")
	}
	md := result["metadata"].(map[string]any)
	if _, ok := md["uid"]; ok {
		t.Error("metadata.uid should be removed")
	}
	if _, ok := md["resource_version"]; ok {
		t.Error("metadata.resource_version should be removed")
	}
	if md["name"] != "hc1" {
		t.Error("metadata.name should remain")
	}
	if md["namespace"] != "prod" {
		t.Error("metadata.namespace should remain")
	}
	if md["labels"] == nil {
		t.Error("metadata.labels should remain")
	}
	if result["spec"] == nil {
		t.Error("spec should remain")
	}
}

func TestForBackup_DoesNotMutateOriginal(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name": "hc1",
			"uid":  "abc-123",
		},
		"system_metadata": map[string]any{"uid": "sys-abc"},
		"spec":            map[string]any{"timeout": float64(3)},
	}
	origJSON, _ := json.Marshal(obj)
	_ = ForBackup(obj)
	afterJSON, _ := json.Marshal(obj)
	if string(origJSON) != string(afterJSON) {
		t.Error("ForBackup should not mutate the original object")
	}
}

func TestForRestore(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":      "hc1",
			"namespace": "prod",
			"labels":    map[string]any{"app": "web"},
		},
		"spec": map[string]any{"timeout": float64(3)},
	}

	result := ForRestore(obj, "staging")

	md := result["metadata"].(map[string]any)
	if md["namespace"] != "staging" {
		t.Errorf("ForRestore should set namespace to target, got %v", md["namespace"])
	}
	if md["name"] != "hc1" {
		t.Error("ForRestore should preserve name")
	}
}
