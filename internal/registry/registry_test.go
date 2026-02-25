package registry

import (
	"strings"
	"testing"
)

func TestAllResources_NotEmpty(t *testing.T) {
	resources := All()
	if len(resources) == 0 {
		t.Fatal("All() returned empty registry")
	}
}

func TestAllResources_HaveRequiredFields(t *testing.T) {
	for _, r := range All() {
		if r.Kind == "" {
			t.Errorf("resource with empty Kind")
		}
		if r.ListPath == "" {
			t.Errorf("resource %q has empty ListPath", r.Kind)
		}
		if r.Tier < 1 || r.Tier > 20 {
			t.Errorf("resource %q has invalid Tier %d", r.Kind, r.Tier)
		}
	}
}

func TestAllResources_UniqueKinds(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range All() {
		if seen[r.Kind] {
			t.Errorf("duplicate Kind: %q", r.Kind)
		}
		seen[r.Kind] = true
	}
}

func TestAllResources_PathsHaveNamespacePlaceholder(t *testing.T) {
	for _, r := range All() {
		if !strings.Contains(r.ListPath, "{namespace}") {
			t.Errorf("resource %q ListPath missing {namespace}: %s", r.Kind, r.ListPath)
		}
	}
}

func TestAllResources_PathsStartWithAPI(t *testing.T) {
	for _, r := range All() {
		if !strings.HasPrefix(r.ListPath, "/api/") {
			t.Errorf("resource %q ListPath doesn't start with /api/: %s", r.Kind, r.ListPath)
		}
	}
}

func TestFilterByTier(t *testing.T) {
	tier1 := FilterByTier(All(), 1)
	for _, r := range tier1 {
		if r.Tier != 1 {
			t.Errorf("FilterByTier(1) returned resource %q with Tier %d", r.Kind, r.Tier)
		}
	}
}

func TestFilterByKinds(t *testing.T) {
	result := FilterByKinds(All(), []string{"healthcheck", "origin-pool"})
	if len(result) != 2 {
		t.Errorf("FilterByKinds returned %d items, want 2", len(result))
	}
}

func TestExcludeKinds(t *testing.T) {
	all := All()
	excluded := ExcludeKinds(all, []string{"healthcheck"})
	if len(excluded) != len(all)-1 {
		t.Errorf("ExcludeKinds removed %d items, want 1", len(all)-len(excluded))
	}
}

func TestTiers(t *testing.T) {
	tiers := Tiers(All())
	if len(tiers) == 0 {
		t.Fatal("Tiers() returned empty")
	}
	for i := 1; i < len(tiers); i++ {
		if tiers[i] <= tiers[i-1] {
			t.Errorf("Tiers not sorted: %v", tiers)
		}
	}
}
