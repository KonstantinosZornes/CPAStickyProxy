package app

import (
	"encoding/json"
	"net/http"
	"testing"

	"stickyproxy/native-plugin/internal/host"
)

func TestFilteredApplyUsesTargetProxyAndExclusions(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"name":"target","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	targetID := created["proxy"].(map[string]any)["id"].(string)
	memory := &memoryHost{
		entries: []host.AuthEntry{
			{AuthIndex: "a", Name: "alice.json", Email: "alice@example.com", Provider: "codex", Type: "oauth", Status: "active"},
			{AuthIndex: "b", Name: "bob.json", Email: "bob@example.com", Provider: "codex", Type: "oauth", Status: "active"},
		},
		docs: map[string]host.AuthDocument{
			"a": authDoc("a", "alice.json", "alice@example.com", ""),
			"b": authDoc("b", "bob.json", "bob@example.com", ""),
		},
	}
	application.Host = memory
	response := application.Handle(Request{
		Method: http.MethodPost,
		Path:   "/v0/management/stickyproxy/apply",
		Body: []byte(`{
			"proxy_id":"` + targetID + `",
			"selection":{
				"kind":"filtered",
				"filters":{"provider":"codex","priority_min":"0","priority_max":"10"},
				"excluded_auth_indexes":["b"]
			}
		}`),
	})
	if response.Status != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Status, response.Body)
	}
	if got := documentProxyURL(t, memory.docs["a"].JSON); got == "" {
		t.Fatal("filtered selected account was not synchronized")
	}
	if got := documentProxyURL(t, memory.docs["b"].JSON); got != "" {
		t.Fatalf("excluded account was synchronized: %q", got)
	}
}

func TestFilteredSelectionOverLimitWritesNothing(t *testing.T) {
	application := newTestApp(t)
	created := call(t, application, http.MethodPost, "/v0/management/stickyproxy/proxies", `{
		"name":"target","platform":"dataimpulse","base_url":"http://login:secret@gw.dataimpulse.com:10000"
	}`)
	targetID := created["proxy"].(map[string]any)["id"].(string)
	memory := &memoryHost{docs: make(map[string]host.AuthDocument)}
	for index := 0; index < 2001; index++ {
		id := "auth-" + decimal(index)
		email := "account" + decimal(index) + "@example.com"
		name := "account" + decimal(index) + ".json"
		memory.entries = append(memory.entries, host.AuthEntry{AuthIndex: id, Name: name, Email: email, Provider: "codex", Type: "oauth", Status: "active"})
		memory.docs[id] = authDoc(id, name, email, "")
	}
	application.Host = memory
	response := application.Handle(Request{
		Method: http.MethodPost,
		Path:   "/v0/management/stickyproxy/apply",
		Body: []byte(`{
			"proxy_id":"` + targetID + `",
			"selection":{"kind":"filtered","filters":{"provider":"codex"}}
		}`),
	})
	if response.Status != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Status, response.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "selection_too_large" {
		t.Fatalf("payload=%#v", payload)
	}
	if memory.saveCalls != 0 {
		t.Fatalf("save calls=%d, want zero", memory.saveCalls)
	}
}
