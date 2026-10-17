package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultChannelsTemplate = `{
  "streams": {
    "espn1": "http://your-iptv-provider.example/stream/espn1.m3u8",
    "espn2": "http://your-iptv-provider.example/stream/espn2.m3u8",
    "espn3": "http://your-iptv-provider.example/stream/espn3.m3u8",
    "espn4": "http://your-iptv-provider.example/stream/espn4.m3u8",
    "espn5": "http://your-iptv-provider.example/stream/espn5.m3u8",
    "espn6": "http://your-iptv-provider.example/stream/espn6.m3u8",
    "espn7": "http://your-iptv-provider.example/stream/espn7.m3u8",
    "dsports": "http://your-iptv-provider.example/stream/dsports.m3u8",
    "dsports2": "http://your-iptv-provider.example/stream/dsports2.m3u8",
    "dsports4": "http://your-iptv-provider.example/stream/dsports4.m3u8",
    "dsports_plus": "http://your-iptv-provider.example/stream/dsports_plus.m3u8",
    "tyc": "http://your-iptv-provider.example/stream/tyc.m3u8",
    "win": "http://your-iptv-provider.example/stream/win.m3u8",
    "win_plus": "http://your-iptv-provider.example/stream/win_plus.m3u8",
    "win_sports_mas": "http://your-iptv-provider.example/stream/win_sports_mas.m3u8",
    "caracol": "http://your-iptv-provider.example/stream/caracol.m3u8",
    "caracol_hd2": "http://your-iptv-provider.example/stream/caracol_hd2.m3u8",
    "gol_caracol": "http://your-iptv-provider.example/stream/gol_caracol.m3u8",
    "rcn": "http://your-iptv-provider.example/stream/rcn.m3u8",
    "rcn_hd2": "http://your-iptv-provider.example/stream/rcn_hd2.m3u8",
    "claro_sports": "http://your-iptv-provider.example/stream/claro_sports.m3u8",
    "realmadrid": "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8",
    "teledeporte": "https://rtvelivestream.akamaized.net/rtvesec/tdp/tdp_main.m3u8"
  },
  "league_channels": {
    "champions": [],
    "premier": []
  },
  "custom_rules": [
    {
      "league": "premier",
      "pattern": "(?i)Arsenal",
      "channels": [
        {
          "displayName": "My Custom Stream",
          "name": "Custom 1080p",
          "url": "http://my-stream.example/live.m3u8",
          "resolution": "1080p",
          "bitrateMode": "max",
          "recommended": true
        }
      ]
    }
  ]
}
`

// getConfigDir determines the Kickoff config directory using os.UserConfigDir(),
// falling back to ~/.config/kickoff on Linux if empty or unavailable.
func getConfigDir() (string, error) {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "kickoff"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "kickoff"), nil
}

// RunInit initializes the channels.json configuration file.
// If channels.json already exists and force is false, it prints a message and exits without overwriting.
// If force is true or the file does not exist, it creates the directory and writes the starter template.
func RunInit(force bool) error {
	cfgDir, err := getConfigDir()
	if err != nil {
		return err
	}

	targetFile := filepath.Join(cfgDir, "channels.json")

	exists := false
	if _, err := os.Stat(targetFile); err == nil {
		exists = true
	}

	if exists && !force {
		fmt.Printf("Configuration file already exists at: %s\nUse 'kickoff init --force' to overwrite.\n", targetFile)
		return nil
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", cfgDir, err)
	}

	if err := os.WriteFile(targetFile, []byte(defaultChannelsTemplate), 0o644); err != nil {
		return fmt.Errorf("could not write file %s: %w", targetFile, err)
	}

	if exists {
		fmt.Printf("File channels.json successfully overwritten at: %s\n", targetFile)
	} else {
		fmt.Printf("File channels.json successfully initialized at: %s\n", targetFile)
	}

	return nil
}
