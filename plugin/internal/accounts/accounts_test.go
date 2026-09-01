package accounts

import (
	"encoding/json"
	"strings"
	"testing"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
)

type fakeHost struct {
	entries []host.AuthEntry
	docs    map[string]host.AuthDocument
}

func (f fakeHost) ListAuths() ([]host.AuthEntry, error)            { return f.entries, nil }
func (f fakeHost) GetAuth(index string) (host.AuthDocument, error) { return f.docs[index], nil }
func (f fakeHost) SaveAuth(string, json.RawMessage) error          { return nil }

type countingHost struct {
	fakeHost
	gets []string
}

func (f *countingHost) GetAuth(index string) (host.AuthDocument, error) {
	f.gets = append(f.gets, index)
	return f.docs[index], nil
}

func doc(index, name, proxyURL string) host.AuthDocument {
	raw, _ := json.Marshal(map[string]any{"proxy_url": proxyURL})
	return host.AuthDocument{AuthIndex: index, Name: name, JSON: raw}
}

func TestListClassifiesAndFiltersProxyState(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	proxy.Name = "Proxy_01"
	client := fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com", Provider: "codex", Type: "oauth", Status: "active", Priority: 10, Note: "production"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com", Provider: "claude", Type: "oauth", Status: "active", Priority: 0},
			{AuthIndex: "c", Name: "carol.json", Email: "carol@example.com", Provider: "codex", Type: "oauth", Status: "active", Priority: 10},
			{AuthIndex: "d", Name: "no-email.json", Provider: "codex", Type: "oauth", Status: "active", Priority: 0},
		},
		docs: map[string]host.AuthDocument{
			"a": doc("a", "alice.json", "http://login:secret@gw.dataimpulse.com:10000"),
			"b": doc("b", "bob.json", ""),
			"c": doc("c", "carol.json", "http://other:pass@other.example:8080"),
		},
	}
	page, err := List(client, []domain.Proxy{proxy}, map[string]string{"a": proxy.ID}, Filters{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || !page.TotalKnown {
		t.Fatalf("total=%d known=%v", page.Total, page.TotalKnown)
	}
	if page.Summary[StateStickyApplied] != 1 || page.Summary[StateInheritsSystem] != 1 || page.Summary[StateCustomOther] != 1 || page.Summary[StateNoEmail] != 1 {
		t.Fatalf("summary=%#v", page.Summary)
	}
	if got, want := page.Facets.Providers, []string{"claude", "codex"}; !sameStrings(got, want) {
		t.Fatalf("providers=%#v want=%#v", got, want)
	}
	if got, want := page.Facets.Types, []string{"oauth"}; !sameStrings(got, want) {
		t.Fatalf("types=%#v want=%#v", got, want)
	}
	if got, want := page.Facets.Statuses, []string{"active"}; !sameStrings(got, want) {
		t.Fatalf("statuses=%#v want=%#v", got, want)
	}
	if got, want := page.Facets.Priorities, []int{0, 10}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("priorities=%#v want=%#v", got, want)
	}
	states := map[string]Account{}
	for _, account := range page.Items {
		states[account.AuthIndex] = account
	}
	if !states["a"].Selectable || states["a"].ProxyState != StateStickyApplied || states["a"].ProxyName != "Proxy_01" {
		t.Fatalf("alice=%#v", states["a"])
	}
	if states["b"].ProxyState != StateInheritsSystem || states["b"].ProxyName != "" || states["c"].ProxyState != StateCustomOther || states["c"].ProxyName != "" || states["d"].Selectable {
		t.Fatalf("states=%#v", states)
	}
	if states["a"].MaskedProxyURL != "" {
		t.Fatalf("mapped account must not expose a proxy URL: %q", states["a"].MaskedProxyURL)
	}
	encoded, err := json.Marshal(states["a"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "masked_expected_url") {
		t.Fatalf("mapped account response contains a derived URL field: %s", encoded)
	}

	filtered, err := List(client, []domain.Proxy{proxy}, map[string]string{"a": proxy.ID}, Filters{Providers: []string{"codex"}, ProxyStates: []string{StateCustomOther}, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.TotalKnown || len(filtered.Items) != 1 || filtered.Items[0].AuthIndex != "c" {
		t.Fatalf("filtered=%#v", filtered)
	}
}

func TestListRecognizesGenericProxy(t *testing.T) {
	proxy, err := domain.NewProxy("generic", "http://operator:secret@proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	proxy.Name = "Generic_01"
	client := fakeHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}},
		docs:    map[string]host.AuthDocument{"a": doc("a", "alice.json", proxy.BaseURL)},
	}
	page, err := List(client, []domain.Proxy{proxy}, map[string]string{"a": proxy.ID}, Filters{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ProxyState != StateStickyApplied || page.Items[0].ProxyName != proxy.Name {
		t.Fatalf("generic account state=%#v", page.Items)
	}
}

func TestListReadsOnlyVisiblePageWithoutProxyStateFilter(t *testing.T) {
	entries := make([]host.AuthEntry, 0, 120)
	docs := make(map[string]host.AuthDocument, 120)
	for index := 0; index < 120; index++ {
		id := string(rune('a'+index/10)) + string(rune('0'+index%10))
		email := id + "@example.com"
		entries = append(entries, host.AuthEntry{AuthIndex: id, Name: id + ".json", Email: email, Provider: "codex", Type: "oauth", Status: "active"})
		docs[id] = doc(id, id+".json", "")
	}
	client := &countingHost{fakeHost: fakeHost{entries: entries, docs: docs}}
	page, err := List(client, nil, nil, Filters{Page: 2, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 120 || len(page.Items) != 50 {
		t.Fatalf("page=%#v", page)
	}
	if len(client.gets) != 50 {
		t.Fatalf("GetAuth calls=%d want=50", len(client.gets))
	}
}

func TestListProxyStateUsesCursorWithoutExactTotal(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	entries := []host.AuthEntry{}
	docs := map[string]host.AuthDocument{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		entries = append(entries, host.AuthEntry{AuthIndex: id, Name: id + ".json", Email: id + "@example.com", Provider: "codex", Type: "oauth", Status: "active"})
		docs[id] = doc(id, id+".json", "http://other:pass@other.example:8080")
	}
	first, err := List(fakeHost{entries: entries, docs: docs}, []domain.Proxy{proxy}, nil, Filters{ProxyStates: []string{StateCustomOther}, Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalKnown || len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" || first.Items[0].AuthIndex != "a" || first.Items[1].AuthIndex != "b" {
		t.Fatalf("first=%#v", first)
	}
	second, err := List(fakeHost{entries: entries, docs: docs}, []domain.Proxy{proxy}, nil, Filters{ProxyStates: []string{StateCustomOther}, Page: 2, PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || !second.HasMore || second.Items[0].AuthIndex != "c" || second.Items[1].AuthIndex != "d" {
		t.Fatalf("second=%#v", second)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestListMappedProxyFilterUsesCursorPagination(t *testing.T) {
	proxy, err := domain.NewProxy("generic", "http://proxy.example:8080")
	if err != nil {
		t.Fatal(err)
	}
	proxy.Name = "Mapped"
	entries := make([]host.AuthEntry, 0, 5)
	mappings := make(map[string]string, 5)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		entries = append(entries, host.AuthEntry{AuthIndex: id, Name: id + ".json", Email: id + "@example.com"})
		mappings[id] = proxy.ID
	}
	client := &countingHost{fakeHost: fakeHost{entries: entries, docs: map[string]host.AuthDocument{}}}
	first, err := List(client, []domain.Proxy{proxy}, mappings, Filters{AppliedProxyIDs: []string{proxy.ID}, Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !first.TotalKnown || first.Total != 5 || !first.HasMore || first.NextCursor == "" || len(first.Items) != 2 || first.Items[0].AuthIndex != "a" || first.Items[1].AuthIndex != "b" {
		t.Fatalf("first=%#v", first)
	}
	second, err := List(client, []domain.Proxy{proxy}, mappings, Filters{AppliedProxyIDs: []string{proxy.ID}, Page: 2, PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if !second.TotalKnown || !second.HasMore || second.NextCursor == "" || len(second.Items) != 2 || second.Items[0].AuthIndex != "c" || second.Items[1].AuthIndex != "d" {
		t.Fatalf("second=%#v", second)
	}
	if len(client.gets) != 0 {
		t.Fatalf("mapped filtering must not read auth files; GetAuth calls=%d", len(client.gets))
	}
}

func TestListFiltersExactDynamicPriority(t *testing.T) {
	entries := []host.AuthEntry{
		{AuthIndex: "a", Name: "a.json", Email: "a@example.com", Provider: "codex", Type: "oauth", Status: "active", Priority: 0},
		{AuthIndex: "b", Name: "b.json", Email: "b@example.com", Provider: "codex", Type: "oauth", Status: "active", Priority: 10},
	}
	docs := map[string]host.AuthDocument{
		"a": doc("a", "a.json", ""),
		"b": doc("b", "b.json", ""),
	}
	page, err := List(fakeHost{entries: entries, docs: docs}, nil, nil, ParseFilters(map[string][]string{"priority": {"0"}}))
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].AuthIndex != "a" {
		t.Fatalf("exact priority filter=%#v", page)
	}
}

func TestListPaginatesAndAcceptsQueryFilters(t *testing.T) {
	entries := []host.AuthEntry{}
	docs := map[string]host.AuthDocument{}
	for _, item := range []struct{ index, email string }{{"a", "a@example.com"}, {"b", "b@example.com"}, {"c", "c@example.com"}} {
		entries = append(entries, host.AuthEntry{AuthIndex: item.index, Name: item.index + ".json", Email: item.email, Provider: "codex", Type: "oauth", Status: "active", Priority: 2})
		docs[item.index] = doc(item.index, item.index+".json", "")
	}
	page, err := List(fakeHost{entries: entries, docs: docs}, nil, nil, ParseFilters(map[string][]string{"page": {"2"}, "page_size": {"2"}, "provider": {"codex"}, "priority": {"2"}}))
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].AuthIndex != "c" {
		t.Fatalf("page=%#v", page)
	}
}
