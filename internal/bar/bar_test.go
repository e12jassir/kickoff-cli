package bar

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type mockClient struct {
	mu              sync.Mutex
	matchesByLeague map[string][]model.Match
	allMatches      []model.Match
	err             error
}

func (m *mockClient) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if matches, ok := m.matchesByLeague[leagueKey]; ok {
		cp := make([]model.Match, len(matches))
		copy(cp, matches)
		return cp, nil
	}
	cp := make([]model.Match, len(m.allMatches))
	copy(cp, m.allMatches)
	return cp, nil
}

func (m *mockClient) SetLeagueMatches(leagueKey string, matches []model.Match) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.matchesByLeague == nil {
		m.matchesByLeague = make(map[string][]model.Match)
	}
	m.matchesByLeague[leagueKey] = matches
}

func (m *mockClient) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	return nil, nil
}

func (m *mockClient) FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*model.MatchDetails, error) {
	return nil, nil
}

func (m *mockClient) FetchStandings(ctx context.Context, leagueKey string) (*model.LeagueStandings, error) {
	return nil, nil
}

func (m *mockClient) FetchStatistics(ctx context.Context, leagueKey string) (*model.LeagueStatistics, error) {
	return nil, nil
}

type mockResolver struct {
	channelsByMatchID map[string][]model.Channel
}

func (r *mockResolver) GetChannels(match model.Match) []model.Channel {
	if r.channelsByMatchID != nil {
		return r.channelsByMatchID[match.ID]
	}
	return nil
}

func sampleLiveMatch() model.Match {
	now := time.Now()
	return model.Match{
		ID:        "live-1",
		Title:     "Real Madrid vs Barcelona",
		LeagueKey: "esp.1",
		HomeTeam: model.Competitor{
			ID:          "rma",
			Name:        "Real Madrid",
			DisplayName: "Real Madrid",
			Score:       "2",
			IsHome:      true,
		},
		AwayTeam: model.Competitor{
			ID:          "bar",
			Name:        "FC Barcelona",
			DisplayName: "Barcelona",
			Score:       "1",
			IsHome:      false,
		},
		Venue:     "Santiago Bernabéu",
		StartTime: now.Add(-72 * time.Minute),
		TimeStr:   "15:00",
		Status: model.MatchStatus{
			State:       "in",
			Detail:      "72'",
			Description: "Segundo Tiempo",
		},
	}
}

func sampleSecondLiveMatch() model.Match {
	now := time.Now()
	return model.Match{
		ID:        "live-2",
		Title:     "Arsenal vs Chelsea",
		LeagueKey: "eng.1",
		HomeTeam: model.Competitor{
			ID:          "ars",
			Name:        "Arsenal",
			DisplayName: "Arsenal",
			Score:       "0",
			IsHome:      true,
		},
		AwayTeam: model.Competitor{
			ID:          "che",
			Name:        "Chelsea",
			DisplayName: "Chelsea",
			Score:       "0",
			IsHome:      false,
		},
		Venue:     "Emirates Stadium",
		StartTime: now.Add(-15 * time.Minute),
		TimeStr:   "16:30",
		Status: model.MatchStatus{
			State:       "in",
			Detail:      "15'",
			Description: "Primer Tiempo",
		},
	}
}

func sampleUpcomingMatch() model.Match {
	t := time.Date(2026, 10, 17, 15, 4, 0, 0, time.UTC)
	return model.Match{
		ID:        "upcoming-1",
		Title:     "Real Madrid vs Barcelona",
		LeagueKey: "esp.1",
		HomeTeam: model.Competitor{
			DisplayName: "Real Madrid",
			IsHome:      true,
		},
		AwayTeam: model.Competitor{
			DisplayName: "FC Barcelona",
			IsHome:      false,
		},
		Venue:     "Santiago Bernabéu",
		StartTime: t,
		TimeStr:   "15:04",
		Status: model.MatchStatus{
			State:       "pre",
			Description: "Programado",
		},
	}
}

