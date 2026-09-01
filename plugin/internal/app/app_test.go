package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
	"stickyproxy/native-plugin/internal/state"
	"stickyproxy/native-plugin/internal/tester"
)

type noOpHost struct{}

func (noOpHost) ListAuths() ([]host.AuthEntry, error)      { return nil, nil }
func (noOpHost) GetAuth(string) (host.AuthDocument, error) { return host.AuthDocument{}, nil }
func (noOpHost) SaveAuth(string, json.RawMessage) error    { return nil }

type memoryHost struct {
	entries   []host.AuthEntry
	docs      map[string]host.AuthDocument
	saveCalls int
	failName  string
}

func (m *memoryHost) ListAuths() ([]host.AuthEntry, error) {
	return append([]host.AuthEntry(nil), m.entries...), nil
}
func (m *memoryHost) GetAuth(index string) (host.AuthDocument, error) { return m.docs[index], nil }
func (m *memoryHost) SaveAuth(name string, raw json.RawMessage) error {
	m.saveCalls++
	if name == m.failName {
		return fmt.Errorf("simulated save failure")
	}
	for index, doc := range m.docs {
		if doc.Name == name {
			doc.JSON = append(json.RawMessage(nil), raw...)
			m.docs[index] = doc
		}
	}
	return nil
}

func authDoc(index, name, email, proxyURL string) host.AuthDocument {
	raw, _ := json.Marshal(map[string]any{"email": email, "proxy_url": proxyURL})
	return host.AuthDocument{AuthIndex: index, Name: name, JSON: raw}
}

type failingHost struct{}

func (failingHost) ListAuths() ([]host.AuthEntry, error) {
	return []host.AuthEntry{{AuthIndex: "auth-1", Name: "alice.json", Email: "alice@example.com"}}, nil
}
func (failingHost) GetAuth(string) (host.AuthDocument, error) {
	return host.AuthDocument{}, fmt.Errorf("simulated host failure")
}
func (failingHost) SaveAuth(string, json.RawMessage) error { return nil }

func newTestApp(t *testing.T) App {
	t.Helper()
	store, err := state.New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return App{Store: store, Host: noOpHost{}}
}

func call(t *testing.T, application App, method, path, body string) map[string]any {
	t.Helper()
	response := application.Handle(Request{
		Method: method, Path: path,
		Headers: map[string][]string{"Accept-Language": {"en-US"}},
		Body:    []byte(body),
	})
	var out map[string]any
	if err := json.Unmarshal(response.Body, &out); err != nil {
		t.Fatalf("status=%d decode response: %v; body=%s", response.Status, err, response.Body)
	}
	if response.Status >= 400 {
		t.Fatalf("status=%d response=%v", response.Status, out)
	}
	return out
}

