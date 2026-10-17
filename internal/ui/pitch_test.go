package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestFormationParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
		ok       bool
	}{
		{"4-3-3", "4-3-3", []int{4, 3, 3}, true},
		{"4-2-3-1", "4-2-3-1", []int{4, 2, 3, 1}, true},
		{"3-5-2", "3-5-2", []int{3, 5, 2}, true},
		{"4-4-2", "4-4-2", []int{4, 4, 2}, true},
		{"5-3-2", "5-3-2", []int{5, 3, 2}, true},
		{"3-4-3", "3-4-3", []int{3, 4, 3}, true},
		{"1-4-3-3 with GK", "1-4-3-3", []int{4, 3, 3}, true},
		{"hyphen variants", "4–3—3", []int{4, 3, 3}, true},
		{"empty", "", nil, false},
		{"invalid format", "unknown", nil, false},
		{"single number", "433", nil, false},
		{"zeros", "0-0-0", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseFormation(tc.input)
			if ok != tc.ok {
				t.Fatalf("ParseFormation(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if tc.ok && !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("ParseFormation(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestFormationFallback_GroupStartersByPosition(t *testing.T) {
	starters := []model.Player{
		{Jersey: "1", DisplayName: "ter Stegen", Position: "GK"},
		{Jersey: "2", DisplayName: "Cubarsí", Position: "DF"},
		{Jersey: "3", DisplayName: "Balde", Position: "DF"},
		{Jersey: "4", DisplayName: "Araújo", Position: "DF"},
		{Jersey: "23", DisplayName: "Koundé", Position: "DF"},
		{Jersey: "6", DisplayName: "Gavi", Position: "MF"},
		{Jersey: "8", DisplayName: "Pedri", Position: "MF"},
		{Jersey: "21", DisplayName: "de Jong", Position: "MF"},
		{Jersey: "9", DisplayName: "Lewandowski", Position: "FW"},
		{Jersey: "11", DisplayName: "Raphinha", Position: "FW"},
		{Jersey: "19", DisplayName: "Yamal", Position: "FW"},
	}

	lineup := model.TeamLineup{
		Formation: "unknown-formation",
		Starters:  starters,
	}

	lines, formLabel := buildPitchRows(lineup, nil, 48, false)
	if formLabel != "4-3-3" {
		t.Errorf("expected derived formation 4-3-3, got %s", formLabel)
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (GK, DF, MF, FW), got %d", len(lines))
	}
	if len(lines[0]) != 1 || lines[0][0].DisplayName != "ter Stegen" {
		t.Errorf("expected GK line with ter Stegen, got %v", lines[0])
	}
	if len(lines[1]) != 4 {
		t.Errorf("expected 4 defenders, got %d", len(lines[1]))
	}
	if len(lines[2]) != 3 {
		t.Errorf("expected 3 midfielders, got %d", len(lines[2]))
	}
	if len(lines[3]) != 3 {
		t.Errorf("expected 3 forwards, got %d", len(lines[3]))
	}
}

func TestFormationFallback_Default433WhenNoPositions(t *testing.T) {
	var starters []model.Player
	for i := 1; i <= 11; i++ {
		starters = append(starters, model.Player{
			Jersey:      "",
			DisplayName: "Player",
			Position:    "",
		})
	}
	lineup := model.TeamLineup{
		Formation: "",
		Starters:  starters,
	}

	lines, formLabel := buildPitchRows(lineup, nil, 48, false)
	if formLabel != "4-3-3" {
		t.Errorf("expected default 4-3-3, got %s", formLabel)
	}
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}
}

func TestShortNameGeneration(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Robert Lewandowski", "LEW"},
		{"R. Lewandowski", "LEW"},
		{"Lewandowski", "LEW"},
		{"Vinícius Jr.", "VINI"},
		{"Vinícius Jr", "VINI"},
		{"Vinicius Jr.", "VINI"},
		{"Lamine Yamal", "YAM"},
		{"Marc-André ter Stegen", "STEG"},
		{"M. ter Stegen", "STEG"},
		{"Pedri", "PED"},
		{"Kylian Mbappé", "MBAP"},
		{"K. Mbappé", "MBAP"},
		{"Antonio Rüdiger", "RÜD"},
		{"A. Rüdiger", "RÜD"},
		{"Luka Modrić", "MOD"},
		{"L. Modric", "MOD"},
		{"F. Valverde", "VAL"},
		{"J. Bellingham", "BEL"},
		{"P. Cubarsí", "CUB"},
		{"A. Balde", "BAL"},
		{"J. Koundé", "KOU"},
		{"R. Araújo", "ARA"},
		{"Gavi", "GAVI"},
		{"Raphinha", "RAPH"},
		{"T. Courtois", "COUR"},
		{"Erling Haaland", "HAAL"},
		{"Lionel Messi", "MES"},
		{"", "JUG"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := GenerateShortName(tc.input)
			if got != tc.expected {
				t.Errorf("GenerateShortName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
			if len([]rune(got)) < 2 || len([]rune(got)) > 5 {
				t.Errorf("GenerateShortName(%q) length %d out of bounds (2-5)", tc.input, len([]rune(got)))
			}
		})
	}
}

func TestEventMatchingAndBadges(t *testing.T) {
	events := []model.MatchEvent{
		{PlayerName: "R. Lewandowski", IsScoring: true, Type: "goal"},
		{PlayerName: "Gavi", IsYellow: true, Type: "card_yellow"},
		{PlayerName: "A. Rüdiger", IsRedCard: true, Type: "card_red"},
		{PlayerName: "K. Mbappé", IsScoring: true, IsPenalty: true, Type: "penalty"},
	}

	lew := model.Player{Jersey: "9", DisplayName: "Robert Lewandowski"}
	gavi := model.Player{Jersey: "6", DisplayName: "Gavi"}
	rud := model.Player{Jersey: "22", DisplayName: "Antonio Rüdiger"}
	mbap := model.Player{Jersey: "9", DisplayName: "Kylian Mbappé"}
	cleanPlayer := model.Player{Jersey: "8", DisplayName: "Pedri"}

	// Emoji mode
	if badge := PlayerEventBadge(lew, events, false); badge != "⚽" {
		t.Errorf("expected goal badge '⚽' for Lewandowski, got %q", badge)
	}
	if badge := PlayerEventBadge(gavi, events, false); badge != "🟨" {
		t.Errorf("expected yellow card badge '🟨' for Gavi, got %q", badge)
	}
	if badge := PlayerEventBadge(rud, events, false); badge != "🟥" {
		t.Errorf("expected red card badge '🟥' for Rüdiger, got %q", badge)
	}
	if badge := PlayerEventBadge(mbap, events, false); badge != "⚽P" {
		t.Errorf("expected penalty goal badge '⚽P' for Mbappé, got %q", badge)
	}
	if badge := PlayerEventBadge(cleanPlayer, events, false); badge != "" {
		t.Errorf("expected empty badge for Pedri, got %q", badge)
	}

	// No-Emoji mode
	if badge := PlayerEventBadge(lew, events, true); badge != "G" {
		t.Errorf("expected goal badge 'G' in noEmoji for Lewandowski, got %q", badge)
	}
	if badge := PlayerEventBadge(gavi, events, true); badge != "A" {
		t.Errorf("expected yellow badge 'A' in noEmoji for Gavi, got %q", badge)
	}
	if badge := PlayerEventBadge(rud, events, true); badge != "R" {
		t.Errorf("expected red badge 'R' in noEmoji for Rüdiger, got %q", badge)
	}
	if badge := PlayerEventBadge(mbap, events, true); badge != "GP" {
		t.Errorf("expected penalty badge 'GP' in noEmoji for Mbappé, got %q", badge)
	}
}

func TestRenderMiniCard(t *testing.T) {
	p := model.Player{Jersey: "9", DisplayName: "Robert Lewandowski"}

	// Emoji mode card with goal
	linesEmoji := RenderMiniCard(p, "⚽", 10, false)
	if len(linesEmoji) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(linesEmoji))
	}
	if !strings.Contains(linesEmoji[0], "┌") || !strings.Contains(linesEmoji[0], "┐") {
		t.Errorf("expected top border, got %s", linesEmoji[0])
	}
	if !strings.Contains(linesEmoji[1], "#9") || !strings.Contains(linesEmoji[1], "LEW") || !strings.Contains(linesEmoji[1], "⚽") {
		t.Errorf("expected inner content to contain '#9', 'LEW', '⚽', got %s", linesEmoji[1])
	}
	if !strings.Contains(linesEmoji[2], "└") || !strings.Contains(linesEmoji[2], "┘") {
		t.Errorf("expected bottom border, got %s", linesEmoji[2])
	}

	// No-emoji card with goal
	linesNoEmoji := RenderMiniCard(p, "G", 10, true)
	if !strings.Contains(linesNoEmoji[1], "G") {
		t.Errorf("expected 'G' in no-emoji card, got %s", linesNoEmoji[1])
	}

	// Width 8 card
	linesW8 := RenderMiniCard(p, "", 8, false)
	if len(linesW8) != 3 {
		t.Fatalf("expected 3 lines for width 8 card, got %d", len(linesW8))
	}
}

func TestNoEmojiModeTacticalPitch(t *testing.T) {
	match := model.Match{
		ID:        "test-match",
		Venue:     "Santiago Bernabéu",
		Referee:   "Clement Turpin",
		HomeTeam:  model.Competitor{ID: "home-1", DisplayName: "Real Madrid"},
		AwayTeam:  model.Competitor{ID: "away-1", DisplayName: "Barcelona"},
		StartTime: time.Now(),
		Events: []model.MatchEvent{
			{PlayerName: "Vinícius Jr", IsScoring: true, Type: "goal", Clock: "21'", TeamID: "home-1"},
		},
		Lineups: &model.MatchLineups{
			Home: model.TeamLineup{
				Formation: "4-3-3",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "Courtois", Position: "GK"},
					{Jersey: "2", DisplayName: "Carvajal", Position: "DF"},
					{Jersey: "3", DisplayName: "Militão", Position: "DF"},
					{Jersey: "22", DisplayName: "Rüdiger", Position: "DF"},
					{Jersey: "23", DisplayName: "Mendy", Position: "DF"},
					{Jersey: "8", DisplayName: "Valverde", Position: "MF"},
					{Jersey: "14", DisplayName: "Tchouaméni", Position: "MF"},
					{Jersey: "5", DisplayName: "Bellingham", Position: "MF"},
					{Jersey: "11", DisplayName: "Rodrygo", Position: "FW"},
					{Jersey: "9", DisplayName: "Mbappé", Position: "FW"},
					{Jersey: "7", DisplayName: "Vinícius Jr", Position: "FW"},
				},
			},
		},
	}

	// Render in noEmoji mode
	outNoEmoji := RenderTacticalPitch(match, 0, 80, true)

	// ASCII substitutes
	if !strings.Contains(outNoEmoji, "Estadio: Santiago Bernabéu") {
		t.Errorf("expected 'Estadio:' in noEmoji mode, got:\n%s", outNoEmoji)
	}
	if !strings.Contains(outNoEmoji, "Árbitro: Clement Turpin") {
		t.Errorf("expected 'Árbitro:' in noEmoji mode, got:\n%s", outNoEmoji)
	}
	if !strings.Contains(outNoEmoji, "(   o   )") {
		t.Errorf("expected '(   o   )' center circle in noEmoji mode, got:\n%s", outNoEmoji)
	}
	if strings.Contains(outNoEmoji, "🏟️") || strings.Contains(outNoEmoji, "👤") {
		t.Errorf("noEmoji mode must not contain emoji icons")
	}

	// Render in emoji mode
	outEmoji := RenderTacticalPitch(match, 0, 80, false)
	if !strings.Contains(outEmoji, "🏟️") || !strings.Contains(outEmoji, "👤") {
		t.Errorf("emoji mode should contain emoji icons, got:\n%s", outEmoji)
	}
	if !strings.Contains(outEmoji, "(   ●   )") {
		t.Errorf("expected '(   ●   )' center circle in emoji mode, got:\n%s", outEmoji)
	}
}