func sampleFinishedMatch() model.Match {
	t := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	return model.Match{
		ID:        "finished-1",
		Title:     "Real Madrid vs Barcelona",
		LeagueKey: "esp.1",
		HomeTeam: model.Competitor{
			DisplayName: "Real Madrid",
			Score:       "2",
			IsHome:      true,
		},
		AwayTeam: model.Competitor{
			DisplayName: "Barcelona",
			Score:       "1",
			IsHome:      false,
		},
		Venue:     "Santiago Bernabéu",
		StartTime: t,
		TimeStr:   "12:00",
		Status: model.MatchStatus{
			State:       "post",
			Completed:   true,
			Description: "Final",
			Detail:      "FT",
		},
	}
}

func TestFormatMatches_Plain(t *testing.T) {
	// 1. Single live match
	m1 := sampleLiveMatch()
	out, err := FormatMatches([]model.Match{m1}, Options{Format: FormatPlain}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "⚽ RMA 2-1 BAR (72')"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}

	// 2. Multiple live matches joined with " | "
	m2 := sampleSecondLiveMatch()
	outMultiple, err := FormatMatches([]model.Match{m1, m2}, Options{Format: FormatPlain}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedMultiple := "⚽ RMA 2-1 BAR (72') | ⚽ ARS 0-0 CHE (15')"
	if outMultiple != expectedMultiple {
		t.Errorf("got %q, expected %q", outMultiple, expectedMultiple)
	}

	// 3. No live matches, upcoming match displayed
	up := sampleUpcomingMatch()
	outUpcoming, err := FormatMatches([]model.Match{up}, Options{Format: FormatPlain}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedUpcoming := "⚽ RMA vs BAR (15:04)"
	if outUpcoming != expectedUpcoming {
		t.Errorf("got %q, expected %q", outUpcoming, expectedUpcoming)
	}

	// 4. Finished match
	fin := sampleFinishedMatch()
	outFin, err := FormatMatches([]model.Match{fin}, Options{Format: FormatPlain}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedFin := "⚽ RMA 2-1 BAR (FT)"
	if outFin != expectedFin {
		t.Errorf("got %q, expected %q", outFin, expectedFin)
	}

	// 5. No matches at all
	outEmpty, err := FormatMatches(nil, Options{Format: FormatPlain}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outEmpty != "" {
		t.Errorf("got %q, expected empty string", outEmpty)
	}
}

func TestFormatMatches_NoEmoji(t *testing.T) {
	m1 := sampleLiveMatch()
	out, err := FormatMatches([]model.Match{m1}, Options{Format: FormatPlain, NoEmoji: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "[*] RMA 2-1 BAR (72')"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}

	// Upcoming match with noEmoji
	up := sampleUpcomingMatch()
	outUp, err := FormatMatches([]model.Match{up}, Options{Format: FormatPlain, NoEmoji: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedUp := "[*] RMA vs BAR (15:04)"
	if outUp != expectedUp {
		t.Errorf("got %q, expected %q", outUp, expectedUp)
	}
}

func TestFormatMatches_Starship(t *testing.T) {
	m1 := sampleLiveMatch()
	m2 := sampleSecondLiveMatch()

	// Should produce compact single-match output even if multiple live matches exist
	out, err := FormatMatches([]model.Match{m1, m2}, Options{Format: FormatStarship}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "⚽ RMA 2-1 BAR 72'"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}

	// Prompt alias
	outPrompt, err := FormatMatches([]model.Match{m1}, Options{Format: FormatPrompt}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outPrompt != expected {
		t.Errorf("got %q, expected %q", outPrompt, expected)
	}

	// Upcoming
	up := sampleUpcomingMatch()
	outUp, err := FormatMatches([]model.Match{up}, Options{Format: FormatStarship}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedUp := "⚽ RMA vs BAR 15:04"
	if outUp != expectedUp {
		t.Errorf("got %q, expected %q", outUp, expectedUp)
	}

	// Finished
	fin := sampleFinishedMatch()
	outFin, err := FormatMatches([]model.Match{fin}, Options{Format: FormatStarship}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedFin := "⚽ RMA 2-1 BAR FT"
	if outFin != expectedFin {
		t.Errorf("got %q, expected %q", outFin, expectedFin)
	}

	// Empty
	outEmpty, err := FormatMatches(nil, Options{Format: FormatStarship}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outEmpty != "" {
		t.Errorf("got %q, expected empty string", outEmpty)
	}
}

func TestFormatMatches_Tmux(t *testing.T) {
	m1 := sampleLiveMatch()
	out, err := FormatMatches([]model.Match{m1}, Options{Format: FormatTmux}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "#[fg=green,bold]⚽ RMA 2-1 BAR #[fg=yellow](72')#[default]"
	if out != expected {
		t.Errorf("got %q, expected %q", out, expected)
	}

	// Multiple live
	m2 := sampleSecondLiveMatch()
	outMult, err := FormatMatches([]model.Match{m1, m2}, Options{Format: FormatTmux}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedMult := "#[fg=green,bold]⚽ RMA 2-1 BAR #[fg=yellow](72')#[default] | #[fg=green,bold]⚽ ARS 0-0 CHE #[fg=yellow](15')#[default]"
	if outMult != expectedMult {
		t.Errorf("got %q, expected %q", outMult, expectedMult)
	}

	// Upcoming
	up := sampleUpcomingMatch()
	outUp, err := FormatMatches([]model.Match{up}, Options{Format: FormatTmux}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedUp := "#[fg=blue,bold]⚽ RMA vs BAR #[fg=yellow](15:04)#[default]"
	if outUp != expectedUp {
		t.Errorf("got %q, expected %q", outUp, expectedUp)
	}

	// Empty
	outEmpty, err := FormatMatches(nil, Options{Format: FormatTmux}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outEmpty != "" {
		t.Errorf("got %q, expected empty string", outEmpty)
	}
}

func TestFormatMatches_Waybar(t *testing.T) {
	m1 := sampleLiveMatch()
	resolver := &mockResolver{
		channelsByMatchID: map[string][]model.Channel{
			"live-1": {
				{DisplayName: "ESPN", Name: "espn"},
				{DisplayName: "Movistar+", Name: "movistar"},
			},
		},
	}

	// Live match with resolved channels
	outStr, err := FormatMatches([]model.Match{m1}, Options{Format: FormatWaybar}, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wOut WaybarOutput
	if err := json.Unmarshal([]byte(outStr), &wOut); err != nil {
		t.Fatalf("invalid waybar json: %v\n%s", err, outStr)
	}

	if wOut.Class != "live" || wOut.Alt != "live" {
		t.Errorf("expected class 'live', got %q", wOut.Class)
	}
	if wOut.Text != "⚽ RMA 2-1 BAR (72')" {
		t.Errorf("unexpected text: %q", wOut.Text)
	}
	if !strings.Contains(wOut.Tooltip, "La Liga") {
		t.Errorf("tooltip missing competition name: %q", wOut.Tooltip)
	}
	if !strings.Contains(wOut.Tooltip, "Santiago Bernabéu") {
		t.Errorf("tooltip missing venue: %q", wOut.Tooltip)
	}
	if !strings.Contains(wOut.Tooltip, "ESPN, Movistar+") {
		t.Errorf("tooltip missing resolved channels: %q", wOut.Tooltip)
	}

	// Upcoming match
	up := sampleUpcomingMatch()
	outUpStr, err := FormatMatches([]model.Match{up}, Options{Format: FormatWaybar}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var wUp WaybarOutput
	if err := json.Unmarshal([]byte(outUpStr), &wUp); err != nil {
		t.Fatalf("invalid waybar json: %v", err)
	}
	if wUp.Class != "upcoming" || wUp.Alt != "upcoming" {
		t.Errorf("expected class 'upcoming', got %q", wUp.Class)
	}
	if wUp.Text != "⚽ RMA vs BAR (15:04)" {
		t.Errorf("unexpected text: %q", wUp.Text)
	}

	// Idle state (no matches)
	outEmptyStr, err := FormatMatches(nil, Options{Format: FormatWaybar}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var wEmpty WaybarOutput
	if err := json.Unmarshal([]byte(outEmptyStr), &wEmpty); err != nil {
		t.Fatalf("invalid waybar json: %v", err)
	}
	if wEmpty.Class != "idle" || wEmpty.Alt != "idle" {
		t.Errorf("expected class 'idle', got %q", wEmpty.Class)
	}
	if wEmpty.Text != "" {
		t.Errorf("expected empty text when idle, got %q", wEmpty.Text)
	}
}

func TestFormatMatches_JSON(t *testing.T) {
	m1 := sampleLiveMatch()
	resolver := &mockResolver{
		channelsByMatchID: map[string][]model.Channel{
			"live-1": {
				{DisplayName: "ESPN"},
			},
		},
	}

	outStr, err := FormatMatches([]model.Match{m1}, Options{Format: FormatJSON}, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var jOut JSONOutput
	if err := json.Unmarshal([]byte(outStr), &jOut); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}

	if !jOut.Live {
		t.Errorf("expected live=true, got %v", jOut.Live)
	}
	if jOut.Count != 1 {
		t.Errorf("expected count=1, got %d", jOut.Count)
	}
	if len(jOut.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(jOut.Matches))
	}
	match := jOut.Matches[0]
	if match.ID != "live-1" || match.Home != "Real Madrid" || match.HomeScore != "2" {
		t.Errorf("unexpected match fields: %+v", match)
	}
	if len(match.Channels) != 1 || match.Channels[0] != "ESPN" {
		t.Errorf("unexpected channels: %+v", match.Channels)
	}
}

func TestFetchAndFilter(t *testing.T) {
	client := &mockClient{
		matchesByLeague: map[string][]model.Match{
			"esp.1": {
				sampleLiveMatch(),
				sampleUpcomingMatch(),
			},
			"eng.1": {
				sampleSecondLiveMatch(),
			},
		},
	}

	ctx := context.Background()

	// 1. Filter by league
	matches, err := FetchAndFilter(ctx, client, Options{League: "esp.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for esp.1, got %d", len(matches))
	}

	// 2. Filter by team
	matchesChelsea, err := FetchAndFilter(ctx, client, Options{League: "eng.1", Team: "Chelsea"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matchesChelsea) != 1 || matchesChelsea[0].AwayTeam.Name != "Chelsea" {
		t.Errorf("expected Chelsea match, got %v", matchesChelsea)
	}

	// 3. Filter by team not in fixtures
	matchesNotFound, err := FetchAndFilter(ctx, client, Options{League: "esp.1", Team: "Juventus"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matchesNotFound) != 0 {
		t.Errorf("expected 0 matches for Juventus in esp.1, got %d", len(matchesNotFound))
	}
}

func TestShortTeam(t *testing.T) {
	tests := []struct {
		input    model.Competitor
		expected string
	}{
		{model.Competitor{DisplayName: "RMA"}, "RMA"},
		{model.Competitor{DisplayName: "Real Madrid"}, "RMA"},
		{model.Competitor{DisplayName: "FC Barcelona"}, "BAR"},
		{model.Competitor{DisplayName: "Manchester City"}, "MCI"},
		{model.Competitor{DisplayName: "Paris Saint-Germain"}, "PSG"},
		{model.Competitor{DisplayName: "Boca Juniors"}, "BOC"},
		{model.Competitor{DisplayName: "River Plate"}, "RIV"},
		{model.Competitor{DisplayName: "Liverpool FC"}, "LIV"},
		{model.Competitor{DisplayName: "Unknown Club"}, "UNK"},
	}

	for _, tt := range tests {
		got := ShortTeam(tt.input)
		if got != tt.expected {
			t.Errorf("ShortTeam(%+v) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		status   model.MatchStatus
		expected string
	}{
		{model.MatchStatus{State: "in", Detail: "72'"}, "72'"},
		{model.MatchStatus{State: "in", Detail: "45"}, "45'"},
		{model.MatchStatus{State: "in", Detail: "HT"}, "HT"},
		{model.MatchStatus{State: "in", Description: "Halftime"}, "HT"},
		{model.MatchStatus{State: "post", Completed: true}, "FT"},
		{model.MatchStatus{State: "pre"}, "PRE"},
	}

	for _, tt := range tests {
		got := FormatClock(tt.status)
		if got != tt.expected {
			t.Errorf("FormatClock(%+v) = %q, expected %q", tt.status, got, tt.expected)
		}
	}
}

func TestFormatGoalNotification(t *testing.T) {
	m := sampleLiveMatch()
	ev := &model.MatchEvent{
		PlayerName: "Vinícius Jr.",
		AssistName: "Jude Bellingham",
		Clock:      "72'",
		IsScoring:  true,
	}

	title, body := FormatGoalNotification(m, "2", "1", ev)
	if strings.Contains(title, "⚽") {
		t.Errorf("title should not contain emojis, got: %q", title)
	}
	if !strings.Contains(title, "Vinícius Jr.") || !strings.Contains(title, "72'") {
		t.Errorf("title should contain goal scorer and clock, got: %q", title)
	}
	if !strings.Contains(body, "Real Madrid 2 - 1 Barcelona") {
		t.Errorf("body should contain match score, got: %q", body)
	}
	if !strings.Contains(body, "Asistencia: Jude Bellingham") {
		t.Errorf("body should contain assist provider, got: %q", body)
	}

	// Penalty goal
	evPen := &model.MatchEvent{
		PlayerName: "Kylian Mbappé",
		Clock:      "88'",
		IsScoring:  true,
		IsPenalty:  true,
	}
	titlePen, bodyPen := FormatGoalNotification(m, "3", "1", evPen)
	if strings.Contains(titlePen, "⚽") {
		t.Errorf("penalty title should not contain emojis, got: %q", titlePen)
	}
	if !strings.Contains(titlePen, "Kylian Mbappé") || !strings.Contains(titlePen, "pen.") {
		t.Errorf("penalty title should contain pen. indication, got: %q", titlePen)
	}
	if !strings.Contains(bodyPen, "Asistencia: Penal") {
		t.Errorf("penalty body should indicate penalty, got: %q", bodyPen)
	}

	// Without event
	titleNoEv, bodyNoEv := FormatGoalNotification(m, "2", "1", nil)
	if strings.Contains(titleNoEv, "⚽") {
		t.Errorf("no-event title should not contain emojis, got: %q", titleNoEv)
	}
	if !strings.Contains(titleNoEv, "¡GOL!") {
		t.Errorf("unexpected title without event: %q", titleNoEv)
	}
	if !strings.Contains(bodyNoEv, "Real Madrid 2 - 1 Barcelona") {
		t.Errorf("unexpected body without event: %q", bodyNoEv)
	}
}

func TestNewNotifier(t *testing.T) {
	notifier := NewNotifier()
	if notifier == nil {
		t.Fatal("expected non-nil Notifier instance")
	}

	// Should not crash even when invoking
	m := sampleLiveMatch()
	err := notifier.NotifyGoal(m, "2", "1", nil)
	if err != nil {
		t.Errorf("unexpected error from notifier: %v", err)
	}
}

