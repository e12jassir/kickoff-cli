package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestParseM3U_Success(t *testing.T) {
	rawM3U := `#EXTM3U
#EXTINF:-1 tvg-id="espn1.co" tvg-name="ESPN Colombia" tvg-logo="https://example.com/logo.png" group-title="Deportes",ESPN 1 HD
http://example.com/live/espn1.m3u8

#EXTINF:0 group-title='Sports' tvg-id=espn2 tvg-name="ESPN 2",ESPN 2 HD
https://example.com/live/espn2.m3u8

#EXTINF:-1 tvg-name="Win+ Sports" group-title="Fútbol",Win+ Fútbol HD
http://example.com/live/winplus.m3u8

#EXTINF:-1,Caracol TV HD
https://example.com/live/caracol.m3u8
`

	channels, err := ParseM3U(strings.NewReader(rawM3U))
	if err != nil {
		t.Fatalf("unexpected error parsing M3U: %v", err)
	}

	if len(channels) != 4 {
		t.Fatalf("expected 4 channels, got %d", len(channels))
	}

	// Channel 1
	if channels[0].Name != "ESPN 1 HD" {
		t.Errorf("expected Name 'ESPN 1 HD', got %q", channels[0].Name)
	}
	if channels[0].TvgName != "ESPN Colombia" {
		t.Errorf("expected TvgName 'ESPN Colombia', got %q", channels[0].TvgName)
	}
	if channels[0].TvgID != "espn1.co" {
		t.Errorf("expected TvgID 'espn1.co', got %q", channels[0].TvgID)
	}
	if channels[0].Group != "Deportes" {
		t.Errorf("expected Group 'Deportes', got %q", channels[0].Group)
	}
	if channels[0].URL != "http://example.com/live/espn1.m3u8" {
		t.Errorf("expected URL 'http://example.com/live/espn1.m3u8', got %q", channels[0].URL)
	}

	// Channel 2 with single quotes and unquoted attributes
	if channels[1].Name != "ESPN 2 HD" {
		t.Errorf("expected Name 'ESPN 2 HD', got %q", channels[1].Name)
	}
	if channels[1].TvgID != "espn2" {
		t.Errorf("expected TvgID 'espn2', got %q", channels[1].TvgID)
	}
	if channels[1].Group != "Sports" {
		t.Errorf("expected Group 'Sports', got %q", channels[1].Group)
	}

	// Channel 3 with Win+ Fútbol
	if channels[2].Name != "Win+ Fútbol HD" {
		t.Errorf("expected Name 'Win+ Fútbol HD', got %q", channels[2].Name)
	}
	if channels[2].TvgName != "Win+ Sports" {
		t.Errorf("expected TvgName 'Win+ Sports', got %q", channels[2].TvgName)
	}

	// Channel 4 without tvg attributes
	if channels[3].Name != "Caracol TV HD" {
		t.Errorf("expected Name 'Caracol TV HD', got %q", channels[3].Name)
	}
	if channels[3].TvgName != "Caracol TV HD" {
		t.Errorf("expected TvgName fallback to Name, got %q", channels[3].TvgName)
	}
}

func TestParseM3U_WindowsCRLF(t *testing.T) {
	crlfM3U := "#EXTM3U\r\n#EXTINF:-1 tvg-id=\"tdp\" group-title=\"Spain\",Teledeporte\r\nhttps://example.com/live/tdp.m3u8\r\n"
	channels, err := ParseM3U(strings.NewReader(crlfM3U))
	if err != nil {
		t.Fatalf("unexpected error parsing CRLF M3U: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].Name != "Teledeporte" {
		t.Errorf("expected 'Teledeporte', got %q", channels[0].Name)
	}
	if channels[0].URL != "https://example.com/live/tdp.m3u8" {
		t.Errorf("expected URL 'https://example.com/live/tdp.m3u8', got %q", channels[0].URL)
	}
}

func TestParseM3U_MalformedAndInvalidURLs(t *testing.T) {
	malformed := `#EXTM3U
# Comment
#EXTINF:-1,Invalid Scheme
ftp://example.com/stream.m3u8

#EXTINF:-1,Valid Channel
http://example.com/valid.m3u8

#EXTINF:-1 with missing URL

#EXTINF:-1,Credentials In URL
http://user:pass@example.com/creds.m3u8
`
	channels, err := ParseM3U(strings.NewReader(malformed))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 valid channel, got %d", len(channels))
	}
	if channels[0].Name != "Valid Channel" {
		t.Errorf("expected 'Valid Channel', got %q", channels[0].Name)
	}
}

