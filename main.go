package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	void* call;
	void* free_buffer;
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
*/
import "C"

import (
	"encoding/json"
	"strings"
	"time"
	"unsafe"
)

const abiVersion uint32 = 1

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}
type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}
type requestTransform struct {
	FromFormat string `json:"FromFormat"`
	ToFormat   string `json:"ToFormat"`
	Model      string `json:"Model"`
	Stream     bool   `json:"Stream"`
	Body       []byte `json:"Body"`
}
type payloadResponse struct {
	Body []byte `json:"Body"`
}

type requestInterceptRequest struct {
	RequestID      string              `json:"RequestID"`
	TraceID        string              `json:"TraceID"`
	SourceFormat   string              `json:"SourceFormat"`
	ToFormat       string              `json:"ToFormat"`
	Model          string              `json:"Model"`
	RequestedModel string              `json:"RequestedModel"`
	Stream         bool                `json:"Stream"`
	Headers        map[string][]string `json:"Headers"`
	Body           []byte              `json:"Body"`
	Metadata       map[string]any      `json:"Metadata"`
}

type requestInterceptResponse struct {
	Headers         map[string][]string `json:"Headers,omitempty"`
	Body            []byte              `json:"Body,omitempty"`
	ClearHeaders    []string            `json:"ClearHeaders,omitempty"`
	Terminate       bool                `json:"Terminate,omitempty"`
	StatusCode      int                 `json:"StatusCode,omitempty"`
	ResponseHeaders map[string][]string `json:"ResponseHeaders,omitempty"`
	ResponseBody    []byte              `json:"ResponseBody,omitempty"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		configure(request)
		return okEnvelope(registrationJSON())
	case "request.normalize":
		return normalizeRequest(request)
	case "request.intercept_before", "request.intercept_after":
		return interceptRequest(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) {
	tz := defaultTZ
	if len(raw) > 0 {
		var req lifecycleRequest
		if json.Unmarshal(raw, &req) == nil && len(req.ConfigYAML) > 0 {
			if v := yamlLike("timezone", string(req.ConfigYAML)); v != "" {
				tz = v
			}
		}
	}
	if _, err := time.LoadLocation(tz); err != nil {
		tz = defaultTZ
	}
	tzName.Store(tz)
}

func yamlLike(key, text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, key+":") {
			v := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func registrationJSON() map[string]any {
	return map[string]any{
		"schema_version": 1,
		"metadata": map[string]any{
			"Name":             "codex-ny-timezone",
			"Version":          "1.0.0",
			"Author":           "local",
			"GitHubRepository": "https://github.com/xionghaizhi/codex-ny-tz-plugin",
			"Logo":             "https://github.com/xionghaizhi/codex-ny-tz-plugin",
			"ConfigFields": []map[string]any{{
				"Name":        "timezone",
				"Type":        "string",
				"Description": "IANA timezone for Codex environment_context rewrite",
			}},
		},
		"capabilities": map[string]any{
			"request_normalizer":  true,
			"request_interceptor": true,
		},
	}
}

func normalizeRequest(raw []byte) ([]byte, error) {
	var req requestTransform
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
	}
	return okEnvelope(payloadResponse{Body: rewriteEnvContext(req.Body)})
}

func interceptRequest(raw []byte) ([]byte, error) {
	var req requestInterceptRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
	}
	for headerName := range req.Headers {
		if strings.EqualFold(headerName, "Accept-Language") {
			return okEnvelope(requestInterceptResponse{
				Headers: map[string][]string{
					"Accept-Language": {"en-US,en;q=0.9"},
				},
			})
		}
	}
	return okEnvelope(requestInterceptResponse{})
}

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
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
