package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
	"github.com/e12jassir/kickoff/internal/tvguide"
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
	allOpts := append([]Option{WithStreams(mockTestStreams)}, opts...)
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
