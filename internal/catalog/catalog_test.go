package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

// mockTestStreams provides dummy endpoints for testing without distributing copyrighted URLs.
var mockTestStreams = map[string]string{
	"espn1":       "http://example.invalid/espn1.m3u8",
	"espn2":       "http://example.invalid/espn2.m3u8",
	"espn3":       "http://example.invalid/espn3.m3u8",
	"espn4":       "http://example.invalid/espn4.m3u8",
	"espn5":       "http://example.invalid/espn5.m3u8",
	"espn6":       "http://example.invalid/espn6.m3u8",
	"espn7":       "http://example.invalid/espn7.m3u8",
	"espn_extra":  "http://example.invalid/espnextra.m3u8",
	"fox1":        "http://example.invalid/fox1.m3u8",
	"fox2":        "http://example.invalid/fox2.m3u8",
	"fox3":        "http://example.invalid/fox3.m3u8",
	"tyc":         "http://example.invalid/tyc.m3u8",
	"win":         "http://example.invalid/win.m3u8",
	"win_plus":    "http://example.invalid/winplus.m3u8",
	"caracol":     "http://example.invalid/caracol.m3u8",
	"caracol_hd2": "http://example.invalid/caracolhd2.m3u8",
	"rcn":         "http://example.invalid/rcn.m3u8",
	"rcn_hd2":     "http://example.invalid/rcnhd2.m3u8",
	"realmadrid":  "https://example.invalid/rmtv/master.m3u8",
	"teledeporte": "https://example.invalid/tdp/tdp_main.m3u8",
}

func newTestCatalog(opts ...Option) *Catalog {
	allOpts := append([]Option{WithConfigFile(filepath.Join(os.TempDir(), "nonexistent_kickoff_channels.json")), WithStreams(mockTestStreams)}, opts...)
	return New(allOpts...)
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid HTTPS stream URL",
			input:       "https://example.invalid/live/stream.m3u8",
			expected:    "https://example.invalid/live/stream.m3u8",
			expectError: false,
		},
		{
			name:        "Valid HTTP stream URL",
			input:       "http://example.invalid/stream.m3u8",
			expected:    "http://example.invalid/stream.m3u8",
			expectError: false,
		},
		{
			name:        "Embedded credentials in URL returns error",
			input:       "https://user:pass@example.invalid/live/stream.m3u8",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Empty URL returns error",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Unsupported scheme returns error",
			input:       "file:///etc/passwd",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Missing host returns error",
			input:       "http://",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := SanitizeURL(tt.input)
			if tt.expectError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, res)
			}
		})
	}
}