func TestParseM3UFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.m3u")
	content := "#EXTM3U\n#EXTINF:-1,ESPN 1\nhttp://example.com/stream.m3u8\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("error creating temp file: %v", err)
	}

	channels, err := ParseM3UFile(filePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(channels) != 1 || channels[0].Name != "ESPN 1" {
		t.Errorf("expected 1 channel named 'ESPN 1', got %+v", channels)
	}

	// Non-existent file
	_, err = ParseM3UFile(filepath.Join(tmpDir, "missing.m3u"))
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestMatchM3UChannel(t *testing.T) {
	channels := []M3UChannel{
		{Name: "ESPN 1 HD", TvgName: "ESPN 1", TvgID: "espn1", URL: "http://example.com/espn1.m3u8"},
		{Name: "ESPN 2 HD", TvgName: "ESPN 2", TvgID: "espn2", URL: "http://example.com/espn2.m3u8"},
		{Name: "ESPN 3 HD", TvgName: "ESPN 3", TvgID: "espn3", URL: "http://example.com/espn3.m3u8"},
		{Name: "Win Sports+ HD", TvgName: "Win+", TvgID: "winplus", URL: "http://example.com/winplus.m3u8"},
		{Name: "Win Sports HD", TvgName: "Win Sports", TvgID: "win", URL: "http://example.com/win.m3u8"},
		{Name: "Caracol TV HD", TvgName: "Caracol", TvgID: "caracol", URL: "http://example.com/caracol.m3u8"},
		{Name: "Caracol HD2", TvgName: "Caracol HD2", TvgID: "caracol2", URL: "http://example.com/caracol2.m3u8"},
		{Name: "Canal RCN HD", TvgName: "RCN", TvgID: "rcn", URL: "http://example.com/rcn.m3u8"},
		{Name: "Fox Sports 1 HD", TvgName: "FS1", TvgID: "fox1", URL: "http://example.com/fox1.m3u8"},
		{Name: "Fox Sports 2 HD", TvgName: "FS2", TvgID: "fox2", URL: "http://example.com/fox2.m3u8"},
		{Name: "TyC Sports HD", TvgName: "TyC", TvgID: "tyc", URL: "http://example.com/tyc.m3u8"},
		{Name: "Teledeporte HD", TvgName: "TDP", TvgID: "tdp", URL: "http://example.com/tdp.m3u8"},
		{Name: "Real Madrid TV", TvgName: "RMTV", TvgID: "rmtv", URL: "http://example.com/rmtv.m3u8"},
	}

	tests := []struct {
		broadcast string
		expected  string
		found     bool
	}{
		{"ESPN 2", "ESPN 2 HD", true},
		{"ESPN2", "ESPN 2 HD", true},
		{"ESPN 1", "ESPN 1 HD", true},
		{"ESPN", "ESPN 1 HD", true},
		{"Win+ Fútbol", "Win Sports+ HD", true},
		{"Win Sports+", "Win Sports+ HD", true},
		{"Win Sports", "Win Sports HD", true},
		{"Caracol", "Caracol TV HD", true},
		{"Caracol HD", "Caracol TV HD", true},
		{"Caracol HD2", "Caracol HD2", true},
		{"Canal RCN", "Canal RCN HD", true},
		{"Fox Sports 1", "Fox Sports 1 HD", true},
		{"FS1", "Fox Sports 1 HD", true},
		{"Fox Sports 2", "Fox Sports 2 HD", true},
		{"TyC Sports", "TyC Sports HD", true},
		{"Teledeporte", "Teledeporte HD", true},
		{"Real Madrid TV", "Real Madrid TV", true},
		{"NonExistentChannel", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.broadcast, func(t *testing.T) {
			ch, ok := MatchM3UChannel(tt.broadcast, channels)
			if ok != tt.found {
				t.Fatalf("broadcast %q: expected found=%v, got %v (channel=%q)", tt.broadcast, tt.found, ok, ch.Name)
			}
			if tt.found && ch.Name != tt.expected {
				t.Errorf("broadcast %q: expected match %q, got %q", tt.broadcast, tt.expected, ch.Name)
			}
		})
	}
}

func TestCatalog_M3UIntegration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m3uChannels := []M3UChannel{
		{Name: "ESPN 2 720p", TvgName: "ESPN 2", TvgID: "espn2", URL: "http://example.com/live/espn2.m3u8"},
		{Name: "Win+ Fútbol 4K", TvgName: "Win+", TvgID: "winplus", URL: "http://example.com/live/winplus.m3u8"},
		{Name: "Caracol TV HD", TvgName: "Caracol", TvgID: "caracol", URL: "http://example.com/live/caracol.m3u8"},
	}

	// 1. WithM3UChannels Option
	cat := New(WithM3UChannels(m3uChannels))

	match := model.Match{
		Title:      "Liverpool vs Arsenal",
		LeagueKey:  "premier",
		Broadcasts: []string{"ESPN 2"},
	}

	chs := cat.GetChannels(match)
	if len(chs) == 0 {
		t.Fatalf("expected channels from M3U, got none")
	}
	if chs[0].Name != "ESPN 2 720p" {
		t.Errorf("expected channel name 'ESPN 2 720p', got %q", chs[0].Name)
	}
	if chs[0].Resolution != "720p" {
		t.Errorf("expected resolution '720p', got %q", chs[0].Resolution)
	}
	if !chs[0].Recommended {
		t.Errorf("expected Recommended=true for primary channel")
	}

	// 2. File-based PlaylistPath loading
	tmpDir := t.TempDir()
	m3uFile := filepath.Join(tmpDir, "test_playlist.m3u")
	m3uData := "#EXTM3U\n#EXTINF:-1 tvg-id=\"caracol\",Caracol TV HD\nhttp://example.com/caracol.m3u8\n"
	if err := os.WriteFile(m3uFile, []byte(m3uData), 0o644); err != nil {
		t.Fatalf("error writing test playlist: %v", err)
	}

	cfgFile := filepath.Join(tmpDir, "channels.json")
	cfgData := `{"playlist_path": "` + m3uFile + `"}`
	if err := os.WriteFile(cfgFile, []byte(cfgData), 0o644); err != nil {
		t.Fatalf("error writing config file: %v", err)
	}

	fileCat := New(WithConfigFile(cfgFile))
	colMatch := model.Match{
		Title:      "Millonarios vs Santa Fe",
		LeagueKey:  "colombia",
		Broadcasts: []string{"Caracol HD"},
	}
	colChs := fileCat.GetChannels(colMatch)
	if len(colChs) == 0 {
		t.Fatalf("expected channel from file-loaded M3U, got none")
	}
	if colChs[0].Name != "Caracol TV HD" {
		t.Errorf("expected 'Caracol TV HD', got %q", colChs[0].Name)
	}
}
