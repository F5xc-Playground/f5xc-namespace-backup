// internal/client/tenant_test.go
package client

import "testing"

func TestNormalizeTenantURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"acme", "https://acme.console.ves.volterra.io"},
		{"acme.console.ves.volterra.io", "https://acme.console.ves.volterra.io"},
		{"https://acme.console.ves.volterra.io", "https://acme.console.ves.volterra.io"},
		{"https://acme.console.ves.volterra.io/", "https://acme.console.ves.volterra.io"},
		{"acme.staging.volterra.us", "https://acme.staging.volterra.us"},
	}
	for _, tt := range tests {
		got := NormalizeTenantURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeTenantURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
