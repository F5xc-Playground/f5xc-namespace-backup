package sanitize

import "encoding/json"

// IsViewOwned returns true if the object is auto-managed by a view object
// (e.g., an http-loadbalancer). These objects have system_metadata.owner_view
// set by the API and should be skipped during backup and diff.
func IsViewOwned(obj map[string]any) bool {
	sm, ok := obj["system_metadata"].(map[string]any)
	if !ok {
		return false
	}
	ov, ok := sm["owner_view"].(map[string]any)
	if !ok {
		return false
	}
	_, hasKind := ov["kind"].(string)
	return hasKind
}

// ForBackup returns a sanitized copy of an API object for writing to disk.
func ForBackup(obj map[string]any) map[string]any {
	result := deepCopy(obj)
	delete(result, "system_metadata")
	delete(result, "resource_version")
	delete(result, "status")
	delete(result, "referring_objects")
	delete(result, "deleted_referred_objects")
	delete(result, "disabled_referred_objects")
	delete(result, "create_form")
	delete(result, "replace_form")
	if md, ok := result["metadata"].(map[string]any); ok {
		delete(md, "uid")
		delete(md, "resource_version")
	}
	return result
}

// ForRestore returns a copy of a backed-up object ready for POST to the API.
//
// It sets metadata.namespace to targetNamespace. When sourceNamespace is set
// and differs from targetNamespace, it also points every object reference in
// spec ({"name": ..., "namespace": sourceNamespace, ...}) at targetNamespace,
// so objects that refer to each other inside the backup still resolve after a
// restore into a different namespace. References to other namespaces, such as
// shared, are left alone. It returns the number of references it changed.
func ForRestore(obj map[string]any, sourceNamespace, targetNamespace string) (map[string]any, int) {
	result := deepCopy(obj)
	if md, ok := result["metadata"].(map[string]any); ok {
		md["namespace"] = targetNamespace
	}
	if sourceNamespace == "" || sourceNamespace == targetNamespace {
		return result, 0
	}
	return result, retarget(result["spec"], sourceNamespace, targetNamespace)
}

func retarget(node any, from, to string) int {
	n := 0
	switch v := node.(type) {
	case map[string]any:
		if ns, ok := v["namespace"].(string); ok && ns == from {
			if _, ok := v["name"].(string); ok {
				v["namespace"] = to
				n++
			}
		}
		for _, child := range v {
			n += retarget(child, from, to)
		}
	case []any:
		for _, child := range v {
			n += retarget(child, from, to)
		}
	}
	return n
}

func deepCopy(obj map[string]any) map[string]any {
	data, _ := json.Marshal(obj)
	var result map[string]any
	json.Unmarshal(data, &result)
	return result
}
