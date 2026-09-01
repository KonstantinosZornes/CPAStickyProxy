// Package syncer writes selected CPA physical accounts' existing proxy_url
// field through host callbacks. Complete proxy URLs remain in the native plugin
// process and host callback path; they never traverse browser JavaScript.
package syncer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"stickyproxy/native-plugin/internal/domain"
	"stickyproxy/native-plugin/internal/host"
	"stickyproxy/native-plugin/internal/i18n"
)

const maxSelectedAccounts = 2000

func MaxSelectedAccounts() int { return maxSelectedAccounts }

type ItemError struct {
	AuthIndex string `json:"auth_index"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type Result struct {
	Mode                 string      `json:"mode"`
	Selected             int         `json:"selected"`
	Eligible             int         `json:"eligible"`
	Scanned              int         `json:"scanned"`
	Updated              int         `json:"updated"`
	AlreadyApplied       int         `json:"already_applied"`
	Skipped              int         `json:"skipped"`
	PortStickyOnly       int         `json:"port_sticky_only"`
	Errors               []ItemError `json:"errors"`
	SucceededAuthIndexes []string    `json:"-"`
	Fatal                bool        `json:"-"`
}

type Synchronizer struct {
	Host host.Client
}

// ApplySelected synchronizes only explicit selected auth indexes. A malformed
// selection is rejected before writes; individual account failures are reported
// and do not prevent later selected accounts from being processed.
func (s Synchronizer) ApplySelected(proxy *domain.Proxy, authIndexes []string) Result {
	if len(authIndexes) == 0 {
		return failedResult(proxy, "", i18n.New("selection_required"))
	}
	if len(authIndexes) > maxSelectedAccounts {
		return failedResult(proxy, "", i18n.New("selection_too_large", maxSelectedAccounts))
	}
	cleaned := make([]string, 0, len(authIndexes))
	seen := make(map[string]struct{}, len(authIndexes))
	for _, authIndex := range authIndexes {
		authIndex = strings.TrimSpace(authIndex)
		if authIndex == "" {
			return failedResult(proxy, "", i18n.New("account_not_selectable"))
		}
		if _, duplicate := seen[authIndex]; duplicate {
			return failedResult(proxy, authIndex, i18n.New("duplicate_account_selection"))
		}
		seen[authIndex] = struct{}{}
		cleaned = append(cleaned, authIndex)
	}
	return s.apply(proxy, cleaned)
}

func (s Synchronizer) list() ([]host.AuthEntry, error) {
	if s.Host == nil {
		return nil, i18n.New("sync_unavailable")
	}
	return s.Host.ListAuths()
}

func (s Synchronizer) apply(proxy *domain.Proxy, authIndexes []string) Result {
	result := Result{Errors: []ItemError{}, SucceededAuthIndexes: []string{}, Selected: len(authIndexes)}
	if proxy == nil {
		result.Mode = "clear"
	} else {
		result.Mode = "apply"
	}
	entries, err := s.list()
	if err != nil {
		return failedResult(proxy, "", err)
	}
	byIndex := make(map[string]host.AuthEntry, len(entries))
	for _, entry := range entries {
		if index := strings.TrimSpace(entry.AuthIndex); index != "" {
			byIndex[index] = entry
		}
	}
	for _, index := range authIndexes {
		entry, exists := byIndex[index]
		if !exists {
			result.Errors = append(result.Errors, itemError(index, i18n.New("account_not_found")))
			continue
		}
		result.Scanned++
		if entry.RuntimeOnly {
			result.Errors = append(result.Errors, itemError(index, i18n.New("account_not_selectable")))
			continue
		}
		if _, emailErr := domain.NormalizeEmail(entry.Email); emailErr != nil {
			result.Errors = append(result.Errors, itemError(index, i18n.New("account_not_selectable")))
			continue
		}
		result.Eligible++
		document, err := s.Host.GetAuth(index)
		if err != nil {
			result.Errors = append(result.Errors, itemError(index, err))
			continue
		}
		name := strings.TrimSpace(document.Name)
		if name == "" || filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".json") {
			result.Errors = append(result.Errors, itemError(index, i18n.New("account_not_selectable")))
			continue
		}
		desired := ""
		if proxy != nil {
			rewrite, err := domain.Rewrite(*proxy, entry.Email)
			if err != nil {
				result.Errors = append(result.Errors, itemError(index, err))
				continue
			}
			desired = rewrite.URL
			if rewrite.PortStickyOnly {
				result.PortStickyOnly++
			}
		}
		updated, already, err := withProxyURL(document.JSON, desired)
		if err != nil {
			result.Errors = append(result.Errors, itemError(index, err))
			continue
		}
		if already {
			result.AlreadyApplied++
			result.SucceededAuthIndexes = append(result.SucceededAuthIndexes, index)
			continue
		}
		if err := s.Host.SaveAuth(name, updated); err != nil {
			result.Errors = append(result.Errors, itemError(index, err))
			continue
		}
		result.Updated++
		result.SucceededAuthIndexes = append(result.SucceededAuthIndexes, index)
	}
	return result
}

func failedResult(proxy *domain.Proxy, index string, err error) Result {
	mode := "apply"
	if proxy == nil {
		mode = "clear"
	}
	return Result{Mode: mode, Errors: []ItemError{itemError(index, err)}, SucceededAuthIndexes: []string{}, Fatal: true}
}

func currentProxyURL(raw json.RawMessage) (string, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil || document == nil {
		if err == nil {
			err = i18n.New("invalid_request")
		}
		return "", err
	}
	value, exists := document["proxy_url"]
	if !exists || len(value) == 0 || string(value) == "null" {
		return "", nil
	}
	var current string
	if err := json.Unmarshal(value, &current); err != nil {
		return "", err
	}
	return strings.TrimSpace(current), nil
}

func withProxyURL(raw json.RawMessage, desired string) (updated json.RawMessage, already bool, err error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil || document == nil {
		if err == nil {
			err = i18n.New("invalid_request")
		}
		return nil, false, err
	}
	current, err := currentProxyURL(raw)
	if err != nil {
		return nil, false, err
	}
	if current == strings.TrimSpace(desired) {
		return append(json.RawMessage(nil), raw...), true, nil
	}
	encoded, err := json.Marshal(desired)
	if err != nil {
		return nil, false, err
	}
	document["proxy_url"] = encoded
	out, err := json.Marshal(document)
	if err != nil {
		return nil, false, err
	}
	return out, false, nil
}

func itemError(authIndex string, err error) ItemError {
	code := i18n.Code(err)
	if code == "internal_error" {
		code = "sync_failed"
	}
	message := err.Error()
	if message == "" {
		message = i18n.Text(i18n.ZH, code)
	}
	return ItemError{AuthIndex: authIndex, Code: code, Message: message}
}

func (r Result) Error() error {
	if len(r.Errors) == 0 {
		return nil
	}
	return fmt.Errorf("sync failed: %s", r.Errors[0].Message)
}