func TestCatalog_GetChannels(t *testing.T) {
	cat := newTestCatalog()

	tests := []struct {
		name             string
		match            model.Match
		expectedChannels []string
		expectedProfiles []string
		mustContainURL   string
		mustNotContain   string
	}{
		{
			name: "La Liga: Real Madrid returns standard competitive broadcast without Real Madrid TV",
			match: model.Match{
				LeagueKey: "laliga",
				Title:     "Real Madrid vs Valencia",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn1.m3u8",
			mustNotContain:   "rmtv",
		},
		{
			name: "Club Friendly: Real Madrid TV resolved only when explicitly in official broadcasts",
			match: model.Match{
				ID:         "friendly-rm-1",
				LeagueKey:  "friendly",
				Title:      "Real Madrid vs AC Milan",
				Broadcasts: []string{"Real Madrid TV"},
			},
			expectedChannels: []string{"Real Madrid TV"},
			expectedProfiles: []string{"1080p"},
			mustContainURL:   "example.invalid",
			mustNotContain:   "espn",
		},
		{
			name: "La Liga: Real Madrid live match does not inject Real Madrid TV without official broadcast",
			match: model.Match{
				ID:        "live-laliga-rm",
				LeagueKey: "laliga",
				Title:     "Real Madrid vs Sevilla",
				SlotIndex: 0,
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn1.m3u8",
			mustNotContain:   "rmtv",
		},
		{
			name: "Premier League: returns ESPN 1 HD (720p & 1080p)",
			match: model.Match{
				LeagueKey: "premier",
				Title:     "Chelsea vs Liverpool",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
		},
		{
			name: "Argentina: returns TyC Sports HD and ESPN 1 HD",
			match: model.Match{
				LeagueKey: "argentina",
				Title:     "Boca Juniors vs River Plate",
			},
			expectedChannels: []string{"TyC Sports HD", "ESPN 1 HD"},
			expectedProfiles: []string{"1080p", "720p"},
			mustContainURL:   "tyc.m3u8",
		},
		{
			name: "Unknown league fallback returns distinct slot ESPN channel without Fox Sports 1 HD",
			match: model.Match{
				LeagueKey: "unknown-league-key",
				Title:     "Team A vs Team B",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn1.m3u8",
			mustNotContain:   "fox1.m3u8",
		},
		{
			name: "Colombia: returns authentic Colombian channels and never falls back to generic ESPN",
			match: model.Match{
				ID:        "col-match-1",
				LeagueKey: "colombia",
				Title:     "Millonarios vs Santa Fe",
			},
			expectedChannels: []string{"Win+ Fútbol", "Win Sports HD", "Caracol TV HD", "Canal RCN HD"},
			expectedProfiles: []string{"1080p", "1080p", "1080p", "1080p"},
			mustContainURL:   "winplus.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Colombia: legacy synthetic struct returns authentic channels",
			match: model.Match{
				LeagueKey: "colombia",
				Title:     "Junior vs Nacional",
			},
			expectedChannels: []string{"Win+ Fútbol", "Win Sports HD", "Caracol TV HD", "Canal RCN HD"},
			expectedProfiles: []string{"1080p", "1080p", "1080p", "1080p"},
			mustContainURL:   "winplus.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Caracol resolves Caracol TV HD & Caracol HD2",
			match: model.Match{
				ID:         "live-caracol-1",
				LeagueKey:  "international",
				Title:      "Colombia vs Brasil",
				Broadcasts: []string{"Gol Caracol"},
			},
			expectedChannels: []string{"Caracol TV HD", "Caracol HD2"},
			expectedProfiles: []string{"1080p", "1080p"},
			mustContainURL:   "caracol.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: RCN resolves Canal RCN HD & RCN HD2",
			match: model.Match{
				ID:         "live-rcn-1",
				LeagueKey:  "international",
				Title:      "Colombia vs Argentina",
				Broadcasts: []string{"Deportes RCN"},
			},
			expectedChannels: []string{"Canal RCN HD", "RCN HD2"},
			expectedProfiles: []string{"1080p", "1080p"},
			mustContainURL:   "rcn.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Win resolves Win+ Fútbol & Win Sports HD",
			match: model.Match{
				ID:         "live-win-1",
				LeagueKey:  "colombia",
				Title:      "America de Cali vs Deportivo Cali",
				Broadcasts: []string{"Win Sports+"},
			},
			expectedChannels: []string{"Win+ Fútbol", "Win Sports HD"},
			expectedProfiles: []string{"1080p", "1080p"},
			mustContainURL:   "win.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Fox Sports 1 verified from FS1 metadata without duplicate guessing",
			match: model.Match{
				ID:         "live-fox-1",
				LeagueKey:  "champions",
				Title:      "Aston Villa vs Fenerbahce",
				Broadcasts: []string{"FS1"},
			},
			expectedChannels: []string{"Fox Sports 1 HD"},
			expectedProfiles: []string{"1080p"},
			mustContainURL:   "fox1.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Fox Sports 2 verified resolution",
			match: model.Match{
				ID:         "live-fox-2",
				LeagueKey:  "bundesliga",
				Title:      "Bayern Munich vs Leverkusen",
				Broadcasts: []string{"Fox Sports 2"},
			},
			expectedChannels: []string{"Fox Sports 2", "Fox Sports 2"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "fox2.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: CBSSN/Paramount falls through to slot ESPN channel when no US stream configured",
			match: model.Match{
				ID:         "live-cbs-fallback",
				LeagueKey:  "champions",
				Title:      "Borussia Dortmund vs Bodo/Glimt",
				Broadcasts: []string{"CBSSN", "Paramount+"},
				SlotIndex:  0,
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn1.m3u8",
		},
		{
			name: "Simultaneous matches without broadcasts: Slot 0 maps to ESPN 1 HD without Fox Sports collision",
			match: model.Match{
				ID:        "simul-1",
				LeagueKey: "champions",
				Title:     "AEK Athens vs Shakhtar Donetsk",
				SlotIndex: 0,
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn1.m3u8",
			mustNotContain:   "fox1.m3u8",
		},
		{
			name: "Simultaneous matches without broadcasts: Slot 1 maps to ESPN 2 HD without collision",
			match: model.Match{
				ID:        "simul-2",
				LeagueKey: "champions",
				Title:     "Slovan Bratislava vs VfB Stuttgart",
				SlotIndex: 1,
			},
			expectedChannels: []string{"ESPN 2 HD", "ESPN 2 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn2.m3u8",
			mustNotContain:   "fox1.m3u8",
		},
		{
			name: "Slot exceeds available linear channels returns no channels for honest placeholder",
			match: model.Match{
				ID:        "simul-8",
				LeagueKey: "champions",
				Title:     "Team Overload A vs Team Overload B",
				SlotIndex: 7,
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "Official broadcast not in catalog returns no synthetic channels",
			match: model.Match{
				ID:         "live-dazn-1",
				LeagueKey:  "premier",
				Title:      "Arsenal vs Chelsea",
				Broadcasts: []string{"DAZN 1 Bar HD"},
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channels := cat.GetChannels(tt.match)
			if len(channels) != len(tt.expectedChannels) {
				t.Fatalf("expected %d channels, got %d", len(tt.expectedChannels), len(channels))
			}

			for i, ch := range channels {
				if ch.Name != tt.expectedChannels[i] {
					t.Errorf("channel[%d] expected name %s, got %s", i, tt.expectedChannels[i], ch.Name)
				}
				if ch.Resolution != tt.expectedProfiles[i] {
					t.Errorf("channel[%d] expected resolution %s, got %s", i, tt.expectedProfiles[i], ch.Resolution)
				}
			}

			if tt.mustContainURL != "" {
				found := false
				for _, ch := range channels {
					if strings.Contains(ch.URL, tt.mustContainURL) {
						found = true
						break
					}
				}
				if !found {
					var urls []string
					for _, ch := range channels {
						urls = append(urls, ch.URL)
					}
					t.Errorf("expected at least one channel URL to contain %s, got channels: %v", tt.mustContainURL, urls)
				}
			}

			if tt.mustNotContain != "" {
				for _, ch := range channels {
					if strings.Contains(ch.URL, tt.mustNotContain) {
						t.Errorf("channel URL should not contain %s, got %s", tt.mustNotContain, ch.URL)
					}
				}
			}
		})
	}
}

func TestCatalog_CBSOfficialBroadcasts(t *testing.T) {
	oldCBSSN := StreamCBSSportsNetwork
	oldGolazo := StreamCBSGolazo
	defer func() {
		StreamCBSSportsNetwork = oldCBSSN
		StreamCBSGolazo = oldGolazo
	}()

	StreamCBSSportsNetwork = "http://example.invalid/cbssn.m3u8"
	StreamCBSGolazo = "http://example.invalid/golazo.m3u8"

	cat := newTestCatalog()
	match := model.Match{
		ID:         "live-cbs-configured",
		LeagueKey:  "champions",
		Title:      "Borussia Dortmund vs Bodo/Glimt",
		Broadcasts: []string{"CBSSN", "Paramount+"},
	}
	channels := cat.GetChannels(match)
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].Name != "CBS Sports Network" {
		t.Errorf("expected channel[0] CBS Sports Network, got %s", channels[0].Name)
	}
	if channels[1].Name != "CBS Sports Golazo Network" {
		t.Errorf("expected channel[1] CBS Sports Golazo Network, got %s", channels[1].Name)
	}
}

func TestDiagnostic_LiveTVGuideResolution(t *testing.T) {
	guideSvc := tvguide.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = guideSvc.Refresh(ctx)
	guides := guideSvc.Guides()

	t.Logf("=== LIVE TV GUIDE DIAGNOSTIC ===")
	t.Logf("Channels crawled: %d", len(guides))
	for _, g := range guides {
		t.Logf(" • %s (%s): %d programs scheduled", g.Name, g.Slug, len(g.Programs))
	}

	cat := newTestCatalog(WithTVGuide(guideSvc))

	sampleMatches := []model.Match{
		{
			LeagueKey: "champions",
			Title:     "LASK Linz vs Liverpool",
			HomeTeam:  model.Competitor{DisplayName: "LASK Linz"},
			AwayTeam:  model.Competitor{DisplayName: "Liverpool"},
		},
		{
			LeagueKey: "champions",
			Title:     "Feyenoord Rotterdam vs Como",
			HomeTeam:  model.Competitor{DisplayName: "Feyenoord Rotterdam"},
			AwayTeam:  model.Competitor{DisplayName: "Como"},
		},
		{
			LeagueKey: "champions",
			Title:     "AS Roma vs Real Madrid",
			HomeTeam:  model.Competitor{DisplayName: "AS Roma"},
			AwayTeam:  model.Competitor{DisplayName: "Real Madrid"},
		},
	}

	for _, m := range sampleMatches {
		chs := cat.GetChannels(m)
		if len(chs) == 0 {
			t.Errorf("[%s] expected channels, got none", m.Title)
			continue
		}
		t.Logf("✓ %s -> Primary: %s (%s, Rec: %v), Total: %d signals",
			m.Title, chs[0].Name, chs[0].Resolution, chs[0].Recommended, len(chs))
	}
}

func TestCatalog_TVGuideIntegration(t *testing.T) {
	guideSvc := tvguide.NewService()
	guideSvc.SetGuides([]tvguide.ChannelGuide{
		{
			Name:      "ESPN 3 HD",
			Slug:      "espn_3_colombia",
			StreamURL: "http://example.invalid/espn3.m3u8",
			Programs:  []string{"Atalanta vs. Arsenal"},
		},
		{
			Name:      "Win+ Fútbol",
			Slug:      "win_sports_plus",
			StreamURL: "http://example.invalid/winplus.m3u8",
			Programs:  []string{"Deportivo Pereira vs. Junior"},
		},
	})

	cat := newTestCatalog(WithTVGuide(guideSvc))

	t.Run("TV Guide resolves match without ESPN broadcasts and sets Recommended: true", func(t *testing.T) {
		match := model.Match{
			LeagueKey: "champions",
			Title:     "Atalanta vs Arsenal",
			HomeTeam:  model.Competitor{DisplayName: "Atalanta"},
			AwayTeam:  model.Competitor{DisplayName: "Arsenal"},
		}
		channels := cat.GetChannels(match)
		if len(channels) == 0 {
			t.Fatalf("expected channels from tvguide, got none")
		}
		if channels[0].Name != "ESPN 3 HD" {
			t.Errorf("expected primary channel ESPN 3 HD, got %s", channels[0].Name)
		}
		if !channels[0].Recommended {
			t.Errorf("expected top channel to have Recommended: true")
		}
		if !strings.Contains(channels[0].URL, "espn3") {
			t.Errorf("expected ESPN 3 stream URL, got %s", channels[0].URL)
		}
	})

	t.Run("Official broadcasts take precedence over TV Guide", func(t *testing.T) {
		match := model.Match{
			LeagueKey:  "champions",
			Title:      "Atalanta vs Arsenal",
			HomeTeam:   model.Competitor{DisplayName: "Atalanta"},
			AwayTeam:   model.Competitor{DisplayName: "Arsenal"},
			Broadcasts: []string{"ESPN 4"},
		}
		channels := cat.GetChannels(match)
		if len(channels) == 0 {
			t.Fatalf("expected channels from official broadcasts, got none")
		}
		if channels[0].Name != "ESPN 4 HD" {
			t.Errorf("expected official broadcast ESPN 4 HD to take precedence over ESPN 3 HD, got %s", channels[0].Name)
		}
	})

	t.Run("TV Guide resolves Colombian league match", func(t *testing.T) {
		match := model.Match{
			LeagueKey: "colombia",
			Title:     "Deportivo Pereira vs Junior FC",
			HomeTeam:  model.Competitor{DisplayName: "Deportivo Pereira"},
			AwayTeam:  model.Competitor{DisplayName: "Junior FC"},
		}
		channels := cat.GetChannels(match)
		if len(channels) == 0 {
			t.Fatalf("expected channels from tvguide, got none")
		}
		if channels[0].Name != "Win+ Fútbol" {
			t.Errorf("expected Win+ Fútbol, got %s", channels[0].Name)
		}
		if !channels[0].Recommended {
			t.Errorf("expected recommended true on top channel")
		}
	})
}

func TestCatalog_ExternalConfig_LeagueAliases(t *testing.T) {
	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "channels.json")

	cfgContent := `{
		"league_channels": {
			"uefa.champions": [
				{"name": "Champions Custom", "url": "https://example.invalid/champions.m3u8", "resolution": "1080p"}
			],
			"eng.1": [
				{"name": "Premier Custom", "url": "https://example.invalid/premier.m3u8", "resolution": "1080p"}
			],
			"laliga": [
				{"name": "LaLiga Custom", "url": "https://example.invalid/laliga.m3u8", "resolution": "1080p"}
			],
			"col.1": [
				{"name": "Colombia Custom", "url": "https://example.invalid/colombia.m3u8", "resolution": "1080p"}
			]
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cat := New(WithConfigFile(cfgPath))

	testCases := []struct {
		leagueKey    string
		expectedName string
	}{
		{"champions", "Champions Custom"},
		{"uefa.champions", "Champions Custom"},
		{"premier", "Premier Custom"},
		{"eng.1", "Premier Custom"},
		{"laliga", "LaLiga Custom"},
		{"esp.1", "LaLiga Custom"},
		{"colombia", "Colombia Custom"},
		{"col.1", "Colombia Custom"},
	}

	for _, tc := range testCases {
		t.Run(tc.leagueKey, func(t *testing.T) {
			m := model.Match{
				LeagueKey: tc.leagueKey,
				Title:     "Team A vs Team B",
			}
			chs := cat.GetChannels(m)
			if len(chs) == 0 {
				t.Fatalf("expected channels for %s, got none", tc.leagueKey)
			}
			if chs[0].Name != tc.expectedName {
				t.Errorf("[%s] expected channel %s, got %s", tc.leagueKey, tc.expectedName, chs[0].Name)
			}
		})
	}
}

// TestNew_AppliesOptionsExactlyOnce guards against New applying opts twice, which
// duplicated every injected M3U channel via WithM3UChannels' append.
func TestNew_AppliesOptionsExactlyOnce(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "channels.json")
	cfgContent := `{"streams": {"espn1": "http://config.invalid/espn1.m3u8"}}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	injected := []M3UChannel{
		{Name: "Injected A", URL: "http://example.invalid/injected-a.m3u8"},
		{Name: "Injected B", URL: "http://example.invalid/injected-b.m3u8"},
	}

	cat := New(
		WithConfigFile(cfgPath),
		WithStreams(map[string]string{"espn1": "http://programmatic.invalid/espn1.m3u8"}),
		WithM3UChannels(injected),
	)

	cat.mu.RLock()
	storedChannels := len(cat.m3uChannels)
	espn1 := cat.userStreams["espn1"]
	cat.mu.RUnlock()

	if storedChannels != len(injected) {
		t.Fatalf("WithM3UChannels must be applied exactly once: expected %d stored channels, got %d", len(injected), storedChannels)
	}
	if espn1 != "http://programmatic.invalid/espn1.m3u8" {
		t.Fatalf("programmatic streams must take precedence over channels.json: got %q", espn1)
	}
}

func TestCatalog_ExternalConfig_PlaylistURL(t *testing.T) {
	mockM3U := `#EXTM3U
#EXTINF:-1 tvg-id="remote-espn1" tvg-name="Remote ESPN 1",Remote ESPN 1 HD
http://example.com/remote/espn1.m3u8
#EXTINF:-1 tvg-id="remote-win" tvg-name="Remote Win",Remote Win Sports HD
http://example.com/remote/win.m3u8
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockM3U))
	}))
	defer ts.Close()

	cfgPath := filepath.Join(t.TempDir(), "channels.json")
	cfgContent := fmt.Sprintf(`{"playlist_url": "%s"}`, ts.URL+"/playlist.m3u")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cat := New(WithConfigFile(cfgPath))

	cat.mu.RLock()
	defer cat.mu.RUnlock()

	if len(cat.m3uChannels) != 2 {
		t.Fatalf("expected 2 remote channels loaded from playlist_url, got %d", len(cat.m3uChannels))
	}
	if cat.m3uChannels[0].Name != "Remote ESPN 1 HD" {
		t.Errorf("expected first channel name 'Remote ESPN 1 HD', got %q", cat.m3uChannels[0].Name)
	}
	if cat.m3uChannels[1].Name != "Remote Win Sports HD" {
		t.Errorf("expected second channel name 'Remote Win Sports HD', got %q", cat.m3uChannels[1].Name)
	}
}

func TestCatalog_ExternalConfig_PlaylistPathPrecedence(t *testing.T) {
	// If both PlaylistPath and PlaylistURL are provided, PlaylistPath wins.
	localM3U := filepath.Join(t.TempDir(), "local.m3u")
	localContent := `#EXTM3U
#EXTINF:-1,Local Channel
http://example.com/local.m3u8
`
	if err := os.WriteFile(localM3U, []byte(localContent), 0o600); err != nil {
		t.Fatalf("failed to write local m3u: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("remote URL should not be fetched when local path is set")
	}))
	defer ts.Close()

	cfgPath := filepath.Join(t.TempDir(), "channels.json")
	cfgContent := fmt.Sprintf(`{"playlist_path": "%s", "playlist_url": "%s"}`, localM3U, ts.URL+"/remote.m3u")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cat := New(WithConfigFile(cfgPath))

	cat.mu.RLock()
	defer cat.mu.RUnlock()

	if len(cat.m3uChannels) != 1 {
		t.Fatalf("expected exactly 1 channel from local file, got %d", len(cat.m3uChannels))
	}
	if cat.m3uChannels[0].Name != "Local Channel" {
		t.Errorf("expected channel 'Local Channel', got %q", cat.m3uChannels[0].Name)
	}
}

func TestCatalog_ProbeStreams_RejectsHTMLErrorPages(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html-error":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body><h1>404 Not Found</h1></body></html>"))
		case "/generic-ok-no-m3u":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("some random text response"))
		case "/valid-stream":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,Valid\nhttp://example.com/segment.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cat := &Catalog{
		streamAlive: make(map[string]bool),
		userStreams: make(map[string]string),
	}

	probeTargets := []string{
		ts.URL + "/html-error",
		ts.URL + "/generic-ok-no-m3u",
		ts.URL + "/valid-stream",
	}

	cat.probeAllStreamsBackground(probeTargets)

	cat.mu.RLock()
	defer cat.mu.RUnlock()

	if cat.streamAlive[ts.URL+"/html-error"] {
		t.Errorf("HTML error page with 200 OK must NOT be marked alive")
	}
	if cat.streamAlive[ts.URL+"/generic-ok-no-m3u"] {
		t.Errorf("Generic plain text response with 200 OK and no #EXTM3U must NOT be marked alive")
	}
	if !cat.streamAlive[ts.URL+"/valid-stream"] {
		t.Errorf("Valid M3U8 stream must be marked alive")
	}
}