func TestStateMasksBaseURLAndPreviewNeverReturnsRequestURL(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"name":"Decodo_US","platform":"decodo",
		"base_url":"http://login:secret@gate.decodo.com:7000"
	}`)
	proxy := created["proxy"].(map[string]any)

	stateResponse := call(t, application, http.MethodGet, "/v0/management/stickyproxy/state", "")
	raw, _ := json.Marshal(stateResponse)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "login-session") {
		t.Fatalf("state response leaks base credentials: %s", raw)
	}

	preview := call(t, application, http.MethodPost, "/v0/management/stickyproxy/preview", `{"proxy_id":"`+proxy["id"].(string)+`","email":"alice@example.com"}`)
	if strings.Contains(preview["masked_effective_url"].(string), "secret") {
		t.Fatalf("preview leaked password: %#v", preview)
	}
	if _, exposed := preview["proxy_url"]; exposed {
		t.Fatalf("preview exposed full proxy URL: %#v", preview)
	}
	response := application.Handle(Request{Method: http.MethodPost, Path: "/v0/management/stickyproxy/resolve", Body: []byte(`{"email":"alice@example.com"}`)})
	if response.Status != http.StatusNotFound {
		t.Fatalf("full resolve must be unavailable: status=%d body=%s", response.Status, response.Body)
	}
}

func TestLegacyAccountProxyConfigsRouteIsUnavailable(t *testing.T) {
	application := newTestApp(t)
	response := application.Handle(Request{Method: http.MethodGet, Path: "/v0/management/stickyproxy/account-proxy-configs"})
	if response.Status != http.StatusNotFound {
		t.Fatalf("legacy account route status=%d, want %d", response.Status, http.StatusNotFound)
	}
}

func Test1024ProxyResponsesRedactCredentialsAndTestSessionData(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"name":"US_1024_Residential","platform":"1024proxy",
		"base_url":"socks5://login-region-US-sid-template-old-t-60:secret@example.test:3000"
	}`)
	proxy := created["proxy"].(map[string]any)
	proxyID := proxy["id"].(string)

	stateResponse := call(t, application, http.MethodGet, "/v0/management/stickyproxy/state", "")
	stateJSON := stringMustJSON(t, stateResponse)
	if strings.Contains(stateJSON, "secret") || strings.Contains(stateJSON, ":secret@") {
		t.Fatalf("state response leaked 1024Proxy credential: %s", stateJSON)
	}
	preview := call(t, application, http.MethodPost, "/v0/management/stickyproxy/preview", `{"proxy_id":"`+proxyID+`","email":"alice@example.com"}`)
	previewURL := preview["masked_effective_url"].(string)
	if strings.Contains(previewURL, "secret") || strings.Contains(previewURL, "alice@example.com") || !strings.Contains(previewURL, ":***@") {
		t.Fatalf("preview did not redact 1024Proxy URL: %#v", preview)
	}

	stored, found, err := application.Store.Proxy(proxyID)
	if err != nil || !found {
		t.Fatalf("stored proxy err=%v found=%v", err, found)
	}
	accountURL, err := domain.Rewrite(stored, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	application.Host = &memoryHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}},
		docs: map[string]host.AuthDocument{
			"a": authDoc("a", "alice.json", "alice@example.com", accountURL.URL),
		},
	}
	accountsResponse := call(t, application, http.MethodGet, "/v0/management/stickyproxy/accounts", "")
	accountsJSON := stringMustJSON(t, accountsResponse)
	if strings.Contains(accountsJSON, "secret") || strings.Contains(accountsJSON, "template-old") || strings.Contains(accountsJSON, accountURL.StableHash) && strings.Contains(accountsJSON, ":password@") {
		t.Fatalf("account response leaked raw 1024Proxy URL: %s", accountsJSON)
	}

	application.ProbeProxy = func(input domain.Proxy) (tester.Result, error) {
		if input.ID != proxyID || !strings.Contains(input.BaseURL, "secret") {
			return tester.Result{}, fmt.Errorf("unexpected probe proxy")
		}
		return tester.Result{Platform: "1024proxy", IP: "198.51.100.9", Country: "United States", CountryCode: "US", Region: "California", City: "Los Angeles"}, nil
	}
	tested := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies/test", `{"proxy_id":"`+proxyID+`"}`)
	testedJSON := stringMustJSON(t, tested)
	if strings.Contains(testedJSON, "secret") || strings.Contains(testedJSON, "template-old") || strings.Contains(testedJSON, "test012") {
		t.Fatalf("test response leaked 1024Proxy auth or session data: %s", testedJSON)
	}
	probe := tested["test"].(map[string]any)
	if probe["platform"] != "1024proxy" || probe["ip"] != "198.51.100.9" {
		t.Fatalf("test response lost expected egress metadata: %#v", tested)
	}
}

