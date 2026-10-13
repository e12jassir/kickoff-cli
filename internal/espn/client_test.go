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
