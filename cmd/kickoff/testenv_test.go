package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateConfigHome points every platform's user-config variable at a fresh
// temp directory and returns both that temp root and the kickoff config
// directory production code will resolve.
//
// os.UserConfigDir honours XDG_CONFIG_HOME on Linux, $HOME/Library/Application
// Support on macOS and %AppData% on Windows. Setting only XDG_CONFIG_HOME (a
// Linux convention) made the macOS and Windows CI legs read and write the real
// user profile, so the tests now set all of them and then ask the same function
// the production code uses where it actually landed.
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
