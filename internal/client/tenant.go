// internal/client/tenant.go
package client

import "strings"

// NormalizeTenantURL converts various tenant URL formats to a canonical https:// URL.
func NormalizeTenantURL(input string) string {
	input = strings.TrimRight(input, "/")
	if strings.HasPrefix(input, "https://") {
		return input
	}
	if strings.Contains(input, ".") {
		return "https://" + input
	}
	return "https://" + input + ".console.ves.volterra.io"
}
