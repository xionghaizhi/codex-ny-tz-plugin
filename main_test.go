package main

import (
	"encoding/json"
	"testing"
)

func TestInterceptRequestReplacesExistingAcceptLanguage(t *testing.T) {
	raw, err := json.Marshal(requestInterceptRequest{
		Headers: map[string][]string{"accept-language": {"zh-CN,zh;q=0.9"}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	result, err := interceptRequest(raw)
	if err != nil {
		t.Fatalf("intercept request: %v", err)
	}
	var envelope envelope
	if err := json.Unmarshal(result, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var response requestInterceptResponse
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.Headers["Accept-Language"]; len(got) != 1 || got[0] != "en-US,en;q=0.9" {
		t.Fatalf("headers = %#v", response.Headers)
	}
}

func TestInterceptRequestLeavesMissingAcceptLanguageUntouched(t *testing.T) {
	result, err := interceptRequest([]byte(`{"Headers":{"User-Agent":["codex"]}}`))
	if err != nil {
		t.Fatalf("intercept request: %v", err)
	}
	var envelope envelope
	if err := json.Unmarshal(result, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var response requestInterceptResponse
	if err := json.Unmarshal(envelope.Result, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Headers) != 0 || len(response.Body) != 0 || response.Terminate {
		t.Fatalf("response = %#v", response)
	}
}
