package syncer

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
)

type fakeHost struct {
	entries   []host.AuthEntry
	documents map[string]host.AuthDocument
	saved     map[string]json.RawMessage
	failName  string
}

func (f *fakeHost) ListAuths() ([]host.AuthEntry, error) {
	return append([]host.AuthEntry(nil), f.entries...), nil
}
func (f *fakeHost) GetAuth(index string) (host.AuthDocument, error) {
	doc, ok := f.documents[index]
	if !ok {
		return host.AuthDocument{}, errors.New("missing auth")
	}
	return doc, nil
}
func (f *fakeHost) SaveAuth(name string, raw json.RawMessage) error {
	if name == f.failName {
		return errors.New("simulated save failure")
	}
	if f.saved == nil {
		f.saved = map[string]json.RawMessage{}
	}
	f.saved[name] = append(json.RawMessage(nil), raw...)
	for index, doc := range f.documents {
		if doc.Name == name {
			doc.JSON = append(json.RawMessage(nil), raw...)
			f.documents[index] = doc
		}
	}
	return nil
}

func authDocument(index, name, email, proxyURL string) host.AuthDocument {
	data, _ := json.Marshal(map[string]any{"type": "codex", "email": email, "access_token": "secret", "proxy_url": proxyURL})
	return host.AuthDocument{AuthIndex: index, Name: name, JSON: data}
}

func proxyURL(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := json.Unmarshal(doc["proxy_url"], &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestApplySynchronizesAndIsIdempotent(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:password@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries:   []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}, {AuthIndex: "ignored", Name: "none.json"}},
		documents: map[string]host.AuthDocument{"a": authDocument("a", "alice.json", "alice@example.com", ""), "b": authDocument("b", "bob.json", "bob@example.com", "")},
	}
	result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Selected != 2 || result.Updated != 2 || result.Skipped != 0 || result.AlreadyApplied != 0 {
		t.Fatalf("first result=%#v", result)
	}
	alice := proxyURL(t, client.saved["alice.json"])
	bob := proxyURL(t, client.saved["bob.json"])
	if alice == bob || !strings.Contains(alice, ";sessid.") {
		t.Fatalf("rewrites alice=%q bob=%q", alice, bob)
	}
	result = (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 0 || result.AlreadyApplied != 2 {
		t.Fatalf("second result=%#v", result)
	}
}

func TestApplySelected1024ProxyIsIdempotent(t *testing.T) {
	proxy, err := domain.NewProxy("1024proxy", "socks5://login-region-US-sid-template-old-t-60:password@example.test:3000")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
		},
		documents: map[string]host.AuthDocument{
			"a": authDocument("a", "alice.json", "alice@example.com", ""),
			"b": authDocument("b", "bob.json", "bob@example.com", ""),
		},
	}

	result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 2 || result.AlreadyApplied != 0 {
		t.Fatalf("first result=%#v", result)
	}
	alice := proxyURL(t, client.documents["a"].JSON)
	bob := proxyURL(t, client.documents["b"].JSON)
	aliceParts, err := domain.Parse(alice)
	if err != nil {
		t.Fatal(err)
	}
	if alice == bob || !strings.Contains(aliceParts.Username, "-sid-") || !strings.HasSuffix(aliceParts.Username, "-t-60") {
		t.Fatalf("unexpected 1024Proxy rewrites: alice=%q bob=%q", alice, bob)
	}

	result = (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 0 || result.AlreadyApplied != 2 {
		t.Fatalf("second result=%#v", result)
	}
}

