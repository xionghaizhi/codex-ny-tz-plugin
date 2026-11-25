package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testHistoryDirectory(t *testing.T) string {
	t.Helper()
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", config)
	t.Setenv("AppData", config)
	userConfig, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(userConfig, "codex-ny-timezone")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCleanupHistoryOnly(t *testing.T) {
	dir := testHistoryDirectory(t)
	target := filepath.Join(dir, historyFile)
	otherState := filepath.Join(filepath.Dir(dir), ".cpa-account-config-manager")
	if err := os.Mkdir(otherState, 0700); err != nil {
		t.Fatal(err)
	}
	keep := []string{filepath.Join(otherState, historyFile), filepath.Join(otherState, "model-pricing-cache.json"), filepath.Join(dir, ".concurrency-history-1")}
	for _, path := range append(keep, target) {
		if err := os.WriteFile(path, []byte("keep or remove by path"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	expectCleanup(t, "GET", "/v0/management"+cleanupRoute, "", 200, "")
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"confirm":"model-pricing-cache.json"}`, 400, "confirmation_required")
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"file":"concurrency-history.json","confirm":"concurrency-history.json"}`, 200, "removed")
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"file":"concurrency-history.json","confirm":"concurrency-history.json"}`, 200, "missing")
	for _, path := range keep {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "keep or remove by path" {
			t.Errorf("unrelated state changed at %s: %s %v", path, raw, err)
		}
	}
}

func TestListHistoryOnly(t *testing.T) {
	dir := testHistoryDirectory(t)
	target := filepath.Join(dir, historyFile)
	if err := os.WriteFile(target, []byte(`{"peak":6,"peak_at":1795514374}`), 0600); err != nil {
		t.Fatal(err)
	}
	otherState := filepath.Join(filepath.Dir(dir), ".cpa-account-config-manager")
	if err := os.Mkdir(otherState, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherState, historyFile), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	status, body := listHistoryResponse()
	if status != 200 {
		t.Fatalf("expected HTTP 200, received %d", status)
	}
	var response struct {
		Files []historyFileInfo `json:"files"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Files) != 1 || response.Files[0].Name != historyFile || response.Files[0].RelativePath != historyDirectory+"/"+historyFile {
		t.Fatalf("listed files = %#v", response.Files)
	}
}

func TestCleanupRejectsDirectoriesAndLinks(t *testing.T) {
	dir := testHistoryDirectory(t)
	target := filepath.Join(dir, historyFile)
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"file":"concurrency-history.json","confirm":"concurrency-history.json"}`, 409, "unavailable")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	otherFile := filepath.Join(other, historyFile)
	if err := os.WriteFile(otherFile, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherFile, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"file":"concurrency-history.json","confirm":"concurrency-history.json"}`, 409, "unavailable")
	if err := errors.Join(os.Remove(target), os.Remove(dir)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, dir); err != nil {
		t.Fatal(err)
	}
	expectCleanup(t, "POST", "/v0/management"+cleanupRoute, `{"file":"concurrency-history.json","confirm":"concurrency-history.json"}`, 409, "unavailable")
	if raw, err := os.ReadFile(otherFile); err != nil || string(raw) != "unchanged" {
		t.Fatalf("symlink destination changed: %s %v", raw, err)
	}
}

func expectCleanup(t *testing.T, method, path, confirmation string, code int, status string) {
	t.Helper()
	request, err := json.Marshal(map[string]any{"Method": method, "Path": path, "Body": []byte(confirmation)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := handleMethod("management.handle", request)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		OK     bool
		Result cleanupResponse
	}
	if err := json.Unmarshal(raw, &response); err != nil || !response.OK || response.Result.StatusCode != code {
		t.Fatalf("expected HTTP %d, received %s (%v)", code, raw, err)
	}
	if status != "" && string(response.Result.Body) != `{"status":"`+status+`"}` {
		t.Fatalf("expected status %s, received %s", status, response.Result.Body)
	}
}
