package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

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
				fmt.Fprint(w, `{
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
				fmt.Fprint(w, `{
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
				fmt.Fprint(w, `{"events": []}`)
			},
			leagueKey:     "seriea",
			expectMatches: 0,
			expectError:   false,
		},
		{
			name: "Corrupted JSON payload returns ErrCorruptedPayload",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"events": [ { broken json ...`)
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
			defer os.RemoveAll(tempDir)

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
	defer os.RemoveAll(tempDir)

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
		CachedAt: cachedTime,
		Matches:  cachedMatches,
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
	defer os.RemoveAll(tempDir)

	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	// Pre-populate summary cache
	sum := cachedSummary{
		CachedAt: cachedTime,
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
		fmt.Fprint(w, `{
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
	defer os.RemoveAll(tempDir)

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
	// 3. Own goal Jugador fallback
	if events[2].PlayerName != "Jugador" || events[2].Clock != "60'" || !events[2].IsOwnGoal || events[2].Type != "owngoal" {
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
		fmt.Fprint(w, `{
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
	defer os.RemoveAll(tempDir)

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
		fmt.Fprint(w, `{
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
	defer os.RemoveAll(tempDir)

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
		fmt.Fprint(w, `{"boxscore": {"teams": []}}`)
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
