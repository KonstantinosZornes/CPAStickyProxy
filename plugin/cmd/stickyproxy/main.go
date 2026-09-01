package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stickyproxy_host;
static void stickyproxy_store_host(const cliproxy_host_api* host) { stickyproxy_host = host; }
static int stickyproxy_call_host(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stickyproxy_host == NULL || stickyproxy_host->call == NULL) return 1;
	return stickyproxy_host->call(stickyproxy_host->host_ctx, method, request, request_len, response);
}
static void stickyproxy_free_host_buffer(void* ptr, size_t len) {
	if (stickyproxy_host != NULL && stickyproxy_host->free_buffer != NULL && ptr != NULL) stickyproxy_host->free_buffer(ptr, len);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"unsafe"

	pluginapp "stickyproxy/native-plugin/internal/app"
	"stickyproxy/native-plugin/internal/host"
	"stickyproxy/native-plugin/internal/state"
	"stickyproxy/native-plugin/web"
)

const (
	abiVersion uint32 = 1

	// schemaVersion mirrors the highest CPA pluginabi RPC contract this plugin
	// understands. Hosts announce their schema_version at register/reconfigure;
	// CPA v7.2.145+ (schema 4) is rejected unless declared here, so this must
	// track pluginabi.SchemaVersion or newer hosts drop the plugin. Schema 4
	// only adds websocket/egress capabilities this plugin does not use, so the
	// v3 wire contract it relies on is unchanged.
	schemaVersion uint32 = 4

	// builtinStatePath is deliberately not a plugin configuration field. CPA
	// resolves this relative to its own working directory, alongside plugins,
	// so reconfiguring the plugin cannot move or duplicate its state. The JSON
	// suffix also gives V3 its own path, leaving incompatible legacy state intact.
	builtinStatePath = "plugins/stickyproxy-state.json"
)