func TestResponsiveLayout(t *testing.T) {
	match := model.Match{
		ID:        "test-responsive",
		Venue:     "Camp Nou",
		Referee:   "Marciniak",
		HomeTeam:  model.Competitor{ID: "home-1", DisplayName: "Barcelona"},
		AwayTeam:  model.Competitor{ID: "away-1", DisplayName: "Real Madrid"},
		StartTime: time.Now(),
		Lineups: &model.MatchLineups{
			Home: model.TeamLineup{
				Formation: "4-3-3",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "ter Stegen", Position: "GK"},
					{Jersey: "2", DisplayName: "Cubarsí", Position: "DF"},
					{Jersey: "3", DisplayName: "Balde", Position: "DF"},
					{Jersey: "4", DisplayName: "Araújo", Position: "DF"},
					{Jersey: "23", DisplayName: "Koundé", Position: "DF"},
					{Jersey: "6", DisplayName: "Gavi", Position: "MF"},
					{Jersey: "8", DisplayName: "Pedri", Position: "MF"},
					{Jersey: "21", DisplayName: "de Jong", Position: "MF"},
					{Jersey: "9", DisplayName: "Lewandowski", Position: "FW"},
					{Jersey: "11", DisplayName: "Raphinha", Position: "FW"},
					{Jersey: "19", DisplayName: "Yamal", Position: "FW"},
				},
				Bench: []model.Player{
					{Jersey: "13", DisplayName: "Iñaki Peña"},
					{Jersey: "7", DisplayName: "Ferran Torres"},
				},
			},
		},
	}

	// 1. Side-by-Side: width >= 72
	outSideBySide := RenderTacticalPitch(match, 0, 80, false)
	if !strings.Contains(outSideBySide, "PIZARRA TÁCTICA") {
		t.Errorf("expected pitch box header in side-by-side mode")
	}
	if !strings.Contains(outSideBySide, "SUPLENTES Y DETALLES") {
		t.Errorf("expected bench box header in side-by-side mode")
	}

	// Verify that pitch and bench are side-by-side:
	// A line in the body should contain both the pitch outline and bench outline characters
	lines := strings.Split(outSideBySide, "\n")
	foundSideBySideLine := false
	for _, l := range lines {
		if strings.Contains(l, "PIZARRA TÁCTICA") && strings.Contains(l, "SUPLENTES") {
			foundSideBySideLine = true
			break
		}
	}
	if !foundSideBySideLine {
		t.Errorf("expected pitch and bench to be rendered horizontally side-by-side at width 80")
	}

	// 2. Stacked: width < 72
	outStacked := RenderTacticalPitch(match, 0, 60, false)
	if !strings.Contains(outStacked, "PIZARRA TÁCTICA") || !strings.Contains(outStacked, "SUPLENTES Y DETALLES") {
		t.Errorf("expected pitch and bench in stacked mode")
	}

	// In stacked mode, the headers should be on separate lines
	foundSeparated := true
	for _, l := range strings.Split(outStacked, "\n") {
		if strings.Contains(l, "PIZARRA TÁCTICA") && strings.Contains(l, "SUPLENTES") {
			foundSeparated = false
			break
		}
	}
	if !foundSeparated {
		t.Errorf("expected stacked layout at width 60 to have pitch and bench on separate vertical rows")
	}
}

