// Package tester performs an on-demand, server-side proxy egress probe.
// Session platforms receive a new cryptographically random identity for each
// test; generic proxies test their configured base URL directly.
package tester

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/i18n"
)

const (
	ipEchoURL    = "https://ipwho.is/"
	probeTimeout = 15 * time.Second
)

type Result struct {
	Platform       string `json:"platform"`
	IP             string `json:"ip"`
	Country        string `json:"country"`
	CountryCode    string `json:"country_code"`
	Region         string `json:"region"`
	City           string `json:"city"`
	PortStickyOnly bool   `json:"port_sticky_only"`
}

type ipWhoResponse struct {
	Success     bool   `json:"success"`
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Region      string `json:"region"`
	City        string `json:"city"`
	Message     string `json:"message"`
}

func RandomSessionID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "test" + hex.EncodeToString(bytes), nil
}

// Probe performs an proxy egress check with a bounded timeout that stays
// below the CPA management request deadline.
func Probe(proxy domain.Proxy) (Result, error) {
	return probe(context.Background(), proxy, ipEchoURL)
}

// probe accepts a caller context and target only to keep the network behavior
// deterministic and directly testable. Production always probes ipEchoURL.
func probe(ctx context.Context, proxy domain.Proxy, targetURL string) (Result, error) {
	return probeWithTimeout(ctx, proxy, targetURL, probeTimeout)
}

func probeWithTimeout(ctx context.Context, proxy domain.Proxy, targetURL string, timeout time.Duration) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	session := ""
	needsSession, err := domain.TestSessionRequired(proxy)
	if err != nil {
		return Result{}, err
	}
	if needsSession {
		session, err = RandomSessionID()
		if err != nil {
			return Result{}, err
		}
	}
	proxyURL, portStickyOnly, err := testProxyURL(proxy, session)
	if err != nil {
		return Result{}, err
	}
	transport, err := transportFor(proxyURL)
	if err != nil {
		return Result{}, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, contextError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("ip lookup returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return Result{}, contextError(ctx, err)
	}
	var lookup ipWhoResponse
	if err := json.Unmarshal(body, &lookup); err != nil {
		return Result{}, err
	}
	if !lookup.Success || strings.TrimSpace(lookup.IP) == "" {
		if lookup.Message == "" {
			lookup.Message = "IP lookup returned no location"
		}
		return Result{}, fmt.Errorf("%s", lookup.Message)
	}
	return Result{
		Platform: string(proxy.Platform), IP: lookup.IP, Country: lookup.Country,
		CountryCode: lookup.CountryCode, Region: lookup.Region, City: lookup.City,
		PortStickyOnly: portStickyOnly,
	}, nil
}

func contextError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// testProxyURL deliberately has no provider branches. The domain module's
// private adapter seam owns every platform's test URL behavior.
func testProxyURL(proxy domain.Proxy, session string) (string, bool, error) {
	return domain.RewriteTestSessionURL(proxy, session)
}

func transportFor(raw string) (*http.Transport, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid test proxy URL")
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	transport := &http.Transport{}
	if ok && base != nil {
		transport = base.Clone()
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
		return transport, nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if parsed.User != nil {
			password, _ := parsed.User.Password()
			auth = &proxy.Auth{User: parsed.User.Username(), Password: password}
		}
		dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		contextDialer, ok := dialer.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("SOCKS5 test proxy does not support context dialing")
		}
		transport.Proxy = nil
		transport.DialContext = contextDialer.DialContext
		return transport, nil
	default:
		return nil, i18n.New("unsupported_scheme")
	}
}
