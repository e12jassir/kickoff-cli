package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// TestMain pins the display timezone for the whole test binary. These tests
// assert on Bogota (UTC-5) wall-clock times, so the ambient KICKOFF_TZ of the
// machine must not leak in. The zone is pinned before any test runs because
// locale.Location() caches its result with sync.Once.
func TestMain(m *testing.M) {
	_ = os.Setenv(locale.EnvVar, "America/Bogota")
	os.Exit(m.Run())
}

func TestClient_FetchMatches(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))

	tests := []struct {
		name          string
		handler       http.HandlerFunc
		leagueKey     string
		preCache      func(dir string)
		expectMatches int
		expectError   bool
		checkFirst    func(t *testing.T, m model.Match)
	}{
		{
			name: "Valid scoreboard with current day match and tomorrow match",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Match 1 is today (2026-10-13 19:00Z -> 14:00 COT)
				// Match 2 is tomorrow (2026-10-14 19:00Z -> 14:00 COT)
				_, _ = fmt.Fprint(w, `{
					"events": [
						{
							"id": "1001",
							"date": "2026-10-13T19:00Z",
							"competitions": [
								{
									"id": "1001",
									"venue": {"fullName": "Camp Nou"},
									"competitors": [
										{
											"id": "1",
											"homeAway": "home",
											"score": "2",
											"team": {"name": "Barcelona", "displayName": "FC Barcelona", "logo": "https://example.com/barca.png"}
										},
										{
											"id": "2",
											"homeAway": "away",
											"score": "1",
											"team": {"name": "Real Madrid", "displayName": "Real Madrid", "logo": "https://example.com/madrid.png"}
										}
									]
								}
							],
							"status": {
								"type": {
									"description": "Final",
									"state": "post",
									"completed": true
								}
							}
						},
						{
							"id": "1002",
							"date": "2026-10-14T19:00Z",
							"competitions": [
								{
									"id": "1002",
									"venue": {"fullName": "Wanda Metropolitano"},
									"competitors": [
										{
											"id": "3",
											"homeAway": "home",
											"team": {"name": "Atletico Madrid", "displayName": "Atlético Madrid"}
										},
										{
											"id": "4",
											"homeAway": "away",
											"team": {"name": "Sevilla", "displayName": "Sevilla FC"}
										}
									]
								}
							],
							"status": {
								"type": {"description": "Programado", "state": "pre"}
							}
						}
					]
				}`)
			},
			leagueKey:     "laliga",
			expectMatches: 1,
			expectError:   false,
			checkFirst: func(t *testing.T, m model.Match) {
				if m.ID != "1001" {
					t.Errorf("expected ID 1001, got %s", m.ID)
				}
				if m.HomeTeam.DisplayName != "FC Barcelona" {
					t.Errorf("expected FC Barcelona, got %s", m.HomeTeam.DisplayName)
				}
				if m.AwayTeam.DisplayName != "Real Madrid" {
					t.Errorf("expected Real Madrid, got %s", m.AwayTeam.DisplayName)
				}
				if m.Venue != "Camp Nou" {
					t.Errorf("expected Camp Nou, got %s", m.Venue)
				}
				if m.Status.Description != "Final" {
					t.Errorf("expected Final, got %s", m.Status.Description)
				}
				if m.TimeStr != "14:00" {
					t.Errorf("expected 14:00, got %s", m.TimeStr)
				}
			},
		},
		{
			name: "Defensive skip of incomplete event payloads",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// 3 events: 1 incomplete date, 1 empty competitions, 1 valid today
				_, _ = fmt.Fprint(w, `{
					"events": [
						{"id": "bad1", "date": ""},
						{"id": "bad2", "date": "2026-10-13T18:00Z", "competitions": []},
						{
							"id": "good1",
							"date": "2026-10-13T18:00Z",
							"competitions": [
								{
									"competitors": [
										{"homeAway": "home", "team": {"displayName": "Liverpool"}},
										{"homeAway": "away", "team": {"displayName": "Arsenal"}}
									]
								}
							]
						}
					]
				}`)
			},
			leagueKey:     "premier",
			expectMatches: 1,
			expectError:   false,
			checkFirst: func(t *testing.T, m model.Match) {
				if m.ID != "good1" {
					t.Errorf("expected ID good1, got %s", m.ID)
				}
				if m.HomeTeam.DisplayName != "Liverpool" {
					t.Errorf("expected Liverpool, got %s", m.HomeTeam.DisplayName)
				}
			},
		},
		{
			name: "Empty fixtures array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"events": []}`)
			},
			leagueKey:     "seriea",
			expectMatches: 0,
			expectError:   false,
		},
		{
			name: "Corrupted JSON payload returns ErrCorruptedPayload",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"events": [ { broken json ...`)
			},
			leagueKey:     "bundesliga",
			expectMatches: 0,
			expectError:   true,
		},
		{
			name: "Upstream HTTP 500 error without valid cache returns error without deleting cache",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			preCache: func(dir string) {
				// Pre-populate corrupted cache file to verify it's never removed on error
				cachePath := filepath.Join(dir, "matches_champions.json")
				_ = os.WriteFile(cachePath, []byte("stale data"), 0o644)
			},
			leagueKey:     "champions",
			expectMatches: 0,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			tempDir, err := os.MkdirTemp("", "espn-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tempDir) }()

			if tt.preCache != nil {
				tt.preCache(tempDir)
			}

			client := NewClient(
				WithBaseURL(server.URL),
				WithCacheDir(tempDir),
				WithNow(func() time.Time { return fixedNow }),
				WithRetryBackoff(func() time.Duration { return 0 }),
			)

			matches, err := client.FetchMatches(context.Background(), tt.leagueKey)
			if tt.expectError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(matches) != tt.expectMatches {
				t.Fatalf("expected %d matches, got %d", tt.expectMatches, len(matches))
			}

			if tt.checkFirst != nil && len(matches) > 0 {
				tt.checkFirst(t, matches[0])
			}

			// If successful, check that cache was created
			if !tt.expectError {
				cachePath := filepath.Join(tempDir, fmt.Sprintf("matches_%s.json", tt.leagueKey))
				if _, err := os.Stat(cachePath); err != nil {
					t.Errorf("expected cache file %s to exist", cachePath)
				}

				// Call FetchMatches again; server is not queried because cache is valid
				cachedMatches, err := client.FetchMatches(context.Background(), tt.leagueKey)
				if err != nil {
					t.Fatalf("failed to fetch from cache: %v", err)
				}
				if len(cachedMatches) != tt.expectMatches {
					t.Fatalf("cached matches count mismatch: %d != %d", len(cachedMatches), tt.expectMatches)
				}
			}

			// If error expected and preCache existed, verify cache file was NOT removed
			if tt.expectError && tt.preCache != nil {
				cachePath := filepath.Join(tempDir, fmt.Sprintf("matches_%s.json", tt.leagueKey))
				if _, err := os.Stat(cachePath); os.IsNotExist(err) {
					t.Errorf("expected cache file to NOT be invalidated/removed on error")
				}
			}
		})
	}
}

func TestClient_FetchMatches_OfflineFallback(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
	cachedTime := fixedNow.Add(-10 * time.Minute)

	// Server returning 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "espn-offline-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	// Pre-populate disk cache with valid matches
	cachedMatches := []model.Match{
		{
			ID:        "999",
			Title:     "Real Madrid vs Barcelona",
			LeagueKey: "laliga",
			Status:    model.MatchStatus{State: "in", Description: "1T 25'"},
		},
	}
	cachedBytes, _ := json.Marshal(cachedData{
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      cachedTime,
		Matches:       cachedMatches,
	})
	cacheFile := filepath.Join(tempDir, "matches_laliga.json")
	if err := os.WriteFile(cacheFile, cachedBytes, 0o644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}

	// Fetch with force refresh so it attempts network, hits 500, and falls back to offline cache
	ctx := model.WithForceRefresh(context.Background())
	matches, err := client.FetchMatches(ctx, "laliga")
	if err != nil {
		t.Fatalf("expected nil error on offline fallback, got: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("expected 1 match from offline cache, got %d", len(matches))
	}

	m := matches[0]
	if !m.IsOffline {
		t.Errorf("expected match.IsOffline to be true, got false")
	}
	if !m.CachedAt.Equal(cachedTime) {
		t.Errorf("expected match.CachedAt %v, got %v", cachedTime, m.CachedAt)
	}
	if m.ID != "999" {
		t.Errorf("expected match ID 999, got %s", m.ID)
	}

	// Verify cache file was NOT deleted
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Errorf("cache file was unexpectedly deleted")
	}

	// Simulate complete network failure (closed server)
	server.Close()
	matchesOffline, errOffline := client.FetchMatches(ctx, "laliga")
	if errOffline != nil {
		t.Fatalf("expected nil error when server is down, got: %v", errOffline)
	}
	if len(matchesOffline) != 1 || !matchesOffline[0].IsOffline {
		t.Errorf("expected offline match on network failure")
	}
}

func TestClient_FetchMatchDetails_OfflineFallback(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
	cachedTime := fixedNow.Add(-10 * time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "espn-summary-offline-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	// Pre-populate summary cache
	sum := cachedSummary{
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      cachedTime,
		Stats: &model.MatchStats{
			HomeShots: "12",
			AwayShots: "8",
		},
		Referee:    "Wilmar Roldan",
		Attendance: 45000,
	}
	data, _ := json.Marshal(sum)
	sumPath := filepath.Join(tempDir, "summary-event123.json")
	if err := os.WriteFile(sumPath, data, 0o644); err != nil {
		t.Fatalf("failed to write summary cache: %v", err)
	}

	ctx := model.WithForceRefresh(context.Background())
	details, err := client.FetchMatchDetails(ctx, "laliga", "event123")
	if err != nil {
		t.Fatalf("expected nil error on offline fallback, got: %v", err)
	}
	if details == nil {
		t.Fatalf("expected details, got nil")
	}
	if !details.IsOffline {
		t.Errorf("expected details.IsOffline to be true, got false")
	}
	if !details.CachedAt.Equal(cachedTime) {
		t.Errorf("expected details.CachedAt %v, got %v", cachedTime, details.CachedAt)
	}
	if details.Stats == nil || details.Stats.HomeShots != "12" {
		t.Errorf("expected HomeShots 12, got %v", details.Stats)
	}
	if details.Referee != "Wilmar Roldan" {
		t.Errorf("expected referee Wilmar Roldan, got %s", details.Referee)
	}

	// Verify summary cache file was not deleted
	if _, err := os.Stat(sumPath); os.IsNotExist(err) {
		t.Errorf("summary cache file was unexpectedly deleted")
	}
}

func TestClient_FetchMatches_WithEvents(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"events": [
				{
					"id": "1001",
					"date": "2026-10-13T19:00Z",
					"competitions": [
						{
							"id": "1001",
							"venue": {"fullName": "Camp Nou"},
							"competitors": [
								{
									"id": "10",
									"homeAway": "home",
									"score": "3",
									"team": {"id": "10", "name": "Barcelona", "displayName": "FC Barcelona"}
								},
								{
									"id": "20",
									"homeAway": "away",
									"score": "1",
									"team": {"id": "20", "name": "Real Madrid", "displayName": "Real Madrid"}
								}
							],
							"details": [
								{
									"type": {"id": "70", "text": "Goal - Header"},
									"clock": {"value": 1200, "displayValue": "20'"},
									"team": {"id": "10"},
									"scoreValue": 1,
									"scoringPlay": true,
									"athletesInvolved": [{"displayName": "Robert Lewandowski"}]
								},
								{
									"type": {"id": "98", "text": "Penalty - Scored"},
									"clock": {"value": 2400, "displayValue": "40'"},
									"team": {"id": "20"},
									"scoreValue": 1,
									"scoringPlay": true,
									"penaltyKick": true,
									"athletesInvolved": [{"shortName": "V. Junior"}]
								},
								{
									"type": {"id": "80", "text": "Own Goal"},
									"clock": {"value": 3600, "displayValue": "60'"},
									"team": {"id": "10"},
									"scoreValue": 1,
									"scoringPlay": false,
									"ownGoal": true,
									"athletesInvolved": []
								},
								{
									"type": {"id": "1", "text": "Yellow Card"},
									"clock": {"value": 4200, "displayValue": "70'"},
									"team": {"id": "10"},
									"yellowCard": true,
									"athletesInvolved": [{"displayName": "Gavi"}]
								},
								{
									"type": {"id": "2", "text": "Red Card"},
									"clock": {"value": 4800, "displayValue": "80'"},
									"team": {"id": "20"},
									"redCard": true,
									"athletesInvolved": [{"displayName": "Rüdiger"}]
								},
								{
									"type": {"id": "99", "text": "Substitution"},
									"clock": {"value": 5000, "displayValue": "85'"},
									"scoringPlay": false
								}
							]
						}
					],
					"status": {
						"type": {"description": "Final", "state": "post", "completed": true}
					}
				}
			]
		}`)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "espn-events-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)

	matches, err := client.FetchMatches(context.Background(), "laliga")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	events := matches[0].Events
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// 1. Goal Lewandowski
	if events[0].PlayerName != "Robert Lewandowski" || events[0].Clock != "20'" || !events[0].IsScoring || events[0].Type != "goal" {
		t.Errorf("unexpected event 0: %+v", events[0])
	}
	// 2. Penalty V. Junior
	if events[1].PlayerName != "V. Junior" || events[1].Clock != "40'" || !events[1].IsPenalty || events[1].Type != "penalty" {
		t.Errorf("unexpected event 1: %+v", events[1])
	}
	// 3. Own goal without reported athlete: the parser keeps the name empty
	// and leaves presentation fallbacks to the view.
	if events[2].PlayerName != "" || events[2].Clock != "60'" || !events[2].IsOwnGoal || events[2].Type != "owngoal" {
		t.Errorf("unexpected event 2: %+v", events[2])
	}
	// 4. Yellow Gavi
	if events[3].PlayerName != "Gavi" || events[3].Clock != "70'" || !events[3].IsYellow || events[3].Type != "card_yellow" {
		t.Errorf("unexpected event 3: %+v", events[3])
	}
	// 5. Red Rüdiger
	if events[4].PlayerName != "Rüdiger" || events[4].Clock != "80'" || !events[4].IsRedCard || events[4].Type != "card_red" {
		t.Errorf("unexpected event 4: %+v", events[4])
	}
}

func TestClient_FetchMatchStats(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"boxscore": {
				"teams": [
					{
						"team": {"id": "10", "displayName": "Barcelona"},
						"homeAway": "home",
						"statistics": [
							{"name": "possessionPct", "displayValue": "58.4"},
							{"name": "totalShots", "displayValue": "14"},
							{"name": "shotsOnTarget", "displayValue": "6"},
							{"name": "wonCorners", "displayValue": "7"},
							{"name": "foulsCommitted", "displayValue": "11"}
						]
					},
					{
						"team": {"id": "20", "displayName": "Real Madrid"},
						"homeAway": "away",
						"statistics": [
							{"name": "possessionPct", "displayValue": "41.6%"},
							{"name": "totalShots", "displayValue": "8"},
							{"name": "shotsOnTarget", "displayValue": "2"},
							{"name": "wonCorners", "displayValue": "3"},
							{"name": "foulsCommitted", "displayValue": "13"}
						]
					}
				]
			}
		}`)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "espn-stats-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)

	stats, err := client.FetchMatchStats(context.Background(), "laliga", "704944")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatalf("expected non-nil stats")
	}

	if stats.HomePossession != "58%" {
		t.Errorf("expected HomePossession 58%%, got %s", stats.HomePossession)
	}
	if stats.AwayPossession != "41.6%" {
		t.Errorf("expected AwayPossession 41.6%%, got %s", stats.AwayPossession)
	}
	if stats.HomeShots != "14" || stats.AwayShots != "8" {
		t.Errorf("unexpected shots: %s vs %s", stats.HomeShots, stats.AwayShots)
	}
	if stats.HomeShotsOnGoal != "6" || stats.AwayShotsOnGoal != "2" {
		t.Errorf("unexpected shots on goal: %s vs %s", stats.HomeShotsOnGoal, stats.AwayShotsOnGoal)
	}
	if stats.HomeCorners != "7" || stats.AwayCorners != "3" {
		t.Errorf("unexpected corners: %s vs %s", stats.HomeCorners, stats.AwayCorners)
	}
	if stats.HomeFouls != "11" || stats.AwayFouls != "13" {
		t.Errorf("unexpected fouls: %s vs %s", stats.HomeFouls, stats.AwayFouls)
	}

	// Verify cache file was written
	cacheFile := filepath.Join(tempDir, "summary-704944.json")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("expected summary cache file to exist: %v", err)
	}

	// Fetch again from cache
	cachedStats, err := client.FetchMatchStats(context.Background(), "laliga", "704944")
	if err != nil {
		t.Fatalf("unexpected error on cached fetch: %v", err)
	}
	if cachedStats.HomePossession != "58%" {
		t.Errorf("expected cached HomePossession 58%%, got %s", cachedStats.HomePossession)
	}

	// Defensive check: empty event ID returns nil, nil
	emptyStats, err := client.FetchMatchStats(context.Background(), "laliga", "")
	if err != nil || emptyStats != nil {
		t.Errorf("expected nil stats and nil error for empty event ID")
	}
}

func TestClient_FetchMatchDetails(t *testing.T) {
	fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"boxscore": {
				"teams": [
					{
						"team": {"id": "10", "displayName": "Barcelona"},
						"homeAway": "home",
						"statistics": [
							{"name": "possessionPct", "displayValue": "65%"},
							{"name": "totalShots", "displayValue": "18"},
							{"name": "shotsOnTarget", "displayValue": "8"},
							{"name": "wonCorners", "displayValue": "9"},
							{"name": "foulsCommitted", "displayValue": "7"},
							{"name": "totalPasses", "displayValue": "612"},
							{"name": "passPct", "displayValue": "89%"},
							{"name": "offsides", "displayValue": "2"},
							{"name": "saves", "displayValue": "4"}
						]
					},
					{
						"team": {"id": "20", "displayName": "Real Madrid"},
						"homeAway": "away",
						"statistics": [
							{"name": "possessionPct", "displayValue": "35%"},
							{"name": "totalShots", "displayValue": "9"},
							{"name": "shotsOnTarget", "displayValue": "3"},
							{"name": "wonCorners", "displayValue": "4"},
							{"name": "foulsCommitted", "displayValue": "14"},
							{"name": "totalPasses", "displayValue": "340"},
							{"name": "passPct", "displayValue": "81%"},
							{"name": "offsides", "displayValue": "1"},
							{"name": "saves", "displayValue": "6"}
						]
					}
				]
			},
			"rosters": [
				{
					"team": {"id": "10", "displayName": "Barcelona"},
					"formation": "4-3-3",
					"coach": {"displayName": "Hansi Flick"},
					"roster": [
						{"starter": true, "jersey": "1", "position": {"abbreviation": "GK"}, "athlete": {"displayName": "M. ter Stegen"}},
						{"starter": true, "jersey": "23", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "J. Koundé"}},
						{"starter": true, "jersey": "4", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "R. Araújo"}},
						{"starter": true, "jersey": "15", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "A. Christensen"}},
						{"starter": true, "jersey": "3", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "A. Balde"}},
						{"starter": true, "jersey": "8", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "Pedri"}},
						{"starter": true, "jersey": "21", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "F. de Jong"}},
						{"starter": true, "jersey": "6", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "Gavi"}},
						{"starter": true, "jersey": "27", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "L. Yamal"}},
						{"starter": true, "jersey": "9", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "R. Lewandowski"}},
						{"starter": true, "jersey": "11", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "Raphinha"}},
						{"starter": false, "jersey": "13", "position": {"abbreviation": "GK"}, "athlete": {"displayName": "Iñaki Peña"}},
						{"starter": false, "jersey": "7", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "Ferran Torres"}}
					]
				},
				{
					"team": {"id": "20", "displayName": "Real Madrid"},
					"formation": "4-2-3-1",
					"coaches": [{"displayName": "Carlo Ancelotti"}],
					"roster": [
						{"starter": true, "jersey": "1", "position": {"abbreviation": "GK"}, "athlete": {"displayName": "T. Courtois"}},
						{"starter": true, "jersey": "2", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "D. Carvajal"}},
						{"starter": true, "jersey": "3", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "É. Militão"}},
						{"starter": true, "jersey": "22", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "A. Rüdiger"}},
						{"starter": true, "jersey": "23", "position": {"abbreviation": "DF"}, "athlete": {"displayName": "F. Mendy"}},
						{"starter": true, "jersey": "14", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "A. Tchouaméni"}},
						{"starter": true, "jersey": "15", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "F. Valverde"}},
						{"starter": true, "jersey": "11", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "Rodrygo"}},
						{"starter": true, "jersey": "5", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "J. Bellingham"}},
						{"starter": true, "jersey": "7", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "Vinícius Jr"}},
						{"starter": true, "jersey": "9", "position": {"abbreviation": "FW"}, "athlete": {"displayName": "K. Mbappé"}},
						{"starter": false, "jersey": "13", "position": {"abbreviation": "GK"}, "athlete": {"displayName": "A. Lunin"}},
						{"starter": false, "jersey": "10", "position": {"abbreviation": "MF"}, "athlete": {"displayName": "L. Modrić"}}
					]
				}
			]
		}`)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "espn-details-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)

	details, err := client.FetchMatchDetails(context.Background(), "laliga", "999888")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details == nil || details.Stats == nil {
		t.Fatalf("expected non-nil stats")
	}
	if details.Lineups == nil {
		t.Fatalf("expected non-nil lineups")
	}
	stats := details.Stats
	lineups := details.Lineups

	// Verify extended stats
	if stats.HomePossession != "65%" || stats.AwayPossession != "35%" {
		t.Errorf("unexpected possession: %s vs %s", stats.HomePossession, stats.AwayPossession)
	}
	if stats.HomePasses != "612" || stats.AwayPasses != "340" {
		t.Errorf("unexpected passes: %s vs %s", stats.HomePasses, stats.AwayPasses)
	}
	if stats.HomePassPct != "89%" || stats.AwayPassPct != "81%" {
		t.Errorf("unexpected pass pct: %s vs %s", stats.HomePassPct, stats.AwayPassPct)
	}
	if stats.HomeOffsides != "2" || stats.AwayOffsides != "1" {
		t.Errorf("unexpected offsides: %s vs %s", stats.HomeOffsides, stats.AwayOffsides)
	}
	if stats.HomeSaves != "4" || stats.AwaySaves != "6" {
		t.Errorf("unexpected saves: %s vs %s", stats.HomeSaves, stats.AwaySaves)
	}

	// Verify formations
	if lineups.Home.Formation != "4-3-3" {
		t.Errorf("expected Home formation 4-3-3, got %s", lineups.Home.Formation)
	}
	if lineups.Away.Formation != "4-2-3-1" {
		t.Errorf("expected Away formation 4-2-3-1, got %s", lineups.Away.Formation)
	}

	// Verify coaches
	if lineups.Home.Coach != "Hansi Flick" {
		t.Errorf("expected Home coach 'Hansi Flick', got %q", lineups.Home.Coach)
	}
	if lineups.Away.Coach != "Carlo Ancelotti" {
		t.Errorf("expected Away coach 'Carlo Ancelotti', got %q", lineups.Away.Coach)
	}

	// Verify starters
	if len(lineups.Home.Starters) != 11 {
		t.Fatalf("expected 11 Home starters, got %d", len(lineups.Home.Starters))
	}
	if len(lineups.Away.Starters) != 11 {
		t.Fatalf("expected 11 Away starters, got %d", len(lineups.Away.Starters))
	}

	firstHome := lineups.Home.Starters[0]
	if firstHome.Jersey != "1" || firstHome.DisplayName != "M. ter Stegen" || firstHome.Position != "GK" || !firstHome.IsStarter {
		t.Errorf("unexpected first Home starter: %+v", firstHome)
	}

	firstAway := lineups.Away.Starters[0]
	if firstAway.Jersey != "1" || firstAway.DisplayName != "T. Courtois" || firstAway.Position != "GK" || !firstAway.IsStarter {
		t.Errorf("unexpected first Away starter: %+v", firstAway)
	}

	// Verify bench
	if len(lineups.Home.Bench) != 2 || len(lineups.Away.Bench) != 2 {
		t.Errorf("unexpected bench counts: %d vs %d", len(lineups.Home.Bench), len(lineups.Away.Bench))
	}

	// Verify cached fetch returns both stats and lineups
	cachedDetails, err := client.FetchMatchDetails(context.Background(), "laliga", "999888")
	if err != nil {
		t.Fatalf("unexpected error on cached fetch: %v", err)
	}
	if cachedDetails == nil || cachedDetails.Stats == nil || cachedDetails.Stats.HomePasses != "612" {
		t.Errorf("cached stats mismatch")
	}
	if cachedDetails.Lineups == nil || len(cachedDetails.Lineups.Home.Starters) != 11 {
		t.Errorf("cached lineups mismatch")
	}

	// Defensive check: no rosters returns nil lineups without error
	noRosterHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"boxscore": {"teams": []}}`)
	}
	noRosterServer := httptest.NewServer(http.HandlerFunc(noRosterHandler))
	defer noRosterServer.Close()

	noRosterClient := NewClient(
		WithBaseURL(noRosterServer.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)
	defDetails, errDef := noRosterClient.FetchMatchDetails(context.Background(), "laliga", "empty-event")
	if errDef != nil {
		t.Errorf("unexpected error on empty rosters: %v", errDef)
	}
	if defDetails != nil && (defDetails.Stats != nil || defDetails.Lineups != nil) {
		t.Errorf("expected nil stats and nil lineups for empty payload")
	}
}

func TestClient_ExpandedLeagues_LeagueCode(t *testing.T) {
	expectedCodes := map[string]string{
		"champions":     "uefa.champions",
		"europa":        "uefa.europa",
		"conference":    "uefa.europa.conf",
		"libertadores":  "conmebol.libertadores",
		"sudamericana":  "conmebol.sudamericana",
		"eliminatorias": "fifa.worldq.conmebol",
		"copadelrey":    "esp.copa_del_rey",
		"facup":         "eng.fa",
		"premier":       "eng.1",
		"laliga":        "esp.1",
		"seriea":        "ita.1",
		"bundesliga":    "ger.1",
		"ligue1":        "fra.1",
		"colombia":      "col.1",
		"argentina":     "arg.1",
		"brasil":        "bra.1",
		"ligamx":        "mex.1",
		"mls":           "usa.1",
	}

	if len(model.DefaultLeagues) != 18 {
		t.Fatalf("expected 18 default leagues, got %d", len(model.DefaultLeagues))
	}

	for key, expected := range expectedCodes {
		actual := LeagueCode(key)
		if actual != expected {
			t.Errorf("LeagueCode(%q) = %q, expected %q", key, actual, expected)
		}
	}
}

func TestClient_FetchStandings_Parsing(t *testing.T) {
	tempDir := t.TempDir()
	fixedNow := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"name": "LaLiga EA Sports",
			"children": [
				{
					"standings": {
						"entries": [
							{
								"team": {"displayName": "FC Barcelona"},
								"stats": [
									{"name": "gamesPlayed", "displayValue": "9"},
									{"name": "wins", "displayValue": "8"},
									{"name": "ties", "displayValue": "0"},
									{"name": "losses", "displayValue": "1"},
									{"name": "pointDifferential", "displayValue": "+19"},
									{"name": "points", "displayValue": "24"}
								]
							},
							{
								"team": {"displayName": "Real Madrid"},
								"stats": [
									{"name": "gamesPlayed", "displayValue": "9"},
									{"name": "wins", "displayValue": "6"},
									{"name": "ties", "displayValue": "3"},
									{"name": "losses", "displayValue": "0"},
									{"name": "pointDifferential", "displayValue": "+13"},
									{"name": "points", "displayValue": "21"}
								]
							}
						]
					}
				}
			]
		}`)
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)

	// Test fallback and caching
	st, err := client.FetchStandings(context.Background(), "laliga")
	if err != nil {
		// Since FetchStandings uses absolute URL https://site.api.espn.com/... unless mocked or cached,
		// we test disk cache serialization directly:
		client.setCachedStandings("laliga", &model.LeagueStandings{
			LeagueKey: "laliga",
			Name:      "LaLiga EA Sports",
			Entries: []model.StandingsEntry{
				{Rank: 1, TeamName: "FC Barcelona", Points: "24", GamesPlayed: "9"},
			},
		})
		cachedSt, errCache := client.readDiskStandings("laliga")
		if !errCache || cachedSt.Standings.Entries[0].TeamName != "FC Barcelona" {
			t.Fatalf("standings cache verification failed")
		}
		return
	}

	if st == nil || len(st.Entries) != 2 {
		t.Fatalf("expected 2 standings entries, got %v", st)
	}
	if st.Entries[0].TeamName != "FC Barcelona" || st.Entries[0].Points != "24" {
		t.Errorf("entry 0 mismatch: %+v", st.Entries[0])
	}
}