func TestTeamToggleBehavior_Update(t *testing.T) {
	client := &mockClient{}
	catalog := &mockCatalog{}
	player := &mockPlayer{}
	m := NewModel(client, catalog, player)
	m.state = StateBrowsing

	// Set matches and enter modal
	m.matches = []model.Match{
		{
			ID:        "toggle-test-match",
			Title:     "Real Madrid vs Barcelona",
			HomeTeam:  model.Competitor{ID: "home-1", DisplayName: "Real Madrid"},
			AwayTeam:  model.Competitor{ID: "away-1", DisplayName: "Barcelona"},
			Status:    model.MatchStatus{State: "in", Description: "1T 30'"},
			StartTime: time.Now(),
			Lineups: &model.MatchLineups{
				Home: model.TeamLineup{Formation: "4-3-3"},
				Away: model.TeamLineup{Formation: "4-2-3-1"},
			},
		},
	}
	m.matchCursor = 0
	m.leagueTab = TabPartidos

	// Enter modal -> StateChannelSelect, modalTab starts at 0, lineupTeam starts at 0
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect, got %d", m.state)
	}
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam 0 on enter modal, got %d", m.LineupTeam())
	}

	// Switch to Tab 2 (Alineaciones) via '3'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = newM.(Model)
	if m.modalTab != 2 {
		t.Fatalf("expected modalTab 2, got %d", m.modalTab)
	}
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam 0, got %d", m.LineupTeam())
	}

	// Toggle with 'tab'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(Model)
	if m.LineupTeam() != 1 {
		t.Errorf("expected lineupTeam 1 after tab, got %d", m.LineupTeam())
	}

	// Toggle back with 'tab'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(Model)
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam 0 after second tab, got %d", m.LineupTeam())
	}

	// Toggle with 'right' / 'l'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = newM.(Model)
	if m.LineupTeam() != 1 {
		t.Errorf("expected lineupTeam 1 after right arrow, got %d", m.LineupTeam())
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam 0 after 'h', got %d", m.LineupTeam())
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = newM.(Model)
	if m.LineupTeam() != 1 {
		t.Errorf("expected lineupTeam 1 after 'l', got %d", m.LineupTeam())
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = newM.(Model)
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam 0 after left arrow, got %d", m.LineupTeam())
	}

	// Exit modal with 'esc' -> should reset lineupTeam to 0
	m.ToggleLineupTeam()
	if m.LineupTeam() != 1 {
		t.Fatalf("expected lineupTeam 1 before exit")
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Fatalf("expected StateBrowsing on esc, got %d", m.state)
	}
	if m.LineupTeam() != 0 {
		t.Errorf("expected lineupTeam reset to 0 after exiting modal, got %d", m.LineupTeam())
	}
}

