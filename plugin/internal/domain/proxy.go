// Package domain owns the small, platform-specific proxy rewriting model.
// It deliberately accepts only direct HTTP(S)/SOCKS proxy URLs; bridge
// protocols are outside the scope of this plugin.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"stickyproxy/native-plugin/internal/i18n"
)

type Platform string

const (
	Decodo      Platform = "decodo"
	DataImpulse Platform = "dataimpulse"
	Resin       Platform = "resin"
	Proxy1024   Platform = "1024proxy"
	Generic     Platform = "generic"
)

var proxyName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Proxy is exactly what the plugin persists: a provider type and its base
// direct-proxy URL. ID is derived from platform + canonical BaseURL.
type Proxy struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Platform Platform `json:"platform"`
	BaseURL  string   `json:"base_url"`
}

type Parts struct {
	Scheme   string
	Host     string
	Port     string
	Username string
	Password string
}

type RewriteResult struct {
	URL            string
	StableHash     string
	PortStickyOnly bool
}

func NormalizeProxyName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || !proxyName.MatchString(name) {
		return "", i18n.New("invalid_proxy_name")
	}
	return name, nil
}

func NormalizePlatform(value string) (Platform, error) {
	platform := Platform(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := providerFor(platform); !ok {
		return "", i18n.New("invalid_platform", value)
	}
	return platform, nil
}

func NormalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if email == "" || strings.Count(email, "@") != 1 || strings.ContainsAny(email, " \t\r\n") {
		return "", i18n.New("invalid_email")
	}
	parts := strings.SplitN(email, "@", 2)
	if parts[0] == "" || parts[1] == "" {
		return "", i18n.New("invalid_email")
	}
	return email, nil
}

// StableHash intentionally has no secret. It is stable purely by account
// email, so the same account keeps its identity when the active proxy is
// switched. It is an identity token, not a secret or security control.
func StableHash(email string) (string, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("stickyproxy:v1\x1f" + normalized))
	return hex.EncodeToString(sum[:])[:24], nil
}

func Parse(raw string) (Parts, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Parts{}, i18n.New("proxy_empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		// net/url may include the full raw URL in its error text, including a
		// password. Keep the UI/API error generic and never echo raw input.
		return Parts{}, i18n.New("invalid_proxy", "could not parse URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "socks" || scheme == "socks5h" {
		scheme = "socks5"
	}
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return Parts{}, i18n.New("unsupported_scheme")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return Parts{}, i18n.New("invalid_proxy", "query and fragment are not supported")
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if host == "" || port == "" {
		return Parts{}, i18n.New("invalid_proxy", "host and port are required")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Parts{}, i18n.New("invalid_proxy", "port must be between 1 and 65535")
	}
	if net.ParseIP(host) == nil && strings.ContainsAny(host, " \t\r\n") {
		return Parts{}, i18n.New("invalid_proxy", "hostname contains whitespace")
	}
	username, password := "", ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	return Parts{Scheme: scheme, Host: host, Port: port, Username: username, Password: password}, nil
}

func Build(parts Parts, username, password string) string {
	proxyURL := &url.URL{
		Scheme: parts.Scheme,
		Host:   net.JoinHostPort(parts.Host, parts.Port),
	}
	if username != "" {
		if password != "" {
			proxyURL.User = url.UserPassword(username, password)
		} else {
			proxyURL.User = url.User(username)
		}
	}
	return proxyURL.String()
}

func Canonical(raw string) (string, error) {
	parts, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return Build(parts, parts.Username, parts.Password), nil
}

func Mask(raw string) string {
	parts, err := Parse(raw)
	if err != nil {
		return "***"
	}
	if parts.Password == "" {
		return Build(parts, parts.Username, "")
	}
	// url.UserPassword encodes literal * as %2A. Build with a URL-safe sentinel
	// first, then replace it for a readable UI/log redaction without ever
	// exposing the actual password.
	const sentinel = "stickyproxypasswordmask"
	return strings.Replace(Build(parts, parts.Username, sentinel), sentinel, "***", 1)
}

func ProxyID(platform Platform, canonicalURL string) string {
	sum := sha256.Sum256([]byte("stickyproxy:proxy:v1\x1f" + string(platform) + "\x1f" + canonicalURL))
	return "px_" + hex.EncodeToString(sum[:])[:16]
}

// NewProxy validates and canonically persists an operator-supplied direct
// proxy URL. Platform-specific credential rules and username templates remain
// behind the providerAdapter seam.
func NewProxy(platformRaw, baseURL string) (Proxy, error) {
	platform, err := NormalizePlatform(platformRaw)
	if err != nil {
		return Proxy{}, err
	}
	adapter, _ := providerFor(platform)
	parts, err := Parse(baseURL)
	if err != nil {
		return Proxy{}, err
	}
	parts, err = adapter.NormalizeBase(parts)
	if err != nil {
		return Proxy{}, err
	}
	if err := adapter.Validate(parts); err != nil {
		return Proxy{}, err
	}
	canonicalURL := Build(parts, parts.Username, parts.Password)
	return Proxy{ID: ProxyID(platform, canonicalURL), Platform: platform, BaseURL: canonicalURL}, nil
}

// Rewrite creates the account-scoped URL used to override an auth's existing
// system proxy. It never mutates the proxy's persisted base URL.
func Rewrite(proxy Proxy, email string) (RewriteResult, error) {
	platform, err := NormalizePlatform(string(proxy.Platform))
	if err != nil {
		return RewriteResult{}, err
	}
	adapter, _ := providerFor(platform)
	parts, err := Parse(proxy.BaseURL)
	if err != nil {
		return RewriteResult{}, err
	}
	parts, err = adapter.NormalizeBase(parts)
	if err != nil {
		return RewriteResult{}, err
	}
	if err := adapter.Validate(parts); err != nil {
		return RewriteResult{}, err
	}
	if !adapter.RequiresAccountIdentity() {
		return adapter.RewriteAccount(parts, "")
	}
	hash, err := StableHash(email)
	if err != nil {
		return RewriteResult{}, err
	}
	return adapter.RewriteAccount(parts, hash)
}

// TestSessionRequired reports whether a platform needs a fresh random session
// for a server-side egress probe. Generic proxies test their base URL directly.
func TestSessionRequired(proxy Proxy) (bool, error) {
	platform, err := NormalizePlatform(string(proxy.Platform))
	if err != nil {
		return false, err
	}
	adapter, _ := providerFor(platform)
	return adapter.RequiresTestSession(), nil
}

// RewriteTestSessionURL creates the URL for a server-side egress probe. Session
// platforms receive a caller-created random session ID; generic proxies ignore
// it and use their base URL directly.
func RewriteTestSessionURL(proxy Proxy, session string) (string, bool, error) {
	platform, err := NormalizePlatform(string(proxy.Platform))
	if err != nil {
		return "", false, err
	}
	adapter, _ := providerFor(platform)
	parts, err := Parse(proxy.BaseURL)
	if err != nil {
		return "", false, err
	}
	parts, err = adapter.NormalizeBase(parts)
	if err != nil {
		return "", false, err
	}
	if err := adapter.Validate(parts); err != nil {
		return "", false, err
	}
	result, err := adapter.RewriteTest(parts, session)
	if err != nil {
		return "", false, err
	}
	return result.URL, result.PortStickyOnly, nil
}
