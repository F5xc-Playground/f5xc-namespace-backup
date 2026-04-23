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
func ForRestore(obj map[string]any, targetNamespace string) map[string]any {
	result := deepCopy(obj)
	if md, ok := result["metadata"].(map[string]any); ok {
		md["namespace"] = targetNamespace
	}
	return result
}

func deepCopy(obj map[string]any) map[string]any {
	data, _ := json.Marshal(obj)
	var result map[string]any
	json.Unmarshal(data, &result)
	return result
}
