package main

import "testing"

func TestManagementRegistrationUsesProxyActions(t *testing.T) {
	routes := managementRegistrationResponse().Routes
	found := map[string]bool{}
	for _, route := range routes {
		found[route.Method+" "+route.Path] = true
	}
	for _, key := range []string{
		"POST /stickyproxy/apply",
		"POST /stickyproxy/clear-proxy",
		"POST /stickyproxy/clear-applied",
	} {
		if !found[key] {
			t.Fatalf("missing proxy route %q", key)
		}
	}
	for _, key := range []string{
		"POST /stickyproxy/sync",
		"POST /stickyproxy/proxies/select",
		"GET /stickyproxy/account-proxy-configs",
	} {
		if found[key] {
			t.Fatalf("legacy V2 route still registered: %q", key)
		}
	}
}
