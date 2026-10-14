package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
	"github.com/e12jassir/kickoff/internal/tvguide"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "Rewrite HTTPS BoldMSS to HTTP",
			input:       "https://example.com/espn5.m3u8",
			expected:    "http://example.com/espn5.m3u8",
			expectError: false,
		},
		{
			name:        "Keep HTTP BoldMSS as HTTP",
			input:       "http://example.com/espn1.m3u8",
			expected:    "http://example.com/espn1.m3u8",
			expectError: false,
		},
		{
			name:        "Keep HTTPS for external CDNs (Akamai, CloudFront)",
			input:       "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8",
			expected:    "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8",
			expectError: false,
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
	cat := New()

	tests := []struct {
		name             string
		match            model.Match
		expectedChannels []string
		expectedProfiles []string
		mustContainURL   string
		mustNotContain   string
	}{
		{
			name: "Champions: Barcelona vs Galatasaray isolated exclusively to ESPN 5 HD (720p & 1080p)",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "FC Barcelona vs Galatasaray",
				HomeTeam:  model.Competitor{DisplayName: "FC Barcelona"},
				AwayTeam:  model.Competitor{DisplayName: "Galatasaray"},
			},
			expectedChannels: []string{"ESPN 5 HD", "ESPN 5 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_5.m3u8",
			mustNotContain:   "ESPN_2",
		},
		{
			name: "Champions: Bayern vs Viking isolated to ESPN 1 HD (720p & 1080p)",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Bayern Munich vs Viking FK",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
		},
		{
			name: "La Liga: Real Madrid returns standard competitive broadcast without Real Madrid TV",
			match: model.Match{
				LeagueKey: "laliga",
				Title:     "Real Madrid vs Valencia",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
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
			mustContainURL:   "rmtv.akamaized.net",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "ESPN/ESPN.isml",
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
			mustContainURL:   "tycsports",
		},
		{
			name: "Unknown league fallback returns distinct slot ESPN channel without Fox Sports 1 HD",
			match: model.Match{
				LeagueKey: "unknown-league-key",
				Title:     "Team A vs Team B",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
			mustNotContain:   "FOX_Sports.m3u8",
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
			mustContainURL:   "127.0.0.1",
			mustNotContain:   "ESPN",
		},
		{
			name: "Colombia: legacy synthetic struct returns authentic channels",
			match: model.Match{
				LeagueKey: "colombia",
				Title:     "Junior vs Nacional",
			},
			expectedChannels: []string{"Win+ Fútbol", "Win Sports HD", "Caracol TV HD", "Canal RCN HD"},
			expectedProfiles: []string{"1080p", "1080p", "1080p", "1080p"},
			mustContainURL:   "127.0.0.1",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "CARACOL",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "RCN",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "WIN-SPORT",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "FOX_Sports.m3u8",
			mustNotContain:   "ESPN",
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
			mustContainURL:   "FOX_SPORTS_2",
			mustNotContain:   "ESPN",
		},
		{
			name: "Champions: LASK Linz vs Liverpool returns ESPN 1 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "LASK Linz vs Liverpool",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
			mustNotContain:   "ESPN_2",
		},
		{
			name: "Champions: Feyenoord vs Como returns ESPN 2 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Feyenoord vs Como",
			},
			expectedChannels: []string{"ESPN 2 HD", "ESPN 2 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_2.m3u8",
			mustNotContain:   "ESPN/ESPN.isml",
		},
		{
			name: "Champions: AS Roma vs Real Madrid returns ESPN 1 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "AS Roma vs Real Madrid",
			},
			expectedChannels: []string{"ESPN 1 HD", "ESPN 1 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
		},
		{
			name: "Champions: Manchester City vs Paris Saint-Germain returns ESPN 2 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Manchester City vs Paris Saint-Germain",
			},
			expectedChannels: []string{"ESPN 2 HD", "ESPN 2 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_2.m3u8",
		},
		{
			name: "Champions: Aston Villa vs Fenerbahçe returns ESPN 3 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Aston Villa vs Fenerbahçe",
			},
			expectedChannels: []string{"ESPN 3 HD", "ESPN 3 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_3.m3u8",
		},
		{
			name: "Champions: Borussia Dortmund vs Bodo/Glimt returns ESPN 5 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Borussia Dortmund vs Bodo/Glimt",
			},
			expectedChannels: []string{"ESPN 5 HD", "ESPN 5 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_5.m3u8",
		},
		{
			name: "Champions: Real Betis vs FC Porto returns ESPN 7 HD",
			match: model.Match{
				LeagueKey: "champions",
				Title:     "Real Betis vs FC Porto",
			},
			expectedChannels: []string{"ESPN 7 HD", "ESPN 7 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_7.m3u8",
		},
		{
			name: "Official broadcasts: CBSSN/Paramount falls through to verified LatAm ESPN channel when no US stream configured",
			match: model.Match{
				ID:         "live-cbs-fallback",
				LeagueKey:  "champions",
				Title:      "Borussia Dortmund vs Bodo/Glimt",
				Broadcasts: []string{"CBSSN", "Paramount+"},
			},
			expectedChannels: []string{"ESPN 5 HD", "ESPN 5 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN_5.m3u8",
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
			mustContainURL:   "ESPN/ESPN.isml",
			mustNotContain:   "FOX_Sports.m3u8",
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
			mustContainURL:   "ESPN_2.m3u8",
			mustNotContain:   "FOX_Sports.m3u8",
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
				// Verify URL sanitization: should never start with https://stream.example.com
				if strings.HasPrefix(ch.URL, "https://stream.example.com") {
					t.Errorf("channel URL was not sanitized: %s", ch.URL)
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
					t.Errorf("expected at least one channel URL to contain %s", tt.mustContainURL)
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

	StreamCBSSportsNetwork = "http://example.com/cbssn.m3u8"
	StreamCBSGolazo = "http://example.com/golazo.m3u8"

	cat := New()
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

func TestDiagnostic_ChampionsFixtures(t *testing.T) {
	cat := New()

	todayFixtures := []struct {
		title           string
		timeStr         string
		expectedChannel string
	}{
		{"LASK Linz vs Liverpool", "11:45", "ESPN 1 HD"},
		{"Feyenoord Rotterdam vs Como", "11:45", "ESPN 2 HD"},
		{"AS Roma vs Real Madrid", "14:00", "ESPN 1 HD"},
		{"Manchester City vs Paris Saint-Germain", "14:00", "ESPN 2 HD"},
		{"Aston Villa vs Fenerbahçe", "14:00", "ESPN 3 HD"},
		{"Borussia Dortmund vs Bodo/Glimt", "14:00", "ESPN 5 HD"},
		{"Real Betis vs FC Porto", "14:00", "ESPN 7 HD"},
	}

	for _, fix := range todayFixtures {
		m := model.Match{
			LeagueKey: "champions",
			Title:     fix.title,
			TimeStr:   fix.timeStr,
		}
		chs := cat.GetChannels(m)
		if len(chs) == 0 {
			t.Fatalf("[%s] expected channels, got none", fix.title)
		}
		if chs[0].Name != fix.expectedChannel {
			t.Errorf("[%s] expected primary channel %s, got %s", fix.title, fix.expectedChannel, chs[0].Name)
		}
		t.Logf("✓ %s (%s) -> %s (OK)", fix.title, fix.timeStr, chs[0].Name)
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

	cat := New(WithTVGuide(guideSvc))

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
			StreamURL: tvguide.StreamESPN3,
			Programs:  []string{"Atalanta vs. Arsenal"},
		},
		{
			Name:      "Win+ Fútbol",
			Slug:      "win_sports_plus",
			StreamURL: tvguide.StreamWinPlus,
			Programs:  []string{"Deportivo Pereira vs. Junior"},
		},
	})

	cat := New(WithTVGuide(guideSvc))

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
		if !strings.Contains(channels[0].URL, "ESPN_3") {
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

