// Package accounts builds the server-side, credential-safe read model for the
// "账号代理配置" page. It sees raw auth JSON only inside the native plugin;
// callers receive account metadata, a proxy status enum, and masked proxy URLs.
package accounts

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
	"stickyproxy/native-plugin/internal/i18n"
)

const (
	StateStickyApplied  = "sticky_applied"
	StateInheritsSystem = "inherits_system"
	StateCustomOther    = "custom_other"
	StateNoEmail        = "no_email"
	StateRuntimeOnly    = "runtime_only"
	StateInvalidProxy   = "invalid_proxy"
	StateUnreadable     = "unreadable"
)

type Filters struct {
	Query           string
	Providers       []string
	Types           []string
	Statuses        []string
	ProxyStates     []string
	AppliedProxyIDs []string
	IncludeDisabled *bool
	Priority        *int
	PriorityMin     *int
	PriorityMax     *int
	Note            string
	Page            int
	PageSize        int
	Cursor          string
}

type Account struct {
	AuthIndex      string `json:"auth_index"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Label          string `json:"label"`
	Provider       string `json:"provider"`
	Type           string `json:"type"`
	AccountType    string `json:"account_type"`
	Status         string `json:"status"`
	Disabled       bool   `json:"disabled"`
	Unavailable    bool   `json:"unavailable"`
	Priority       int    `json:"priority"`
	Note           string `json:"note"`
	RuntimeOnly    bool   `json:"runtime_only"`
	Selectable     bool   `json:"selectable"`
	ProxyState     string `json:"proxy_state"`
	ProxyID        string `json:"proxy_id,omitempty"`
	ProxyName      string `json:"proxy_name,omitempty"`
	MaskedProxyURL string `json:"masked_proxy_url,omitempty"`
}

type Facets struct {
	Providers  []string `json:"providers"`
	Types      []string `json:"types"`
	Statuses   []string `json:"statuses"`
	Priorities []int    `json:"priorities"`
}

type Page struct {
	Items      []Account      `json:"items"`
	Total      int            `json:"total,omitempty"`
	TotalKnown bool           `json:"total_known"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	HasMore    bool           `json:"has_more"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Summary    map[string]int `json:"summary"`
	Facets     Facets         `json:"facets"`
}

type rawAuth struct {
	ProxyURL string `json:"proxy_url"`
}

// List builds metadata facets from host.auth.list, then reads auth JSON only for
// the visible page. Proxy-state filters are the deliberate exception: the host
// exposes proxy_url only through one auth document at a time, so evaluating such
// a filter must inspect every metadata-matched account. This keeps ordinary
// pagination and free-text filtering responsive even for very large auth sets.
func List(client host.Client, proxies []domain.Proxy, mappings map[string]string, filters Filters) (Page, error) {
	entries, err := client.ListAuths()
	if err != nil {
		return Page{}, err
	}
	proxyNames := proxyNamesByID(proxies)
	all := make([]Account, 0, len(entries))
	filtered := make([]Account, 0, len(entries))
	for _, entry := range entries {
		account := accountFromEntry(entry)
		classifyMapping(&account, proxyNames, mappings)
		all = append(all, account)
		if matchesMetadata(account, filters) {
			filtered = append(filtered, account)
		}
	}
	facets := collectFacets(all)

	sortAccounts(filtered)
	page, pageSize := normalizePage(filters.Page, filters.PageSize)
	if len(filters.ProxyStates) == 0 && len(filters.AppliedProxyIDs) == 0 {
		start := (page - 1) * pageSize
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + pageSize
		if end > len(filtered) {
			end = len(filtered)
		}
		items := append([]Account(nil), filtered[start:end]...)
		for index := range items {
			inspect(client, &items[index])
		}
		return pageResult(items, len(filtered), true, page, pageSize, end < len(filtered), "", facets), nil
	}
	if onlyMappedProxyFilters(filters) {
		items := make([]Account, 0, len(filtered))
		for _, account := range filtered {
			if matchesProxy(account, filters) {
				items = append(items, account)
			}
		}
		// The Web UI uses cursors for every proxy-related filter. Keep this
		// metadata-only fast path cursor-compatible without inspecting auth files.
		start, err := cursorStart(items, filters.Cursor)
		if err != nil {
			return Page{}, err
		}
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		hasMore := end < len(items)
		nextCursor := ""
		if hasMore {
			nextCursor = encodeCursor(items[end-1])
		}
		return pageResult(items[start:end], len(items), true, page, pageSize, hasMore, nextCursor, facets), nil
	}

	// Proxy-state filters need proxy_url, which exists only in each individual
	// auth document. Walk from a stable cursor and stop once this page plus one
	// look-ahead match is known; do not scan the entire auth set for a total.
	start, err := cursorStart(filtered, filters.Cursor)
	if err != nil {
		return Page{}, err
	}
	items := make([]Account, 0, pageSize)
	hasMore := false
	for index := start; index < len(filtered); index++ {
		account := filtered[index]
		inspect(client, &account)
		if !matchesProxy(account, filters) {
			continue
		}
		if len(items) == pageSize {
			hasMore = true
			break
		}
		items = append(items, account)
	}
	nextCursor := ""
	if hasMore {
		nextCursor = encodeCursor(items[len(items)-1])
	}
	return pageResult(items, 0, false, page, pageSize, hasMore, nextCursor, facets), nil
}

// ResolvedSelection is the server-only account selection for a current filter
// snapshot. Its indexes never pass through the browser.
type ResolvedSelection struct {
	AuthIndexes []string
	Matching    int
}

// ResolveFilteredSelection evaluates all matching accounts inside the plugin.
// It returns only selectable auth indexes, ignores explicitly excluded indexes,
// and stops as soon as the safety limit is exceeded. The operation handler then
// passes the result to the existing transactional synchronizer.
func ResolveFilteredSelection(client host.Client, proxies []domain.Proxy, mappings map[string]string, filters Filters, excluded []string, limit int) (ResolvedSelection, error) {
	entries, err := client.ListAuths()
	if err != nil {
		return ResolvedSelection{}, err
	}
	proxyNames := proxyNamesByID(proxies)
	if limit > 0 && len(excluded) > limit {
		return ResolvedSelection{}, i18n.New("selection_too_large", limit)
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, index := range excluded {
		index = strings.TrimSpace(index)
		if index == "" {
			return ResolvedSelection{}, i18n.New("account_not_selectable")
		}
		if _, duplicate := excludedSet[index]; duplicate {
			return ResolvedSelection{}, i18n.New("duplicate_account_selection")
		}
		excludedSet[index] = struct{}{}
	}

	resolved := ResolvedSelection{AuthIndexes: make([]string, 0)}
	for _, entry := range entries {
		account := accountFromEntry(entry)
		classifyMapping(&account, proxyNames, mappings)
		if !matchesMetadata(account, filters) {
			continue
		}
		// A compact filtered selection must resolve to the same safe set as an
		// explicit selection. Verify document readability here rather than
		// trusting metadata or the browser's earlier page view.
		inspect(client, &account)
		if !matchesProxy(account, filters) || !account.Selectable {
			continue
		}
		resolved.Matching++
		if _, omitted := excludedSet[account.AuthIndex]; omitted {
			continue
		}
		resolved.AuthIndexes = append(resolved.AuthIndexes, account.AuthIndex)
		if limit > 0 && len(resolved.AuthIndexes) > limit {
			return ResolvedSelection{}, i18n.New("selection_too_large", limit)
		}
	}
	return resolved, nil
}

func accountFromEntry(entry host.AuthEntry) Account {
	return Account{
		AuthIndex: entry.AuthIndex, Name: entry.Name, Email: entry.Email,
		Label: entry.Label, Provider: entry.Provider, Type: entry.Type,
		AccountType: entry.AccountType, Status: entry.Status, Disabled: entry.Disabled,
		Unavailable: entry.Unavailable, Priority: entry.Priority, Note: entry.Note,
		RuntimeOnly: entry.RuntimeOnly,
	}
}

func metadataSelectable(account Account) bool {
	if account.RuntimeOnly || strings.TrimSpace(account.AuthIndex) == "" {
		return false
	}
	_, err := domain.NormalizeEmail(account.Email)
	return err == nil
}

func inspect(client host.Client, account *Account) {
	if account.ProxyState == StateStickyApplied {
		return
	}
	if account.RuntimeOnly {
		account.ProxyState = StateRuntimeOnly
		return
	}
	if _, emailErr := domain.NormalizeEmail(account.Email); emailErr != nil {
		account.ProxyState = StateNoEmail
		return
	}
	account.Selectable = strings.TrimSpace(account.AuthIndex) != ""
	doc, err := client.GetAuth(account.AuthIndex)
	if err != nil {
		account.ProxyState = StateUnreadable
		account.Selectable = false
		return
	}
	if name := strings.TrimSpace(doc.Name); name == "" || filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".json") {
		account.ProxyState = StateUnreadable
		account.Selectable = false
		return
	}
	proxyURL, validJSON := proxyURLFromDocument(doc.JSON)
	if !validJSON {
		account.ProxyState = StateUnreadable
		account.Selectable = false
		return
	}
	classifyProxyURL(account, proxyURL)
}

func sortAccounts(items []Account) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Email), strings.ToLower(items[j].Email)
		if left != right {
			return left < right
		}
		left, right = strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left != right {
			return left < right
		}
		return items[i].AuthIndex < items[j].AuthIndex
	})
}

type cursor struct {
	Email     string `json:"e"`
	Name      string `json:"n"`
	AuthIndex string `json:"a"`
}

func encodeCursor(account Account) string {
	payload, _ := json.Marshal(cursor{
		Email: strings.ToLower(account.Email), Name: strings.ToLower(account.Name), AuthIndex: account.AuthIndex,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func cursorStart(items []Account, encoded string) (int, error) {
	if strings.TrimSpace(encoded) == "" {
		return 0, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, i18n.New("invalid_request")
	}
	var marker cursor
	if err := json.Unmarshal(payload, &marker); err != nil || marker.AuthIndex == "" {
		return 0, i18n.New("invalid_request")
	}
	needle := Account{Email: marker.Email, Name: marker.Name, AuthIndex: marker.AuthIndex}
	// The account list can change between pages. Advance past every item no
	// later than the last cursor, which avoids duplicates even when its original
	// row was removed in the meantime.
	return sort.Search(len(items), func(index int) bool { return accountLess(needle, items[index]) }), nil
}

func accountLess(left, right Account) bool {
	leftEmail, rightEmail := strings.ToLower(left.Email), strings.ToLower(right.Email)
	if leftEmail != rightEmail {
		return leftEmail < rightEmail
	}
	leftName, rightName := strings.ToLower(left.Name), strings.ToLower(right.Name)
	if leftName != rightName {
		return leftName < rightName
	}
	return left.AuthIndex < right.AuthIndex
}

func pageResult(items []Account, total int, totalKnown bool, page, pageSize int, hasMore bool, nextCursor string, facets Facets) Page {
	summary := make(map[string]int)
	for _, account := range items {
		summary[account.ProxyState]++
	}
	pageOut := Page{
		Items: items, TotalKnown: totalKnown, Page: page, PageSize: pageSize,
		HasMore: hasMore, NextCursor: nextCursor, Summary: summary, Facets: facets,
	}
	if totalKnown {
		pageOut.Total = total
	}
	return pageOut
}

func collectFacets(items []Account) Facets {
	providers := make(map[string]struct{})
	types := make(map[string]struct{})
	statuses := make(map[string]struct{})
	priorities := make(map[int]struct{})
	for _, account := range items {
		if value := strings.TrimSpace(account.Provider); value != "" {
			providers[value] = struct{}{}
		}
		if value := strings.TrimSpace(account.Type); value != "" {
			types[value] = struct{}{}
		} else if value := strings.TrimSpace(account.AccountType); value != "" {
			types[value] = struct{}{}
		}
		if value := strings.TrimSpace(account.Status); value != "" {
			statuses[value] = struct{}{}
		}
		priorities[account.Priority] = struct{}{}
	}
	facets := Facets{
		Providers:  sortedStrings(providers),
		Types:      sortedStrings(types),
		Statuses:   sortedStrings(statuses),
		Priorities: sortedInts(priorities),
	}
	return facets
}

func sortedStrings(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func sortedInts(values map[int]struct{}) []int {
	out := make([]int, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func proxyURLFromDocument(raw json.RawMessage) (string, bool) {
	var doc rawAuth
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", false
	}
	return strings.TrimSpace(doc.ProxyURL), true
}

func proxyNamesByID(proxies []domain.Proxy) map[string]string {
	index := make(map[string]string, len(proxies))
	for _, proxy := range proxies {
		index[proxy.ID] = proxy.Name
	}
	return index
}

func classifyMapping(account *Account, proxyNames map[string]string, mappings map[string]string) {
	if account.RuntimeOnly {
		account.ProxyState = StateRuntimeOnly
		return
	}
	if _, err := domain.NormalizeEmail(account.Email); err != nil {
		account.ProxyState = StateNoEmail
		return
	}
	account.Selectable = strings.TrimSpace(account.AuthIndex) != ""
	proxyID := mappings[account.AuthIndex]
	proxyName, found := proxyNames[proxyID]
	if proxyID == "" || !found {
		return
	}
	account.ProxyState = StateStickyApplied
	account.ProxyID = proxyID
	account.ProxyName = proxyName
}

func classifyProxyURL(account *Account, actual string) {
	if actual == "" {
		account.ProxyState = StateInheritsSystem
		return
	}
	_, observedErr := domain.Canonical(actual)
	if observedErr != nil {
		account.ProxyState = StateInvalidProxy
		account.MaskedProxyURL = maskedFallback(actual)
		return
	}
	account.MaskedProxyURL = domain.Mask(actual)
	account.ProxyState = StateCustomOther
}

func maskedFallback(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return "***"
	}
	if parsed.User == nil {
		return parsed.Scheme + "://" + parsed.Host
	}
	return parsed.Scheme + "://" + parsed.User.Username() + ":***@" + parsed.Host
}

func matchesMetadata(account Account, filters Filters) bool {
	if filters.IncludeDisabled != nil && account.Disabled != *filters.IncludeDisabled {
		return false
	}
	accountType := account.Type
	if strings.TrimSpace(accountType) == "" {
		accountType = account.AccountType
	}
	if !matchesSet(account.Provider, filters.Providers) || !matchesSet(accountType, filters.Types) || !matchesSet(account.Status, filters.Statuses) {
		return false
	}
	if filters.Priority != nil && account.Priority != *filters.Priority {
		return false
	}
	if filters.PriorityMin != nil && account.Priority < *filters.PriorityMin {
		return false
	}
	if filters.PriorityMax != nil && account.Priority > *filters.PriorityMax {
		return false
	}
	if note := strings.ToLower(strings.TrimSpace(filters.Note)); note != "" && !strings.Contains(strings.ToLower(account.Note), note) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filters.Query))
	if query == "" {
		return true
	}
	for _, value := range []string{account.Email, account.Name, account.Label, account.Note} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func matchesProxy(account Account, filters Filters) bool {
	return matchesSet(account.ProxyState, filters.ProxyStates) && matchesSet(account.ProxyID, filters.AppliedProxyIDs)
}

func onlyMappedProxyFilters(filters Filters) bool {
	if len(filters.ProxyStates) == 0 && len(filters.AppliedProxyIDs) == 0 {
		return false
	}
	for _, state := range filters.ProxyStates {
		if !strings.EqualFold(strings.TrimSpace(state), StateStickyApplied) {
			return false
		}
	}
	return true
}

func matchesSet(value string, selected []string) bool {
	if len(selected) == 0 {
		return true
	}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, item := range selected {
		if value == strings.ToLower(strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func ParseFilters(values map[string][]string) Filters {
	filters := Filters{Query: first(values, "q"), Note: first(values, "note")}
	filters.Providers = split(values["provider"])
	filters.Types = split(values["type"])
	filters.Statuses = split(values["status"])
	filters.ProxyStates = split(values["proxy_state"])
	filters.AppliedProxyIDs = split(values["proxy_id"])
	filters.Page = parseInt(first(values, "page"), 1)
	filters.PageSize = parseInt(first(values, "page_size"), 50)
	filters.Cursor = first(values, "cursor")
	if value := first(values, "include_disabled"); value != "" {
		parsed := strings.EqualFold(value, "true") || value == "1"
		filters.IncludeDisabled = &parsed
	}
	if value := first(values, "priority"); value != "" {
		parsed := parseInt(value, 0)
		filters.Priority = &parsed
	}
	if value := first(values, "priority_min"); value != "" {
		parsed := parseInt(value, 0)
		filters.PriorityMin = &parsed
	}
	if value := first(values, "priority_max"); value != "" {
		parsed := parseInt(value, 0)
		filters.PriorityMax = &parsed
	}
	return filters
}

func first(values map[string][]string, key string) string {
	for name, items := range values {
		if strings.EqualFold(name, key) && len(items) > 0 {
			return strings.TrimSpace(items[0])
		}
	}
	return ""
}

func split(values []string) []string {
	out := make([]string, 0)
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if clean := strings.TrimSpace(item); clean != "" {
				out = append(out, clean)
			}
		}
	}
	return out
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
