package domain

import (
	"strings"
	"testing"

	"stickyproxy/native-plugin/internal/i18n"
)

func TestNormalizeProxyName(t *testing.T) {
	for _, name := range []string{"Proxy01", "US_proxy-01", "A_B-C9"} {
		if got, err := NormalizeProxyName(name); err != nil || got != name {
			t.Fatalf("valid name %q: got=%q err=%v", name, got, err)
		}
	}
	for _, name := range []string{"", "proxy name", "proxy.name", "代理", "proxy/01", "proxy@01"} {
		if _, err := NormalizeProxyName(name); err == nil {
			t.Fatalf("invalid name %q accepted", name)
		}
	}
}

func TestNormalizePlatformIncludesSupportedPlatforms(t *testing.T) {
	for raw, want := range map[string]Platform{
		" 1024PROXY ": Proxy1024,
		" generic ":   Generic,
	} {
		platform, err := NormalizePlatform(raw)
		if err != nil || platform != want {
			t.Fatalf("NormalizePlatform(%q) = %q, %v; want %q, nil", raw, platform, err, want)
		}
	}
	if _, err := NormalizePlatform("unknown"); err == nil {
		t.Fatal("unknown platform was accepted")
	}
}

func TestStableHashDependsOnlyOnNormalizedEmail(t *testing.T) {
	first, err := StableHash("Alice@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := StableHash(" alice@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("hash differs for same normalized email: %q != %q", first, second)
	}
	third, err := StableHash("bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("distinct emails yielded same stable hash")
	}
	if len(first) != 24 {
		t.Fatalf("hash length = %d, want 24", len(first))
	}
	// Freeze the public, no-key hash contract used by all three adapters.
	if first != "f2783dad6a985ee016173293" {
		t.Fatalf("hash = %q, want fixed v1 vector", first)
	}
}

func TestRewriteDataImpulseReplacesExistingSession(t *testing.T) {
	proxy, err := NewProxy("dataimpulse", "http://login__cr.us;sessid.old:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.URL, "sessid.old") {
		t.Fatalf("old sessid remained: %s", result.URL)
	}
	if got := strings.Count(result.URL, ";sessid."); got != 1 {
		t.Fatalf("sessid occurrences = %d, want 1: %s", got, result.URL)
	}
	if !strings.Contains(result.URL, ";sessid."+result.StableHash) {
		t.Fatalf("new hash is absent: %s", result.URL)
	}
}

func TestRewriteDecodoUsesStableSessionUsername(t *testing.T) {
	proxy, err := NewProxy("decodo", "https://login-session-old-sessionduration-10:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := "user-login-session-" + result.StableHash + "-sessionduration-30"
	if !strings.Contains(result.URL, want) {
		t.Fatalf("Decodo rewrite = %s, want username containing %s", result.URL, want)
	}
	if strings.Contains(result.URL, "session-old") || strings.Contains(result.URL, "sessionduration-10") {
		t.Fatalf("legacy Decodo marker remained: %s", result.URL)
	}
}

func TestRewriteDecodoStripsHyphenatedLegacySession(t *testing.T) {
	proxy, err := NewProxy("decodo", "http://login-session-old-token-sessionduration-10:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.URL, "old-token") || strings.Contains(result.URL, "sessionduration-10") {
		t.Fatalf("legacy marker remained: %s", result.URL)
	}
	if strings.Count(result.URL, "-session-") != 1 || strings.Count(result.URL, "-sessionduration-") != 1 {
		t.Fatalf("expected exactly one canonical marker: %s", result.URL)
	}
}

func TestRewriteDecodoISPRemainsPortSticky(t *testing.T) {
	proxy, err := NewProxy("decodo", "http://login:secret@isp.decodo.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !result.PortStickyOnly {
		t.Fatal("expected port-based sticky marker")
	}
	if strings.Contains(result.URL, "-session-") {
		t.Fatalf("unexpected Decodo session injection: %s", result.URL)
	}
}

func TestRewriteResinUsesHashAccount(t *testing.T) {
	proxy, err := NewProxy("resin", "socks5://ignored:resin-token@resin.example:2260")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.URL, "Default.sp_"+result.StableHash) {
		t.Fatalf("Resin rewrite = %s", result.URL)
	}
	if strings.Contains(result.URL, "alice%40") || strings.Contains(result.URL, "alice@example") {
		t.Fatalf("Resin URL leaked email: %s", result.URL)
	}
}

