// Package app contains the authenticated StickyProxy management API. It owns
// proxy CRUD, account proxy configuration status, and explicit selected
// account synchronization. Complete proxy URLs stay inside the native plugin
// and CPA host callback path.
package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"stickyproxy/native-plugin/internal/accounts"
	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
	"stickyproxy/native-plugin/internal/i18n"
	"stickyproxy/native-plugin/internal/state"
	"stickyproxy/native-plugin/internal/syncer"
	"stickyproxy/native-plugin/internal/tester"
)

type Request struct {
	Method  string
	Path    string
	Headers map[string][]string
	Query   map[string][]string
	Body    []byte
}

type Response struct {
	Status int
	Body   []byte
}

type App struct {
	Store  *state.Store
	Host   host.Client
	SyncMu *sync.Mutex
	// ProbeProxy is injectable for tests; production defaults to tester.Probe.
	ProbeProxy func(domain.Proxy) (tester.Result, error)
}

var fallbackSyncMu sync.Mutex

type proxyView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	MaskedBaseURL string `json:"masked_base_url"`
	Scheme        string `json:"scheme"`
	Host          string `json:"host"`
}

type proxyInput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	BaseURL  string `json:"base_url"`
}

type bulkInput struct {
	Platform string `json:"platform"`
	Text     string `json:"text"`
	Items    []struct {
		Name     string `json:"name"`
		Platform string `json:"platform"`
		BaseURL  string `json:"base_url"`
	} `json:"items"`
}

type proxyIDInput struct {
	ProxyID string `json:"proxy_id"`
}