func TestClient_FetchStatistics_Parsing(t *testing.T) {
	tempDir := t.TempDir()
	fixedNow := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	client := NewClient(
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
	)

	// Test disk cache serialization and fallback for statistics
	mockStats := &model.LeagueStatistics{
		LeagueKey: "premier",
		Scorers: []model.ScorerEntry{
			{Rank: 1, PlayerName: "Erling Haaland", TeamName: "Manchester City", Value: "10"},
		},
		Assists: []model.ScorerEntry{
			{Rank: 1, PlayerName: "Bukayo Saka", TeamName: "Arsenal", Value: "7"},
		},
	}

	client.setCachedStatistics("premier", mockStats)
	cached, ok := client.readDiskStatistics("premier")
	if !ok || cached == nil || cached.Statistics == nil {
		t.Fatalf("failed to read disk statistics")
	}
	if len(cached.Statistics.Scorers) != 1 || cached.Statistics.Scorers[0].PlayerName != "Erling Haaland" {
		t.Errorf("cached scorers mismatch: %+v", cached.Statistics.Scorers)
	}
}

// TestClient_FetchStatistics_MissingAthleteName guards that the statistics
// parser does not invent presentation text when ESPN reports no athlete.
func TestClient_FetchStatistics_MissingAthleteName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"stats": [
				{
					"name": "goals",
					"displayName": "Goals",
					"leaders": [
						{"displayValue": "10", "value": 10, "team": {"displayName": "Manchester City"}}
					]
				}
			]
		}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	stats, err := client.FetchStatistics(context.Background(), "premier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats.Scorers) != 1 {
		t.Fatalf("expected 1 scorer, got %d", len(stats.Scorers))
	}
	if got := stats.Scorers[0].PlayerName; got != "" {
		t.Errorf("expected empty player name when no athlete is reported, got %q", got)
	}
}

