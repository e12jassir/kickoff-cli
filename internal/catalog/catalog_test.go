package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/extractor"
	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/parrilla"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

// mockTestStreams provides dummy endpoints for testing without distributing copyrighted URLs.
var mockTestStreams = map[string]string{
	"espn1":          "http://example.invalid/espn1.m3u8",
	"espn2":          "http://example.invalid/espn2.m3u8",
	"espn3":          "http://example.invalid/espn3.m3u8",
	"espn4":          "http://example.invalid/espn4.m3u8",
	"espn5":          "http://example.invalid/espn5.m3u8",
	"espn6":          "http://example.invalid/espn6.m3u8",
	"espn7":          "http://example.invalid/espn7.m3u8",
	"espn_deportes":  "http://example.invalid/espndeportes.m3u8",
	"dsports":        "http://example.invalid/dsports.m3u8",
	"dsports2":       "http://example.invalid/dsports2.m3u8",
	"dsports4":       "http://example.invalid/dsports4.m3u8",
	"dsports_plus":   "http://example.invalid/dsportsplus.m3u8",
	"claro_sports":   "http://example.invalid/clarosports.m3u8",
	"tyc":            "http://example.invalid/tyc.m3u8",
	"win":            "http://example.invalid/win.m3u8",
	"win_plus":       "http://example.invalid/winplus.m3u8",
	"win_sports_mas": "http://example.invalid/winsportsmas.m3u8",
	"caracol":        "http://example.invalid/caracol.m3u8",
	"caracol_hd2":    "http://example.invalid/caracolhd2.m3u8",
	"gol_caracol":    "http://example.invalid/golcaracol.m3u8",
	"rcn":            "http://example.invalid/rcn.m3u8",
	"rcn_hd2":        "http://example.invalid/rcnhd2.m3u8",
	"realmadrid":     "https://example.invalid/rmtv/master.m3u8",
	"teledeporte":    "https://example.invalid/tdp/tdp_main.m3u8",
}

func newTestCatalog(opts ...Option) *Catalog {
	allOpts := append([]Option{
		WithConfigFile(filepath.Join(os.TempDir(), "nonexistent_kickoff_channels.json")),
		WithStreams(mockTestStreams),
		WithParrilla(offlineParrilla{}),
		WithExtractor(nil),
	}, opts...)
	return New(allOpts...)
}

// parrillaFake is a deterministic, offline parrilla fetcher for tests. It records
// how many times Fetch was called so per-day caching can be asserted.
type parrillaFake struct {
	mu    sync.Mutex
	snap  parrilla.Snapshot
	err   error
	calls int
}

func (f *parrillaFake) Fetch(ctx context.Context, date time.Time) (parrilla.Snapshot, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return parrilla.Snapshot{}, f.err
	}
	return f.snap, nil
}