func TestRenderTacticalPitch_NilLineups(t *testing.T) {
	match := model.Match{
		ID:        "nil-lineup-match",
		HomeTeam:  model.Competitor{DisplayName: "Liverpool"},
		AwayTeam:  model.Competitor{DisplayName: "Arsenal"},
		Lineups:   nil,
		StartTime: time.Now(),
	}

	out := RenderTacticalPitch(match, 0, 60, false)
	if !strings.Contains(out, "Liverpool") || !strings.Contains(out, "Arsenal") {
		t.Errorf("expected team names in pill selector, got:\n%s", out)
	}
	if !strings.Contains(out, "Alineaciones no disponibles aún para este encuentro.") {
		t.Errorf("expected unavailable notice, got:\n%s", out)
	}
}

func TestFormatPitchPlayerNameAndLabel(t *testing.T) {
	tests := []struct {
		name        string
		player      model.Player
		badge       string
		maxLen      int
		expectedSub string
	}{
		{
			name:        "Lewandowski goal badge",
			player:      model.Player{DisplayName: "R. Lewandowski", Jersey: "9"},
			badge:       "⚽",
			maxLen:      20,
			expectedSub: "Lewandowski (9) ⚽",
		},
		{
			name:        "Pedri goal badge",
			player:      model.Player{DisplayName: "Pedri", Jersey: "8"},
			badge:       "⚽",
			maxLen:      20,
			expectedSub: "Pedri (8) ⚽",
		},
		{
			name:        "Vini Jr",
			player:      model.Player{DisplayName: "Vinícius Jr", Jersey: "7"},
			badge:       "",
			maxLen:      20,
			expectedSub: "Vini Jr (7)",
		},
		{
			name:        "Mbappé penalty goal",
			player:      model.Player{DisplayName: "K. Mbappé", Jersey: "9"},
			badge:       "⚽",
			maxLen:      20,
			expectedSub: "Mbappé (9) ⚽",
		},
		{
			name:        "Empty player name fallback",
			player:      model.Player{DisplayName: "", Jersey: "10"},
			badge:       "",
			maxLen:      20,
			expectedSub: "Jugador (10)",
		},
		{
			name:        "Truncation on tight slot",
			player:      model.Player{DisplayName: "A. Christensen", Jersey: "15"},
			badge:       "🟨",
			maxLen:      10,
			expectedSub: "(15) 🟨",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatPitchPlayerLabel(tc.player, tc.badge, tc.maxLen)
			if !strings.Contains(got, tc.expectedSub) {
				t.Errorf("FormatPitchPlayerLabel() = %q, want substring %q", got, tc.expectedSub)
			}
		})
	}
}