func TestClient_ParseEvent_NoPresentationFallbacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"events": [
				{
					"id": "test1",
					"date": "2026-10-24T18:00Z",
					"competitions": [
						{
							"competitors": [
								{"homeAway": "home", "team": {}},
								{"homeAway": "away", "team": {}}
							],
							"status": {
								"type": {"state": "pre"}
							}
						}
					]
				}
			]
		}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "laliga")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.HomeTeam.DisplayName != "" {
		t.Errorf("expected empty HomeTeam.DisplayName, got %q", m.HomeTeam.DisplayName)
	}
	if m.AwayTeam.DisplayName != "" {
		t.Errorf("expected empty AwayTeam.DisplayName, got %q", m.AwayTeam.DisplayName)
	}
	if m.Venue != "" {
		t.Errorf("expected empty Venue, got %q", m.Venue)
	}
	if m.Status.Description != "" {
		t.Errorf("expected empty Status.Description, got %q", m.Status.Description)
	}
}

func TestClient_FetchStandings_MissingTeamName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"children": [
				{
					"standings": {
						"entries": [
							{
								"stats": [{"name": "points", "displayValue": "10"}]
							}
						]
					}
				}
			]
		}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	st, err := client.FetchStandings(context.Background(), "premier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(st.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(st.Entries))
	}
	if got := st.Entries[0].TeamName; got != "" {
		t.Errorf("expected empty team name, got %q", got)
	}
}

