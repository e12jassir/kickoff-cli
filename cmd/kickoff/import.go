package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/catalog"
)

const LegalDisclaimer = `Notice: Kickoff does not host, provide, or distribute media streams.
You are solely responsible for ensuring you have the legal right to access
and stream content from the configured sources.`

func backupExistingConfig(targetFile string) error {
	if _, err := os.Stat(targetFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	bakFile := targetFile + ".bak"
	data, err := os.ReadFile(targetFile)
	if err != nil {
		return fmt.Errorf("error reading existing config for backup: %w", err)
	}
	if err := os.WriteFile(bakFile, data, 0o644); err != nil {
		return fmt.Errorf("error creating backup file %s: %w", bakFile, err)
	}
	return nil
}

func readSourceContent(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, fmt.Errorf("error fetching %s: %w", source, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP request failed with status code %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}
		return data, nil
	}

	path := source
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read local file %s: %w", path, err)
	}
	return data, nil
}

// RunConfigImport imports a channels.json configuration or M3U playlist from a local path or HTTP URL.
func RunConfigImport(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	fmt.Println(LegalDisclaimer)
	fmt.Println()

	data, err := readSourceContent(source)
	if err != nil {
		return err
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return fmt.Errorf("source content is empty")
	}

	cfgDir, err := getConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", cfgDir, err)
	}

	channelsPath := filepath.Join(cfgDir, "channels.json")

	isM3U := bytes.HasPrefix(trimmed, []byte("#EXTM3U")) || bytes.Contains(trimmed, []byte("#EXTINF"))
	if isM3U {
		channels, err := catalog.ParseM3U(bytes.NewReader(trimmed))
		if err != nil {
			return fmt.Errorf("error parsing M3U playlist: %w", err)
		}
		if len(channels) == 0 {
			return fmt.Errorf("no valid stream channels found in M3U playlist")
		}

		// Save M3U playlist file
		playlistFile := filepath.Join(cfgDir, "playlist.m3u")
		if err := os.WriteFile(playlistFile, trimmed, 0o644); err != nil {
			return fmt.Errorf("could not save playlist to %s: %w", playlistFile, err)
		}

		// Backup existing channels.json if it exists
		if err := backupExistingConfig(channelsPath); err != nil {
			return err
		}

		// Read existing configuration or create new one
		var ext catalog.ExternalConfig
		if existingData, err := os.ReadFile(channelsPath); err == nil {
			_ = json.Unmarshal(existingData, &ext)
		}
		ext.PlaylistPath = playlistFile

		encoded, err := json.MarshalIndent(ext, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode configuration: %w", err)
		}

		if err := os.WriteFile(channelsPath, encoded, 0o644); err != nil {
			return fmt.Errorf("could not write channels configuration: %w", err)
		}

		fmt.Printf("Successfully imported M3U playlist with %d entries!\n", len(channels))
		fmt.Printf("Playlist saved to: %s\n", playlistFile)
		fmt.Printf("Configuration saved to: %s\n", channelsPath)
		return nil
	}

	// Try JSON configuration
	var ext catalog.ExternalConfig
	if err := json.Unmarshal(trimmed, &ext); err != nil {
		return fmt.Errorf("invalid configuration format: content is neither valid M3U nor valid JSON (%w)", err)
	}

	// Backup existing channels.json if it exists
	if err := backupExistingConfig(channelsPath); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode configuration: %w", err)
	}

	if err := os.WriteFile(channelsPath, encoded, 0o644); err != nil {
		return fmt.Errorf("could not write channels configuration: %w", err)
	}

	fmt.Printf("Successfully imported configuration with %d configured stream endpoints!\n", len(ext.Streams))
	fmt.Printf("Configuration saved to: %s\n", channelsPath)
	return nil
}