func (f *parrillaFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// offlineParrilla keeps the shared test catalog offline: the parrilla tier then
// degrades to nothing exactly as it does when the Gol Caracol source is
// unreachable, so existing expectations are unaffected.
type offlineParrilla struct{}

func (offlineParrilla) Fetch(ctx context.Context, date time.Time) (parrilla.Snapshot, error) {
	return parrilla.Snapshot{}, errors.New("parrilla offline in tests")
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

// TestCatalog_GetChannels pins the evidence-only contract: channels are resolved
// ONLY from verified evidence (ESPN broadcast metadata for the specific match, or
// a TV guide hit). With no evidence the catalog must return nothing instead of
// guessing.
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
			name: "No evidence: unknown league returns no channels",
			match: model.Match{
				LeagueKey: "unknown-league-key",
				Title:     "Team A vs Team B",
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "No evidence: La Liga match without broadcasts returns no channels",
			match: model.Match{
				LeagueKey: "laliga",
				Title:     "Real Madrid vs Valencia",
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "No evidence: Colombia match without broadcasts returns no channels",
			match: model.Match{
				ID:        "col-match-1",
				LeagueKey: "colombia",
				Title:     "Millonarios vs Santa Fe",
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "No evidence: Argentina match without broadcasts returns no channels",
			match: model.Match{
				LeagueKey: "argentina",
				Title:     "Boca Juniors vs River Plate",
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "No evidence: SlotIndex must never fabricate an ESPN channel",
			match: model.Match{
				ID:        "simul-1",
				LeagueKey: "champions",
				Title:     "AEK Athens vs Shakhtar Donetsk",
				SlotIndex: 1,
			},
			expectedChannels: []string{},
			expectedProfiles: []string{},
		},
		{
			name: "Official broadcasts: ESPN 4 resolves a configured dual profile",
			match: model.Match{
				ID:         "live-espn-4",
				LeagueKey:  "champions",
				Title:      "Atalanta vs Arsenal",
				Broadcasts: []string{"ESPN 4"},
			},
			expectedChannels: []string{"ESPN 4 HD", "ESPN 4 HD"},
			expectedProfiles: []string{"720p", "1080p"},
			mustContainURL:   "espn4.m3u8",
		},
		{
			name: "Official broadcasts: DSports resolves DSports HD",
			match: model.Match{
				ID:         "live-dsports",
				LeagueKey:  "argentina",
				Title:      "Boca Juniors vs River Plate",
				Broadcasts: []string{"DSports"},
			},
			expectedChannels: []string{"DSports HD"},
			expectedProfiles: []string{"1080p"},
			mustContainURL:   "dsports.m3u8",
		},
		{
			name: "Official broadcasts: DSports+ resolves the DSports+ key",
			match: model.Match{
				ID:         "live-dsports-plus",
				LeagueKey:  "libertadores",
				Title:      "Flamengo vs Palmeiras",
				Broadcasts: []string{"DSports+"},
			},
			expectedChannels: []string{"DSports+"},
			expectedProfiles: []string{"1080p"},
			mustContainURL:   "dsportsplus.m3u8",
		},
		{
			name: "Official broadcasts: Claro Sports resolves Claro Sports",
			match: model.Match{
				ID:         "live-claro",
				LeagueKey:  "ligamx",
				Title:      "America vs Chivas",
				Broadcasts: []string{"Claro Sports"},
			},
			expectedChannels: []string{"Claro Sports"},
			expectedProfiles: []string{"1080p"},
			mustContainURL:   "clarosports.m3u8",
		},
		{
			name: "Official broadcasts: Win Sports Más resolves its key plus Win feeds",
			match: model.Match{
				ID:         "live-win-mas",
				LeagueKey:  "colombia",
				Title:      "America de Cali vs Deportivo Cali",
				Broadcasts: []string{"Win Sports Más"},
			},
			expectedChannels: []string{"Win Sports Más", "Win+ Fútbol", "Win Sports HD"},
			expectedProfiles: []string{"1080p", "1080p", "1080p"},
			mustContainURL:   "winsportsmas.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Gol Caracol resolves its key plus Caracol feeds",
			match: model.Match{
				ID:         "live-gol-caracol",
				LeagueKey:  "international",
				Title:      "Colombia vs Brasil",
				Broadcasts: []string{"Gol Caracol"},
			},
			expectedChannels: []string{"Gol Caracol", "Caracol TV HD", "Caracol HD2"},
			expectedProfiles: []string{"1080p", "1080p", "1080p"},
			mustContainURL:   "golcaracol.m3u8",
			mustNotContain:   "espn",
		},
		{
			name: "Official broadcasts: Caracol resolves Caracol TV HD & Caracol HD2",
			match: model.Match{
				ID:         "live-caracol-1",
				LeagueKey:  "international",
				Title:      "Colombia vs Brasil",
				Broadcasts: []string{"Caracol"},
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
			name: "Official broadcasts: Real Madrid TV resolved only when explicitly in broadcasts",
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

			wantPlayable := len(tt.expectedChannels) > 0
			if got := cat.HasPlayableSource(tt.match); got != wantPlayable {
				t.Errorf("HasPlayableSource = %v, want %v", got, wantPlayable)
			}
		})
	}
}

// TestCatalog_ESPNExtraRule pins the 2024-02-15 Latin American rebrand: Fox Sports
// was discontinued and ESPN Extra was renamed to ESPN 4 in Colombia. Legacy
// "ESPN Extra" broadcast metadata must resolve to the espn4 key and must never
// fall through to the generic "espn" needle (espn1). The specific needle also has
// to leave the other ESPN 2..7 rules and the bare "ESPN" rule untouched.
func TestCatalog_ESPNExtraRule(t *testing.T) {
	cat := newTestCatalog()

	tests := []struct {
		name        string
		broadcast   string
		wantName    string
		wantURLPart string
	}{
		{"ESPN Extra maps to ESPN 4 after the rename", "ESPN Extra", "ESPN 4 HD", "espn4.m3u8"},
		{"ESPN Extra needle is case-insensitive", "espn extra", "ESPN 4 HD", "espn4.m3u8"},
		{"bare ESPN still maps to ESPN 1", "ESPN", "ESPN 1 HD", "espn1.m3u8"},
		{"ESPN 2 behaviour unchanged", "ESPN 2", "ESPN 2 HD", "espn2.m3u8"},
		{"ESPN Deportes maps to distinct ESPN Deportes", "ESPN Deportes", "ESPN Deportes", "espndeportes.m3u8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := model.Match{
				ID:         "espn-extra",
				LeagueKey:  "champions",
				Title:      "Team A vs Team B",
				Broadcasts: []string{tt.broadcast},
			}
			chs := cat.GetChannels(match)
			if len(chs) == 0 {
				t.Fatalf("broadcast %q: expected a channel, got none", tt.broadcast)
			}
			if chs[0].Name != tt.wantName {
				t.Errorf("broadcast %q: expected primary channel %q, got %q", tt.broadcast, tt.wantName, chs[0].Name)
			}
			if !strings.Contains(chs[0].URL, tt.wantURLPart) {
				t.Errorf("broadcast %q: expected URL containing %q, got %q", tt.broadcast, tt.wantURLPart, chs[0].URL)
			}
		})
	}
}

// TestCatalog_NoGuessFallback proves the SlotIndex de-duplication counter is not
// treated as broadcast truth: a counter is not evidence, so no channel is emitted.
func TestCatalog_NoGuessFallback(t *testing.T) {
	cat := newTestCatalog()

	for _, slot := range []int{0, 1, 2, 5, 7} {
		match := model.Match{
			ID:        fmt.Sprintf("simul-%d", slot),
			LeagueKey: "champions",
			Title:     "Team A vs Team B",
			SlotIndex: slot,
		}
		if chs := cat.GetChannels(match); len(chs) != 0 {
			t.Fatalf("slot %d: expected no channels without evidence, got %d", slot, len(chs))
		}
		if cat.HasPlayableSource(match) {
			t.Fatalf("slot %d: expected HasPlayableSource=false without evidence", slot)
		}
	}
}

// TestCatalog_PlayabilityGate proves a match aired only on a channel key we have
// no configured URL for yields nothing, and that the same match becomes playable
// once the key is configured.
func TestCatalog_PlayabilityGate(t *testing.T) {
	match := model.Match{
		ID:         "gate-1",
		LeagueKey:  "champions",
		Title:      "Arsenal vs Chelsea",
		Broadcasts: []string{"ESPN 4"},
	}

	// Only espn1 is configured; ESPN 4 is not playable.
	unplayable := New(
		WithConfigFile(filepath.Join(t.TempDir(), "none.json")),
		WithStreams(map[string]string{"espn1": "http://example.invalid/espn1.m3u8"}),
		WithParrilla(offlineParrilla{}),
	)
	if chs := unplayable.GetChannels(match); len(chs) != 0 {
		t.Fatalf("expected no channels for an unconfigured key, got %d", len(chs))
	}
	if unplayable.HasPlayableSource(match) {
		t.Fatalf("expected HasPlayableSource=false when the only key has no configured URL")
	}

	// The same match is playable once ESPN 4 has a configured URL.
	playable := New(
		WithConfigFile(filepath.Join(t.TempDir(), "none.json")),
		WithStreams(map[string]string{"espn4": "http://example.invalid/espn4.m3u8"}),
		WithParrilla(offlineParrilla{}),
	)
	chs := playable.GetChannels(match)
	if len(chs) == 0 {
		t.Fatalf("expected channels for a configured key, got none")
	}
	if !playable.HasPlayableSource(match) {
		t.Fatalf("expected HasPlayableSource=true for a configured key")
	}
}

// TestCatalog_OverridesTakePrecedence proves the user's explicit custom_rules and
// league_channels still win over any catalog evidence.
func TestCatalog_OverridesTakePrecedence(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "channels.json")
	cfgContent := `{
		"league_channels": {
			"champions": [
				{"name": "League Override", "url": "https://example.invalid/override.m3u8", "resolution": "1080p"}
			]
		},
		"custom_rules": [
			{
				"league": "premier",
				"pattern": "(?i)Arsenal",
				"channels": [
					{"name": "Custom Rule Channel", "url": "https://example.invalid/custom.m3u8", "resolution": "1080p"}
				]
			}
		]
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cat := New(WithConfigFile(cfgPath), WithStreams(mockTestStreams), WithParrilla(offlineParrilla{}))

	// A custom rule wins even when ESPN metadata lists a different broadcaster.
	ruleMatch := model.Match{
		LeagueKey:  "premier",
		Title:      "Arsenal vs Chelsea",
		Broadcasts: []string{"ESPN 4"},
	}
	ruleChs := cat.GetChannels(ruleMatch)
	if len(ruleChs) != 1 || ruleChs[0].Name != "Custom Rule Channel" {
		t.Fatalf("expected custom rule channel to win, got %+v", ruleChs)
	}
	if !cat.HasPlayableSource(ruleMatch) {
		t.Errorf("expected custom rule channel to count as a playable source")
	}

	// A league override wins for a league without a matching custom rule.
	leagueMatch := model.Match{
		LeagueKey:  "champions",
		Title:      "Bayern Munich vs Real Madrid",
		Broadcasts: []string{"ESPN 1"},
	}
	leagueChs := cat.GetChannels(leagueMatch)
	if len(leagueChs) != 1 || leagueChs[0].Name != "League Override" {
		t.Fatalf("expected league override to win, got %+v", leagueChs)
	}
	if !cat.HasPlayableSource(leagueMatch) {
		t.Errorf("expected league override channel to count as a playable source")
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

// TestDiagnostic_LiveTVGuideResolution is a live, network-dependent diagnostic.
// It only reports; it never asserts a guess. When no guide is available (offline)
// it skips, because the catalog now correctly returns nothing without evidence.
func TestDiagnostic_LiveTVGuideResolution(t *testing.T) {
	guideSvc := tvguide.NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = guideSvc.Refresh(ctx)
	guides := guideSvc.Guides()
	if len(guides) == 0 {
		t.Skip("TV guide unavailable (offline); live resolution diagnostic skipped")
	}

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
	}

	for _, m := range sampleMatches {
		chs := cat.GetChannels(m)
		t.Logf("• %s -> %d signals resolved from verified guide evidence", m.Title, len(chs))
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

	cat := New(WithConfigFile(cfgPath), WithParrilla(offlineParrilla{}))

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

func TestCatalog_AdjacentChannelsAndM3UFallback(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("could not chdir to tmpDir: %v", err)
	}

	// 1. Write channels.json in working directory
	cfgContent := `{"streams": {"adjacent_test": "http://stream.example/live.m3u8"}}`
	if err := os.WriteFile("channels.json", []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("could not write channels.json: %v", err)
	}

	cat := New()
	endpoint, ok := cat.StreamEndpoint("adjacent_test")
	if !ok || endpoint != "http://stream.example/live.m3u8" {
		t.Errorf("expected adjacent channels.json to be loaded, got ok=%v, endpoint=%s", ok, endpoint)
	}

	// 2. Test adjacent signals.m3u
	tmpDir2 := t.TempDir()
	if err := os.Chdir(tmpDir2); err != nil {
		t.Fatalf("could not chdir to tmpDir2: %v", err)
	}
	m3uContent := "#EXTM3U\n#EXTINF:-1 tvg-name=\"ESPN 1 HD\",ESPN 1 HD\nhttp://example.com/espn1.m3u8\n"
	if err := os.WriteFile("signals.m3u", []byte(m3uContent), 0o600); err != nil {
		t.Fatalf("could not write signals.m3u: %v", err)
	}

	cat2 := New()
	cat2.mu.RLock()
	defer cat2.mu.RUnlock()
	if len(cat2.m3uChannels) == 0 {
		t.Errorf("expected adjacent signals.m3u to be loaded into m3uChannels, got 0")
	}
}

// newParrillaMatch builds a fixed-kickoff fixture for parrilla tests. The kickoff
// is expressed in the display timezone so the local calendar day (and therefore
// the per-day cache key) is stable regardless of the host timezone.
func newParrillaMatch(broadcasts []string) model.Match {
	return model.Match{
		ID:         "parrilla-fixture",
		LeagueKey:  "champions",
		Title:      "Arsenal vs Chelsea",
		HomeTeam:   model.Competitor{DisplayName: "Arsenal"},
		AwayTeam:   model.Competitor{DisplayName: "Chelsea"},
		StartTime:  time.Date(2026, time.October, 17, 14, 0, 0, 0, locale.Location()),
		Broadcasts: broadcasts,
	}
}

// parrillaSnap builds a one-entry snapshot for the fixture returned by
// newParrillaMatch, carrying the given published channel names (nil means the
// entry is still "por definir").
func parrillaSnap(channels []string) parrilla.Snapshot {
	return parrilla.Snapshot{
		Date: "2026-10-17",
		Entries: []parrilla.Entry{{
			Competition: "Premier League",
			HomeTeam:    "Arsenal",
			AwayTeam:    "Chelsea",
			Raw:         "Arsenal vs. Chelsea",
			StartLocal:  "2:00 p.m.",
			StartMins:   14 * 60,
			Channels:    channels,
		}},
	}
}

// TestCatalog_ParrillaTier covers the three core parrilla outcomes: a both-teams
// playable entry is emitted, a "por definir" entry emits nothing, and a listing
// whose channels have no resolvable URL emits nothing.
func TestCatalog_ParrillaTier(t *testing.T) {
	t.Run("both teams with a playable entry emits the dual profile", func(t *testing.T) {
		cat := newTestCatalog(WithParrilla(&parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}))

		match := newParrillaMatch(nil)
		chs := cat.GetChannels(match)
		if len(chs) != 2 {
			t.Fatalf("expected the parrilla entry to emit a dual profile, got %+v", chs)
		}
		for _, ch := range chs {
			if ch.Name != "ESPN 2" {
				t.Fatalf("expected the parrilla entry channel ESPN 2, got %+v", chs)
			}
		}
		if !cat.HasPlayableSource(match) {
			t.Fatalf("expected HasPlayableSource=true for a configured parrilla channel")
		}
	})

	t.Run("empty channel list (por definir) emits nothing", func(t *testing.T) {
		cat := newTestCatalog(WithParrilla(&parrillaFake{snap: parrillaSnap(nil)}))

		match := newParrillaMatch(nil)
		if chs := cat.GetChannels(match); len(chs) != 0 {
			t.Fatalf("expected no channels for a por-definir entry, got %+v", chs)
		}
		if cat.HasPlayableSource(match) {
			t.Fatalf("expected HasPlayableSource=false for a por-definir entry")
		}
	})

	t.Run("unknown and unconfigured streaming channels emit nothing", func(t *testing.T) {
		// "Disney+ Premium" is not a known channel, and "DSports+" is known but
		// its stream key is not configured in this catalog. Neither may resolve.
		cat := New(
			WithConfigFile(filepath.Join(t.TempDir(), "none.json")),
			WithStreams(map[string]string{"espn2": "http://example.invalid/espn2.m3u8"}),
			WithParrilla(&parrillaFake{snap: parrillaSnap([]string{"Disney+ Premium", "DSports+"})}),
		)

		match := newParrillaMatch(nil)
		if chs := cat.GetChannels(match); len(chs) != 0 {
			t.Fatalf("expected no channels when no listed channel has a URL, got %+v", chs)
		}
		if cat.HasPlayableSource(match) {
			t.Fatalf("expected HasPlayableSource=false when no listed channel has a URL")
		}
	})
}

// TestParrillaDay_IsTimezoneIndependent proves the source schedule is always
// interpreted in Colombia, regardless of the user's display timezone. A 19:00
// COT kickoff is already 02:00 of the next day in Madrid, so a display-zone
// interpretation would request the wrong parrilla date.
func TestParrillaDay_IsTimezoneIndependent(t *testing.T) {
	// 2026-10-18T00:00Z is 2026-10-17 19:00 in Colombia and 2026-10-18 02:00 in Madrid.
	kickoff := time.Date(2026, time.October, 18, 0, 0, 0, 0, time.UTC)
	match := model.Match{StartTime: kickoff}

	for _, tz := range []string{"Europe/Madrid", "America/New_York"} {
		t.Run(tz, func(t *testing.T) {
			t.Setenv(locale.EnvVar, tz)

			day := parrillaDay(match)
			if got := day.Format("2006-01-02"); got != "2026-10-17" {
				t.Fatalf("expected the Colombian date 2026-10-17, got %s", got)
			}
			if day.Hour() != 19 || day.Minute() != 0 {
				t.Fatalf("expected 19:00 Colombian wall clock, got %02d:%02d", day.Hour(), day.Minute())
			}
			if got := day.Location().String(); got != "COT" {
				t.Fatalf("expected the source zone COT, got %q", got)
			}
		})
	}
}

// TestParrillaTier_UsesM3UFallback proves the parrilla tier can resolve a
// listed channel from an imported M3U playlist when no stream map entry exists,
// and that the resolved signal is emitted as a dual 720p/min + 1080p/max pair.
func TestParrillaTier_UsesM3UFallback(t *testing.T) {
	const streamURL = "http://example.invalid/espn2.m3u8"

	cat := New(
		WithConfigFile(filepath.Join(t.TempDir(), "none.json")),
		WithParrilla(&parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}),
		WithM3UChannels([]M3UChannel{{Name: "ESPN 2 HD", URL: streamURL}}),
		WithExtractor(nil),
	)

	match := newParrillaMatch(nil)
	match.StartTime = time.Date(2026, time.October, 17, 14, 0, 0, 0, time.FixedZone("COT", -5*3600))

	chs := cat.GetChannels(match)
	if len(chs) != 2 {
		t.Fatalf("expected a dual profile pair from the M3U fallback, got %+v", chs)
	}
	if chs[0].Resolution != "720p" || chs[0].BitrateMode != "min" {
		t.Errorf("expected the first profile to be 720p/min, got %s/%s", chs[0].Resolution, chs[0].BitrateMode)
	}
	if chs[1].Resolution != "1080p" || chs[1].BitrateMode != "max" {
		t.Errorf("expected the second profile to be 1080p/max, got %s/%s", chs[1].Resolution, chs[1].BitrateMode)
	}
	for _, ch := range chs {
		if ch.URL != streamURL {
			t.Errorf("expected the M3U URL %q, got %q", streamURL, ch.URL)
		}
	}
}

// TestParrillaTier_EmitsSecondaryChannels proves the parrilla expansion mirrors
// tier 2 for compound national broadcasters: a primary dual profile plus the
// secondary feed resolved independently.
func TestParrillaTier_EmitsSecondaryChannels(t *testing.T) {
	cat := New(
		WithConfigFile(filepath.Join(t.TempDir(), "none.json")),
		WithStreams(map[string]string{"caracol": "http://example.invalid/caracol.m3u8"}),
		WithM3UChannels([]M3UChannel{{Name: "Caracol HD2", URL: "http://example.invalid/caracolhd2.m3u8"}}),
		WithParrilla(&parrillaFake{snap: parrillaSnap([]string{"Caracol TV"})}),
		WithExtractor(nil),
	)

	match := newParrillaMatch(nil)
	match.StartTime = time.Date(2026, time.October, 17, 14, 0, 0, 0, time.FixedZone("COT", -5*3600))

	chs := cat.GetChannels(match)

	var primary, secondary int
	for _, ch := range chs {
		switch ch.Name {
		case "Caracol TV":
			primary++
		case "Caracol HD2":
			secondary++
		default:
			t.Errorf("unexpected channel %q", ch.Name)
		}
	}
	if primary != 2 {
		t.Errorf("expected two Caracol TV profiles, got %d (%+v)", primary, chs)
	}
	if secondary != 1 {
		t.Errorf("expected one Caracol HD2 secondary channel, got %d (%+v)", secondary, chs)
	}
}

// TestCatalog_ParrillaSnapshotFetchedOncePerDay proves the daily parrilla is
// fetched at most once per local day, even across many resolutions.
func TestCatalog_ParrillaSnapshotFetchedOncePerDay(t *testing.T) {
	fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
	cat := newTestCatalog(WithParrilla(fake))

	match := newParrillaMatch(nil)
	for i := 0; i < 5; i++ {
		if chs := cat.GetChannels(match); len(chs) != 2 {
			t.Fatalf("call %d: expected the dual parrilla profile, got %+v", i, chs)
		}
	}

	// A different fixture on the same local day must reuse the same snapshot.
	other := newParrillaMatch(nil)
	other.ID = "parrilla-fixture-2"
	other.Title = "Liverpool vs Everton"
	other.HomeTeam = model.Competitor{DisplayName: "Liverpool"}
	other.AwayTeam = model.Competitor{DisplayName: "Everton"}
	_ = cat.GetChannels(other)

	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 parrilla fetch for the whole local day, got %d", got)
	}
}

// TestCatalog_ParrillaSnapshotFailureTTL proves a failed parrilla fetch is retried
// after a short failure TTL instead of being cached for the whole day, which would
// leave the user without a grid until midnight.
func TestCatalog_ParrillaSnapshotFailureTTL(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, locale.BogotaLocation())
	fake := &parrillaFake{err: errors.New("source down")}
	cat := newTestCatalog(
		WithParrilla(fake),
		WithNow(func() time.Time { return now }),
	)

	match := newParrillaMatch(nil)
	if chs := cat.GetChannels(match); len(chs) != 0 {
		t.Fatalf("expected no channels from a failed parrilla, got %+v", chs)
	}
	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected 1 fetch, got %d", got)
	}

	// Within the failure TTL the negative result is reused instead of hammering
	// the source on every resolution.
	now = now.Add(parrillaFailureTTL - time.Minute)
	_ = cat.GetChannels(match)
	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected failure cached for %v, got %d fetches", parrillaFailureTTL, got)
	}

	// Past the TTL the source is retried, so a transient outage heals during the
	// same day.
	now = now.Add(2 * time.Minute)
	_ = cat.GetChannels(match)
	if got := fake.callCount(); got != 2 {
		t.Fatalf("expected a retry after the failure TTL, got %d fetches", got)
	}
}

// TestCatalog_ParrillaSnapshotSuccessCachedForDay proves a successful snapshot is
// still reused for the whole source day: the failure TTL must not shorten it.
func TestCatalog_ParrillaSnapshotSuccessCachedForDay(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, locale.BogotaLocation())
	fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
	cat := newTestCatalog(
		WithParrilla(fake),
		WithNow(func() time.Time { return now }),
	)

	match := newParrillaMatch(nil)
	for i := 0; i < 3; i++ {
		if chs := cat.GetChannels(match); len(chs) != 2 {
			t.Fatalf("call %d: expected the dual parrilla profile, got %+v", i, chs)
		}
		now = now.Add(3 * time.Hour)
	}
	if got := fake.callCount(); got != 1 {
		t.Fatalf("expected the successful snapshot to stay cached, got %d fetches", got)
	}
}

// TestCatalog_ParrillaPrecedence pins the tier order: user custom_rules and league
// overrides still win (without even fetching the parrilla), ESPN metadata outranks
// the parrilla, and an unplayable non-US ESPN broadcaster falls through to it.
func TestCatalog_ParrillaPrecedence(t *testing.T) {
	t.Run("custom rule wins and parrilla is never fetched", func(t *testing.T) {
		cfgPath := filepath.Join(t.TempDir(), "channels.json")
		cfgContent := `{
			"custom_rules": [
				{
					"league": "champions",
					"pattern": "(?i)Arsenal",
					"channels": [
						{"name": "Rule Wins", "url": "https://example.invalid/rule.m3u8", "resolution": "1080p"}
					]
				}
			]
		}`
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
		cat := New(WithConfigFile(cfgPath), WithStreams(mockTestStreams), WithParrilla(fake))

		chs := cat.GetChannels(newParrillaMatch(nil))
		if len(chs) != 1 || chs[0].Name != "Rule Wins" {
			t.Fatalf("expected the custom rule channel to win, got %+v", chs)
		}
		if got := fake.callCount(); got != 0 {
			t.Fatalf("expected the parrilla never to be fetched for a user rule, got %d calls", got)
		}
	})

	t.Run("league override wins and parrilla is never fetched", func(t *testing.T) {
		cfgPath := filepath.Join(t.TempDir(), "channels.json")
		cfgContent := `{
			"league_channels": {
				"champions": [
					{"name": "League Override", "url": "https://example.invalid/override.m3u8", "resolution": "1080p"}
				]
			}
		}`
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
		cat := New(WithConfigFile(cfgPath), WithStreams(mockTestStreams), WithParrilla(fake))

		chs := cat.GetChannels(newParrillaMatch(nil))
		if len(chs) != 1 || chs[0].Name != "League Override" {
			t.Fatalf("expected the league override to win, got %+v", chs)
		}
		if got := fake.callCount(); got != 0 {
			t.Fatalf("expected the parrilla never to be fetched for a league override, got %d calls", got)
		}
	})

	t.Run("ESPN metadata outranks parrilla", func(t *testing.T) {
		fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
		cat := newTestCatalog(WithParrilla(fake))

		chs := cat.GetChannels(newParrillaMatch([]string{"ESPN 4"}))
		if len(chs) == 0 {
			t.Fatalf("expected ESPN 4 to resolve from metadata")
		}
		for _, ch := range chs {
			if ch.Name != "ESPN 4 HD" {
				t.Fatalf("expected only ESPN 4 HD, got %s", ch.Name)
			}
		}
		if got := fake.callCount(); got != 0 {
			t.Fatalf("expected the parrilla to be skipped when ESPN metadata resolves, got %d calls", got)
		}
	})

	t.Run("unplayable non-US broadcast falls through to parrilla", func(t *testing.T) {
		fake := &parrillaFake{snap: parrillaSnap([]string{"ESPN 2"})}
		cat := newTestCatalog(WithParrilla(fake))

		chs := cat.GetChannels(newParrillaMatch([]string{"DAZN 1 Bar HD"}))
		if len(chs) == 0 || chs[0].Name != "ESPN 2" {
			t.Fatalf("expected parrilla ESPN 2 when ESPN lists only unplayable DAZN, got %+v", chs)
		}
	})
}

type mockExtractor struct {
	mu          sync.Mutex
	calls       int
	resolveFunc func(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error)
}

func (m *mockExtractor) FetchAgenda(ctx context.Context) ([]extractor.ExtractedEvent, error) {
	return nil, nil
}

func (m *mockExtractor) ResolveStream(ctx context.Context, stream *extractor.ExtractedStream) error {
	return nil
}

func (m *mockExtractor) ProbeStream(ctx context.Context, stream *extractor.ExtractedStream) bool {
	return true
}

func (m *mockExtractor) MatchEvent(match model.Match, events []extractor.ExtractedEvent) (*extractor.ExtractedEvent, int) {
	return nil, 0
}

func (m *mockExtractor) ResolveMatchStreams(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.resolveFunc != nil {
		return m.resolveFunc(ctx, match)
	}
	return nil, nil
}

func (m *mockExtractor) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestCatalog_Tier5_DynamicExtractor(t *testing.T) {
	t.Run("activates when no higher tier resolves channels", func(t *testing.T) {
		mock := &mockExtractor{
			resolveFunc: func(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error) {
				return []extractor.ExtractedStream{
					{
						ChannelName: "DSports",
						StreamURL:   "https://edge.invalid/live/dsports.m3u8",
						Resolution:  "1080p",
						Alive:       true,
						Headers: map[string]string{
							"Referer":    "https://portal.invalid/live",
							"User-Agent": "Mozilla/5.0 (CustomUA)",
						},
					},
				}, nil
			},
		}

		// Catalog without dsports in userStreams so Tier 2 cannot resolve it
		cat := New(
			WithConfigFile(filepath.Join(os.TempDir(), "nonexistent_kickoff_channels.json")),
			WithStreams(map[string]string{}),
			WithParrilla(offlineParrilla{}),
			WithExtractor(mock),
		)

		// Match with DSports broadcast (unconfigured locally, e.g. DirecTV exclusive)
		match := model.Match{
			Title:      "Real Madrid vs Barcelona",
			LeagueKey:  "laliga",
			Broadcasts: []string{"DSports"},
		}

		chs := cat.GetChannels(match)
		if got := mock.callCount(); got != 1 {
			t.Fatalf("expected extractor to be called once, got %d", got)
		}

		if len(chs) != 1 {
			t.Fatalf("expected 1 channel from Tier 5 extractor, got %d", len(chs))
		}

		ch := chs[0]
		if ch.Name != "[Web] DSports HD" {
			t.Errorf("expected channel name '[Web] DSports HD', got %q", ch.Name)
		}
		if ch.DisplayName != "[Web] DSports HD" {
			t.Errorf("expected display name '[Web] DSports HD', got %q", ch.DisplayName)
		}
		if ch.URL != "https://edge.invalid/live/dsports.m3u8" {
			t.Errorf("expected URL 'https://edge.invalid/live/dsports.m3u8', got %q", ch.URL)
		}
		if ch.BitrateMode != "max" {
			t.Errorf("expected BitrateMode 'max', got %q", ch.BitrateMode)
		}
		if ch.Resolution != "1080p" {
			t.Errorf("expected Resolution '1080p', got %q", ch.Resolution)
		}
		if !ch.Recommended {
			t.Errorf("expected first channel to be recommended")
		}
		if ch.Headers == nil || ch.Headers["Referer"] != "https://portal.invalid/live" {
			t.Errorf("expected Referer header preserved, got %+v", ch.Headers)
		}
		if ch.Headers["User-Agent"] != "Mozilla/5.0 (CustomUA)" {
			t.Errorf("expected User-Agent header preserved, got %+v", ch.Headers)
		}
	})

	t.Run("aggregates official channels and dynamic web streams together", func(t *testing.T) {
		mock := &mockExtractor{
			resolveFunc: func(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error) {
				return []extractor.ExtractedStream{
					{
						ChannelName: "Direct",
						StreamURL:   "https://edge.invalid/live/direct.m3u8",
						Resolution:  "1080p",
						Alive:       true,
					},
				}, nil
			},
		}

		cat := newTestCatalog(WithExtractor(mock))

		// Tier 2: ESPN 4 is configured in mockTestStreams
		match := model.Match{
			Title:      "Arsenal vs Chelsea",
			LeagueKey:  "premier",
			Broadcasts: []string{"ESPN 4"},
		}

		chs := cat.GetChannels(match)
		if len(chs) < 2 {
			t.Fatalf("expected both official and web channels, got %d", len(chs))
		}
		if got := mock.callCount(); got != 1 {
			t.Fatalf("expected extractor callCount 1, got %d", got)
		}
	})

	t.Run("handles multiple streams with correct ordering and recommendations", func(t *testing.T) {
		mock := &mockExtractor{
			resolveFunc: func(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error) {
				return []extractor.ExtractedStream{
					{
						ChannelName: "DSports",
						StreamURL:   "https://edge.invalid/live/dsports1.m3u8",
						Resolution:  "1080p",
						Alive:       true,
					},
					{
						ChannelName: "DSports 2",
						StreamURL:   "https://edge.invalid/live/dsports2.m3u8",
						Resolution:  "1080p",
						Alive:       true,
					},
				}, nil
			},
		}

		cat := newTestCatalog(WithExtractor(mock))
		match := model.Match{
			Title:     "Boca Juniors vs River Plate",
			LeagueKey: "argentina",
		}

		chs := cat.GetChannels(match)
		if len(chs) != 2 {
			t.Fatalf("expected 2 channels, got %d", len(chs))
		}

		if chs[0].Name != "[Web] DSports HD" || !chs[0].Recommended {
			t.Errorf("expected first channel '[Web] DSports HD' recommended=true, got name=%q rec=%v", chs[0].Name, chs[0].Recommended)
		}
		if chs[1].Name != "[Web] DSports 2 HD" || chs[1].Recommended {
			t.Errorf("expected second channel '[Web] DSports 2 HD' recommended=false, got name=%q rec=%v", chs[1].Name, chs[1].Recommended)
		}
	})

	t.Run("returns nil gracefully on extractor error or empty streams", func(t *testing.T) {
		mock := &mockExtractor{
			resolveFunc: func(ctx context.Context, match model.Match) ([]extractor.ExtractedStream, error) {
				return nil, errors.New("upstream timeout")
			},
		}

		cat := newTestCatalog(WithExtractor(mock))
		match := model.Match{
			Title:     "Unknown Match",
			LeagueKey: "unknown",
		}

		chs := cat.GetChannels(match)
		if len(chs) != 0 {
			t.Fatalf("expected nil or empty channels on extractor error, got %+v", chs)
		}
		if got := mock.callCount(); got != 1 {
			t.Fatalf("expected extractor callCount 1, got %d", got)
		}
	})
}
