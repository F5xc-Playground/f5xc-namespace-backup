package sanitize

import "encoding/json"

// ForBackup returns a sanitized copy of an API object for writing to disk.
func ForBackup(obj map[string]any) map[string]any {
	result := deepCopy(obj)
	delete(result, "system_metadata")
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