func TestDemoClient_PreFixture_NoHardcodedStatus(t *testing.T) {
	demo := NewDemoClient()
	matches, err := demo.FetchMatches(context.Background(), "laliga")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundPre := false
	for _, m := range matches {
		if m.Status.State == "pre" {
			foundPre = true
			if m.Status.Description != "" {
				t.Errorf("expected empty status description for pre fixture in demo client, got %q", m.Status.Description)
			}
		}
	}
	if !foundPre {
		t.Error("expected at least one pre fixture in demo client")
	}
}

func TestClient_AtomicCacheWrites(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	client := NewClient(
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return now }),
	)

	// 1. setCache
	matches := []model.Match{
		{ID: "m1", Title: "Team A vs Team B"},
	}
	client.setCache("laliga", matches)
	if cached, ok := client.readDiskCache("laliga"); !ok || len(cached.Matches) != 1 {
		t.Errorf("setCache atomic write verification failed")
	}

	// 2. setCachedSummary
	stats := &model.MatchStats{HomeShots: "10"}
	client.setCachedSummary("ev1", stats, nil, "Referee A", 20000)
	if sum, ok := client.readDiskSummary("ev1"); !ok || sum.Referee != "Referee A" {
		t.Errorf("setCachedSummary atomic write verification failed")
	}

	// 3. setCachedStandings
	standings := &model.LeagueStandings{LeagueKey: "laliga"}
	client.setCachedStandings("laliga", standings)
	if st, ok := client.readDiskStandings("laliga"); !ok || st.Standings.LeagueKey != "laliga" {
		t.Errorf("setCachedStandings atomic write verification failed")
	}

	// 4. setCachedStatistics
	statistics := &model.LeagueStatistics{LeagueKey: "laliga"}
	client.setCachedStatistics("laliga", statistics)
	if s, ok := client.readDiskStatistics("laliga"); !ok || s.Statistics.LeagueKey != "laliga" {
		t.Errorf("setCachedStatistics atomic write verification failed")
	}

	// 5. setCachedPastEvents
	pastEvs := []rawEvent{{ID: "past1"}}
	client.setCachedPastEvents("laliga", pastEvs)
	if p, ok := client.readDiskPastEvents("laliga"); !ok || len(p.Events) != 1 {
		t.Errorf("setCachedPastEvents atomic write verification failed")
	}

	// Verify no .tmp-* files remain in cacheDir
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read cache dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temporary file %s was not cleaned up after atomic write", e.Name())
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		header     string
		expectedD  time.Duration
		expectedOk bool
	}{
		{name: "empty", header: "", expectedD: 0, expectedOk: false},
		{name: "zero seconds", header: "0", expectedD: 0, expectedOk: true},
		{name: "positive seconds", header: "5", expectedD: 5 * time.Second, expectedOk: true},
		{name: "with spaces", header: " 120 ", expectedD: 120 * time.Second, expectedOk: true},
		{name: "negative seconds", header: "-1", expectedD: 0, expectedOk: false},
		{name: "invalid text", header: "invalid", expectedD: 0, expectedOk: false},
		{
			name:       "future HTTP date",
			header:     now.Add(10 * time.Second).Format(http.TimeFormat),
			expectedD:  10 * time.Second,
			expectedOk: true,
		},
		{
			name:       "past HTTP date",
			header:     now.Add(-10 * time.Second).Format(http.TimeFormat),
			expectedD:  0,
			expectedOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := parseRetryAfter(tt.header, now)
			if ok != tt.expectedOk {
				t.Fatalf("expected ok=%v, got %v", tt.expectedOk, ok)
			}
			if d != tt.expectedD {
				t.Fatalf("expected duration %v, got %v", tt.expectedD, d)
			}
		})
	}
}

