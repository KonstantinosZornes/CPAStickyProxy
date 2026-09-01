package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stickyproxy/native-plugin/internal/i18n"
)

func TestOnlyProxiesPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	decodo, err := store.Upsert("", "Decodo_US", "decodo", "http://login:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	resin, err := store.Upsert("", "Resin", "resin", "socks5://ignored:token@resin.example:2260")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "http://login:secret@gate.decodo.com:7000") {
		t.Fatal("base URL should persist")
	}
	if strings.Contains(text, "active_proxy_id") {
		t.Fatalf("state must not persist an active proxy: %s", text)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	proxies, err := reloaded.Proxies()
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 2 || proxies[0].ID != decodo.ID || proxies[1].ID != resin.ID {
		t.Fatalf("proxies=%#v", proxies)
	}
}

func TestProxyNameAllowsOnlyASCIILettersDigitsHyphensAndUnderscores(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{
		"http://login:secret@gate.decodo.com:7000",
		"http://other:secret@gate.decodo.com:7000",
		"http://third:secret@gate.decodo.com:7000",
	}
	for index, name := range []string{"Proxy_01", "US-residential", "A_B-C9"} {
		if _, err := store.Upsert("", name, "decodo", urls[index]); err != nil {
			t.Fatalf("valid proxy name %q: %v", name, err)
		}
	}
	for _, name := range []string{"Proxy name", "proxy.name", "代理"} {
		if _, err := store.Upsert("", name, "decodo", "http://fourth:secret@gate.decodo.com:7000"); err == nil {
			t.Fatalf("invalid proxy name %q accepted", name)
		}
	}
}

func TestCreateDuplicatePlatformAndURLIsRejected(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.Upsert("", "Decodo_US", "decodo", "http://login:secret@gate.decodo.com:7000")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMappings([]AccountProxyMapping{{AuthIndex: "auth-a", ProxyID: original.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert("", "Renamed", "decodo", "http://login:secret@gate.decodo.com:7000"); i18n.Code(err) != "duplicate_proxy" {
		t.Fatalf("duplicate create err=%v, want duplicate_proxy", err)
	}
	proxies, err := store.Proxies()
	if err != nil || len(proxies) != 1 || proxies[0].ID != original.ID || proxies[0].Name != "Decodo_US" {
		t.Fatalf("existing proxy must be untouched: proxies=%#v err=%v", proxies, err)
	}
	_, mappings, err := store.Snapshot()
	if err != nil || len(mappings) != 1 || mappings[0].AuthIndex != "auth-a" {
		t.Fatalf("mappings must survive a rejected create: mappings=%#v err=%v", mappings, err)
	}
}

func TestDeleteProxyIsIndependent(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := store.Upsert("", "DataImpulse", "dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(proxy.ID); err != nil {
		t.Fatal(err)
	}
	proxies, err := store.Proxies()
	if err != nil || len(proxies) != 0 {
		t.Fatalf("proxies=%#v err=%v", proxies, err)
	}
}

func TestMappingsPersistAndProxyChangesRemoveThem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := store.Upsert("", "DataImpulse", "dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMappings([]AccountProxyMapping{{AuthIndex: "auth-a", ProxyID: proxy.ID}, {AuthIndex: "auth-b", ProxyID: proxy.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_, mappings, err := reloaded.Snapshot()
	if err != nil || len(mappings) != 2 {
		t.Fatalf("mappings=%#v err=%v", mappings, err)
	}
	changed, err := reloaded.Upsert(proxy.ID, "DataImpulse", "dataimpulse", "http://other:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID == proxy.ID {
		t.Fatal("base URL update must create a new proxy ID")
	}
	_, mappings, err = reloaded.Snapshot()
	if err != nil || len(mappings) != 0 {
		t.Fatalf("mappings after proxy update=%#v err=%v", mappings, err)
	}
}

func TestDeleteRemovesProxyMappings(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := store.Upsert("", "DataImpulse", "dataimpulse", "http://login:secret@gw.dataimpulse.com:10000")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMappings([]AccountProxyMapping{{AuthIndex: "auth-a", ProxyID: proxy.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(proxy.ID); err != nil {
		t.Fatal(err)
	}
	_, mappings, err := store.Snapshot()
	if err != nil || len(mappings) != 0 {
		t.Fatalf("mappings=%#v err=%v", mappings, err)
	}
}

func TestStateRejectsTrailingContent(t *testing.T) {
	valid := `{"proxies":[],"account_proxy_mappings":[]}`
	for name, raw := range map[string]string{
		"trailing garbage":  valid + "garbage",
		"second JSON value": valid + "\n{\"proxies\":[]}",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := New(path); err == nil {
				t.Fatalf("accepted trailing state content: %q", raw)
			}
		})
	}
}

func TestStateRejectsLegacyStateFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := `{"` + strings.Join([]string{"end", "points"}, "") + `":[],"` +
		strings.Join([]string{"account_end", "point_mappings"}, "") + `":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("legacy state fields must be rejected")
	}
}
