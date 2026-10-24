package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateConfigHome points every platform's user-config variable at a fresh
// temp directory and returns both that temp root and the kickoff config
// directory the production code resolves.
//
// os.UserConfigDir honours XDG_CONFIG_HOME on Linux, $HOME/Library/Application
// Support on macOS and %AppData% on Windows, so all three are set; the tests then
// ask the same function production uses where the write actually landed.
func isolateConfigHome(t *testing.T) (tempRoot, configDir string) {
	t.Helper()

	tempRoot = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)
	t.Setenv("AppData", tempRoot)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("could not resolve an isolated user config dir: %v", err)
	}
	if base == "" || !strings.HasPrefix(base, tempRoot) {
		t.Fatalf("user config dir %q is not isolated under the temp root %q", base, tempRoot)
	}

	return tempRoot, filepath.Join(base, "kickoff")
}

// writeConfigBody writes raw config.json content so a test can exercise values
// the writer itself refuses to produce.
func writeConfigBody(t *testing.T, configDir, body string) {
	t.Helper()

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("could not create config dir %q: %v", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("could not write config body: %v", err)
	}
}

func TestConfigPath(t *testing.T) {
	_, configDir := isolateConfigHome(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error: %v", err)
	}
	if want := filepath.Join(configDir, "config.json"); path != want {
		t.Errorf("ConfigPath() = %q, want %q", path, want)
	}
}

// TestWriteConfigLanguageRoundTrip covers the whole write path: the file is
// created where ConfigPath says, holds {"lang": ...}, leaves no scratch file
// behind and wins layer 2 of the precedence chain.
func TestWriteConfigLanguageRoundTrip(t *testing.T) {
	_, configDir := isolateConfigHome(t)
	setLanguageEnv(t, EnvVar, "")
	setLanguageEnv(t, "LC_ALL", "")
	setLanguageEnv(t, "LC_MESSAGES", "")
	setLanguageEnv(t, "LANG", "es_ES.UTF-8")

	if err := WriteConfigLanguage(EN); err != nil {
		t.Fatalf("WriteConfigLanguage(EN) error: %v", err)
	}

	path := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read the written config: %v", err)
	}

	var decoded struct {
		Lang string `json:"lang"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("written config is not valid JSON: %v (%q)", err, data)
	}
	if decoded.Lang != string(EN) {
		t.Errorf("written lang = %q, want %q", decoded.Lang, EN)
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatalf("could not read the config dir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "config.json" {
			t.Errorf("atomic write left %q behind in the config dir", entry.Name())
		}
	}

	if got := ResolveLanguage(); got != EN {
		t.Errorf("ResolveLanguage() = %q after writing %q, want %q", got, EN, EN)
	}
}

func TestWriteConfigLanguageRejectsUnsupportedLanguage(t *testing.T) {
	_, configDir := isolateConfigHome(t)

	if err := WriteConfigLanguage(Lang("fr")); err == nil {
		t.Fatal("WriteConfigLanguage(fr) returned nil error")
	}

	if _, err := os.Stat(filepath.Join(configDir, "config.json")); !os.IsNotExist(err) {
		t.Errorf("a rejected language still created config.json (stat error: %v)", err)
	}
}

// TestResolveLanguageToleratesMalformedConfig pins that a broken configuration
// never crashes resolution and never invents a language.
func TestResolveLanguageToleratesMalformedConfig(t *testing.T) {
	cases := []struct {
		name string
		body string
		want Lang
	}{
		{"empty file", "", ES},
		{"not json", "not json at all", ES},
		{"json array", "[]", ES},
		{"json null", "null", ES},
		{"lang is a number", `{"lang": 42}`, ES},
		{"lang is empty", `{"lang": ""}`, ES},
		{"lang is whitespace", `{"lang": "  "}`, ES},
		{"other keys only", `{"theme": "dark"}`, ES},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, configDir := isolateConfigHome(t)
			writeConfigBody(t, configDir, tc.body)
			setLanguageEnv(t, EnvVar, "")
			setLanguageEnv(t, "LC_ALL", "")
			setLanguageEnv(t, "LC_MESSAGES", "")
			setLanguageEnv(t, "LANG", "")

			if got := ResolveLanguage(); got != tc.want {
				t.Errorf("ResolveLanguage() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConfigFilePerm keeps the written configuration readable by the user and
// nothing wider, matching the rest of the app's user files.
func TestConfigFilePerm(t *testing.T) {
	_, configDir := isolateConfigHome(t)

	if err := WriteConfigLanguage(ES); err != nil {
		t.Fatalf("WriteConfigLanguage(ES) error: %v", err)
	}

	info, err := os.Stat(filepath.Join(configDir, "config.json"))
	if err != nil {
		t.Fatalf("could not stat the written config: %v", err)
	}
	// Windows synthesizes the reported mode from the read-only attribute, so the
	// POSIX bits requested here cannot be observed there.
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("config.json perm = %#o, want %#o", got, 0o644)
		}
	}
}