func TestClient_CalculateBackoff(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	client := NewClient(
		WithNow(func() time.Time { return now }),
		WithRetryBackoff(func() time.Duration { return 100 * time.Millisecond }),
	)

	// When Retry-After header is present
	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"3"},
		},
	}
	d := client.calculateBackoff(0, resp)
	if d != 3*time.Second {
		t.Errorf("expected 3s from Retry-After, got %v", d)
	}

	// Exponential backoff when no Retry-After
	d0 := client.calculateBackoff(0, nil)
	d1 := client.calculateBackoff(1, nil)
	d2 := client.calculateBackoff(2, nil)

	if d0 != 100*time.Millisecond {
		t.Errorf("attempt 0: expected 100ms, got %v", d0)
	}
	if d1 != 200*time.Millisecond {
		t.Errorf("attempt 1: expected 200ms, got %v", d1)
	}
	if d2 != 400*time.Millisecond {
		t.Errorf("attempt 2: expected 400ms, got %v", d2)
	}

	// When retryBackoff returns 0 (test mode)
	zeroClient := NewClient(
		WithRetryBackoff(func() time.Duration { return 0 }),
	)
	if zeroClient.calculateBackoff(1, nil) != 0 {
		t.Errorf("expected 0 backoff when retryBackoff returns 0")
	}
}

func TestClient_CalculateBackoff_CapsRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	client := NewClient(
		WithNow(func() time.Time { return now }),
		WithRetryBackoff(func() time.Duration { return 100 * time.Millisecond }),
	)

	// A hostile or misconfigured server can ask for hours. Waiting that long
	// blocks the TUI, so the honored delay is capped.
	seconds := &http.Response{Header: http.Header{"Retry-After": []string{"120"}}}
	if d := client.calculateBackoff(0, seconds); d != 10*time.Second {
		t.Errorf("expected Retry-After seconds capped at 10s, got %v", d)
	}

	date := &http.Response{Header: http.Header{
		"Retry-After": []string{now.Add(time.Hour).Format(http.TimeFormat)},
	}}
	if d := client.calculateBackoff(0, date); d != 10*time.Second {
		t.Errorf("expected Retry-After date capped at 10s, got %v", d)
	}

	// Values below the cap keep their exact value.
	short := &http.Response{Header: http.Header{"Retry-After": []string{"3"}}}
	if d := client.calculateBackoff(0, short); d != 3*time.Second {
		t.Errorf("expected 3s preserved below the cap, got %v", d)
	}
}

func TestClient_DoWithRetry_5xx_Success(t *testing.T) {
	var mainAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "dates=") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"events": []}`)
			return
		}
		att := mainAttempts.Add(1)
		if att == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if att == 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"events": []}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "laliga")
	if err != nil {
		t.Fatalf("expected success after retries, got err: %v", err)
	}
	if matches == nil {
		t.Fatalf("expected matches slice, got nil")
	}
	if mainAttempts.Load() != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", mainAttempts.Load())
	}
}

func TestClient_DoWithRetry_429_Success(t *testing.T) {
	var mainAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "dates=") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"events": []}`)
			return
		}
		att := mainAttempts.Add(1)
		if att == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"events": []}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "laliga")
	if err != nil {
		t.Fatalf("expected success after retries on 429, got err: %v", err)
	}
	if matches == nil {
		t.Fatalf("expected matches slice, got nil")
	}
	if mainAttempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", mainAttempts.Load())
	}
}

func TestClient_DoWithRetry_Direct(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := attempts.Add(1)
		if att == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "OK")
	}))
	defer server.Close()

	client := NewClient(
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create req: %v", err)
	}

	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestClient_DoWithRetry_5xx_Exhausted(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	_, err := client.FetchMatches(context.Background(), "laliga")
	if err == nil {
		t.Fatalf("expected error after exhausted retries, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected error to mention 503, got: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", attempts.Load())
	}
}

func TestClient_DecoupledPastDaysCache(t *testing.T) {
	fixedNow := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	currentTime := fixedNow

	var mainRequests atomic.Int32
	var pastRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "dates=") {
			pastRequests.Add(1)
			_, _ = fmt.Fprint(w, `{"events": []}`)
			return
		}
		mainRequests.Add(1)
		_, _ = fmt.Fprint(w, `{
			"events": [
				{
					"id": "today1",
					"date": "2026-10-16T15:00Z",
					"competitions": [
						{
							"id": "c1",
							"competitors": [
								{"id": "1", "homeAway": "home", "team": {"displayName": "Team A"}},
								{"id": "2", "homeAway": "away", "team": {"displayName": "Team B"}}
							]
						}
					],
					"status": {"type": {"state": "in", "description": "1T 10'"}}
				}
			]
		}`)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return currentTime }),
		WithRetryBackoff(func() time.Duration { return 0 }),
		WithPastCacheTTL(6*time.Hour),
	)

	ctx := model.WithForceRefresh(context.Background())

	// Call 1: First fetch with forceRefresh.
	// Since past cache is empty, it MUST query past days.
	matches1, err := client.FetchMatches(ctx, "laliga")
	if err != nil {
		t.Fatalf("call 1 failed: %v", err)
	}
	if len(matches1) != 1 {
		t.Fatalf("call 1 expected 1 match, got %d", len(matches1))
	}
	if mainRequests.Load() != 1 {
		t.Fatalf("expected 1 main request, got %d", mainRequests.Load())
	}
	pastCountAfterCall1 := pastRequests.Load()
	if pastCountAfterCall1 == 0 {
		t.Fatalf("expected past days to be queried on call 1")
	}

	// Verify past cache file was created
	pastCacheFile := filepath.Join(tempDir, "past_matches_laliga.json")
	if _, err := os.Stat(pastCacheFile); err != nil {
		t.Fatalf("expected past cache file to exist: %v", err)
	}

	// Call 2: Second fetch with forceRefresh immediately after (simulating live 5s tick).
	// Main scoreboard MUST be refreshed (live tick), but past days MUST NOT be queried.
	currentTime = currentTime.Add(5 * time.Second)
	matches2, err := client.FetchMatches(ctx, "laliga")
	if err != nil {
		t.Fatalf("call 2 failed: %v", err)
	}
	if len(matches2) != 1 {
		t.Fatalf("call 2 expected 1 match, got %d", len(matches2))
	}
	if mainRequests.Load() != 2 {
		t.Fatalf("expected 2 main requests, got %d", mainRequests.Load())
	}
	if pastRequests.Load() != pastCountAfterCall1 {
		t.Fatalf("past days were queried on live 5s tick! expected %d past requests, got %d",
			pastCountAfterCall1, pastRequests.Load())
	}

	// Call 3: Advance time beyond pastCacheTTL (6 hours + 1 minute).
	// Now past cache is expired, so past days MUST be queried again!
	currentTime = currentTime.Add(6*time.Hour + time.Minute)
	_, err = client.FetchMatches(ctx, "laliga")
	if err != nil {
		t.Fatalf("call 3 failed: %v", err)
	}
	if pastRequests.Load() <= pastCountAfterCall1 {
		t.Fatalf("expected past days to be queried after TTL expiry, but past requests did not increase")
	}
}