func TestRenderFullTacticalBoard_SymmetricalMathAndCoaches(t *testing.T) {
	match := model.Match{
		ID:        "demo-tactical-match",
		Title:     "FC Barcelona vs Real Madrid",
		Venue:     "Spotify Camp Nou",
		Referee:   "Szymon Marciniak",
		HomeTeam:  model.Competitor{ID: "home-1", DisplayName: "FC Barcelona"},
		AwayTeam:  model.Competitor{ID: "away-1", DisplayName: "Real Madrid"},
		StartTime: time.Now(),
		Events: []model.MatchEvent{
			{Type: "goal", PlayerName: "R. Lewandowski", Clock: "24'", TeamID: "home-1", IsScoring: true},
			{Type: "card_yellow", PlayerName: "Gavi", Clock: "41'", TeamID: "home-1", IsYellow: true},
			{Type: "penalty", PlayerName: "K. Mbappé", Clock: "54'", TeamID: "away-1", IsScoring: true, IsPenalty: true},
			{Type: "card_red", PlayerName: "A. Rüdiger", Clock: "63'", TeamID: "away-1", IsRedCard: true},
		},
		Lineups: &model.MatchLineups{
			Home: model.TeamLineup{
				Formation: "4-3-3",
				Coach:     "Hansi Flick",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "M. ter Stegen", Position: "GK"},
					{Jersey: "23", DisplayName: "J. Koundé", Position: "DF"},
					{Jersey: "4", DisplayName: "R. Araújo", Position: "DF"},
					{Jersey: "2", DisplayName: "P. Cubarsí", Position: "DF"},
					{Jersey: "3", DisplayName: "A. Balde", Position: "DF"},
					{Jersey: "8", DisplayName: "Pedri", Position: "MF"},
					{Jersey: "21", DisplayName: "F. de Jong", Position: "MF"},
					{Jersey: "6", DisplayName: "Gavi", Position: "MF"},
					{Jersey: "19", DisplayName: "Lamine Yamal", Position: "FW"},
					{Jersey: "9", DisplayName: "R. Lewandowski", Position: "FW"},
					{Jersey: "11", DisplayName: "Raphinha", Position: "FW"},
				},
				Bench: []model.Player{
					{Jersey: "13", DisplayName: "Iñaki Peña", Position: "GK"},
					{Jersey: "7", DisplayName: "Ferran Torres", Position: "FW"},
				},
			},
			Away: model.TeamLineup{
				Formation: "4-2-3-1",
				Coach:     "Carlo Ancelotti",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "T. Courtois", Position: "GK"},
					{Jersey: "2", DisplayName: "D. Carvajal", Position: "DF"},
					{Jersey: "3", DisplayName: "É. Militão", Position: "DF"},
					{Jersey: "22", DisplayName: "A. Rüdiger", Position: "DF"},
					{Jersey: "23", DisplayName: "F. Mendy", Position: "DF"},
					{Jersey: "14", DisplayName: "A. Tchouaméni", Position: "MF"},
					{Jersey: "8", DisplayName: "F. Valverde", Position: "MF"},
					{Jersey: "11", DisplayName: "Rodrygo", Position: "MF"},
					{Jersey: "5", DisplayName: "J. Bellingham", Position: "MF"},
					{Jersey: "7", DisplayName: "Vinícius Jr", Position: "FW"},
					{Jersey: "9", DisplayName: "K. Mbappé", Position: "FW"},
				},
				Bench: []model.Player{
					{Jersey: "13", DisplayName: "A. Lunin", Position: "GK"},
					{Jersey: "10", DisplayName: "L. Modrić", Position: "MF"},
				},
			},
		},
	}

	// 1. Side-by-Side Render (totalWidth = 100 >= 96)
	board := RenderFullTacticalBoard(match, 100, 32, false)

	// Check DT / Coaches in sidebar
	if !strings.Contains(board, "DT: Hansi Flick") {
		t.Errorf("expected Home coach 'DT: Hansi Flick' in sidebar, got:\n%s", board)
	}
	if !strings.Contains(board, "DT: Carlo Ancelotti") {
		t.Errorf("expected Away coach 'DT: Carlo Ancelotti' in sidebar, got:\n%s", board)
	}

	// Check Benches in sidebar
	if !strings.Contains(board, "Iñaki Peña") || !strings.Contains(board, "Ferran Torres") {
		t.Errorf("expected Home bench players in sidebar, got:\n%s", board)
	}
	if !strings.Contains(board, "A. Lunin") || !strings.Contains(board, "L. Modrić") {
		t.Errorf("expected Away bench players in sidebar, got:\n%s", board)
	}

	// Check Venue & Referee in sidebar
	if !strings.Contains(board, "Spotify Camp Nou") || !strings.Contains(board, "Szymon Marciniak") {
		t.Errorf("expected venue and referee in sidebar, got:\n%s", board)
	}

	// Check Highlights
	if !strings.Contains(board, "DESTACADOS") {
		t.Errorf("expected highlights header in sidebar, got:\n%s", board)
	}

	// Check Footer Hint
	if !strings.Contains(board, "[esc / c: Volver]") {
		t.Errorf("expected footer hint [esc / c: Volver], got:\n%s", board)
	}

	// Check Midfield Divider
	if !strings.Contains(board, "(   ●   )") {
		t.Errorf("expected midfield circle '(   ●   )', got:\n%s", board)
	}

	// Check players on pitch
	if !strings.Contains(board, "Lewandowski") || !strings.Contains(board, "Vini Jr") {
		t.Errorf("expected key player names on tactical pitch, got:\n%s", board)
	}

	// Mathematical Symmetry check:
	// Verify that each player dot line contains '●' and follows a name line
	lines := strings.Split(board, "\n")
	dotLineCount := 0
	for _, l := range lines {
		if strings.Contains(l, "●") && !strings.Contains(l, "(   ●   )") {
			dotLineCount++
		}
	}
	// Total rows: Home has GK, DEF, MID, FWD (4 rows). Away has FWD, MID, DEF, GK (4 rows). Total 8 dot lines!
	if dotLineCount < 8 {
		t.Errorf("expected at least 8 player dot lines across Home and Away teams, got %d", dotLineCount)
	}
}

