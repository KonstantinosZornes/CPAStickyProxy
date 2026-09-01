// Package state persists proxies and their account assignments. It never
// stores account emails, session IDs, hashes, or generated proxy URLs.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/i18n"
)

type proxyRecord struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	BaseURL  string `json:"base_url"`
}

type mappingRecord struct {
	AuthIndex string `json:"auth_index"`
	ProxyID   string `json:"proxy_id"`
}

type persisted struct {
	Proxies              []proxyRecord   `json:"proxies"`
	AccountProxyMappings []mappingRecord `json:"account_proxy_mappings"`
}

// AccountProxyMapping records the proxy that StickyProxy last successfully
// applied to a CPA physical account. AuthIndex is the only account identifier
// persisted here.
type AccountProxyMapping struct {
	AuthIndex string
	ProxyID   string
}

type Store struct {
	mu    sync.RWMutex
	path  string
	state persisted
}

func New(path string) (*Store, error) {
	if path == "" {
		path = "stickyproxy-state.json"
	}
	store := &Store{path: path, state: persisted{Proxies: []proxyRecord{}, AccountProxyMappings: []mappingRecord{}}}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
	}
	var loaded persisted
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&loaded); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("state contains multiple JSON values")
		}
		return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
	}
	if loaded.Proxies == nil {
		loaded.Proxies = []proxyRecord{}
	}
	if loaded.AccountProxyMappings == nil {
		loaded.AccountProxyMappings = []mappingRecord{}
	}
	for _, record := range loaded.Proxies {
		if _, err := proxyFromRecord(record); err != nil {
			return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
		}
	}
	if err := validateMappings(loaded.AccountProxyMappings); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
	}
	// The user chose plaintext base URLs. Restrict the state file itself, but
	// do not chmod an arbitrary operator-owned parent directory on reload.
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_read_failed"), err)
	}
	s.state = loaded
	return nil
}

func clone(input persisted) persisted {
	output := input
	output.Proxies = append([]proxyRecord(nil), input.Proxies...)
	output.AccountProxyMappings = append([]mappingRecord(nil), input.AccountProxyMappings...)
	return output
}

func (s *Store) saveLocked(next persisted) error {
	if next.Proxies == nil {
		next.Proxies = []proxyRecord{}
	}
	if next.AccountProxyMappings == nil {
		next.AccountProxyMappings = []mappingRecord{}
	}
	if err := validateMappings(next.AccountProxyMappings); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	tmp, err := os.CreateTemp(dir, ".stickyproxy-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), err)
	}
	// Rename made next the current on-disk state. Keep the in-memory view in
	// sync even if the subsequent directory durability confirmation fails.
	s.state = next
	if directory, err := os.Open(dir); err == nil {
		if syncErr := directory.Sync(); syncErr != nil {
			_ = directory.Close()
			return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), syncErr)
		}
		if closeErr := directory.Close(); closeErr != nil {
			return fmt.Errorf("%w: %v", i18n.New("state_write_failed"), closeErr)
		}
	}
	return nil
}

func validateMappings(records []mappingRecord) error {
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.AuthIndex) == "" || strings.TrimSpace(record.ProxyID) == "" {
			return i18n.New("invalid_request")
		}
		if _, duplicate := seen[record.AuthIndex]; duplicate {
			return i18n.New("invalid_request")
		}
		seen[record.AuthIndex] = struct{}{}
	}
	return nil
}

func proxyFromRecord(record proxyRecord) (domain.Proxy, error) {
	proxy, err := domain.NewProxy(record.Platform, record.BaseURL)
	if err != nil {
		return domain.Proxy{}, err
	}
	name, err := domain.NormalizeProxyName(record.Name)
	if err != nil {
		return domain.Proxy{}, err
	}
	proxy.Name = name
	return proxy, nil
}

func (s *Store) Proxies() ([]domain.Proxy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Proxy, 0, len(s.state.Proxies))
	for _, record := range s.state.Proxies {
		proxy, err := proxyFromRecord(record)
		if err != nil {
			return nil, err
		}
		out = append(out, proxy)
	}
	return out, nil
}

// Snapshot returns detached proxy and account-mapping views from the same
// saved state, so callers cannot observe a mismatched pair.
func (s *Store) Snapshot() ([]domain.Proxy, []AccountProxyMapping, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proxies := make([]domain.Proxy, 0, len(s.state.Proxies))
	for _, record := range s.state.Proxies {
		proxy, err := proxyFromRecord(record)
		if err != nil {
			return nil, nil, err
		}
		proxies = append(proxies, proxy)
	}
	mappings := make([]AccountProxyMapping, 0, len(s.state.AccountProxyMappings))
	for _, record := range s.state.AccountProxyMappings {
		mappings = append(mappings, AccountProxyMapping{AuthIndex: record.AuthIndex, ProxyID: record.ProxyID})
	}
	return proxies, mappings, nil
}

