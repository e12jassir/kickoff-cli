package catalog

import (
	"strings"
	"testing"

	"github.com/e12jassir/kickoff/internal/model"
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
			expectedChannels: []string{"ESPN 5 HD (720p)", "ESPN 5 HD (1080p)"},
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
			expectedChannels: []string{"ESPN 1 HD (720p)", "ESPN 1 HD (1080p)"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "ESPN/ESPN.isml",
		},
		{
			name: "La Liga: Real Madrid returns ESPN 1 HD and Real Madrid TV",
			match: model.Match{
				LeagueKey: "laliga",
				Title:     "Real Madrid vs Valencia",
			},
			expectedChannels: []string{"ESPN 1 HD (720p)", "Real Madrid TV"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "rmtv.akamaized.net",
		},
		{
			name: "Premier League: returns ESPN 1 HD (720p & 1080p)",
			match: model.Match{
				LeagueKey: "premier",
				Title:     "Chelsea vs Liverpool",
			},
			expectedChannels: []string{"ESPN 1 HD (720p)", "ESPN 1 HD (1080p)"},
			expectedProfiles: []string{"720p", "1080p"},
		},
		{
			name: "Argentina: returns TyC Sports and ESPN 1 HD",
			match: model.Match{
				LeagueKey: "argentina",
				Title:     "Boca Juniors vs River Plate",
			},
			expectedChannels: []string{"TyC Sports", "ESPN 1 HD (720p)"},
			expectedProfiles: []string{"1080p", "720p"},
			mustContainURL:   "tycsports",
		},
		{
			name: "Unknown league fallback returns ESPN 1 HD and Fox Sports 1",
			match: model.Match{
				LeagueKey: "unknown-league-key",
				Title:     "Team A vs Team B",
			},
			expectedChannels: []string{"ESPN 1 HD (720p)", "Fox Sports 1"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "FOX_Sports.m3u8",
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
