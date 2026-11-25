package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const cleanupRoute = "/plugins/codex-ny-timezone/cleanup-history"
const cleanupResourcePath = "/v0/resource/plugins/codex-ny-timezone"
const historyDirectory = "codex-ny-timezone"
const historyFile = "concurrency-history.json"

//go:embed web/cleanup.html
var cleanupPage []byte

//go:embed web/cleanup.js
var cleanupScript []byte

type cleanupResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

type historyFileInfo struct {
	Name         string `json:"name"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	ModifiedAt   string `json:"modified_at"`
}

func serveCleanup(raw []byte) ([]byte, error) {
	var request struct {
		Method, Path string
		Body         []byte
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	response := cleanupResponse{StatusCode: http.StatusNotFound, Headers: http.Header{
		"Content-Type":           {"text/plain; charset=utf-8"},
		"Cache-Control":          {"no-store"},
		"X-Content-Type-Options": {"nosniff"},
	}}
	switch {
	case request.Method == http.MethodGet && request.Path == "/v0/management"+cleanupRoute:
		response.StatusCode, response.Body = listHistoryResponse()
		response.Headers.Set("Content-Type", "application/json")
	case request.Method == http.MethodPost && request.Path == "/v0/management"+cleanupRoute:
		response.StatusCode, response.Body = clearHistoryResponse(request.Body)
		response.Headers.Set("Content-Type", "application/json")
	case request.Method == http.MethodGet && request.Path == cleanupResourcePath+"/cleanup.html":
		response.StatusCode, response.Body = http.StatusOK, cleanupPage
		response.Headers.Set("Content-Type", "text/html; charset=utf-8")
		response.Headers.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
	case request.Method == http.MethodGet && request.Path == cleanupResourcePath+"/cleanup.js":
		response.StatusCode, response.Body = http.StatusOK, cleanupScript
		response.Headers.Set("Content-Type", "text/javascript; charset=utf-8")
	}
	return okEnvelope(response)
}

func clearHistoryResponse(body []byte) (int, []byte) {
	var selection struct {
		File    string `json:"file"`
		Confirm string `json:"confirm"`
	}
	if err := json.Unmarshal(body, &selection); err != nil {
		return http.StatusBadRequest, []byte(`{"status":"confirmation_required"}`)
	}
	if selection.File != historyFile || selection.Confirm != historyFile {
		return http.StatusBadRequest, []byte(`{"status":"confirmation_required"}`)
	}
	if err := clearConcurrencyHistory(); errors.Is(err, os.ErrNotExist) {
		return http.StatusOK, []byte(`{"status":"missing"}`)
	} else if err != nil {
		return http.StatusConflict, []byte(`{"status":"unavailable"}`)
	}
	return http.StatusOK, []byte(`{"status":"removed"}`)
}

func listHistoryResponse() (int, []byte) {
	configDir, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(configDir) {
		return http.StatusConflict, []byte(`{"status":"unavailable"}`)
	}
	historyDir := filepath.Join(configDir, historyDirectory)
	dir, err := os.Lstat(historyDir)
	if os.IsNotExist(err) {
		return http.StatusOK, []byte(`{"files":[]}`)
	}
	if err != nil || !dir.IsDir() {
		return http.StatusConflict, []byte(`{"status":"unavailable"}`)
	}
	historyPath := filepath.Join(historyDir, historyFile)
	file, err := os.Lstat(historyPath)
	if os.IsNotExist(err) {
		return http.StatusOK, []byte(`{"files":[]}`)
	}
	if err != nil || !file.Mode().IsRegular() {
		return http.StatusConflict, []byte(`{"status":"unavailable"}`)
	}
	payload := struct {
		Files []historyFileInfo `json:"files"`
	}{Files: []historyFileInfo{{
		Name: historyFile, RelativePath: historyDirectory + "/" + historyFile,
		Size: file.Size(), ModifiedAt: file.ModTime().Format(time.RFC3339),
	}}}
	body, err := json.Marshal(payload)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{"status":"unavailable"}`)
	}
	return http.StatusOK, body
}

func clearConcurrencyHistory() error {
	configDir, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(configDir) {
		return errors.New("CPA user configuration directory is unavailable")
	}
	// Match the old writer's path, but never follow a replacement plugin directory.
	historyDir := filepath.Join(configDir, historyDirectory)
	dir, err := os.Lstat(historyDir)
	if err != nil {
		return err
	}
	if !dir.IsDir() {
		return errors.New("history directory is not a directory")
	}
	historyPath := filepath.Join(historyDir, historyFile)
	file, err := os.Lstat(historyPath)
	if err != nil {
		return err
	}
	if !file.Mode().IsRegular() {
		return errors.New("history file is not a regular file")
	}
	return os.Remove(historyPath)
}