func Test1024ProxyValidationResponsesExposeOnlyStableErrorCodes(t *testing.T) {
	application := newTestApp(t)
	response := application.Handle(Request{
		Method: http.MethodPost,
		Path:   "/v0/management/stickyproxy/proxies",
		Body:   []byte(`{"name":"US_1024","platform":"1024proxy","base_url":"socks5://login-sid-:secret@example.test:3000"}`),
	})
	if response.Status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Status, response.Body)
	}
	if strings.Contains(string(response.Body), "secret") || strings.Contains(string(response.Body), "login-sid-") {
		t.Fatalf("validation response leaked input: %s", response.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "invalid_1024_sid" || len(payload) != 2 {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestGenericProxyPreviewUsesBaseURLWithoutIdentity(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"name":"Office_Egress","platform":"generic",
		"base_url":"socks5h://operator:secret@example.test:1080"
	}`)
	proxy := created["proxy"].(map[string]any)
	if proxy["platform"] != "generic" || strings.Contains(stringMustJSON(t, proxy), "secret") {
		t.Fatalf("generic proxy response=%#v", proxy)
	}
	preview := call(t, application, http.MethodPost, "/v0/management/stickyproxy/preview", `{"proxy_id":"`+proxy["id"].(string)+`","email":"not-an-email"}`)
	masked := preview["masked_effective_url"].(string)
	if preview["platform"] != "generic" || strings.Contains(masked, "secret") || !strings.Contains(masked, "operator:***@example.test:1080") {
		t.Fatalf("generic preview=%#v", preview)
	}
}

func TestFailureResponsesExposeOnlyStableErrorCodes(t *testing.T) {
	application := newTestApp(t)
	response := application.Handle(Request{Method: http.MethodPost, Path: "/v0/management/stickyproxy/proxies", Body: []byte(`{"name":"invalid name","platform":"decodo","base_url":"http://login:secret@gate.decodo.com:7000"}`)})
	if response.Status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Status, response.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "invalid_proxy_name" {
		t.Fatalf("payload=%#v", payload)
	}
	if _, hasText := payload["error"]; hasText {
		t.Fatalf("backend must only expose codes: %#v", payload)
	}
}

func TestPreviewRequiresExplicitProxy(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	preview := call(t, application, http.MethodPost, "/v0/management/stickyproxy/preview", `{"proxy_id":"`+proxyID+`","email":"alice@example.com"}`)
	if _, ok := preview["masked_effective_url"].(string); !ok {
		t.Fatalf("preview=%#v", preview)
	}
}

func TestApplyProxyChangesOnlySelectedAccounts(t *testing.T) {
	application := newTestApp(t)
	first := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_A","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"}`)
	second := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_B","platform":"decodo","base_url":"http://other:secret@gate.decodo.com:7000"}`)
	firstID := first["proxy"].(map[string]any)["id"].(string)
	secondID := second["proxy"].(map[string]any)["id"].(string)
	memory := &memoryHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}},
		docs:    map[string]host.AuthDocument{"a": authDoc("a", "alice.json", "alice@example.com", ""), "b": authDoc("b", "bob.json", "bob@example.com", "")},
	}
	application.Host = memory
	call(t, application, http.MethodPost, "/v0/management/stickyproxy/apply", `{"proxy_id":"`+firstID+`","auth_indexes":["a"]}`)
	_, mappings, err := application.Store.Snapshot()
	if err != nil || len(mappings) != 1 || mappings[0].AuthIndex != "a" || mappings[0].ProxyID != firstID {
		t.Fatalf("mappings after apply=%#v err=%v", mappings, err)
	}
	aliceA := documentProxyURL(t, memory.docs["a"].JSON)
	call(t, application, http.MethodPost, "/v0/management/stickyproxy/apply", `{"proxy_id":"`+secondID+`","auth_indexes":["b"]}`)
	if got := documentProxyURL(t, memory.docs["a"].JSON); got != aliceA {
		t.Fatalf("alice changed while applying B to bob: %q", got)
	}
	if got := documentProxyURL(t, memory.docs["b"].JSON); !strings.Contains(got, "-session-") {
		t.Fatalf("bob did not receive B: %q", got)
	}
	call(t, application, http.MethodPost, "/v0/management/stickyproxy/apply", `{"proxy_id":"`+secondID+`","auth_indexes":["a"]}`)
	if got := documentProxyURL(t, memory.docs["a"].JSON); got == aliceA {
		t.Fatalf("alice was not overwritten by B")
	}
	_, mappings, err = application.Store.Snapshot()
	if err != nil || len(mappings) != 2 {
		t.Fatalf("mappings after proxy switch=%#v err=%v", mappings, err)
	}
	for _, mapping := range mappings {
		if mapping.AuthIndex == "a" && mapping.ProxyID != secondID {
			t.Fatalf("alice mapping=%#v want proxy %s", mapping, secondID)
		}
	}
}