func TestRenderFullTacticalBoard_StackedAndNoEmoji(t *testing.T) {
	match := model.Match{
		ID:        "stacked-noemoji",
		Venue:     "Santiago Bernabéu",
		Referee:   "Clément Turpin",
		HomeTeam:  model.Competitor{DisplayName: "Real Madrid"},
		AwayTeam:  model.Competitor{DisplayName: "Barcelona"},
		StartTime: time.Now(),
		Lineups: &model.MatchLineups{
			Home: model.TeamLineup{
				Formation: "4-3-3",
				Coach:     "Carlo Ancelotti",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "Courtois", Position: "GK"},
					{Jersey: "2", DisplayName: "Carvajal", Position: "DF"},
					{Jersey: "3", DisplayName: "Militão", Position: "DF"},
					{Jersey: "22", DisplayName: "Rüdiger", Position: "DF"},
					{Jersey: "23", DisplayName: "Mendy", Position: "DF"},
					{Jersey: "8", DisplayName: "Valverde", Position: "MF"},
					{Jersey: "14", DisplayName: "Tchouaméni", Position: "MF"},
					{Jersey: "5", DisplayName: "Bellingham", Position: "MF"},
					{Jersey: "11", DisplayName: "Rodrygo", Position: "FW"},
					{Jersey: "9", DisplayName: "Mbappé", Position: "FW"},
					{Jersey: "7", DisplayName: "Vinícius Jr", Position: "FW"},
				},
			},
			Away: model.TeamLineup{
				Formation: "4-3-3",
				Coach:     "Hansi Flick",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "ter Stegen", Position: "GK"},
					{Jersey: "2", DisplayName: "Cubarsí", Position: "DF"},
					{Jersey: "3", DisplayName: "Balde", Position: "DF"},
					{Jersey: "4", DisplayName: "Araújo", Position: "DF"},
					{Jersey: "23", DisplayName: "Koundé", Position: "DF"},
					{Jersey: "6", DisplayName: "Gavi", Position: "MF"},
					{Jersey: "8", DisplayName: "Pedri", Position: "MF"},
					{Jersey: "21", DisplayName: "de Jong", Position: "MF"},
					{Jersey: "9", DisplayName: "Lewandowski", Position: "FW"},
					{Jersey: "11", DisplayName: "Raphinha", Position: "FW"},
					{Jersey: "19", DisplayName: "Yamal", Position: "FW"},
				},
			},
		},
	}

	// Render in stacked layout (width = 80 < 96) and noEmoji = true
	board := RenderFullTacticalBoard(match, 80, 50, true)

	// In noEmoji mode, center circle must be (   o   )
	if !strings.Contains(board, "(   o   )") {
		t.Errorf("expected '(   o   )' in noEmoji mode, got:\n%s", board)
	}

	// In noEmoji mode, stadium and referee should use text labels
	if !strings.Contains(board, "Estadio: Santiago Bernabéu") {
		t.Errorf("expected 'Estadio:' label in noEmoji mode, got:\n%s", board)
	}
	if !strings.Contains(board, "Árbitro: Clément Turpin") {
		t.Errorf("expected 'Árbitro:' label in noEmoji mode, got:\n%s", board)
	}
	if strings.Contains(board, "🏟️") || strings.Contains(board, "👤") {
		t.Errorf("board in noEmoji mode must not contain raw emoji icons")
	}

	// Check coaches
	if !strings.Contains(board, "DT: Carlo Ancelotti") || !strings.Contains(board, "DT: Hansi Flick") {
		t.Errorf("expected coach names in stacked board, got:\n%s", board)
	}
}

func TestRenderFullTacticalBoard_NilLineups(t *testing.T) {
	match := model.Match{
		ID:        "empty-lineups",
		HomeTeam:  model.Competitor{DisplayName: "Chelsea"},
		AwayTeam:  model.Competitor{DisplayName: "Arsenal"},
		Lineups:   nil,
		StartTime: time.Now(),
	}

	out := RenderFullTacticalBoard(match, 100, 30, false)
	if !strings.Contains(out, "Alineaciones no disponibles aún para este encuentro.") {
		t.Errorf("expected fallback notice for nil lineups, got:\n%s", out)
	}
	if !strings.Contains(out, "[ esc / c: Volver ]") {
		t.Errorf("expected footer hint in nil lineups fallback, got:\n%s", out)
	}
}

