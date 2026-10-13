package espn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
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
			name: "Upstream HTTP 500 error returns error and invalidates cache",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			preCache: func(dir string) {
				// Pre-populate stale cache file to verify invalidation
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

			// If error expected and preCache existed, check that cache file was removed
			if tt.expectError && tt.preCache != nil {
				cachePath := filepath.Join(tempDir, fmt.Sprintf("matches_%s.json", tt.leagueKey))
				if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
					t.Errorf("expected cache file to be invalidated/removed on error")
				}
			}
		})
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
