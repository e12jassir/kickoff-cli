package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff/internal/catalog"
)

func TestRunConfigImport_JSONLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	sourceJSON := filepath.Join(tmpDir, "source.json")
	content := `{
		"streams": {
			"espn1": "http://example.com/espn1.m3u8",
			"win_plus": "http://example.com/winplus.m3u8"
		}
	}`
	if err := os.WriteFile(sourceJSON, []byte(content), 0o644); err != nil {
		t.Fatalf("error creating source json: %v", err)
	}

	if err := RunConfigImport(sourceJSON); err != nil {
		t.Fatalf("unexpected error importing JSON: %v", err)
	}

	target := filepath.Join(tmpDir, "kickoff", "channels.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("error reading target channels.json: %v", err)
	}

	var ext catalog.ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
		t.Fatalf("target channels.json is not valid JSON: %v", err)
	}

	if len(ext.Streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(ext.Streams))
	}
	if ext.Streams["espn1"] != "http://example.com/espn1.m3u8" {
		t.Errorf("expected espn1 stream, got %s", ext.Streams["espn1"])
	}
}

func TestRunConfigImport_JSONHTTPServer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"streams": {
				"caracol": "http://example.com/caracol.m3u8"
			}
		}`))
	}))
	defer server.Close()

	if err := RunConfigImport(server.URL); err != nil {
		t.Fatalf("unexpected error importing JSON from HTTP: %v", err)
	}

	target := filepath.Join(tmpDir, "kickoff", "channels.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("error reading target channels.json: %v", err)
	}

	var ext catalog.ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
		t.Fatalf("target channels.json is not valid JSON: %v", err)
	}

	if ext.Streams["caracol"] != "http://example.com/caracol.m3u8" {
		t.Errorf("expected caracol stream, got %s", ext.Streams["caracol"])
	}
}

func TestRunConfigImport_M3ULocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	m3uPath := filepath.Join(tmpDir, "playlist_src.m3u")
	m3uContent := `#EXTM3U
#EXTINF:-1 tvg-id="espn1" tvg-name="ESPN 1",ESPN 1 HD
http://example.com/espn1.m3u8
#EXTINF:-1 tvg-id="winplus" tvg-name="Win+",Win+ Fútbol
http://example.com/winplus.m3u8
`
	if err := os.WriteFile(m3uPath, []byte(m3uContent), 0o644); err != nil {
		t.Fatalf("error creating test m3u: %v", err)
	}

	if err := RunConfigImport(m3uPath); err != nil {
		t.Fatalf("unexpected error importing M3U: %v", err)
	}

	// Verify playlist.m3u was saved
	playlistTarget := filepath.Join(tmpDir, "kickoff", "playlist.m3u")
	savedM3U, err := os.ReadFile(playlistTarget)
	if err != nil {
		t.Fatalf("playlist.m3u not saved: %v", err)
	}
	if !strings.Contains(string(savedM3U), "ESPN 1 HD") {
		t.Errorf("saved playlist missing expected content")
	}

	// Verify channels.json was updated with playlist_path
	channelsTarget := filepath.Join(tmpDir, "kickoff", "channels.json")
	channelsData, err := os.ReadFile(channelsTarget)
	if err != nil {
		t.Fatalf("channels.json not written: %v", err)
	}

	var ext catalog.ExternalConfig
	if err := json.Unmarshal(channelsData, &ext); err != nil {
		t.Fatalf("channels.json is not valid JSON: %v", err)
	}
	if ext.PlaylistPath != playlistTarget {
		t.Errorf("expected PlaylistPath %q, got %q", playlistTarget, ext.PlaylistPath)
	}
}

func TestRunConfigImport_M3UHTTPServer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegurl")
		_, _ = w.Write([]byte(`#EXTM3U
#EXTINF:-1 tvg-id="tdp",Teledeporte
http://example.com/tdp.m3u8
`))
	}))
	defer server.Close()

	if err := RunConfigImport(server.URL); err != nil {
		t.Fatalf("unexpected error importing M3U from HTTP: %v", err)
	}

	playlistTarget := filepath.Join(tmpDir, "kickoff", "playlist.m3u")
	if _, err := os.Stat(playlistTarget); err != nil {
		t.Fatalf("playlist.m3u does not exist: %v", err)
	}
}

func TestRunConfigImport_BackupCreation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, "kickoff")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("failed to create cfgDir: %v", err)
	}

	targetChannels := filepath.Join(cfgDir, "channels.json")
	initialContent := `{"streams": {"initial": "http://example.com/initial.m3u8"}}`
	if err := os.WriteFile(targetChannels, []byte(initialContent), 0o644); err != nil {
		t.Fatalf("failed to write initial channels.json: %v", err)
	}

	newSource := filepath.Join(tmpDir, "new.json")
	newContent := `{"streams": {"updated": "http://example.com/updated.m3u8"}}`
	if err := os.WriteFile(newSource, []byte(newContent), 0o644); err != nil {
		t.Fatalf("failed to write new.json: %v", err)
	}

	if err := RunConfigImport(newSource); err != nil {
		t.Fatalf("unexpected error importing new config: %v", err)
	}

	// Verify backup file exists and has initial content
	bakFile := filepath.Join(cfgDir, "channels.json.bak")
	bakData, err := os.ReadFile(bakFile)
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}
	if !strings.Contains(string(bakData), "initial.m3u8") {
		t.Errorf("backup file content mismatch, got: %s", string(bakData))
	}

	// Verify active config has new content
	activeData, err := os.ReadFile(targetChannels)
	if err != nil {
		t.Fatalf("failed reading active channels.json: %v", err)
	}
	if !strings.Contains(string(activeData), "updated.m3u8") {
		t.Errorf("active channels.json does not contain updated stream")
	}
}

func TestRunConfigImport_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// 1. Empty source
	if err := RunConfigImport(""); err == nil {
		t.Error("expected error for empty source, got nil")
	}

	// 2. Non-existent file
	if err := RunConfigImport(filepath.Join(tmpDir, "missing.json")); err == nil {
		t.Error("expected error for missing file, got nil")
	}

	// 3. HTTP 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	if err := RunConfigImport(server.URL); err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}

	// 4. Invalid format
	invalidFile := filepath.Join(tmpDir, "invalid.txt")
	if err := os.WriteFile(invalidFile, []byte("this is random non-json and non-m3u text"), 0o644); err != nil {
		t.Fatalf("failed writing invalid file: %v", err)
	}
	if err := RunConfigImport(invalidFile); err == nil {
		t.Error("expected error for invalid format, got nil")
	}
}