type selectionFilters struct {
	Query           string `json:"q"`
	Provider        string `json:"provider"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	ProxyState      string `json:"proxy_state"`
	ProxyID         string `json:"proxy_id"`
	IncludeDisabled string `json:"include_disabled"`
	Priority        string `json:"priority"`
	PriorityMin     string `json:"priority_min"`
	PriorityMax     string `json:"priority_max"`
	Note            string `json:"note"`
}

type filteredSelectionInput struct {
	Kind                string           `json:"kind"`
	Filters             selectionFilters `json:"filters"`
	ExcludedAuthIndexes []string         `json:"excluded_auth_indexes"`
}

type applyInput struct {
	ProxyID     string                  `json:"proxy_id"`
	AuthIndexes []string                `json:"auth_indexes"`
	Selection   *filteredSelectionInput `json:"selection,omitempty"`
}

type clearInput struct {
	AuthIndexes []string                `json:"auth_indexes"`
	Selection   *filteredSelectionInput `json:"selection,omitempty"`
}

type clearAppliedInput struct {
	ProxyID     string                  `json:"proxy_id"`
	AuthIndexes []string                `json:"auth_indexes"`
	Selection   *filteredSelectionInput `json:"selection,omitempty"`
}

func (a App) Handle(req Request) Response {
	locale := i18n.FromHeaders(req.Headers)
	if a.Store == nil {
		return a.error(locale, http.StatusServiceUnavailable, i18n.New("state_read_failed"))
	}
	path := strings.TrimPrefix(req.Path, "/v0/management")
	if path == "" {
		path = req.Path
	}

	switch {
	case req.Method == http.MethodGet && path == "/stickyproxy/state":
		return a.getState(locale)
	case req.Method == http.MethodGet && path == "/stickyproxy/accounts":
		return a.listAccounts(locale, req.Query)
	case req.Method == http.MethodPost && path == "/stickyproxy/proxies":
		return a.upsertProxy(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/proxies/bulk":
		return a.bulkProxies(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/proxies/delete":
		return a.deleteProxy(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/proxies/test":
		return a.testProxy(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/apply":
		return a.applyProxy(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/clear-proxy":
		return a.clearAccountProxies(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/clear-applied":
		return a.clearAppliedProxy(locale, req.Body)
	case req.Method == http.MethodPost && path == "/stickyproxy/preview":
		return a.preview(locale, req.Body)
	default:
		return a.error(locale, http.StatusNotFound, i18n.New("not_found"))
	}
}

func (a App) syncLock() *sync.Mutex {
	if a.SyncMu != nil {
		return a.SyncMu
	}
	return &fallbackSyncMu
}

func (a App) getState(locale i18n.Locale) Response {
	proxies, err := a.Store.Proxies()
	if err != nil {
		return a.error(locale, http.StatusInternalServerError, err)
	}
	items := make([]proxyView, 0, len(proxies))
	for _, proxy := range proxies {
		items = append(items, safeProxy(proxy))
	}
	return a.json(http.StatusOK, map[string]any{
		"ok": true, "proxies": items,
	})
}

func (a App) listAccounts(locale i18n.Locale, query map[string][]string) Response {
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	proxies, mappings, err := a.Store.Snapshot()
	if err != nil {
		return a.error(locale, http.StatusInternalServerError, err)
	}
	filters := accounts.ParseFilters(query)
	page, err := accounts.List(a.Host, proxies, mappingIndex(mappings), filters)
	if err != nil {
		return a.error(locale, http.StatusBadGateway, i18n.New("accounts_load_failed"))
	}
	return a.json(http.StatusOK, map[string]any{
		"ok":    true,
		"items": page.Items, "total": page.Total, "total_known": page.TotalKnown,
		"page": page.Page, "page_size": page.PageSize, "has_more": page.HasMore,
		"next_cursor": page.NextCursor, "summary": page.Summary, "facets": page.Facets,
	})
}

func (a App) upsertProxy(locale i18n.Locale, raw []byte) Response {
	var input proxyInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	proxy, err := a.Store.Upsert(input.ID, input.Name, input.Platform, input.BaseURL)
	if err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	return a.json(http.StatusOK, map[string]any{
		"ok": true, "proxy": safeProxy(proxy),
	})
}

func (a App) bulkProxies(locale i18n.Locale, raw []byte) Response {
	var input bulkInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	items := input.Items
	if len(items) == 0 {
		for index, line := range strings.Split(input.Text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			// One proxy per line. Only the last pipe separates the platform;
			// every pipe before it belongs to the proxy URL itself.
			baseURL := line
			platform := input.Platform
			if marker := strings.LastIndex(line, "|"); marker >= 0 {
				baseURL = strings.TrimSpace(line[:marker])
				if suffix := strings.TrimSpace(line[marker+1:]); suffix != "" {
					platform = suffix
				}
			}
			items = append(items, struct {
				Name     string `json:"name"`
				Platform string `json:"platform"`
				BaseURL  string `json:"base_url"`
			}{Name: "Proxy_" + decimal(index+1), Platform: platform, BaseURL: baseURL})
		}
	}

	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	created := make([]proxyView, 0, len(items))
	errorsOut := make([]map[string]any, 0)
	for index, item := range items {
		proxy, err := a.Store.Upsert("", item.Name, item.Platform, item.BaseURL)
		if err != nil {
			errorsOut = append(errorsOut, map[string]any{
				"line": index + 1, "code": i18n.Code(err),
			})
			continue
		}
		created = append(created, safeProxy(proxy))
	}
	return a.json(http.StatusOK, map[string]any{
		"ok": true, "items": created, "errors": errorsOut,
		"created": len(created), "rejected": len(errorsOut),
	})
}

// Delete never rewrites account proxy_url values. It removes mappings for the
// deleted proxy; operators clear selected account proxies explicitly with
// clear-proxy.
func (a App) deleteProxy(locale i18n.Locale, raw []byte) Response {
	var input proxyIDInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	if err := a.Store.Delete(strings.TrimSpace(input.ProxyID)); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	return a.json(http.StatusOK, map[string]any{"ok": true})
}

func (a App) testProxy(locale i18n.Locale, raw []byte) Response {
	var input proxyIDInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	proxy, found, err := a.Store.Proxy(strings.TrimSpace(input.ProxyID))
	probe := a.ProbeProxy
	lock.Unlock()
	if err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	if !found {
		return a.error(locale, http.StatusNotFound, i18n.New("proxy_not_found"))
	}
	if probe == nil {
		probe = tester.Probe
	}
	result, err := probe(proxy)
	if err != nil {
		return a.error(locale, http.StatusBadGateway, i18n.New("test_failed"))
	}
	return a.json(http.StatusOK, map[string]any{"ok": true, "test": result})
}

func (a App) applyProxy(locale i18n.Locale, raw []byte) Response {
	var input applyInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	proxies, mappings, err := a.Store.Snapshot()
	if err != nil {
		return a.error(locale, http.StatusInternalServerError, err)
	}
	proxy, found := proxyByID(proxies, strings.TrimSpace(input.ProxyID))
	if !found {
		return a.error(locale, http.StatusNotFound, i18n.New("proxy_not_found"))
	}
	authIndexes, selectionErr := a.resolveSelection(input.AuthIndexes, input.Selection, proxies, mappingIndex(mappings))
	if selectionErr != nil {
		return a.selectionError(locale, selectionErr)
	}
	if len(authIndexes) == 0 {
		// An empty selection is a malformed request, not an upstream failure.
		return a.error(locale, http.StatusBadRequest, i18n.New("selection_required"))
	}
	result := (syncer.Synchronizer{Host: a.Host}).ApplySelected(&proxy, authIndexes)
	if result.Fatal {
		return a.syncError(result, http.StatusBadGateway)
	}
	upserts := make([]state.AccountProxyMapping, 0, len(result.SucceededAuthIndexes))
	for _, authIndex := range result.SucceededAuthIndexes {
		upserts = append(upserts, state.AccountProxyMapping{AuthIndex: authIndex, ProxyID: proxy.ID})
	}
	if len(upserts) > 0 {
		if err := a.Store.UpdateMappings(upserts, nil); err != nil {
			return a.syncError(result, http.StatusInternalServerError)
		}
	}
	return a.json(http.StatusOK, map[string]any{
		"ok": true, "proxy_id": proxy.ID, "sync": a.syncView(result),
	})
}

func (a App) clearAccountProxies(locale i18n.Locale, raw []byte) Response {
	var input clearInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	proxies, mappings, err := a.Store.Snapshot()
	if err != nil {
		return a.error(locale, http.StatusInternalServerError, err)
	}
	authIndexes, selectionErr := a.resolveSelection(input.AuthIndexes, input.Selection, proxies, mappingIndex(mappings))
	if selectionErr != nil {
		return a.selectionError(locale, selectionErr)
	}
	if len(authIndexes) == 0 {
		return a.error(locale, http.StatusBadRequest, i18n.New("selection_required"))
	}
	result := (syncer.Synchronizer{Host: a.Host}).ApplySelected(nil, authIndexes)
	if result.Fatal {
		return a.syncError(result, http.StatusBadGateway)
	}
	if len(result.SucceededAuthIndexes) > 0 {
		if err := a.Store.UpdateMappings(nil, result.SucceededAuthIndexes); err != nil {
			return a.syncError(result, http.StatusInternalServerError)
		}
	}
	return a.json(http.StatusOK, map[string]any{"ok": true, "sync": a.syncView(result)})
}

func (a App) clearAppliedProxy(locale i18n.Locale, raw []byte) Response {
	var input clearAppliedInput
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	lock := a.syncLock()
	lock.Lock()
	defer lock.Unlock()
	proxies, mappings, err := a.Store.Snapshot()
	if err != nil {
		return a.error(locale, http.StatusInternalServerError, err)
	}
	proxy, found := proxyByID(proxies, strings.TrimSpace(input.ProxyID))
	if !found {
		return a.error(locale, http.StatusNotFound, i18n.New("proxy_not_found"))
	}
	indexes := mappingIndex(mappings)
	authIndexes, selectionErr := a.resolveSelection(input.AuthIndexes, input.Selection, proxies, indexes)
	if selectionErr != nil {
		return a.selectionError(locale, selectionErr)
	}
	if len(authIndexes) == 0 {
		return a.error(locale, http.StatusBadRequest, i18n.New("selection_required"))
	}
	matching := make([]string, 0, len(authIndexes))
	for _, authIndex := range authIndexes {
		if indexes[authIndex] == proxy.ID {
			matching = append(matching, authIndex)
		}
	}
	if len(matching) == 0 {
		return a.json(http.StatusOK, map[string]any{
			"ok": true, "proxy_id": proxy.ID, "sync": a.syncView(syncer.Result{Mode: "clear", Selected: len(authIndexes), Errors: []syncer.ItemError{}}),
		})
	}
	result := (syncer.Synchronizer{Host: a.Host}).ApplySelected(nil, matching)
	result.Selected = len(authIndexes)
	if result.Fatal {
		return a.syncError(result, http.StatusBadGateway)
	}
	if len(result.SucceededAuthIndexes) > 0 {
		if err := a.Store.UpdateMappings(nil, result.SucceededAuthIndexes); err != nil {
			return a.syncError(result, http.StatusInternalServerError)
		}
	}
	return a.json(http.StatusOK, map[string]any{
		"ok": true, "proxy_id": proxy.ID, "sync": a.syncView(result),
	})
}

// resolveSelection keeps the browser-facing selection compact. Explicit IDs
// remain available for individual checkbox use; a filtered selection instead
// re-evaluates the page's filter snapshot inside the synchronization lock.
func (a App) resolveSelection(explicit []string, selected *filteredSelectionInput, proxies []domain.Proxy, mappings map[string]string) ([]string, error) {
	if selected == nil {
		return explicit, nil
	}
	if len(explicit) != 0 || !strings.EqualFold(strings.TrimSpace(selected.Kind), "filtered") {
		return nil, i18n.New("invalid_request")
	}
	filters := accounts.ParseFilters(map[string][]string{
		"q":                {selected.Filters.Query},
		"provider":         {selected.Filters.Provider},
		"type":             {selected.Filters.Type},
		"status":           {selected.Filters.Status},
		"proxy_state":      {selected.Filters.ProxyState},
		"proxy_id":         {selected.Filters.ProxyID},
		"include_disabled": {selected.Filters.IncludeDisabled},
		"priority":         {selected.Filters.Priority},
		"priority_min":     {selected.Filters.PriorityMin},
		"priority_max":     {selected.Filters.PriorityMax},
		"note":             {selected.Filters.Note},
	})
	resolved, err := accounts.ResolveFilteredSelection(
		a.Host,
		proxies,
		mappings,
		filters,
		selected.ExcludedAuthIndexes,
		syncer.MaxSelectedAccounts(),
	)
	if err != nil {
		return nil, err
	}
	return resolved.AuthIndexes, nil
}

func mappingIndex(mappings []state.AccountProxyMapping) map[string]string {
	index := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		index[mapping.AuthIndex] = mapping.ProxyID
	}
	return index
}

func proxyByID(proxies []domain.Proxy, id string) (domain.Proxy, bool) {
	for _, proxy := range proxies {
		if proxy.ID == id {
			return proxy, true
		}
	}
	return domain.Proxy{}, false
}

func (a App) preview(locale i18n.Locale, raw []byte) Response {
	var input struct {
		ProxyID string `json:"proxy_id"`
		Email   string `json:"email"`
	}
	if err := decodeObject(raw, &input); err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	proxy, found, err := a.Store.Proxy(strings.TrimSpace(input.ProxyID))
	if err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	if !found {
		return a.error(locale, http.StatusNotFound, i18n.New("proxy_not_found"))
	}
	rewrite, err := domain.Rewrite(proxy, input.Email)
	if err != nil {
		return a.error(locale, http.StatusBadRequest, err)
	}
	out := map[string]any{
		"ok": true, "platform": string(proxy.Platform), "port_sticky_only": rewrite.PortStickyOnly,
		"masked_effective_url": domain.Mask(rewrite.URL),
	}
	if rewrite.PortStickyOnly {
		out["notice_code"] = "proxy_static"
	}
	return a.json(http.StatusOK, out)
}

func safeProxy(proxy domain.Proxy) proxyView {
	parts, _ := domain.Parse(proxy.BaseURL)
	return proxyView{
		ID: proxy.ID, Name: proxy.Name, Platform: string(proxy.Platform),
		MaskedBaseURL: domain.Mask(proxy.BaseURL), Scheme: parts.Scheme, Host: parts.Host,
	}
}

func (a App) selectionError(locale i18n.Locale, err error) Response {
	if i18n.Code(err) == "selection_too_large" {
		return a.error(locale, http.StatusBadRequest, err)
	}
	// Filtered selections call the same host callbacks as a sync. A host read
	// failure is an upstream availability problem, not a malformed browser body.
	if i18n.Code(err) == "internal_error" {
		return a.error(locale, http.StatusBadGateway, i18n.New("accounts_load_failed"))
	}
	return a.error(locale, http.StatusBadRequest, err)
}

func (a App) syncView(result syncer.Result) map[string]any {
	errorsOut := make([]map[string]any, 0, len(result.Errors))
	for _, item := range result.Errors {
		code := item.Code
		if code == "" || code == "internal_error" {
			code = "sync_failed"
		}
		errorsOut = append(errorsOut, map[string]any{
			"auth_index": item.AuthIndex,
			"code":       code,
		})
	}
	return map[string]any{
		"mode": result.Mode, "selected": result.Selected, "eligible": result.Eligible,
		"scanned": result.Scanned, "updated": result.Updated,
		"already_applied": result.AlreadyApplied, "skipped": result.Skipped,
		"port_sticky_only": result.PortStickyOnly, "errors": errorsOut,
	}
}

func (a App) syncError(result syncer.Result, status int) Response {
	return a.json(status, map[string]any{
		"ok": false, "code": "sync_failed",
		"sync": a.syncView(result),
	})
}

func (a App) json(status int, value any) Response {
	body, err := json.Marshal(value)
	if err != nil {
		body = []byte(`{"ok":false,"code":"internal_error"}`)
		status = http.StatusInternalServerError
	}
	return Response{Status: status, Body: body}
}

func (a App) error(_ i18n.Locale, status int, err error) Response {
	code := i18n.Code(err)
	if code == "internal_error" {
		code = "state_read_failed"
	}
	return a.json(status, map[string]any{"ok": false, "code": code})
}

func decodeObject(raw []byte, out any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return i18n.New("invalid_request")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return i18n.New("invalid_json", err)
	}
	return nil
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var scratch [20]byte
	i := len(scratch)
	for value > 0 {
		i--
		scratch[i] = byte('0' + value%10)
		value /= 10
	}
	return string(scratch[i:])
}