func TestRewrite1024ProxyReplacesTemplateSIDAndPreservesTTL(t *testing.T) {
	proxy, err := NewProxy("1024proxy", "socks5://login-region-US-sid-template-old-t-60:secret@us.1024proxy.io:3000")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(proxy.BaseURL, "-sid-template-old-t-60") {
		t.Fatalf("template SID or TTL was not persisted: %q", proxy.BaseURL)
	}

	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := Parse(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := "login-region-US-sid-" + result.StableHash + "-t-60"
	if parts.Username != want {
		t.Fatalf("1024Proxy username = %q, want %q", parts.Username, want)
	}
	if strings.Contains(parts.Username, "template-old") {
		t.Fatalf("template SID remained in account URL: %q", parts.Username)
	}
}

func Test1024ProxyCanonicalizationAndEscapedCredentials(t *testing.T) {
	proxy, err := NewProxy("1024proxy", "socks5h://login-region-US:secret%3Avalue@example.test:3000")
	if err != nil {
		t.Fatal(err)
	}
	if proxy.BaseURL != "socks5://login-region-US:secret%3Avalue@example.test:3000" {
		t.Fatalf("canonical base URL = %q", proxy.BaseURL)
	}

	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.URL, "alice%40") || strings.Contains(result.URL, "alice@example.com") {
		t.Fatalf("account URL leaked email: %q", result.URL)
	}
	parts, err := Parse(result.URL)
	if err != nil || parts.Password != "secret:value" {
		t.Fatalf("rewritten proxy did not preserve escaped password: parts=%#v err=%v", parts, err)
	}
}

func TestRewrite1024ProxyInjectsSIDWithoutInjectingTTL(t *testing.T) {
	proxy, err := NewProxy("1024proxy", "socks5://login-region-US:secret@us.1024proxy.io:3000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := Parse(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := "login-region-US-sid-" + result.StableHash
	if parts.Username != want || strings.Contains(parts.Username, "-t-") {
		t.Fatalf("1024Proxy username = %q, want %q without TTL", parts.Username, want)
	}
}

func TestGenericProxyPreservesCanonicalBaseURLWithoutIdentity(t *testing.T) {
	proxy, err := NewProxy("generic", " socks5h://operator:secret%3Avalue@GATEWAY.EXAMPLE:1080 ")
	if err != nil {
		t.Fatal(err)
	}
	want := "socks5://operator:secret%3Avalue@gateway.example:1080"
	if proxy.BaseURL != want {
		t.Fatalf("base URL = %q, want %q", proxy.BaseURL, want)
	}

	result, err := Rewrite(proxy, "not-an-email")
	if err != nil {
		t.Fatalf("generic rewrite unexpectedly requires email: %v", err)
	}
	if result.URL != want || result.StableHash != "" || result.PortStickyOnly {
		t.Fatalf("generic rewrite = %#v, want base URL without identity", result)
	}
	if needsSession, err := TestSessionRequired(proxy); err != nil || needsSession {
		t.Fatalf("generic test session requirement = %v, %v; want false, nil", needsSession, err)
	}
	testURL, portStickyOnly, err := RewriteTestSessionURL(proxy, "unused-session")
	if err != nil || portStickyOnly || testURL != want {
		t.Fatalf("generic test URL = %q, portStickyOnly=%v, err=%v", testURL, portStickyOnly, err)
	}
}

func TestGenericProxyAllowsOptionalAuthentication(t *testing.T) {
	for _, raw := range []string{
		"http://proxy.example:8080",
		"https://operator@proxy.example:443",
		"socks5://operator:secret@proxy.example:1080",
	} {
		t.Run(raw, func(t *testing.T) {
			proxy, err := NewProxy("generic", raw)
			if err != nil {
				t.Fatalf("generic proxy rejected %q: %v", raw, err)
			}
			result, err := Rewrite(proxy, "")
			if err != nil {
				t.Fatalf("generic rewrite rejected empty email: %v", err)
			}
			if result.URL != proxy.BaseURL {
				t.Fatalf("generic URL = %q, want %q", result.URL, proxy.BaseURL)
			}
		})
	}
}

func TestProxyIDIncludesPlatformForGenericProxy(t *testing.T) {
	const raw = "http://operator:secret@isp.decodo.com:10000"
	generic, err := NewProxy("generic", raw)
	if err != nil {
		t.Fatal(err)
	}
	decodo, err := NewProxy("decodo", raw)
	if err != nil {
		t.Fatal(err)
	}
	if generic.ID == decodo.ID {
		t.Fatalf("generic and Decodo proxies share ID %q", generic.ID)
	}
}

func TestRewrite1024ProxyInsertsSIDBeforeExistingTTL(t *testing.T) {
	proxy, err := NewProxy("1024proxy", "socks5://login-region-US-t-opaque-value:secret@us.1024proxy.io:3000")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := Parse(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	want := "login-region-US-sid-" + result.StableHash + "-t-opaque-value"
	if parts.Username != want {
		t.Fatalf("1024Proxy username = %q, want %q", parts.Username, want)
	}
}

func Test1024ProxyRequiresCredentialsAndUnambiguousSID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		code string
	}{
		{"missing username", "socks5://:secret@us.1024proxy.io:3000", "missing_username"},
		{"missing password", "socks5://login@us.1024proxy.io:3000", "missing_proxy_password"},
		{"empty sid", "socks5://login-sid-:secret@us.1024proxy.io:3000", "invalid_1024_sid"},
		{"repeated sid", "socks5://login-sid-first-sid-second:secret@us.1024proxy.io:3000", "invalid_1024_sid"},
		{"empty username before TTL", "socks5://-t-60:secret@us.1024proxy.io:3000", "invalid_1024_sid"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewProxy("1024proxy", test.raw)
			if err == nil {
				t.Fatalf("invalid 1024Proxy configuration was accepted: %q", test.raw)
			}
			if got := i18n.Code(err); got != test.code {
				t.Fatalf("error code = %q, want %q (err=%v)", got, test.code, err)
			}
		})
	}
}

