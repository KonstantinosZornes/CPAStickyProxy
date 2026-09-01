package accounts

import (
	"testing"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
)

func TestListIdentifiesAppliedProxyWithoutCurrentSelection(t *testing.T) {
	first, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	first.Name = "Residential"
	second, err := domain.NewProxy("decodo", "http://other:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	second.Name = "Datacenter"

	client := fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
			{AuthIndex: "c", Name: "carol.json", Email: "carol@example.com"},
		},
		docs: map[string]host.AuthDocument{
			"a": doc("a", "alice.json", "http://manual:manual@other.example:8080"),
			"b": doc("b", "bob.json", "http://manual:manual@other.example:8080"),
			"c": doc("c", "carol.json", "http://manual:manual@other.example:8080"),
		},
	}
	page, err := List(client, []domain.Proxy{first, second}, map[string]string{"a": first.ID, "b": second.ID}, Filters{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	byIndex := map[string]Account{}
	for _, account := range page.Items {
		byIndex[account.AuthIndex] = account
	}
	if account := byIndex["a"]; account.ProxyState != StateStickyApplied || account.ProxyID != first.ID || account.ProxyName != "Residential" {
		t.Fatalf("alice=%#v", account)
	}
	if account := byIndex["b"]; account.ProxyState != StateStickyApplied || account.ProxyID != second.ID || account.ProxyName != "Datacenter" {
		t.Fatalf("bob=%#v", account)
	}
	if account := byIndex["c"]; account.ProxyState != StateCustomOther {
		t.Fatalf("carol=%#v", account)
	}
}

func TestProxyViewFiltersAccountsByAppliedProxy(t *testing.T) {
	first, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewProxy("decodo", "http://other:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	client := fakeHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com"},
		},
		docs: map[string]host.AuthDocument{
			"a": doc("a", "alice.json", ""),
			"b": doc("b", "bob.json", ""),
		},
	}
	page, err := List(client, []domain.Proxy{first, second}, map[string]string{"a": first.ID, "b": second.ID}, Filters{
		AppliedProxyIDs: []string{second.ID}, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !page.TotalKnown || len(page.Items) != 1 || page.Items[0].AuthIndex != "b" || page.Items[0].ProxyID != second.ID {
		t.Fatalf("filtered page=%#v", page)
	}
}

func TestFilteredSelectionRecognizesAnyAppliedProxy(t *testing.T) {
	proxy, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	proxy.Name = "Residential"
	client := fakeHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}},
		docs:    map[string]host.AuthDocument{"a": doc("a", "alice.json", "")},
	}
	resolved, err := ResolveFilteredSelection(client, []domain.Proxy{proxy}, map[string]string{"a": proxy.ID}, Filters{
		ProxyStates:     []string{StateStickyApplied},
		AppliedProxyIDs: []string{proxy.ID},
	}, nil, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.AuthIndexes) != 1 || resolved.AuthIndexes[0] != "a" {
		t.Fatalf("resolved=%#v", resolved)
	}
}

func TestListDoesNotClaimProxyFromMatchingURLWithoutMapping(t *testing.T) {
	configured, err := domain.NewProxy("dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := domain.Rewrite(configured, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	page, err := List(fakeHost{
		entries: []host.AuthEntry{{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com"}},
		docs:    map[string]host.AuthDocument{"a": doc("a", "alice.json", rewritten.URL)},
	}, []domain.Proxy{configured}, nil, Filters{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ProxyState != StateCustomOther || page.Items[0].ProxyID != "" {
		t.Fatalf("account=%#v", page.Items)
	}
}
