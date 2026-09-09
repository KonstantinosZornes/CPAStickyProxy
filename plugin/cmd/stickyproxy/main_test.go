package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginRegistrationDoesNotExposeStatePathConfig(t *testing.T) {
	registration := pluginRegistration()
	if len(registration.Metadata.ConfigFields) != 0 {
		t.Fatalf("config fields = %#v, want none", registration.Metadata.ConfigFields)
	}
}

func TestPluginRegistrationUsesBuildVersion(t *testing.T) {
	original := pluginVersion
	defer func() { pluginVersion = original }()
	pluginVersion = "9.8.7"
	if got := pluginRegistration().Metadata.Version; got != "9.8.7" {
		t.Fatalf("registration version = %q, want injected build version", got)
	}
}

func TestPluginRegistrationNegotiatesSchemaVersion(t *testing.T) {
	tests := []struct {
		name string
		host uint32
		want uint32
	}{
		{name: "legacy or unspecified host", host: 0, want: schemaVersion},
		{name: "older host", host: schemaVersion - 1, want: schemaVersion - 1},
		{name: "same version", host: schemaVersion, want: schemaVersion},
		{name: "newer host", host: schemaVersion + 1, want: schemaVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pluginRegistrationForHost(tt.host).SchemaVersion; got != tt.want {
				t.Fatalf("negotiated schema = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHandleMethodAcceptsFutureHostSchema(t *testing.T) {
	current = runtime{}
	defer func() { current = runtime{} }()

	workingDir := t.TempDir()
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWorkingDir)

	request, err := json.Marshal(lifecycleRequest{SchemaVersion: schemaVersion + 100})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := handleMethod(methodPluginRegister, request)
	if err != nil {
		t.Fatal(err)
	}
	var response envelope
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("registration response = %#v, want success", response)
	}
	var registration registration
	if err := json.Unmarshal(response.Result, &registration); err != nil {
		t.Fatal(err)
	}
	if registration.SchemaVersion != schemaVersion {
		t.Fatalf("registration schema = %d, want known schema %d", registration.SchemaVersion, schemaVersion)
	}
}

func TestConfigureUsesJSONStatePathAndIgnoresLegacyState(t *testing.T) {
	// runtime contains synchronization primitives and must not be copied.
	// Tests do not run in parallel, so reset its global state before and after.
	current = runtime{}
	defer func() { current = runtime{} }()

	workingDir := t.TempDir()
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWorkingDir)

	legacyPath := filepath.Join("plugins", "stickyproxy-state")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"endpoints":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	configuredPath := filepath.Join(t.TempDir(), "operator-selected-state.json")
	configYAML := "plugins:\n  configs:\n    stickyproxy:\n      state_path: " + configuredPath
	request, err := json.Marshal(lifecycleRequest{
		ConfigYAML: []byte(configYAML),
		// A newer host schema must not prevent state initialization.
		SchemaVersion: schemaVersion + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := configure(request); err != nil {
		t.Fatal(err)
	}
	if got := current.statePath; got != builtinStatePath {
		t.Fatalf("state path = %q, want builtin %q", got, builtinStatePath)
	}
	if got := current.app.Store.Path(); got != builtinStatePath {
		t.Fatalf("store path = %q, want builtin %q", got, builtinStatePath)
	}
	if proxies, err := current.app.Store.Proxies(); err != nil || len(proxies) != 0 {
		t.Fatalf("new state proxies = %#v, err=%v", proxies, err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy state should remain untouched: %v", err)
	}
	if _, err := current.app.Store.Upsert("", "TestProxy", "dataimpulse", "http://login:secret@gw.dataimpulse.com:10000"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(builtinStatePath)
	if err != nil {
		t.Fatalf("builtin state file missing: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("builtin state mode = %o, want 600", got)
	}
	if _, err := os.Stat(configuredPath); !os.IsNotExist(err) {
		t.Fatalf("configured path was used: stat error=%v", err)
	}
}
