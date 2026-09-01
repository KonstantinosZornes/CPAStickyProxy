package i18n

import "testing"

func TestProxyAndRouteNotFoundMessagesAreDistinct(t *testing.T) {
	if got := Text(ZH, "proxy_not_found"); got != "代理不存在" {
		t.Fatalf("Chinese proxy-not-found message = %q", got)
	}
	if got := Text(EN, "proxy_not_found"); got != "Proxy not found" {
		t.Fatalf("English proxy-not-found message = %q", got)
	}
	if got := Text(EN, "not_found"); got != "Route not found" {
		t.Fatalf("English route-not-found message = %q", got)
	}
}
