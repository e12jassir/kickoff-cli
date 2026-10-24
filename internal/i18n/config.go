package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/e12jassir/kickoff-cli/internal/fsutil"
)

const (
	// configDirName is the directory under the user config root that holds the
	// Kickoff user files.
	configDirName = "kickoff"

	// configFileName is the user configuration file that stores the chosen
	// language. It sits at precedence layer 2, below KICKOFF_LANG, and reading it
	// must never crash: a missing or malformed file is simply ignored.
	configFileName = "config.json"
)

// userConfig is the on-disk shape of config.json. Language is the only user
// setting today; a second key would be a Fase 3 concern.
type userConfig struct {
	Lang string `json:"lang"`
}

// ConfigPath returns the path of the user configuration file.
//
// It resolves through os.UserConfigDir, which honours XDG_CONFIG_HOME on Linux,
// $HOME/Library/Application Support on macOS and %AppData% on Windows, and falls
// back to ~/.config on Linux containers where the variable is unset.
func ConfigPath() (string, error) {
	if base, err := os.UserConfigDir(); err == nil && base != "" {
		return filepath.Join(base, configDirName, configFileName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine the user config directory: %w", err)
	}
	return filepath.Join(home, ".config", configDirName, configFileName), nil
}

// WriteConfigLanguage stores l in config.json.
//
// The write goes through fsutil.AtomicWriteFile so readers observe either the
// previous file or the complete new one, and so the parent directory is created
// on first use. An unsupported language is rejected before touching the disk.
func WriteConfigLanguage(l Lang) error {
	if l != ES && l != EN {
		return fmt.Errorf("kickoff: unsupported language %q", string(l))
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.Marshal(userConfig{Lang: string(l)})
	if err != nil {
		return fmt.Errorf("could not encode %s: %w", configFileName, err)
	}
	data = append(data, '\n')

	if err := fsutil.AtomicWriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	SetLanguage(l)
	return nil
}

// readConfigLanguage returns the raw lang value stored in config.json.
//
// A missing, unreadable or malformed file reports ok == false so the caller keeps
// walking the precedence chain: the resolver never invents a language from a file
// it could not understand.
func readConfigLanguage() (string, bool) {
	path, err := ConfigPath()
	if err != nil {
		return "", false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}

	var cfg userConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", false
	}

	raw := strings.TrimSpace(cfg.Lang)
	if raw == "" {
		return "", false
	}
	return raw, true
}