func TestClearAppliedOnlyClearsSelectedMatchingAccounts(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_A","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	proxy, found, err := application.Store.Proxy(proxyID)
	if err != nil || !found {
		t.Fatalf("proxy err=%v found=%v", err, found)
	}
	alice, _ := domain.Rewrite(proxy, "alice@example.com")
	bob, _ := domain.Rewrite(proxy, "bob@example.com")
	memory := &memoryHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}, {AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"}},
		docs:    map[string]host.AuthDocument{"a": authDoc("a", "alice.json", "alice@example.com", alice.URL), "b": authDoc("b", "bob.json", "bob@example.com", bob.URL), "c": authDoc("c", "carol.json", "carol@example.com", "http://manual:manual@other.example:8080")},
	}
	application.Host = memory
	if err := application.Store.UpdateMappings([]state.AccountProxyMapping{{AuthIndex: "a", ProxyID: proxyID}, {AuthIndex: "b", ProxyID: proxyID}}, nil); err != nil {
		t.Fatal(err)
	}
	call(t, application, http.MethodPost, "/v0/management/stickyproxy/clear-applied", `{"proxy_id":"`+proxyID+`","auth_indexes":["a","c"]}`)
	if got := documentProxyURL(t, memory.docs["a"].JSON); got != "" {
		t.Fatalf("selected matching account not cleared: %q", got)
	}
	if got := documentProxyURL(t, memory.docs["b"].JSON); got != bob.URL {
		t.Fatalf("unselected matching account cleared: %q", got)
	}
	if got := documentProxyURL(t, memory.docs["c"].JSON); got != "http://manual:manual@other.example:8080" {
		t.Fatalf("manual account cleared: %q", got)
	}
}

func TestApplyContinuesAfterAccountFailureAndMapsOnlySuccesses(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_A","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	memory := &memoryHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}, {AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"}},
		docs: map[string]host.AuthDocument{
			"a": authDoc("a", "alice.json", "alice@example.com", ""),
			"b": authDoc("b", "bob.json", "bob@example.com", ""),
			"c": authDoc("c", "carol.json", "carol@example.com", ""),
		},
		failName: "bob.json",
	}
	application.Host = memory
	response := call(t, application, http.MethodPost, "/v0/management/stickyproxy/apply", `{"proxy_id":"`+proxyID+`","auth_indexes":["a","b","c"]}`)
	sync := response["sync"].(map[string]any)
	if sync["updated"] != float64(2) || len(sync["errors"].([]any)) != 1 {
		t.Fatalf("sync=%#v", sync)
	}
	if documentProxyURL(t, memory.docs["a"].JSON) == "" || documentProxyURL(t, memory.docs["b"].JSON) != "" || documentProxyURL(t, memory.docs["c"].JSON) == "" {
		t.Fatalf("documents=%#v", memory.docs)
	}
	_, mappings, err := application.Store.Snapshot()
	if err != nil || len(mappings) != 2 {
		t.Fatalf("mappings=%#v err=%v", mappings, err)
	}
	for _, mapping := range mappings {
		if mapping.ProxyID != proxyID || mapping.AuthIndex == "b" {
			t.Fatalf("unexpected mapping=%#v", mapping)
		}
	}
}

func TestEmptySelectionReturnsBadRequestWithSelectionRequired(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_A","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	for _, path := range []string{"/apply", "/clear-proxy", "/clear-applied"} {
		response := application.Handle(Request{
			Method: http.MethodPost,
			Path:   "/v0/management/stickyproxy" + path,
			Headers: map[string][]string{"Accept-Language": {"en-US"}},
			Body:   []byte(`{"proxy_id":"` + proxyID + `","auth_indexes":[]}`),
		})
		if response.Status != http.StatusBadRequest {
			t.Errorf("%s status=%d body=%s, want %d", path, response.Status, response.Body, http.StatusBadRequest)
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["code"] != "selection_required" || payload["ok"] != false {
			t.Errorf("%s payload=%#v", path, payload)
		}
	}
}

func TestFilteredSelectionMatchingNothingIsRejected(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{"name":"Proxy_A","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	memory := &memoryHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com", Provider: "codex"}},
		docs:    map[string]host.AuthDocument{"a": authDoc("a", "alice.json", "alice@example.com", "")},
	}
	application.Host = memory
	response := application.Handle(Request{
		Method: http.MethodPost,
		Path:   "/v0/management/stickyproxy/apply",
		Body:   []byte(`{"proxy_id":"` + proxyID + `","selection":{"kind":"filtered","filters":{"provider":"nomatch"}}}`),
	})
	if response.Status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want %d", response.Status, response.Body, http.StatusBadRequest)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "selection_required" {
		t.Fatalf("payload=%#v", payload)
	}
}