const (
	methodPluginRegister     = "plugin.register"
	methodPluginReconfigure  = "plugin.reconfigure"
	methodManagementRegister = "management.register"
	methodManagementHandle   = "management.handle"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      metadata               `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type metadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	ConfigFields     []configField `json:"ConfigFields"`
}

type configField struct {
	Name        string `json:"Name"`
	Type        string `json:"Type"`
	Description string `json:"Description"`
}

type registrationCapability struct {
	ManagementAPI bool `json:"management_api"`
}

type managementRegistration struct {
	Routes    []managementRoute `json:"routes,omitempty"`
	Resources []resourceRoute   `json:"resources,omitempty"`
}

type managementRoute struct {
	Method string `json:"Method"`
	Path   string `json:"Path"`
}

type resourceRoute struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type managementRequest struct {
	Method  string              `json:"Method"`
	Path    string              `json:"Path"`
	Headers map[string][]string `json:"Headers"`
	Query   map[string][]string `json:"Query"`
	Body    []byte              `json:"Body"`
}

type managementResponse struct {
	StatusCode int                 `json:"StatusCode"`
	Headers    map[string][]string `json:"Headers"`
	Body       []byte              `json:"Body"`
}

type runtime struct {
	mu        sync.RWMutex
	syncMu    sync.Mutex
	statePath string
	app       pluginapp.App
}

// pluginVersion is overridden by release builds with -X main.pluginVersion.
// Keep a useful value for local development builds.
var pluginVersion = "0.1.0-dev"

var current runtime

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(hostAPI *C.cliproxy_host_api, pluginAPI *C.cliproxy_plugin_api) C.int {
	if pluginAPI == nil || hostAPI == nil || uint32(hostAPI.abi_version) != abiVersion {
		return 1
	}
	C.stickyproxy_store_host(hostAPI)
	pluginAPI.abi_version = C.uint32_t(abiVersion)
	pluginAPI.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	pluginAPI.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	pluginAPI.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLength C.size_t, response *C.cliproxy_buffer) (result C.int) {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			// Never stringify a panic value: proxy URLs and credentials may be
			// present in a lower-level error.
			writeResponse(response, errorEnvelope("plugin_panic", "plugin internal error", http.StatusInternalServerError))
			result = 1
		}
	}()
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required", http.StatusBadRequest))
		return 1
	}
	var rawRequest []byte
	if request != nil && requestLength > 0 {
		rawRequest = C.GoBytes(unsafe.Pointer(request), C.int(requestLength))
	}
	payload, err := handleMethod(C.GoString(method), rawRequest)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error(), http.StatusInternalServerError))
		return 1
	}
	writeResponse(response, payload)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	current.mu.Lock()
	current.app = pluginapp.App{}
	current.statePath = ""
	current.mu.Unlock()
}

func handleMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case methodPluginRegister, methodPluginReconfigure:
		if err := configure(raw); err != nil {
			// State parsing may encounter a plaintext proxy URL. Do not return
			// implementation error text through the ABI envelope.
			return errorEnvelope("configure_failed", "plugin configuration failed", http.StatusInternalServerError), nil
		}
		return okEnvelope(pluginRegistration()), nil
	case methodManagementRegister:
		return okEnvelope(managementRegistrationResponse()), nil
	case methodManagementHandle:
		var request managementRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return okEnvelope(jsonManagement(http.StatusBadRequest, []byte(`{"ok":false,"code":"invalid_json"}`))), nil
		}
		return okEnvelope(handleManagement(request)), nil
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method, http.StatusBadRequest), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: schemaVersion,
		Metadata: metadata{
			Name: "StickyProxy", Version: pluginVersion, Author: "StickyProxy",
			GitHubRepository: "https://github.com/KonstantinosZornes/CPAStickyProxy",
			ConfigFields:     []configField{},
		},
		Capabilities: registrationCapability{ManagementAPI: true},
	}
}

func managementRegistrationResponse() managementRegistration {
	return managementRegistration{
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: "/stickyproxy/state"},
			{Method: http.MethodGet, Path: "/stickyproxy/accounts"},
			{Method: http.MethodPost, Path: "/stickyproxy/proxies"},
			{Method: http.MethodPost, Path: "/stickyproxy/proxies/bulk"},
			{Method: http.MethodPost, Path: "/stickyproxy/proxies/delete"},
			{Method: http.MethodPost, Path: "/stickyproxy/proxies/test"},
			{Method: http.MethodPost, Path: "/stickyproxy/apply"},
			{Method: http.MethodPost, Path: "/stickyproxy/clear-proxy"},
			{Method: http.MethodPost, Path: "/stickyproxy/clear-applied"},
			{Method: http.MethodPost, Path: "/stickyproxy/preview"},
		},
		Resources: []resourceRoute{{
			// CPA renders this one static registration string in its own navigation
			// and breadcrumb. Use the language-neutral plugin name; dashboard
			// content itself remains translated from the user's CPA language.
			Path: "/dashboard", Menu: "StickyProxy",
			Description: "Manage StickyProxy account-level proxy configurations.",
		}},
	}
}

func configure(raw []byte) error {
	var request lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return err
		}
	}
	if request.SchemaVersion > schemaVersion {
		return fmt.Errorf("unsupported plugin schema version %d", request.SchemaVersion)
	}
	path := builtinStatePath
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.app.Store != nil && current.statePath == path {
		return nil
	}
	store, err := state.New(path)
	if err != nil {
		return err
	}
	current.statePath = path
	current.app = pluginapp.App{Store: store, Host: host.CallbackClient{Call: callHost}, SyncMu: &current.syncMu}
	return nil
}

func handleManagement(request managementRequest) managementResponse {
	const dashboardPath = "/v0/resource/plugins/stickyproxy/dashboard"
	if request.Method == http.MethodGet && request.Path == dashboardPath {
		return managementResponse{
			StatusCode: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type":            {"text/html; charset=utf-8"},
				"Cache-Control":           {"no-store"},
				"X-Content-Type-Options":  {"nosniff"},
				"Referrer-Policy":         {"no-referrer"},
				"Content-Security-Policy": {"default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'self'"},
			},
			Body: web.Page,
		}
	}

	current.mu.RLock()
	application := current.app
	current.mu.RUnlock()
	if application.Store == nil {
		if err := configure(nil); err != nil {
			return jsonManagement(http.StatusInternalServerError, []byte(`{"ok":false,"code":"state_read_failed"}`))
		}
		current.mu.RLock()
		application = current.app
		current.mu.RUnlock()
	}
	response := application.Handle(pluginapp.Request{
		Method: request.Method, Path: request.Path, Headers: request.Headers, Query: request.Query, Body: request.Body,
	})
	return jsonManagement(response.Status, response.Body)
}

func jsonManagement(status int, body []byte) managementResponse {
	return managementResponse{
		StatusCode: status,
		Headers: map[string][]string{
			"Content-Type":  {"application/json; charset=utf-8"},
			"Cache-Control": {"no-store"},
		},
		Body: body,
	}
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal host callback %s: %w", method, err)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var request *C.uint8_t
	if len(rawPayload) > 0 {
		ptr := C.CBytes(rawPayload)
		if ptr == nil {
			return nil, fmt.Errorf("allocate host callback payload")
		}
		defer C.free(ptr)
		request = (*C.uint8_t)(ptr)
	}
	var response C.cliproxy_buffer
	code := C.stickyproxy_call_host(cMethod, request, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.stickyproxy_free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response (code=%d)", method, int(code))
	}
	var responseEnvelope envelope
	if err := json.Unmarshal(rawResponse, &responseEnvelope); err != nil {
		return nil, fmt.Errorf("decode host callback %s: %w", method, err)
	}
	if !responseEnvelope.OK {
		if responseEnvelope.Error != nil {
			return nil, fmt.Errorf("%s: %s", responseEnvelope.Error.Code, responseEnvelope.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if code != 0 {
		return nil, fmt.Errorf("host callback %s returned code=%d", method, int(code))
	}
	return append(json.RawMessage(nil), responseEnvelope.Result...), nil
}

func okEnvelope(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return errorEnvelope("encode_error", err.Error(), http.StatusInternalServerError)
	}
	out, _ := json.Marshal(envelope{OK: true, Result: raw})
	return out
}

func errorEnvelope(code, message string, status int) []byte {
	out, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message, HTTPStatus: status}})
	return out
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