// UpdateMappings applies a batch of successful account changes in one atomic
// state-file replacement. Deletes win only until an upsert for the same
// account is supplied in this batch.
func (s *Store) UpdateMappings(upserts []AccountProxyMapping, deletes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	byAccount := make(map[string]string, len(next.AccountProxyMappings)+len(upserts))
	for _, record := range next.AccountProxyMappings {
		byAccount[record.AuthIndex] = record.ProxyID
	}
	for _, authIndex := range deletes {
		delete(byAccount, strings.TrimSpace(authIndex))
	}
	for _, mapping := range upserts {
		authIndex := strings.TrimSpace(mapping.AuthIndex)
		proxyID := strings.TrimSpace(mapping.ProxyID)
		if authIndex == "" || proxyID == "" {
			return i18n.New("invalid_request")
		}
		byAccount[authIndex] = proxyID
	}

	validProxies := make(map[string]struct{}, len(next.Proxies))
	for _, record := range next.Proxies {
		proxy, err := proxyFromRecord(record)
		if err != nil {
			return err
		}
		validProxies[proxy.ID] = struct{}{}
	}
	next.AccountProxyMappings = make([]mappingRecord, 0, len(byAccount))
	for authIndex, proxyID := range byAccount {
		if _, exists := validProxies[proxyID]; exists {
			next.AccountProxyMappings = append(next.AccountProxyMappings, mappingRecord{AuthIndex: authIndex, ProxyID: proxyID})
		}
	}
	sort.Slice(next.AccountProxyMappings, func(i, j int) bool {
		return next.AccountProxyMappings[i].AuthIndex < next.AccountProxyMappings[j].AuthIndex
	})
	return s.saveLocked(next)
}

// Proxy returns a detached proxy view for a persisted ID. It lets the
// management module synchronize the target proxy URLs before committing a new
// active selection.
func (s *Store) Proxy(id string) (domain.Proxy, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, record := range s.state.Proxies {
		proxy, err := proxyFromRecord(record)
		if err != nil {
			return domain.Proxy{}, false, err
		}
		if proxy.ID == id {
			return proxy, true, nil
		}
	}
	return domain.Proxy{}, false, nil
}

// Upsert saves one proxy. A blank baseURL during an update means keep the
// old base URL; the management page never has to send a stored password back.
// Creating a proxy whose platform and canonical base URL already exist fails
// with duplicate_proxy instead of silently renaming the stored record; use an
// update (non-blank id) to rename.
func (s *Store) Upsert(id, name, platform, baseURL string) (domain.Proxy, error) {
	name = strings.TrimSpace(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	index := -1
	var previous domain.Proxy
	if id != "" {
		for i, record := range next.Proxies {
			existing, err := proxyFromRecord(record)
			if err != nil {
				return domain.Proxy{}, err
			}
			if existing.ID == id {
				index, previous = i, existing
				break
			}
		}
		if index < 0 {
			return domain.Proxy{}, i18n.New("proxy_not_found")
		}
		if baseURL == "" {
			baseURL = previous.BaseURL
		}
		if platform == "" {
			platform = string(previous.Platform)
		}
		if name == "" {
			name = previous.Name
		}
	}

	proxy, err := domain.NewProxy(platform, baseURL)
	if err != nil {
		return domain.Proxy{}, err
	}
	proxy.Name = name
	if proxy.Name == "" {
		proxy.Name = defaultName(proxy)
	}
	proxy.Name, err = domain.NormalizeProxyName(proxy.Name)
	if err != nil {
		return domain.Proxy{}, err
	}
	for i, record := range next.Proxies {
		if i == index {
			continue
		}
		existing, err := proxyFromRecord(record)
		if err != nil {
			return domain.Proxy{}, err
		}
		if existing.ID == proxy.ID {
			return domain.Proxy{}, i18n.New("duplicate_proxy")
		}
	}
	if index >= 0 {
		next.Proxies[index] = proxyRecord{Name: proxy.Name, Platform: string(proxy.Platform), BaseURL: proxy.BaseURL}
		if previous.ID != proxy.ID {
			next.AccountProxyMappings = withoutProxy(next.AccountProxyMappings, previous.ID)
		}
	} else {
		next.Proxies = append(next.Proxies, proxyRecord{Name: proxy.Name, Platform: string(proxy.Platform), BaseURL: proxy.BaseURL})
	}
	if err := s.saveLocked(next); err != nil {
		return domain.Proxy{}, err
	}
	return proxy, nil
}

func defaultName(proxy domain.Proxy) string {
	return string(proxy.Platform) + "_" + proxy.ID[len(proxy.ID)-6:]
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	index := -1
	for i, record := range next.Proxies {
		proxy, convertErr := proxyFromRecord(record)
		if convertErr != nil {
			return convertErr
		}
		if proxy.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return i18n.New("proxy_not_found")
	}
	next.Proxies = append(next.Proxies[:index], next.Proxies[index+1:]...)
	proxy, err := proxyFromRecord(s.state.Proxies[index])
	if err != nil {
		return err
	}
	next.AccountProxyMappings = withoutProxy(next.AccountProxyMappings, proxy.ID)
	return s.saveLocked(next)
}

func withoutProxy(mappings []mappingRecord, proxyID string) []mappingRecord {
	out := mappings[:0]
	for _, mapping := range mappings {
		if mapping.ProxyID != proxyID {
			out = append(out, mapping)
		}
	}
	return out
}
