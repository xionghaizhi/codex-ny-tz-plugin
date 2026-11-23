package main

import (
	"encoding/json"
	"strings"
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

func TestConcurrencyRegistersOnlyStaticResources(t *testing.T) {
	raw, err := handleMethod("management.register", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result struct {
			Routes    []json.RawMessage
			Resources []struct{ Path, Menu string }
		}
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	resources := response.Result.Resources
	if len(response.Result.Routes) != 0 || len(resources) != 2 ||
		resources[0].Path != "/index.html" || resources[0].Menu != "并发查看" ||
		resources[1].Path != "/concurrency.js" || resources[1].Menu != "" {
		t.Fatalf("unexpected routes: %s", raw)
	}
}

func TestConcurrencyResources(t *testing.T) {
	for _, resource := range []struct {
		method, path, mime, content string
		status                      int
	}{
		{"GET", "/index.html", "text/html", "./concurrency.js", 200},
		{"GET", "/concurrency.js", "text/javascript", "page_size=1000", 200},
		{"GET", "/anything", "text/plain", "", 404},
		{"POST", "/index.html", "text/plain", "", 404},
	} {
		request, _ := json.Marshal(map[string]string{"Method": resource.method, "Path": concurrencyResourcePath + resource.path})
		raw, err := handleMethod("management.handle", request)
		if err != nil {
			t.Fatal(err)
		}
		var response struct{ Result concurrencyResourceResponse }
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		result := response.Result
		if result.StatusCode != resource.status || !strings.HasPrefix(result.Headers.Get("Content-Type"), resource.mime) ||
			!strings.Contains(string(result.Body), resource.content) || (resource.status == 404 && len(result.Body) != 0) {
			t.Errorf("%s %s: status=%d type=%s", resource.method, resource.path, result.StatusCode, result.Headers.Get("Content-Type"))
		}
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
