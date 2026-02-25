package refs

import "testing"

func TestFindSharedRefs(t *testing.T) {
	obj := map[string]any{
		"metadata": map[string]any{
			"name":      "main-lb",
			"namespace": "prod",
		},
		"spec": map[string]any{
			"app_firewall": map[string]any{
				"name":      "default-waf",
				"namespace": "shared",
				"tenant":    "acme",
			},
			"origin_pools": []any{
				map[string]any{
					"pool": map[string]any{
						"name":      "local-pool",
						"namespace": "prod",
					},
				},
				map[string]any{
					"pool": map[string]any{
						"name":      "shared-pool",
						"namespace": "shared",
					},
				},
			},
		},
	}

	refs := FindSharedRefs("http-loadbalancer", "main-lb", obj)
	if len(refs) != 2 {
		t.Fatalf("FindSharedRefs returned %d refs, want 2", len(refs))
	}

	foundWAF := false
	foundPool := false
	for _, ref := range refs {
		if ref.TargetName == "default-waf" {
			foundWAF = true
		}
		if ref.TargetName == "shared-pool" {
			foundPool = true
		}
	}
	if !foundWAF {
		t.Error("missing shared ref to default-waf")
	}
	if !foundPool {
		t.Error("missing shared ref to shared-pool")
	}
}

func TestFindSharedRefs_NoSharedRefs(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"pool": map[string]any{
				"name":      "local-pool",
				"namespace": "prod",
			},
		},
	}
	refs := FindSharedRefs("origin-pool", "pool1", obj)
	if len(refs) != 0 {
		t.Errorf("FindSharedRefs returned %d refs, want 0", len(refs))
	}
}

func TestFindSharedRefs_OmittedNamespace(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"ref": map[string]any{
				"name": "some-object",
			},
		},
	}
	refs := FindSharedRefs("http-loadbalancer", "lb1", obj)
	if len(refs) != 0 {
		t.Errorf("omitted namespace should not be treated as shared, got %d refs", len(refs))
	}
}