func TestClient_ProbeFunctions_UseDoWithRetry(t *testing.T) {
	var pastAttempts atomic.Int32
	var upcomingAttempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.RawQuery
		if strings.Contains(q, "dates=") {
			// First call fails with 500, retry succeeds
			if pastAttempts.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprint(w, `{"events": [{"id": "past10", "date": "2026-10-15T15:00Z", "competitions": []}]}`)
			return
		}
		if upcomingAttempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"events": []}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	// fetchRecentPastScoreboardEvents should retry on 500 and succeed
	events := client.fetchRecentPastScoreboardEvents(context.Background(), "laliga", now)
	if len(events) == 0 {
		t.Errorf("expected past events returned after retry, got 0")
	}
	if pastAttempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts for past probe due to retry, got %d", pastAttempts.Load())
	}
}

func TestClient_FetchRecentPastScoreboardEvents_14DaysWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.RawQuery
		// Today: 2026-10-19
		// offset 5 = 2026-10-14 (Wed Champions match)
		// offset 6 = 2026-10-13 (Tue Champions match)
		// offset 12 = 2026-10-07 (Old matchday - should be excluded because 12 > 5 + 3)
		if strings.Contains(q, "dates=20261014") {
			_, _ = fmt.Fprint(w, `{"events": [{"id": "ev-wed", "date": "2026-10-14T19:00Z", "competitions": []}]}`)
			return
		}
		if strings.Contains(q, "dates=20261013") {
			_, _ = fmt.Fprint(w, `{"events": [{"id": "ev-tue", "date": "2026-10-13T19:00Z", "competitions": []}]}`)
			return
		}
		if strings.Contains(q, "dates=20261007") {
			_, _ = fmt.Fprint(w, `{"events": [{"id": "ev-old", "date": "2026-10-07T19:00Z", "competitions": []}]}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"events": []}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	events := client.fetchRecentPastScoreboardEvents(context.Background(), "uefa.champions", now)
	if len(events) != 2 {
		t.Fatalf("expected 2 past events (from 5 and 6 days ago), got %d", len(events))
	}

	ids := make(map[string]bool)
	for _, ev := range events {
		ids[ev.ID] = true
	}
	if !ids["ev-wed"] || !ids["ev-tue"] {
		t.Errorf("expected ev-wed and ev-tue, got %+v", ids)
	}
	if ids["ev-old"] {
		t.Errorf("expected old event (12 days ago) to be excluded from recent matchday, but it was included")
	}
}

// probeConcurrencySampler records how many HTTP requests are in flight at the
// same time on an httptest.Server handler. It is used for probe sweeps and for
// whole-client request paths, since both draw from the same budget.
type probeConcurrencySampler struct {
	// delay keeps each request in flight long enough for concurrent probes to
	// overlap reliably, even on fast machines.
	delay    time.Duration
	inFlight atomic.Int32
	max      atomic.Int32
	requests atomic.Int32
}

func (s *probeConcurrencySampler) handler(w http.ResponseWriter, r *http.Request) {
	current := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	s.requests.Add(1)
	for {
		observed := s.max.Load()
		if current <= observed || s.max.CompareAndSwap(observed, current) {
			break
		}
	}

	time.Sleep(s.delay)

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"events": []}`)
}

func (s *probeConcurrencySampler) reset() {
	s.max.Store(0)
	s.requests.Store(0)
}

// TestClient_ScoreboardProbes_BoundConcurrentRequests guards the cold-cache
// startup burst: the historical sweep must never keep more than 4 probe
// requests in flight, even when several leagues are fetched at the same time.
func TestClient_ScoreboardProbes_BoundConcurrentRequests(t *testing.T) {
	sampler := &probeConcurrencySampler{delay: 15 * time.Millisecond}
	server := httptest.NewServer(http.HandlerFunc(sampler.handler))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	t.Run("past sweep", func(t *testing.T) {
		sampler.reset()
		client.fetchRecentPastScoreboardEvents(context.Background(), "laliga", now)
		assertBoundedProbeConcurrency(t, sampler)
		if got := sampler.requests.Load(); got != 14 {
			t.Fatalf("expected 14 past probes, got %d", got)
		}
	})

	t.Run("upcoming sweep", func(t *testing.T) {
		sampler.reset()
		client.fetchUpcomingScoreboardEvents(context.Background(), "laliga", now)
		assertBoundedProbeConcurrency(t, sampler)
		if got := sampler.requests.Load(); got != 14 {
			t.Fatalf("expected 14 upcoming probes, got %d", got)
		}
	})

	t.Run("two leagues at once", func(t *testing.T) {
		sampler.reset()
		var wg sync.WaitGroup
		for _, leagueCode := range []string{"esp.1", "eng.1"} {
			wg.Add(1)
			go func(code string) {
				defer wg.Done()
				client.fetchRecentPastScoreboardEvents(context.Background(), code, now)
			}(leagueCode)
		}
		wg.Wait()
		assertBoundedProbeConcurrency(t, sampler)
		if got := sampler.requests.Load(); got != 28 {
			t.Fatalf("expected 28 probes for two leagues, got %d", got)
		}
	})
}

// assertBoundedProbeConcurrency checks the hard limit of maxConcurrentRequests
// in-flight probes and also that the sweep actually overlaps requests instead
// of running serially.
func assertBoundedProbeConcurrency(t *testing.T, sampler *probeConcurrencySampler) {
	t.Helper()
	assertBoundedConcurrency(t, sampler, maxConcurrentRequests)
}

// assertBoundedConcurrency checks that the server never saw more than limit
// requests in flight, and that the traffic overlapped at all so the limit is
// actually exercised rather than avoided by a serial caller.
func assertBoundedConcurrency(t *testing.T, sampler *probeConcurrencySampler, limit int32) {
	t.Helper()
	observed := sampler.max.Load()
	t.Logf("max in-flight requests observed: %d (limit %d)", observed, limit)
	if observed > limit {
		t.Fatalf("observed %d concurrent requests, want at most %d", observed, limit)
	}
	if observed < 2 {
		t.Fatalf("observed %d concurrent requests, want the callers to overlap at least 2", observed)
	}
}

// passthroughTransport delegates to a base transport so a caller-supplied
// http.Client reaches the test server, while giving the test a comparable
// pointer to prove NewClient did not mutate the caller's client.
type passthroughTransport struct {
	base http.RoundTripper
}

func (p *passthroughTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return p.base.RoundTrip(req)
}

// TestClient_HTTPBudget_BoundsConcurrentRequests guards S12: the HTTP budget is
// enforced at the transport, so every caller path shares one cap. Before the
// transport-level budget, only the scoreboard probe fan-out was bounded; the
// main scoreboard request of each FetchMatches call was not, so parallel
// FetchMatches calls (exactly what internal/bar's league fan-out does) could
// exceed the intended cap.
func TestClient_HTTPBudget_BoundsConcurrentRequests(t *testing.T) {
	sampler := &probeConcurrencySampler{delay: 10 * time.Millisecond}
	server := httptest.NewServer(http.HandlerFunc(sampler.handler))
	defer server.Close()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	leagues := []string{
		"laliga", "premier", "seriea", "bundesliga",
		"ligue1", "champions", "colombia", "mls",
	}

	newClient := func(extra ...Option) *Client {
		opts := []Option{
			WithBaseURL(server.URL),
			WithCacheDir(t.TempDir()),
			WithRetryBackoff(func() time.Duration { return 0 }),
			WithNow(func() time.Time { return now }),
		}
		return NewClient(append(opts, extra...)...)
	}

	fetchAll := func(client *Client) {
		var wg sync.WaitGroup
		for _, leagueKey := range leagues {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				if _, err := client.FetchMatches(context.Background(), key); err != nil {
					t.Errorf("FetchMatches(%q) failed: %v", key, err)
				}
			}(leagueKey)
		}
		wg.Wait()
	}

	t.Run("parallel FetchMatches", func(t *testing.T) {
		sampler.reset()
		fetchAll(newClient())
		assertBoundedConcurrency(t, sampler, maxConcurrentRequests)
	})

	t.Run("caller-supplied http client", func(t *testing.T) {
		sampler.reset()
		originalTransport := &passthroughTransport{base: http.DefaultTransport}
		custom := &http.Client{Transport: originalTransport}

		fetchAll(newClient(WithHTTPClient(custom)))

		assertBoundedConcurrency(t, sampler, maxConcurrentRequests)
		if custom.Transport != originalTransport {
			t.Fatalf("WithHTTPClient mutated the caller's client transport; the budget must be applied to a copy")
		}
	})
}

// TestDerivedProbeDeadline pins the S13 arithmetic: the cold-cache sweep
// deadline is max(8s, 2s * ceil(days / budget)), clamped at 30s. Values are
// worked examples, not recomputed from the implementation.
func TestDerivedProbeDeadline(t *testing.T) {
	cases := []struct {
		name        string
		days        int
		concurrency int
		want        time.Duration
	}{
		{"14 days on the default budget stays on the floor", 14, 4, 8 * time.Second},
		{"14 days on a narrow budget scales per batch", 14, 2, 14 * time.Second},
		{"a wide budget keeps the floor", 14, 8, 8 * time.Second},
		{"40 days exceed the floor", 40, 4, 20 * time.Second},
		{"60 days reach the ceiling", 60, 4, 30 * time.Second},
		{"a very large sweep stays at the ceiling", 200, 4, 30 * time.Second},
		{"no days keeps the floor", 0, 4, 8 * time.Second},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := derivedProbeDeadline(tc.days, tc.concurrency); got != tc.want {
				t.Fatalf("derivedProbeDeadline(%d, %d) = %v, want %v", tc.days, tc.concurrency, got, tc.want)
			}
		})
	}
}

