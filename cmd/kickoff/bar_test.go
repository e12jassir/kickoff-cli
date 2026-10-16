package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/bar"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

type barMockClient struct {
	mu              sync.Mutex
	matchesByLeague map[string][]model.Match
	allMatches      []model.Match
}

func (m *barMockClient) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if matches, ok := m.matchesByLeague[leagueKey]; ok {
		cp := make([]model.Match, len(matches))
		copy(cp, matches)
		return cp, nil
	}
	cp := make([]model.Match, len(m.allMatches))
	copy(cp, m.allMatches)
	return cp, nil
}

func (m *barMockClient) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	return nil, nil
}

func (m *barMockClient) FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*model.MatchDetails, error) {
	return nil, nil
}

func (m *barMockClient) FetchStandings(ctx context.Context, leagueKey string) (*model.LeagueStandings, error) {
	return nil, nil
}

func (m *barMockClient) FetchStatistics(ctx context.Context, leagueKey string) (*model.LeagueStatistics, error) {
	return nil, nil
}

func sampleBarLiveMatch() model.Match {
	return model.Match{
		ID:        "live-1",
		Title:     "Real Madrid vs FC Barcelona",
		LeagueKey: "esp.1",
		HomeTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "2", IsHome: true},
		AwayTeam:  model.Competitor{DisplayName: "FC Barcelona", Score: "1", IsHome: false},
		StartTime: time.Now(),
		Status:    model.MatchStatus{State: "in", Detail: "72'"},
	}
}

func sampleBarUpcomingMatch() model.Match {
	return model.Match{
		ID:        "up-1",
		Title:     "Arsenal vs Chelsea",
		LeagueKey: "eng.1",
		HomeTeam:  model.Competitor{DisplayName: "Arsenal", IsHome: true},
		AwayTeam:  model.Competitor{DisplayName: "Chelsea", IsHome: false},
		StartTime: time.Now().Add(2 * time.Hour),
		TimeStr:   "18:00",
		Status:    model.MatchStatus{State: "pre"},
	}
}

func TestRunBar_Plain(t *testing.T) {
	client := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}

	var buf bytes.Buffer
	err := RunBar([]string{"-f", "plain"}, client, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	expected := "⚽ RMA 2-1 BAR (72')"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}
}

func TestRunBar_NoEmoji(t *testing.T) {
	client := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}

	var buf bytes.Buffer
	err := RunBar([]string{"-no-emoji"}, client, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	expected := "[*] RMA 2-1 BAR (72')"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}
}

func TestRunBar_Waybar(t *testing.T) {
	client := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}

	var buf bytes.Buffer
	err := RunBar([]string{"-format", "waybar"}, client, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wOut bar.WaybarOutput
	if err := json.Unmarshal(buf.Bytes(), &wOut); err != nil {
		t.Fatalf("invalid waybar json: %v", err)
	}

	if wOut.Class != "live" {
		t.Errorf("expected class live, got %q", wOut.Class)
	}
	if wOut.Text != "⚽ RMA 2-1 BAR (72')" {
		t.Errorf("unexpected text: %q", wOut.Text)
	}
}

func TestRunBar_JSON(t *testing.T) {
	client := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}

	var buf bytes.Buffer
	err := RunBar([]string{"-f", "json"}, client, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var jOut bar.JSONOutput
	if err := json.Unmarshal(buf.Bytes(), &jOut); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	if !jOut.Live || jOut.Count != 1 {
		t.Errorf("unexpected json output: %+v", jOut)
	}
}

func TestRunBar_StarshipAndTmux(t *testing.T) {
	client := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}

	// Starship
	var bufS bytes.Buffer
	if err := RunBar([]string{"-f", "starship"}, client, nil, &bufS); err != nil {
		t.Fatalf("unexpected starship error: %v", err)
	}
	outS := strings.TrimSpace(bufS.String())
	if outS != "⚽ RMA 2-1 BAR 72'" {
		t.Errorf("got %q, expected %q", outS, "⚽ RMA 2-1 BAR 72'")
	}

	// Tmux
	var bufT bytes.Buffer
	if err := RunBar([]string{"-f", "tmux"}, client, nil, &bufT); err != nil {
		t.Fatalf("unexpected tmux error: %v", err)
	}
	outT := strings.TrimSpace(bufT.String())
	if !strings.Contains(outT, "#[fg=green,bold]") || !strings.Contains(outT, "RMA 2-1 BAR") {
		t.Errorf("unexpected tmux output: %q", outT)
	}
}

func TestRunBar_HasLive(t *testing.T) {
	// Case 1: Live match exists
	clientWithLive := &barMockClient{
		allMatches: []model.Match{sampleBarLiveMatch()},
	}
	var buf1 bytes.Buffer
	err1 := RunBar([]string{"-has-live"}, clientWithLive, nil, &buf1)
	if err1 != nil {
		t.Errorf("expected nil error when live matches exist, got: %v", err1)
	}

	// Case 2: Only upcoming matches exist
	clientUpcomingOnly := &barMockClient{
		allMatches: []model.Match{sampleBarUpcomingMatch()},
	}
	var buf2 bytes.Buffer
	err2 := RunBar([]string{"-has-live"}, clientUpcomingOnly, nil, &buf2)
	if !errors.Is(err2, ErrNoLiveMatches) {
		t.Errorf("expected ErrNoLiveMatches, got: %v", err2)
	}
}

func TestRunBar_Filtering(t *testing.T) {
	client := &barMockClient{
		matchesByLeague: map[string][]model.Match{
			"esp.1": {sampleBarLiveMatch()},
			"eng.1": {sampleBarUpcomingMatch()},
		},
	}

	var buf bytes.Buffer
	err := RunBar([]string{"-l", "esp.1", "-t", "barcelona"}, client, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, "BAR") {
		t.Errorf("expected Barcelona match in output, got %q", out)
	}

	// Filter for team not in fixtures
	var bufEmpty bytes.Buffer
	err = RunBar([]string{"-l", "esp.1", "-t", "bayern"}, client, nil, &bufEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bufEmpty.Len() != 0 {
		t.Errorf("expected empty output for filtered out team, got %q", bufEmpty.String())
	}
}

func TestRunBar_Help(t *testing.T) {
	var buf bytes.Buffer
	err := RunBar([]string{"-h"}, nil, nil, &buf)
	if err != nil {
		t.Errorf("expected nil error on -h, got: %v", err)
	}
}
