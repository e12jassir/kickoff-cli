package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunInit_CreatesChannelsJSON(t *testing.T) {
	_, cfgDir := isolateConfigHome(t)

	expectedPath := filepath.Join(cfgDir, "channels.json")

	// 1. Initial creation
	if err := RunInit(false); err != nil {
		t.Fatalf("RunInit(false) failed: %v", err)
	}

	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read expected channels.json at %s: %v", expectedPath, err)
	}

	if !json.Valid(data) {
		t.Fatalf("generated channels.json is not valid JSON:\n%s", string(data))
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated channels.json: %v", err)
	}

	streams, ok := parsed["streams"].(map[string]any)
	if !ok || len(streams) == 0 {
		t.Fatalf("expected non-empty streams map in channels.json, got: %v", parsed["streams"])
	}

	if _, ok := streams["espn1"]; !ok {
		t.Errorf("expected 'espn1' placeholder stream in streams, got: %v", streams)
	}
}

func TestRunInit_DoesNotOverwriteWithoutForce(t *testing.T) {
	_, cfgDir := isolateConfigHome(t)

	expectedPath := filepath.Join(cfgDir, "channels.json")

	// Initial run
	if err := RunInit(false); err != nil {
		t.Fatalf("RunInit(false) failed: %v", err)
	}

	customContent := `{"custom": "donotoverwrite"}`
	if err := os.WriteFile(expectedPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("failed to write custom content: %v", err)
	}

	// Second run with force=false
	if err := RunInit(false); err != nil {
		t.Fatalf("RunInit(false) returned unexpected error: %v", err)
	}

	currentData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read channels.json: %v", err)
	}

	if string(currentData) != customContent {
		t.Errorf("expected file not to be overwritten, got:\n%s", string(currentData))
	}
}

func TestRunInit_OverwritesWithForce(t *testing.T) {
	_, cfgDir := isolateConfigHome(t)

	expectedPath := filepath.Join(cfgDir, "channels.json")

	// Initial run
	if err := RunInit(false); err != nil {
		t.Fatalf("RunInit(false) failed: %v", err)
	}

	customContent := `{"custom": "shouldbeoverwritten"}`
	if err := os.WriteFile(expectedPath, []byte(customContent), 0o644); err != nil {
		t.Fatalf("failed to write custom content: %v", err)
	}

	// Run with force=true
	if err := RunInit(true); err != nil {
		t.Fatalf("RunInit(true) failed: %v", err)
	}

	currentData, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read channels.json: %v", err)
	}

	if string(currentData) == customContent {
		t.Errorf("expected file to be overwritten when force=true, but content was unchanged")
	}

	if !strings.Contains(string(currentData), "your-iptv-provider.example") {
		t.Errorf("expected starter template content after force overwrite, got:\n%s", string(currentData))
	}
}

func TestRunInit_HomeFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserConfigDir is driven by AppData on Windows, so the HOME fallback cannot be exercised there")
	}

	tmpHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tmpHome)

	configBase, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("could not resolve the user config dir: %v", err)
	}
	if !strings.HasPrefix(configBase, tmpHome) {
		t.Fatalf("test setup did not isolate the config dir: %s", configBase)
	}
	expectedPath := filepath.Join(configBase, "kickoff", "channels.json")

	if err := RunInit(false); err != nil {
		t.Fatalf("RunInit(false) failed with HOME fallback: %v", err)
	}

	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file to exist at %s: %v", expectedPath, err)
	}
}
