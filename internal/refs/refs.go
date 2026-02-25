package refs

// SharedRef represents a cross-namespace reference to the "shared" namespace.
type SharedRef struct {
	SourceKind string `json:"source_kind"`
	SourceName string `json:"source_name"`
	TargetName string `json:"target_name"`
	FieldPath  string `json:"field_path"`
}

// FindSharedRefs walks an object's JSON tree and returns all references
// to the "shared" namespace.
func FindSharedRefs(sourceKind, sourceName string, obj map[string]any) []SharedRef {
	var refs []SharedRef
	walkJSON(obj, "", func(path string, v map[string]any) {
		ns, hasNS := v["namespace"]
		name, hasName := v["name"]
		if hasNS && hasName {
			if nsStr, ok := ns.(string); ok && nsStr == "shared" {
				if nameStr, ok := name.(string); ok {
					refs = append(refs, SharedRef{
						SourceKind: sourceKind,
						SourceName: sourceName,
						TargetName: nameStr,
						FieldPath:  path,
					})
				}
			}
		}
	})
	return refs
}

func walkJSON(v any, path string, fn func(string, map[string]any)) {
	switch val := v.(type) {
	case map[string]any:
		fn(path, val)
		for k, child := range val {
			childPath := path
			if childPath != "" {
				childPath += "."
			}
			childPath += k
			walkJSON(child, childPath, fn)
		}
	case []any:
		for _, child := range val {
			walkJSON(child, path, fn)
		}
	}
}