func Test1024ProxyStandaloneOpaqueTTLIsPreserved(t *testing.T) {
	for _, username := range []string{
		"login-region-US-t-60",
		"login-region-US-t-opaque-value",
		"login-region-US-t-",
	} {
		t.Run(username, func(t *testing.T) {
			proxy, err := NewProxy("1024proxy", "socks5://"+username+":secret@example.test:3000")
			if err != nil {
				t.Fatal(err)
			}
			account, err := Rewrite(proxy, "alice@example.com")
			if err != nil {
				t.Fatal(err)
			}
			testURL, stickyOnly, err := RewriteTestSessionURL(proxy, "test0123456789abcdef01234567")
			if err != nil || stickyOnly {
				t.Fatalf("test rewrite = %q stickyOnly=%v err=%v", testURL, stickyOnly, err)
			}
			for _, raw := range []string{account.URL, testURL} {
				parts, err := Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(parts.Username, strings.TrimPrefix(username, "login-region-US")) {
					t.Fatalf("TTL was not preserved in %q", parts.Username)
				}
			}
		})
	}
}

func Test1024ProxyAccountAndTestSessionsStayDistinct(t *testing.T) {
	proxy, err := NewProxy("1024proxy", "socks5://login-region-US:secret@example.test:3000")
	if err != nil {
		t.Fatal(err)
	}
	first, err := Rewrite(proxy, "Alice@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Rewrite(proxy, " alice@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Rewrite(proxy, "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first.URL != again.URL || first.URL == second.URL || strings.Contains(first.URL, "alice") {
		t.Fatalf("account rewrites were not deterministic and distinct: first=%q again=%q second=%q", first.URL, again.URL, second.URL)
	}
	testURL, stickyOnly, err := RewriteTestSessionURL(proxy, "test0123456789abcdef01234567")
	if err != nil || stickyOnly {
		t.Fatalf("test rewrite = %q stickyOnly=%v err=%v", testURL, stickyOnly, err)
	}
	if testURL == first.URL || strings.Contains(testURL, first.StableHash) || !strings.Contains(testURL, "-sid-test0123456789abcdef01234567") {
		t.Fatalf("test session overlapped account session: account=%q test=%q", first.URL, testURL)
	}
}

func TestMaskUsesReadablePasswordRedaction(t *testing.T) {
	masked := Mask("http://login:secret@gate.decodo.com:7000")
	if strings.Contains(masked, "secret") || !strings.Contains(masked, ":***@") || strings.Contains(masked, "%2A") {
		t.Fatalf("masked URL = %q", masked)
	}
}

func TestMalformedURLErrorDoesNotLeakCredentials(t *testing.T) {
	_, err := NewProxy("decodo", "http://login:secret%zz@gate.decodo.com:7000")
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked credential: %q", err)
	}
}

func TestOnlyDirectSchemesAreAccepted(t *testing.T) {
	if _, err := NewProxy("decodo", "vless://uuid@example.com:443"); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
	proxy, err := NewProxy("resin", "socks://ignored:token@resin.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(proxy.BaseURL, "socks5://") {
		t.Fatalf("alias was not canonicalized: %s", proxy.BaseURL)
	}
}
