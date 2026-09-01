package tester

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"stickyproxy/native-plugin/internal/domain"
)

func TestProbeHonorsCallerDeadlineThroughHTTPProxy(t *testing.T) {
	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{}, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-requestStarted:
		default:
			close(requestStarted)
		}
		<-r.Context().Done()
		select {
		case requestCanceled <- struct{}{}:
		default:
		}
	}))
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := domain.NewProxy("dataimpulse", "http://login:secret@"+proxyURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = probe(ctx, proxy, "http://example.test/")
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe error = %v, want context deadline exceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("probe ignored caller deadline: elapsed=%s", elapsed)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("probe never reached HTTP proxy")
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("HTTP proxy request was not canceled")
	}
}

func TestProbeAppliesItsOwnTimeoutThroughHTTPProxy(t *testing.T) {
	requestStarted := make(chan struct{})
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := domain.NewProxy("dataimpulse", "http://login:secret@"+proxyURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err = probeWithTimeout(context.Background(), proxy, "http://example.test/", 200*time.Millisecond)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe error = %v, want context deadline exceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("probe exceeded its timeout: elapsed=%s", elapsed)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("probe never reached HTTP proxy")
	}
}

func TestSOCKS5TransportHonorsDialContextDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	handshakeStarted := make(chan struct{})
	releaseServer := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		greeting := make([]byte, 2)
		if _, readErr := io.ReadFull(conn, greeting); readErr != nil {
			return
		}
		methods := make([]byte, int(greeting[1]))
		if _, readErr := io.ReadFull(conn, methods); readErr != nil {
			return
		}
		close(handshakeStarted)
		<-releaseServer
	}()

	transport, err := transportFor("socks5://user:secret@" + listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	started := time.Now()
	go func() {
		_, dialErr := transport.DialContext(ctx, "tcp", "example.test:80")
		result <- dialErr
	}()
	select {
	case dialErr := <-result:
		if dialErr == nil {
			t.Fatal("SOCKS5 dial unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		close(releaseServer)
		<-serverDone
		t.Fatal("SOCKS5 dial ignored its context deadline")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("SOCKS5 dial ignored deadline: elapsed=%s", elapsed)
	}
	select {
	case <-handshakeStarted:
	case <-time.After(time.Second):
		t.Fatal("dial never reached SOCKS5 listener")
	}
	close(releaseServer)
	<-serverDone
}

func TestRandomSessionIDChangesEveryCall(t *testing.T) {
	first, err := RandomSessionID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := RandomSessionID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "test") || len(first) != 28 {
		t.Fatalf("unexpected test IDs: %q / %q", first, second)
	}
}

func TestRandomDataImpulseProxyUsesFreshSession(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login;sessid.old:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, portStickyOnly, err := testProxyURL(proxy, "testabc")
	if err != nil {
		t.Fatal(err)
	}
	if portStickyOnly || strings.Contains(proxyURL, "sessid.old") || !strings.Contains(proxyURL, ";sessid.testabc") {
		t.Fatalf("random DataImpulse URL = %q", proxyURL)
	}
}

func TestRandomDecodoProxyUsesFreshSession(t *testing.T) {
	proxy, err := domain.NewProxy("decodo", "http://login-session-old-token-sessionduration-10:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, portStickyOnly, err := testProxyURL(proxy, "testabc")
	if err != nil {
		t.Fatal(err)
	}
	want := "user-login-session-testabc-sessionduration-30"
	if portStickyOnly || strings.Contains(proxyURL, "old-token") || !strings.Contains(proxyURL, want) {
		t.Fatalf("random Decodo URL = %q", proxyURL)
	}
}

func TestRandom1024ProxyUsesFreshSIDAndPreservesTTL(t *testing.T) {
	proxy, err := domain.NewProxy("1024proxy", "socks5://login-region-US-sid-template-old-t-opaque-value:secret@us.1024proxy.io:3000")
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := RandomSessionID()
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := RandomSessionID()
	if err != nil {
		t.Fatal(err)
	}
	firstURL, portStickyOnly, err := testProxyURL(proxy, firstSession)
	if err != nil {
		t.Fatal(err)
	}
	secondURL, secondPortStickyOnly, err := testProxyURL(proxy, secondSession)
	if err != nil {
		t.Fatal(err)
	}
	first, err := domain.Parse(firstURL)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.Parse(secondURL)
	if err != nil {
		t.Fatal(err)
	}
	if portStickyOnly || secondPortStickyOnly || firstURL == secondURL ||
		first.Username != "login-region-US-sid-"+firstSession+"-t-opaque-value" ||
		second.Username != "login-region-US-sid-"+secondSession+"-t-opaque-value" ||
		strings.Contains(first.Username, "template-old") {
		t.Fatalf("random 1024Proxy URLs = %q / %q", firstURL, secondURL)
	}
	account, err := domain.Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(firstURL, account.StableHash) || firstURL == account.URL {
		t.Fatalf("test session overlapped account session: test=%q account=%q", firstURL, account.URL)
	}
}

func TestRandomResinProxyUsesTestAccount(t *testing.T) {
	proxy, err := domain.NewProxy("resin", "socks5://ignored:token@resin.example:2260")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, portStickyOnly, err := testProxyURL(proxy, "testabc")
	if err != nil {
		t.Fatal(err)
	}
	if portStickyOnly || !strings.Contains(proxyURL, "Default.test_testabc") {
		t.Fatalf("random Resin URL = %q", proxyURL)
	}
}

func TestGenericProxyUsesBaseURLWithoutTestSession(t *testing.T) {
	proxy, err := domain.NewProxy("generic", "http://operator:secret@proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, portStickyOnly, err := testProxyURL(proxy, "unused-session")
	if err != nil || portStickyOnly || proxyURL != proxy.BaseURL {
		t.Fatalf("generic test URL = %q, portStickyOnly=%v, err=%v", proxyURL, portStickyOnly, err)
	}
}