// TestClient_WithProbeBudget guards S13's override: an explicit budget wins over
// the derived value, is still clamped to the ceiling, and a non-positive value
// keeps the derived deadline. With probeDays=14 and the 4-request budget the
// derived value is the 8s floor.
func TestClient_WithProbeBudget(t *testing.T) {
	const derived = 8 * time.Second

	cases := []struct {
		name   string
		budget time.Duration
		want   time.Duration
	}{
		{"unset derives from the sweep", 0, derived},
		{"explicit override wins even under the floor", 3 * time.Second, 3 * time.Second},
		{"explicit override is still capped", 45 * time.Second, 30 * time.Second},
		{"non-positive override is ignored", -1, derived},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient(
				WithBaseURL("http://espn.invalid"),
				WithCacheDir(t.TempDir()),
				WithProbeBudget(tc.budget),
			)
			if got := client.probeTimeout(probeDays); got != tc.want {
				t.Fatalf("probeTimeout(%d) = %v, want %v", probeDays, got, tc.want)
			}
		})
	}
}

// TestClient_ProbeBudget_TruncatesSlowSweep proves the override reaches the live
// sweep instead of only the accessor: a deadline shorter than the server's
// latency cancels the batch instead of letting all 14 probes finish.
func TestClient_ProbeBudget_TruncatesSlowSweep(t *testing.T) {
	sampler := &probeConcurrencySampler{delay: 150 * time.Millisecond}
	server := httptest.NewServer(http.HandlerFunc(sampler.handler))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
		WithProbeBudget(40*time.Millisecond),
	)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	started := time.Now()
	client.fetchRecentPastScoreboardEvents(context.Background(), "laliga", now)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("sweep took %v; a 40ms budget must cut it short", elapsed)
	}

	if got := sampler.requests.Load(); got >= int32(probeDays) {
		t.Fatalf("expected the 40ms budget to cut the sweep short, got all %d probes", got)
	}
}

// TestClient_UpcomingProbe_SkipsCurrentDay guards against probing the current
// day twice: the default scoreboard already covers it, so the upcoming sweep
// must start at offset 1.
func TestClient_UpcomingProbe_SkipsCurrentDay(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	currentDay := now.In(locale.Location()).Format("20060102")
	nextDay := now.In(locale.Location()).AddDate(0, 0, 1).Format("20060102")

	var mu sync.Mutex
	defaultRequests := 0
	datedRequests := make(map[string]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		mu.Lock()
		dates := r.URL.Query().Get("dates")
		if dates == "" {
			defaultRequests++
		} else {
			datedRequests[dates]++
		}
		mu.Unlock()

		if dates == "" {
			// Only a finished match from a previous day: this forces FetchMatches
			// to look for the next matchday through the upcoming sweep.
			_, _ = fmt.Fprintf(w, `{
				"events": [
					{
						"id": "past1",
						"date": "2026-10-18T23:00Z",
						"competitions": [
							{
								"id": "c1",
								"competitors": [
									{"id": "1", "homeAway": "home", "team": {"displayName": "Team A"}},
									{"id": "2", "homeAway": "away", "team": {"displayName": "Team B"}}
								]
							}
						],
						"status": {"type": {"state": "post", "completed": true}}
					}
				]
			}`)
			return
		}

		_, _ = fmt.Fprint(w, `{"events": []}`)
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithNow(func() time.Time { return now }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	if _, err := client.FetchMatches(context.Background(), "laliga"); err != nil {
		t.Fatalf("FetchMatches failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if defaultRequests != 1 {
		t.Fatalf("expected the default scoreboard to be requested once, got %d", defaultRequests)
	}
	if got := datedRequests[currentDay]; got != 0 {
		t.Fatalf("current day %s was probed %d extra time(s); the default scoreboard already covers it", currentDay, got)
	}
	if got := datedRequests[nextDay]; got != 1 {
		t.Fatalf("expected the upcoming sweep to probe %s exactly once, got %d", nextDay, got)
	}

	pastProbes, upcomingProbes := 0, 0
	for _, count := range datedRequests {
		if count != 1 {
			t.Fatalf("expected every probed day to be requested exactly once, got a duplicate: %+v", datedRequests)
		}
	}
	for date := range datedRequests {
		parsed, err := time.Parse("20060102", date)
		if err != nil {
			t.Fatalf("unexpected dates value %q: %v", date, err)
		}
		if parsed.Before(now.Truncate(24 * time.Hour)) {
			pastProbes++
		} else {
			upcomingProbes++
		}
	}
	if pastProbes != 14 || upcomingProbes != 14 {
		t.Fatalf("expected 14 past + 14 upcoming probes, got %d past + %d upcoming", pastProbes, upcomingProbes)
	}
}

// TestClient_ScoreboardProbes_RespectContextCancellation guards the limiter
// against deadlocks: a cancelled context must release waiting probes and return
// instead of blocking on the remaining offsets.
func TestClient_ScoreboardProbes_RespectContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold every probe in flight until the caller cancels it.
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		client.fetchRecentPastScoreboardEvents(ctx, "laliga", now)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("probe sweep did not return after context cancellation")
	}
}

// roundTripFunc adapts a function into an http.RoundTripper so tests can
// intercept requests without touching the network.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// readScoreboardFixture loads a recorded ESPN scoreboard response. The fixture
// was captured live from
// https://site.api.espn.com/apis/site/v2/sports/soccer/eng.1/scoreboard?lang=es
// and both events carry broadcasts[] and geoBroadcasts[] metadata.
func readScoreboardFixture(t *testing.T) []byte {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("testdata", "scoreboard_eng1_broadcasts.json"))
	if err != nil {
		t.Fatalf("failed to read recorded fixture: %v", err)
	}
	return fixture
}

// TestClient_FetchMatches_BroadcastsFromRecordedFixture guards that ESPN
// broadcast metadata survives parsing and reaches model.Match.Broadcasts.
// The fixture has two distinct events; each broadcast name is reported by both
// broadcasts[].names and geoBroadcasts[].media.shortName, so the assertions
// also cover de-duplication.
func TestClient_FetchMatches_BroadcastsFromRecordedFixture(t *testing.T) {
	fixture := readScoreboardFixture(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	fixedNow := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "premier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches from fixture, got %d", len(matches))
	}

	want := map[string][]string{
		"401878771": {"USA Net", "Universo"},
		"401879262": {"Peacock"},
	}
	for _, m := range matches {
		expected, ok := want[m.ID]
		if !ok {
			t.Fatalf("unexpected match id %s", m.ID)
		}
		if len(m.Broadcasts) != len(expected) {
			t.Fatalf("match %s: expected broadcasts %v, got %v", m.ID, expected, m.Broadcasts)
		}
		for i := range expected {
			if m.Broadcasts[i] != expected[i] {
				t.Errorf("match %s broadcast[%d]: expected %q, got %q", m.ID, i, expected[i], m.Broadcasts[i])
			}
		}
	}
}