func TestApplyAndClearGenericProxy(t *testing.T) {
	proxy, err := domain.NewProxy("generic", "socks5://operator:password@proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
			{AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"},
		},
		documents: map[string]host.AuthDocument{
			"a": authDocument("a", "alice.json", "alice@example.com", ""),
			"b": authDocument("b", "bob.json", "bob@example.com", ""),
			"c": authDocument("c", "carol.json", "carol@example.com", "http://manual:manual@other.example:8080"),
		},
	}

	result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 2 || proxyURL(t, client.documents["a"].JSON) != proxy.BaseURL || proxyURL(t, client.documents["b"].JSON) != proxy.BaseURL {
		t.Fatalf("generic apply result=%#v alice=%q bob=%q", result, proxyURL(t, client.documents["a"].JSON), proxyURL(t, client.documents["b"].JSON))
	}

	result = (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 0 || result.AlreadyApplied != 2 {
		t.Fatalf("generic repeat result=%#v", result)
	}

	result = (Synchronizer{Host: client}).ApplySelected(nil, []string{"a"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 || proxyURL(t, client.documents["a"].JSON) != "" || proxyURL(t, client.documents["b"].JSON) != proxy.BaseURL || proxyURL(t, client.documents["c"].JSON) != "http://manual:manual@other.example:8080" {
		t.Fatalf("generic clear result=%#v", result)
	}
}

func TestClearMatching1024ProxyOnlyClearsExpectedURLs(t *testing.T) {
	proxy, err := domain.NewProxy("1024proxy", "socks5://login-region-US-t-60:password@example.test:3000")
	if err != nil {
		t.Fatal(err)
	}
	other, err := domain.NewProxy("1024proxy", "socks5://other-region-US:password@example.test:3000")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := domain.Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := domain.Rewrite(other, "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
			{AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"},
		},
		documents: map[string]host.AuthDocument{
			"a": authDocument("a", "alice.json", "alice@example.com", alice.URL),
			"b": authDocument("b", "bob.json", "bob@example.com", bob.URL),
			"c": authDocument("c", "carol.json", "carol@example.com", "http://manual:manual@other.example:8080"),
		},
	}

	result := (Synchronizer{Host: client}).ApplySelected(nil, []string{"a", "b", "c"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Selected != 3 || result.Updated != 3 {
		t.Fatalf("clear result=%#v", result)
	}
	if got := proxyURL(t, client.documents["a"].JSON); got != "" {
		t.Fatalf("matching 1024Proxy URL was not cleared: %q", got)
	}
	if got := proxyURL(t, client.documents["b"].JSON); got != "" {
		t.Fatalf("selected 1024Proxy configuration was not cleared: %q", got)
	}
	if got := proxyURL(t, client.documents["c"].JSON); got != "" {
		t.Fatalf("selected manual proxy was not cleared: %q", got)
	}
}

func TestApplyContinuesAfterSaveFailure(t *testing.T) {
	proxy, err := domain.NewProxy("decodo", "http://login:password@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	originalAlice := authDocument("a", "alice.json", "alice@example.com", "http://old:old@proxy.example:8080")
	originalBob := authDocument("b", "bob.json", "bob@example.com", "http://old:old@proxy.example:8080")
	client := &fakeHost{
		entries:   []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}},
		documents: map[string]host.AuthDocument{"a": originalAlice, "b": originalBob}, failName: "bob.json",
	}
	result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "b"})
	if len(result.Errors) != 1 || result.Updated != 1 || len(result.SucceededAuthIndexes) != 1 || result.SucceededAuthIndexes[0] != "a" {
		t.Fatalf("result=%#v", result)
	}
	if got := proxyURL(t, client.documents["a"].JSON); got == "http://old:old@proxy.example:8080" {
		t.Fatalf("alice was not updated: %q", got)
	}
}

func TestApplySelectedOnlyMutatesExplicitAccounts(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:password@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"}},
		documents: map[string]host.AuthDocument{
			"a": authDocument("a", "alice.json", "alice@example.com", ""),
			"b": authDocument("b", "bob.json", "bob@example.com", "http://manual:manual@other.example:8080"),
		},
	}
	result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Selected != 1 || result.Updated != 1 || result.Eligible != 1 {
		t.Fatalf("result=%#v", result)
	}
	if got := proxyURL(t, client.documents["b"].JSON); got != "http://manual:manual@other.example:8080" {
		t.Fatalf("unselected account was modified: %q", got)
	}
	if got := proxyURL(t, client.documents["a"].JSON); !strings.Contains(got, ";sessid.") {
		t.Fatalf("selected account did not receive sticky proxy: %q", got)
	}
}

func TestApplySelectedRejectsDuplicateAndNoEmail(t *testing.T) {
	client := &fakeHost{
		entries:   []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}, {AuthIndex: "n", Name: "none.json"}},
		documents: map[string]host.AuthDocument{"a": authDocument("a", "alice.json", "alice@example.com", "")},
	}
	proxy, err := domain.NewProxy("generic", "http://proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	if result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"a", "a"}); len(result.Errors) == 0 || len(client.saved) != 0 {
		t.Fatalf("duplicate selection result=%#v", result)
	}
	if result := (Synchronizer{Host: client}).ApplySelected(&proxy, []string{"n"}); len(result.Errors) == 0 {
		t.Fatalf("no-email selection result=%#v", result)
	}
}

func TestClearSelectedClearsOnlySelectedAccounts(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:password@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	other, err := domain.NewProxy("decodo", "http://other:password@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := domain.Rewrite(proxy, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := domain.Rewrite(other, "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
			{AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"},
		},
		documents: map[string]host.AuthDocument{
			"a": authDocument("a", "alice.json", "alice@example.com", alice.URL),
			"b": authDocument("b", "bob.json", "bob@example.com", bob.URL),
			"c": authDocument("c", "carol.json", "carol@example.com", "http://manual:manual@other.example:8080"),
		},
	}
	result := (Synchronizer{Host: client}).ApplySelected(nil, []string{"a", "c"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "clear" || result.Selected != 2 || result.Updated != 2 {
		t.Fatalf("result=%#v", result)
	}
	if got := proxyURL(t, client.documents["a"].JSON); got != "" {
		t.Fatalf("matched account not cleared: %q", got)
	}
	if got := proxyURL(t, client.documents["b"].JSON); got != bob.URL {
		t.Fatalf("other proxy was cleared: %q", got)
	}
	if got := proxyURL(t, client.documents["c"].JSON); got != "" {
		t.Fatalf("selected manual proxy was not cleared: %q", got)
	}
}

func TestApplyNilClearsExistingProxyURLs(t *testing.T) {
	client := &fakeHost{
		entries:   []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}},
		documents: map[string]host.AuthDocument{"a": authDocument("a", "alice.json", "alice@example.com", "http://old:old@proxy.example:8080")},
	}
	result := (Synchronizer{Host: client}).ApplySelected(nil, []string{"a"})
	if err := result.Error(); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "clear" || result.Selected != 1 || result.Updated != 1 {
		t.Fatalf("result=%#v", result)
	}
	if got := proxyURL(t, client.documents["a"].JSON); got != "" {
		t.Fatalf("proxy URL=%q, want empty", got)
	}
}
