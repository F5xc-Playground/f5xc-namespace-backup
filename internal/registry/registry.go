package registry

import "sort"

// Resource describes a namespace-scoped F5 XC resource type.
type Resource struct {
	Kind       string // canonical resource name, e.g., "healthcheck", "http-loadbalancer"
	Domain     string // API domain, e.g., "virtual", "dns", "network_security"
	Tier       int    // dependency tier for restore ordering (1 = no deps, higher = more deps)
	ListPath   string // API path template for listing objects, uses {namespace} placeholder
	IsView     bool   // whether this is a "view" object that auto-creates children
	ManagedBy  string // Kind of the view that manages this resource (empty if standalone)
}

func All() []Resource {
	return allResources
}

func FilterByTier(resources []Resource, tier int) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.Tier == tier {
			result = append(result, r)
		}
	}
	return result
}

func FilterByKinds(resources []Resource, kinds []string) []Resource {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	var result []Resource
	for _, r := range resources {
		if set[r.Kind] {
			result = append(result, r)
		}
	}
	return result
}

func ExcludeKinds(resources []Resource, kinds []string) []Resource {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	var result []Resource
	for _, r := range resources {
		if !set[r.Kind] {
			result = append(result, r)
		}
	}
	return result
}

func Tiers(resources []Resource) []int {
	set := make(map[int]bool)
	for _, r := range resources {
		set[r.Tier] = true
	}
	tiers := make([]int, 0, len(set))
	for t := range set {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	return tiers
}

func ByKind(resources []Resource) map[string]Resource {
	m := make(map[string]Resource, len(resources))
	for _, r := range resources {
		m[r.Kind] = r
	}
	return m
}

func ViewManaged(resources []Resource) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.ManagedBy != "" {
			result = append(result, r)
		}
	}
	return result
}

func Standalone(resources []Resource) []Resource {
	var result []Resource
	for _, r := range resources {
		if r.ManagedBy == "" {
			result = append(result, r)
		}
	}
	return result
}