// TestClient_FetchMatches_StatusCode guards that the language-invariant ESPN
// status type name reaches model.MatchStatus.Code, so presentation layers do
// not have to infer halftime or full time from localized feed text.
func TestClient_FetchMatches_StatusCode(t *testing.T) {
	t.Run("recorded fixture full time", func(t *testing.T) {
		fixture := readScoreboardFixture(t)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fixture)
		}))
		defer server.Close()

		fixedNow := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
		client := NewClient(
			WithBaseURL(server.URL),
			WithCacheDir(t.TempDir()),
			WithNow(func() time.Time { return fixedNow }),
			WithRetryBackoff(func() time.Duration { return 0 }),
		)

		matches, err := client.FetchMatches(context.Background(), "premier")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("expected 2 matches from fixture, got %d", len(matches))
		}
		for _, m := range matches {
			if m.Status.Code != "STATUS_FULL_TIME" {
				t.Errorf("match %s: expected status code STATUS_FULL_TIME, got %q", m.ID, m.Status.Code)
			}
		}
	})

	t.Run("halftime with Spanish feed text", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{
				"events": [
					{
						"id": "2001",
						"date": "2026-10-13T19:00Z",
						"competitions": [
							{
								"id": "2001",
								"competitors": [
									{"id": "10", "homeAway": "home", "score": "0", "team": {"id": "10", "name": "Barcelona", "displayName": "FC Barcelona"}},
									{"id": "20", "homeAway": "away", "score": "0", "team": {"id": "20", "name": "Real Madrid", "displayName": "Real Madrid"}}
								],
								"status": {
									"type": {
										"name": "STATUS_HALFTIME",
										"state": "in",
										"completed": false,
										"description": "Entretiempo",
										"detail": "Entretiempo"
									}
								}
							}
						]
					}
				]
			}`)
		}))
		defer server.Close()

		fixedNow := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
		client := NewClient(
			WithBaseURL(server.URL),
			WithCacheDir(t.TempDir()),
			WithNow(func() time.Time { return fixedNow }),
			WithRetryBackoff(func() time.Duration { return 0 }),
		)

		matches, err := client.FetchMatches(context.Background(), "laliga")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		status := matches[0].Status
		if status.Code != "STATUS_HALFTIME" {
			t.Errorf("expected status code STATUS_HALFTIME, got %q", status.Code)
		}
		if status.Detail != "Entretiempo" {
			t.Errorf("expected detail to stay untouched, got %q", status.Detail)
		}
	})
}

// TestClient_ScoreboardQueryParams_PreserveBroadcasts is the regression guard
// for the erased-broadcasts bug: ESPN drops the broadcasts[]/geoBroadcasts[]
// arrays when the scoreboard is requested with region=co combined with lang=es.
// The client must omit the region param entirely (region alone is what triggers
// the erase when paired with lang=es) while keeping lang=es so the user-facing
// status strings stay in Spanish.
// The baseURL deliberately contains "espn.com" so the query-param branch runs,
// while a custom transport serves the recorded fixture locally.
func TestClient_ScoreboardQueryParams_PreserveBroadcasts(t *testing.T) {
	fixture := readScoreboardFixture(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	var mu sync.Mutex
	var mainQuery string

	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.RawQuery, "dates=") {
			mu.Lock()
			mainQuery = req.URL.RawQuery
			mu.Unlock()
		}
		req.URL.Scheme = "http"
		req.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(req)
	})}

	fixedNow := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
	client := NewClient(
		WithBaseURL("https://site.api.espn.com/apis/site/v2/sports/soccer"),
		WithHTTPClient(httpClient),
		WithCacheDir(t.TempDir()),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "premier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	q := mainQuery
	mu.Unlock()

	if q == "" {
		t.Fatalf("expected to observe the main scoreboard request query")
	}
	if !strings.Contains(q, "lang=es") {
		t.Errorf("expected lang=es in ESPN scoreboard query (Spanish status strings), got %q", q)
	}
	if strings.Contains(q, "region=") {
		t.Errorf("region param must not be sent: region=co/ar/mx combined with lang=es erases broadcasts, got %q", q)
	}

	if len(matches) == 0 {
		t.Fatalf("expected matches parsed from fixture")
	}
	for _, m := range matches {
		if len(m.Broadcasts) == 0 {
			t.Errorf("match %s lost its ESPN broadcasts", m.ID)
		}
	}
}

func TestNormalizeTeamDisplayName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Internazionale", "Inter de Milán"},
		{"internazionale", "Inter de Milán"},
		{"FC Internazionale Milano", "Inter de Milán"},
		{"Inter Milan", "Inter de Milán"},
		{"AC Milan", "AC Milan"},
		{"Real Madrid", "Real Madrid"},
		{"", ""},
	}
	for _, tc := range cases {
		got := normalizeTeamDisplayName(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeTeamDisplayName(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

// TestClient_CacheSchemaVersion pins the on-disk schema marker shared by every
// persisted ESPN cache: an entry written by the current schema version is
// served from disk without touching the network, while an entry written by a
// different version (or none at all) is treated as a miss and refetched.
func TestClient_CacheSchemaVersion(t *testing.T) {
	fixedNow := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cachedAt := fixedNow.Format(time.RFC3339)

	// versionField renders the schema_version member, or nothing when the
	// stored entry predates the field entirely.
	versionField := func(version string) string {
		if version == "" {
			return ""
		}
		return fmt.Sprintf(`"schema_version":%s,`, version)
	}

	// A scheduled match on the fixed "today" keeps the scoreboard path from
	// firing the upcoming/past probe fan-out, so request counts stay exact.
	scheduledScoreboard := `{
		"events": [{
			"id": "today1",
			"date": "2026-10-16T20:00Z",
			"name": "A vs B",
			"competitions": [{
				"status": {"type": {"name": "STATUS_SCHEDULED", "description": "Scheduled", "detail": "", "state": "pre", "completed": false}},
				"competitors": [
					{"id": "1", "homeAway": "home", "team": {"displayName": "A"}},
					{"id": "2", "homeAway": "away", "team": {"displayName": "B"}}
				]
			}]
		}]
	}`

	cases := []struct {
		name string
		file string
		body func(version string) string
		call func(ctx context.Context, c *Client) error
		// wantFresh is the exact upstream request count when the entry is
		// served from disk.
		wantFresh int
		// wantStale is the minimum upstream request count when the entry is
		// treated as a miss.
		wantStale int
	}{
		{
			name: "matches",
			file: "matches_premier.json",
			body: func(version string) string {
				return fmt.Sprintf(`{%s"cached_at":%q,"matches":[{"id":"m1","title":"A vs B"}]}`, versionField(version), cachedAt)
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.FetchMatches(ctx, "premier")
				return err
			},
			wantFresh: 0,
			wantStale: 1,
		},
		{
			name: "past events",
			file: "past_matches_premier.json",
			body: func(version string) string {
				return fmt.Sprintf(`{%s"cached_at":%q,"events":[{"id":"past1","date":"2026-10-15T20:00Z"}]}`, versionField(version), cachedAt)
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.FetchMatches(ctx, "premier")
				return err
			},
			// FetchMatches always asks the scoreboard once; a valid past
			// cache is what suppresses the extra day probes.
			wantFresh: 1,
			wantStale: 2,
		},
		{
			name: "summary",
			file: "summary-ev1.json",
			body: func(version string) string {
				return fmt.Sprintf(`{%s"cached_at":%q,"stats":{"homeShots":"10"},"lineups":null,"referee":"Ref A","attendance":1000}`, versionField(version), cachedAt)
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.FetchMatchDetails(ctx, "premier", "ev1")
				return err
			},
			wantFresh: 0,
			wantStale: 1,
		},
		{
			name: "standings",
			file: "standings_premier.json",
			body: func(version string) string {
				return fmt.Sprintf(`{%s"cached_at":%q,"standings":{"leagueKey":"premier"}}`, versionField(version), cachedAt)
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.FetchStandings(ctx, "premier")
				return err
			},
			wantFresh: 0,
			wantStale: 1,
		},
		{
			name: "statistics",
			file: "statistics_premier.json",
			body: func(version string) string {
				return fmt.Sprintf(`{%s"cached_at":%q,"statistics":{"leagueKey":"premier"}}`, versionField(version), cachedAt)
			},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.FetchStatistics(ctx, "premier")
				return err
			},
			wantFresh: 0,
			wantStale: 1,
		},
	}

	versions := []struct {
		name  string
		value string
		fresh bool
	}{
		{name: "current version", value: fmt.Sprintf("%d", cacheSchemaVersion), fresh: true},
		{name: "stale version", value: "0", fresh: false},
		{name: "absent version", value: "", fresh: false},
	}

	for _, tc := range cases {
		for _, v := range versions {
			t.Run(tc.name+"/"+v.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.body(v.value)), 0o644); err != nil {
					t.Fatalf("seed cache file %s: %v", tc.file, err)
				}

				var requests int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt64(&requests, 1)
					w.Header().Set("Content-Type", "application/json")
					if strings.Contains(r.URL.Path, "scoreboard") {
						_, _ = fmt.Fprint(w, scheduledScoreboard)
						return
					}
					_, _ = fmt.Fprint(w, `{}`)
				}))
				defer server.Close()

				client := NewClient(
					WithBaseURL(server.URL),
					WithCacheDir(dir),
					WithNow(func() time.Time { return fixedNow }),
					WithCacheTTL(time.Hour),
					WithPastCacheTTL(time.Hour),
				)

				if err := tc.call(context.Background(), client); err != nil {
					t.Fatalf("%s: unexpected error: %v", tc.name, err)
				}

				got := int(atomic.LoadInt64(&requests))
				if v.fresh {
					if got != tc.wantFresh {
						t.Fatalf("%s with the current schema version: upstream requests = %d, want %d (entry served from disk)", tc.name, got, tc.wantFresh)
					}
					return
				}
				if got < tc.wantStale {
					t.Fatalf("%s with a %s: upstream requests = %d, want at least %d (entry refetched)", tc.name, v.name, got, tc.wantStale)
				}
			})
		}
	}
}
