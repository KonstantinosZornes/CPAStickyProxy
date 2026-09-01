// Package host defines the tiny portion of the CLIProxyAPI host callback
// contract used for server-side account proxy synchronization.
package host

import (
	"encoding/json"
	"fmt"
)

const (
	MethodAuthList = "host.auth.list"
	MethodAuthGet  = "host.auth.get"
	MethodAuthSave = "host.auth.save"
)

type AuthEntry struct {
	ID          string `json:"id"`
	AuthIndex   string `json:"auth_index"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Label       string `json:"label"`
	Provider    string `json:"provider"`
	Type        string `json:"type"`
	AccountType string `json:"account_type"`
	Status      string `json:"status"`
	Disabled    bool   `json:"disabled"`
	Unavailable bool   `json:"unavailable"`
	Priority    int    `json:"priority"`
	Note        string `json:"note"`
	RuntimeOnly bool   `json:"runtime_only"`
}

type AuthDocument struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name"`
	JSON      json.RawMessage `json:"json"`
}

// Client is narrow enough to replace with a fake in unit tests. It prevents
// the domain and persistence modules from knowing anything about C callbacks.
type Client interface {
	ListAuths() ([]AuthEntry, error)
	GetAuth(authIndex string) (AuthDocument, error)
	SaveAuth(name string, raw json.RawMessage) error
}

// Caller is supplied by the cgo ABI layer. It returns only the result payload
// after it has decoded the host JSON envelope.
type Caller func(method string, payload any) (json.RawMessage, error)

type CallbackClient struct{ Call Caller }

type listResponse struct {
	Files []AuthEntry `json:"files"`
}

type getRequest struct {
	AuthIndex string `json:"auth_index"`
}

type getResponse struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name"`
	JSON      json.RawMessage `json:"json"`
}

type saveRequest struct {
	Name string          `json:"name"`
	JSON json.RawMessage `json:"json"`
}

func (c CallbackClient) ListAuths() ([]AuthEntry, error) {
	if c.Call == nil {
		return nil, fmt.Errorf("host callback unavailable")
	}
	raw, err := c.Call(MethodAuthList, map[string]any{})
	if err != nil {
		return nil, err
	}
	var result listResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode host.auth.list: %w", err)
	}
	return result.Files, nil
}

func (c CallbackClient) GetAuth(authIndex string) (AuthDocument, error) {
	if c.Call == nil {
		return AuthDocument{}, fmt.Errorf("host callback unavailable")
	}
	raw, err := c.Call(MethodAuthGet, getRequest{AuthIndex: authIndex})
	if err != nil {
		return AuthDocument{}, err
	}
	var result getResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return AuthDocument{}, fmt.Errorf("decode host.auth.get: %w", err)
	}
	return AuthDocument{AuthIndex: result.AuthIndex, Name: result.Name, JSON: append(json.RawMessage(nil), result.JSON...)}, nil
}

func (c CallbackClient) SaveAuth(name string, raw json.RawMessage) error {
	if c.Call == nil {
		return fmt.Errorf("host callback unavailable")
	}
	_, err := c.Call(MethodAuthSave, saveRequest{Name: name, JSON: append(json.RawMessage(nil), raw...)})
	return err
}