func documentProxyURL(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value struct {
		ProxyURL string `json:"proxy_url"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value.ProxyURL
}

func TestProxyTestDoesNotHoldSyncLockDuringProxyUpdate(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	proxyID := created["proxy"].(map[string]any)["id"].(string)
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var startOnce sync.Once
	application.ProbeProxy = func(domain.Proxy) (tester.Result, error) {
		startOnce.Do(func() { close(probeStarted) })
		<-releaseProbe
		return tester.Result{Platform: "dataimpulse", IP: "198.51.100.9"}, nil
	}

	testDone := make(chan Response, 1)
	go func() {
		testDone <- application.Handle(Request{
			Method: http.MethodPost, Path: "/v0/management/stickyproxy/proxies/test",
			Body: []byte(`{"proxy_id":"` + proxyID + `"}`),
		})
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}

	updateDone := make(chan Response, 1)
	go func() {
		updateDone <- application.Handle(Request{
			Method: http.MethodPost, Path: "/v0/management/stickyproxy/proxies",
			Body: []byte(`{"name":"second","platform":"dataimpulse","base_url":"http://other:secret@gw.dataimpulse.com:10000"}`),
		})
	}()
	select {
	case response := <-updateDone:
		if response.Status != http.StatusOK {
			t.Errorf("proxy update status=%d body=%s", response.Status, response.Body)
		}
	case <-time.After(time.Second):
		close(releaseProbe)
		<-testDone
		t.Fatal("proxy update blocked while proxy probe was running")
	}
	close(releaseProbe)
	select {
	case response := <-testDone:
		if response.Status != http.StatusOK {
			t.Errorf("probe status=%d body=%s", response.Status, response.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("probe request did not finish after release")
	}
}

func TestProxyTestReturnsOnlyEgressMetadata(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	proxy := created["proxy"].(map[string]any)
	application.ProbeProxy = func(input domain.Proxy) (tester.Result, error) {
		if strings.Contains(input.BaseURL, "secret") {
			return tester.Result{Platform: "dataimpulse", IP: "198.51.100.9", Country: "United States", CountryCode: "US", Region: "California", City: "Los Angeles"}, nil
		}
		return tester.Result{}, fmt.Errorf("unexpected proxy")
	}
	result := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies/test", `{"proxy_id":"`+proxy["id"].(string)+`"}`)
	probe := result["test"].(map[string]any)
	if probe["ip"] != "198.51.100.9" || probe["country"] != "United States" || strings.Contains(stringMustJSON(t, result), "secret") {
		t.Fatalf("test result=%#v", result)
	}
}

func TestProxyTestFailsWithoutLeakingCredential(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	proxy := created["proxy"].(map[string]any)
	application.ProbeProxy = func(domain.Proxy) (tester.Result, error) { return tester.Result{}, fmt.Errorf("secret failure") }
	response := application.Handle(Request{Method: http.MethodPost, Path: "/v0/management/stickyproxy/proxies/test", Body: []byte(`{"proxy_id":"` + proxy["id"].(string) + `"}`)})
	if response.Status != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", response.Status, response.Body)
	}
	if strings.Contains(string(response.Body), "secret") {
		t.Fatalf("test error leaked proxy credential: %s", response.Body)
	}
}

func stringMustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestBulkAcceptsSupportedLinesAndReportsRejectedOne(t *testing.T) {
	application := newTestApp(t)
	response := application.Handle(Request{
		Method: http.MethodPost, Path: "/v0/management/stickyproxy/proxies/bulk",
		Body: []byte("http://login:secret@gate.decodo.com:7000|decodo\nvmess://unsupported|decodo\nsocks5://ignored:token@resin.example:2260|resin"),
	})
	// This deliberately uses an invalid JSON body; ensure public errors remain structured.
	if response.Status != http.StatusBadRequest {
		t.Fatalf("invalid bulk status=%d", response.Status)
	}
	valid := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies/bulk", `{
		"text":"http://login:secret@gate.decodo.com:7000|decodo\nvmess://unsupported|decodo\nsocks5://ignored:token@resin.example:2260|resin\nhttp://user:pa%7Css@gate.decodo.com:7999|decodo"
	}`)
	if valid["created"].(float64) != 3 || valid["rejected"].(float64) != 1 {
		t.Fatalf("bulk result=%#v", valid)
	}
	// The fourth line's password contains an encoded pipe; only the final
	// separator may split, so the full host:port must survive parsing.
	last := valid["items"].([]any)[2].(map[string]any)
	if last["host"] != "gate.decodo.com" {
		t.Fatalf("piped-password line lost its host: %#v", last)
	}
}
