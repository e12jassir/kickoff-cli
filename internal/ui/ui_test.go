package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

type mockClient struct {
	matches    []model.Match
	err        error
	stats      *model.MatchStats
	lineups    *model.MatchLineups
	standings  *model.LeagueStandings
	statistics *model.LeagueStatistics
}

func (m *mockClient) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	return m.matches, m.err
}

func (m *mockClient) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	return m.stats, m.err
}

func (m *mockClient) FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*model.MatchDetails, error) {
	if m.stats == nil && m.lineups == nil {
		return nil, m.err
	}
	return &model.MatchDetails{
		Stats:   m.stats,
		Lineups: m.lineups,
	}, m.err
}

func (m *mockClient) FetchStandings(ctx context.Context, leagueKey string) (*model.LeagueStandings, error) {
	if m.standings != nil {
		return m.standings, m.err
	}
	return &model.LeagueStandings{
		LeagueKey: leagueKey,
		Name:      "Standings",
		Entries: []model.StandingsEntry{
			{Rank: 1, TeamName: "Real Madrid", GamesPlayed: "10", Wins: "8", Ties: "1", Losses: "1", PointDiff: "+15", Points: "25"},
			{Rank: 2, TeamName: "Barcelona", GamesPlayed: "10", Wins: "7", Ties: "2", Losses: "1", PointDiff: "+12", Points: "23"},
		},
	}, m.err
}

func (m *mockClient) FetchStatistics(ctx context.Context, leagueKey string) (*model.LeagueStatistics, error) {
	if m.statistics != nil {
		return m.statistics, m.err
	}
	return &model.LeagueStatistics{
		LeagueKey: leagueKey,
		Scorers: []model.ScorerEntry{
			{Rank: 1, PlayerName: "R. Lewandowski", TeamName: "Barcelona", Value: "10", DisplayValue: "10 Goles"},
		},
		Assists: []model.ScorerEntry{
			{Rank: 1, PlayerName: "Lamine Yamal", TeamName: "Barcelona", Value: "6", DisplayValue: "6 Asistencias"},
		},
	}, m.err
}

type mockCatalog struct {
	channels []model.Channel
}

func (m *mockCatalog) GetChannels(match model.Match) []model.Channel {
	return m.channels
}

type mockPlayer struct {
	playedReq *model.PlayRequest
	stopped   bool
	err       error
}

func (m *mockPlayer) PlayStream(ctx context.Context, req model.PlayRequest) error {
	m.playedReq = &req
	return m.err
}

func (m *mockPlayer) Stop() error {
	m.stopped = true
	return nil
}

func (m *mockPlayer) Wait() error {
	return nil
}

func TestModel_MainMenu_Navigation(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Initial state is StateMainMenu with first category expanded
	if m.state != StateMainMenu {
		t.Fatalf("expected initial state StateMainMenu, got %d", m.state)
	}
	if !m.expandedCats[m.categories[0]] {
		t.Fatalf("expected first category to be expanded initially")
	}

	// Move down with 'j': moves from category header into the first league of expanded category
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.leagueCursor != 0 || m.categoryMode {
		t.Errorf("expected leagueCursor 0 and categoryMode false after first 'j', got %d (mode %v)", m.leagueCursor, m.categoryMode)
	}

	// Next 'j': moves to next league in category
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.leagueCursor != 1 {
		t.Errorf("expected leagueCursor 1 after second 'j', got %d", m.leagueCursor)
	}

	// Move up with 'k': back to first league
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.leagueCursor != 0 {
		t.Errorf("expected leagueCursor 0 after 'k', got %d", m.leagueCursor)
	}

	// Next 'k': back to category header
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if !m.categoryMode {
		t.Errorf("expected categoryMode true after moving up to header")
	}

	// Check View rendering in StateMainMenu
	viewStr := m.View()
	if !strings.Contains(viewStr, "Terminal Live Football") {
		t.Errorf("expected view to contain subtitle 'Terminal Live Football'")
	}
	if !strings.Contains(viewStr, "TORNEOS INTERNACIONALES") {
		t.Errorf("expected view to contain category 'TORNEOS INTERNACIONALES'")
	}
	if !strings.Contains(viewStr, "LIGAS EUROPEAS") {
		t.Errorf("expected view to contain category 'LIGAS EUROPEAS'")
	}
	if !strings.Contains(viewStr, "LIGAS SUDAMERICANAS") {
		t.Errorf("expected view to contain category 'LIGAS SUDAMERICANAS'")
	}
	if !strings.Contains(viewStr, "›") {
		t.Errorf("expected view to contain active cursor '›'")
	}
}

func TestModel_Enter_Transition_To_Matches(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Press Enter on main menu (index 0: UEFA Champions League)
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.state != StateLoading {
		t.Fatalf("expected state StateLoading after Enter, got %d", m.state)
	}
	if cmd == nil {
		t.Fatalf("expected fetch command after Enter")
	}
	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0, got %d", m.activeTab)
	}

	mockMatches := []model.Match{
		{
			ID:        "m1",
			Title:     "Real Madrid vs Barcelona",
			HomeTeam:  model.Competitor{DisplayName: "Real Madrid"},
			AwayTeam:  model.Competitor{DisplayName: "Barcelona"},
			TimeStr:   "15:00",
			Venue:     "Santiago Bernabéu",
			LeagueKey: m.CurrentLeague().Key,
		},
		{
			ID:        "m2",
			Title:     "Atletico vs Sevilla",
			HomeTeam:  model.Competitor{DisplayName: "Atletico"},
			AwayTeam:  model.Competitor{DisplayName: "Sevilla"},
			TimeStr:   "17:00",
			LeagueKey: m.CurrentLeague().Key,
		},
	}

	// Deliver loaded matches
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   mockMatches,
	})
	m = newM.(Model)

	if m.state != StateBrowsing {
		t.Fatalf("expected state StateBrowsing, got %d", m.state)
	}
	if len(m.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.matches))
	}
	if m.leagueTab != TabResumen {
		t.Fatalf("expected default tab TabResumen, got %d", m.leagueTab)
	}

	// Switch to Tab 2 (Partidos) to view upcoming matches
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)

	viewStr := m.View()
	if !strings.Contains(viewStr, "KICKOFF") {
		t.Errorf("expected header to contain 'KICKOFF', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Real Madrid") {
		t.Errorf("expected view to contain 'Real Madrid', got:\n%s", viewStr)
	}
}

func TestModel_Esc_Back_To_MainMenu(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Transition to browsing with matches
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches: []model.Match{
			{ID: "m1", Title: "Team A vs Team B"},
		},
	})
	m = newM.(Model)

	if m.state != StateBrowsing {
		t.Fatalf("expected state StateBrowsing, got %d", m.state)
	}

	// Press Esc to return to MainMenu
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)

	if m.state != StateMainMenu {
		t.Fatalf("expected state StateMainMenu after Esc, got %d", m.state)
	}
	if m.matches != nil {
		t.Errorf("expected matches to be nil after returning to main menu")
	}

	viewStr := m.View()
	if !strings.Contains(viewStr, "Terminal Live Football") {
		t.Errorf("expected main menu view after Esc, got:\n%s", viewStr)
	}

	// Test Backspace key returns to main menu as well
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   []model.Match{{ID: "m2"}},
	})
	m = newM.(Model)

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = newM.(Model)
	if m.state != StateMainMenu {
		t.Fatalf("expected state StateMainMenu after Backspace, got %d", m.state)
	}

	// Verify 'h' key cycles tabs in browsing mode instead of exiting
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   []model.Match{{ID: "m3"}},
	})
	m = newM.(Model)
	if m.leagueTab != TabResumen {
		t.Fatalf("expected TabResumen, got %d", m.leagueTab)
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.leagueTab != TabGoleadores {
		t.Fatalf("expected TabGoleadores after 'h' from TabResumen, got %d", m.leagueTab)
	}
}

func TestModel_Channel_Modal_And_Playback(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{
		channels: []model.Channel{
			{Name: "ESPN 1 HD (720p)", DisplayName: "ESPN 1 HD (720p)", URL: "http://example.com/1.m3u8", BitrateMode: "min"},
			{Name: "ESPN 1 HD (1080p)", DisplayName: "ESPN 1 HD (1080p)", URL: "http://example.com/2.m3u8", BitrateMode: "max"},
		},
	}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Enter league and load matches
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches: []model.Match{
			{ID: "m1", Title: "Liverpool vs Arsenal"},
			{ID: "m2", Title: "Chelsea vs City"},
		},
	})
	m = newM.(Model)

	// Switch to TabPartidos to browse upcoming match streams
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)

	// Navigate down to match 1
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.matchCursor != 1 {
		t.Errorf("expected matchCursor 1, got %d", m.matchCursor)
	}

	// Press Enter to open channel modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect, got %d", m.state)
	}
	if len(m.channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(m.channels))
	}

	viewStr := m.View()
	if !strings.Contains(strings.ToUpper(viewStr), "SEÑALES DE TRANSMISIÓN") {
		t.Errorf("expected view to contain 'SEÑALES DE TRANSMISIÓN', got:\n%s", viewStr)
	}

	// Navigate channel cursor
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.channelCursor != 1 {
		t.Errorf("expected channelCursor 1, got %d", m.channelCursor)
	}

	// Press Esc to dismiss modal and return to StateBrowsing (keeping matches intact)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Fatalf("expected StateBrowsing after Esc in modal, got %d", m.state)
	}
	if len(m.matches) != 2 {
		t.Fatalf("expected matches to remain intact (count 2), got %d", len(m.matches))
	}

	// Reopen modal and select channel with Enter
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, playCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if playCmd == nil {
		t.Fatalf("expected playCmd after channel select")
	}
	if m.state != StatePlaying {
		t.Fatalf("expected StatePlaying, got %d", m.state)
	}
	if !strings.Contains(m.statusMsg, "Conectando con") {
		t.Errorf("expected statusMsg connecting, got %s", m.statusMsg)
	}

	// Delivery playback started message
	newM, waitCmd := m.Update(playbackStartedMsg{playbackID: m.playbackID, channel: m.channels[0]})
	m = newM.(Model)
	if !strings.Contains(m.statusMsg, "Reproduciendo") {
		t.Errorf("expected statusMsg playing, got %s", m.statusMsg)
	}
	if waitCmd == nil {
		t.Fatalf("expected waitCmd to monitor playback completion")
	}

	// Delivery playback finished message
	newM, _ = m.Update(playbackFinishedMsg{playbackID: m.playbackID})
	m = newM.(Model)
	if m.statusMsg != "" {
		t.Errorf("expected statusMsg to be cleared after playback finishes, got %s", m.statusMsg)
	}
}

func TestModel_Finished_Match_Modal(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{
		channels: nil, // no channels for finished match
	}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Enter league and load finished match
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches: []model.Match{
			{
				ID:       "m1",
				Title:    "Barcelona vs Real Madrid",
				HomeTeam: model.Competitor{DisplayName: "Barcelona", Score: "2"},
				AwayTeam: model.Competitor{DisplayName: "Real Madrid", Score: "1"},
				Status:   model.MatchStatus{State: "post", Completed: true},
			},
		},
	})
	m = newM.(Model)

	// Press Enter to view finished match modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect, got %d", m.state)
	}
	if len(m.channels) != 0 {
		t.Fatalf("expected 0 channels for finished match, got %d", len(m.channels))
	}

	viewStr := m.View()
	if !strings.Contains(viewStr, "FINALIZADO") {
		t.Errorf("expected modal to show 'FINALIZADO', got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "[ ⏹ Transmisión finalizada • No hay señales activas ]") {
		t.Errorf("expected modal to inform match transmission finished, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Resumen") || !strings.Contains(viewStr, "Estadísticas") || !strings.Contains(viewStr, "Alineaciones") {
		t.Errorf("expected modal to contain segmented tab labels, got:\n%s", viewStr)
	}

	// Pressing Enter when no channels should not start playback
	newM, playCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if playCmd != nil {
		t.Errorf("expected nil playCmd when no channels are available")
	}
	if m.state != StateChannelSelect {
		t.Errorf("expected state to remain StateChannelSelect, got %d", m.state)
	}

	// Esc returns to StateBrowsing
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Errorf("expected state StateBrowsing after Esc, got %d", m.state)
	}
}

func TestModel_EmptyMatches_And_Error(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Enter league
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	// Simulate empty matches loaded
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   []model.Match{},
	})
	m = newM.(Model)

	viewStr := m.View()
	if !strings.Contains(viewStr, "No hay partidos finalizados recientemente") {
		t.Errorf("expected empty state message in TabResumen, got:\n%s", viewStr)
	}

	// Switch to Tab 2 (Partidos) to verify empty state message for upcoming matches
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)
	viewStr = m.View()
	if !strings.Contains(viewStr, "No hay próximos partidos programados") {
		t.Errorf("expected empty state message in TabPartidos, got:\n%s", viewStr)
	}

	// Simulate error
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		err:       errors.New("connection failed"),
	})
	m = newM.(Model)
	if m.state != StateError {
		t.Fatalf("expected StateError, got %d", m.state)
	}
	viewStr = m.View()
	if !strings.Contains(viewStr, "connection failed") {
		t.Errorf("expected error message in view, got:\n%s", viewStr)
	}

	// Press Esc to return to MainMenu from StateError
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateMainMenu {
		t.Fatalf("expected StateMainMenu after Esc from StateError, got %d", m.state)
	}
}

func TestModel_Quit(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("expected tea.Quit cmd on 'q'")
	}
	if !pla.stopped {
		t.Errorf("expected player.Stop() to be called on quit")
	}
}

func TestModel_Match_Events_And_Stats_Modal(t *testing.T) {
	mockStats := &model.MatchStats{
		HomePossession:  "58%",
		AwayPossession:  "42%",
		HomeShots:       "14",
		AwayShots:       "8",
		HomeShotsOnGoal: "6",
		AwayShotsOnGoal: "2",
		HomeCorners:     "7",
		AwayCorners:     "3",
		HomeFouls:       "11",
		AwayFouls:       "13",
		HomePasses:      "520",
		AwayPasses:      "340",
		HomePassPct:     "89%",
		AwayPassPct:     "81%",
		HomeOffsides:    "2",
		AwayOffsides:    "1",
		HomeSaves:       "4",
		AwaySaves:       "2",
	}

	mockLineups := &model.MatchLineups{
		Home: model.TeamLineup{
			Formation: "4-3-3",
			Starters: []model.Player{
				{Jersey: "1", DisplayName: "M. ter Stegen", Position: "GK", IsStarter: true},
				{Jersey: "23", DisplayName: "J. Koundé", Position: "DF", IsStarter: true},
				{Jersey: "9", DisplayName: "R. Lewandowski", Position: "FW", IsStarter: true},
			},
		},
		Away: model.TeamLineup{
			Formation: "4-2-3-1",
			Starters: []model.Player{
				{Jersey: "1", DisplayName: "T. Courtois", Position: "GK", IsStarter: true},
				{Jersey: "2", DisplayName: "D. Carvajal", Position: "DF", IsStarter: true},
				{Jersey: "7", DisplayName: "Vinícius Jr", Position: "FW", IsStarter: true},
			},
		},
	}

	cli := &mockClient{
		stats:   mockStats,
		lineups: mockLineups,
	}
	cat := &mockCatalog{
		channels: []model.Channel{
			{Name: "ESPN 1 HD", DisplayName: "ESPN 1 HD", Resolution: "720p", BitrateMode: "min"},
			{Name: "ESPN 2 HD", DisplayName: "ESPN 2 HD", Resolution: "1080p", BitrateMode: "max"},
		},
	}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Enter league
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	// Load match with events
	matchWithEvents := model.Match{
		ID:       "match-123",
		Title:    "Barcelona vs Real Madrid",
		HomeTeam: model.Competitor{ID: "team-barca", DisplayName: "Barcelona", Score: "2"},
		AwayTeam: model.Competitor{ID: "team-madrid", DisplayName: "Real Madrid", Score: "1"},
		Status:   model.MatchStatus{State: "in", Description: "Segundo Tiempo", Detail: "65'"},
		Events: []model.MatchEvent{
			{
				Type:       "goal",
				PlayerName: "R. Lewandowski",
				Clock:      "38'",
				TeamID:     "team-barca",
				IsScoring:  true,
			},
			{
				Type:       "card_yellow",
				PlayerName: "Gavi",
				Clock:      "44'",
				TeamID:     "team-barca",
				IsYellow:   true,
			},
			{
				Type:       "goal",
				PlayerName: "Vinicius Jr",
				Clock:      "55'",
				TeamID:     "team-madrid",
				IsScoring:  true,
				IsPenalty:  true,
			},
			{
				Type:       "card_red",
				PlayerName: "Rüdiger",
				Clock:      "62'",
				TeamID:     "team-madrid",
				IsRedCard:  true,
			},
		},
		Referee:    "Clement Turpin",
		Attendance: 85000,
	}

	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   []model.Match{matchWithEvents},
	})
	m = newM.(Model)

	// Switch to TabPartidos to select the upcoming match
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)

	// Select match with Enter -> transitions to StateChannelSelect and returns stats fetch command
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect, got %d", m.state)
	}
	if cmd == nil {
		t.Fatalf("expected stats fetch command when match stats are nil")
	}

	// Deliver stats & lineups message
	statsMsg := cmd()
	newM, _ = m.Update(statsMsg)
	m = newM.(Model)

	if len(m.matches) == 0 || m.matches[0].Stats == nil || m.matches[0].Lineups == nil {
		t.Fatalf("expected match stats and lineups to be populated")
	}

	// --- 1. TAB 0: RESUMEN ---
	if m.modalTab != 0 {
		t.Errorf("expected initial modalTab 0, got %d", m.modalTab)
	}
	viewTab0 := m.View()

	// Verify header & scoreboard
	if !strings.Contains(viewTab0, "Barcelona") || !strings.Contains(viewTab0, "Real Madrid") {
		t.Errorf("expected view to contain team names")
	}
	if !strings.Contains(viewTab0, "2 ─ 1") {
		t.Errorf("expected view to contain score 2 ─ 1")
	}

	// Verify segmented pills in Tab 0
	if !strings.Contains(viewTab0, "[ ● Resumen ]") || !strings.Contains(viewTab0, "○ Estadísticas") || !strings.Contains(viewTab0, "○ Alineaciones") {
		t.Errorf("expected active pill [ ● Resumen ] and inactive others, got:\n%s", viewTab0)
	}

	// Verify events in Tab 0: goals, cards, penalty tag
	if !strings.Contains(viewTab0, "R. Lewandowski") || !strings.Contains(viewTab0, "38'") {
		t.Errorf("expected view to contain goal scorer Lewandowski 38'")
	}
	if !strings.Contains(viewTab0, "Gavi") || !strings.Contains(viewTab0, "🟨") {
		t.Errorf("expected view to contain yellow card for Gavi")
	}
	if !strings.Contains(viewTab0, "Vinicius Jr") || !strings.Contains(viewTab0, "(pen.)") {
		t.Errorf("expected view to contain penalty goal for Vinicius Jr")
	}
	if !strings.Contains(viewTab0, "Rüdiger") || !strings.Contains(viewTab0, "🟥") {
		t.Errorf("expected view to contain red card for Rüdiger")
	}

	// Verify vertical timeline formatting and halftime divider in Tab 0
	if !strings.Contains(viewTab0, "──┤ 38' │") || !strings.Contains(viewTab0, "│ 55' ├──") {
		t.Errorf("expected timeline axis nodes for 38' and 55', got:\n%s", viewTab0)
	}
	if !strings.Contains(viewTab0, "│ E.T │") {
		t.Errorf("expected halftime separator in timeline, got:\n%s", viewTab0)
	}

	// Verify referee in Tab 0
	if !strings.Contains(viewTab0, "Clement Turpin") {
		t.Errorf("expected referee in Tab 0")
	}

	// Verify channel list in Tab 0
	if !strings.Contains(viewTab0, "SEÑALES DE TRANSMISIÓN") || !strings.Contains(viewTab0, "ESPN 1 HD") {
		t.Errorf("expected channel selection section in Tab 0")
	}

	// Verify footer helper
	if !strings.Contains(viewTab0, "pestaña") || !strings.Contains(viewTab0, "canal") {
		t.Errorf("expected footer helper in Tab 0, got:\n%s", viewTab0)
	}

	// --- 2. SWITCH TO TAB 1: ESTADÍSTICAS ---
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(Model)
	if m.modalTab != 1 {
		t.Fatalf("expected modalTab 1 after Tab key, got %d", m.modalTab)
	}
	viewTab1 := m.View()

	// Verify active pill in Tab 1
	if !strings.Contains(viewTab1, "[ ● Estadísticas ]") {
		t.Errorf("expected active pill [ ● Estadísticas ], got:\n%s", viewTab1)
	}

	// Verify visual possession progress bar
	if !strings.Contains(viewTab1, "Posesión") || !strings.Contains(viewTab1, "58%") || !strings.Contains(viewTab1, "42%") {
		t.Errorf("expected possession stats and label in Tab 1")
	}
	if !strings.Contains(viewTab1, "█") || !strings.Contains(viewTab1, "░") {
		t.Errorf("expected Unicode block comparison bar in Tab 1")
	}

	// Verify 6 comparison rows
	if !strings.Contains(viewTab1, "Tiros (Al arco)") || !strings.Contains(viewTab1, "14 (6)") || !strings.Contains(viewTab1, "8 (2)") {
		t.Errorf("expected shots row")
	}
	if !strings.Contains(viewTab1, "Pases (Precisión)") || !strings.Contains(viewTab1, "520 (89%)") || !strings.Contains(viewTab1, "340 (81%)") {
		t.Errorf("expected passes row")
	}
	if !strings.Contains(viewTab1, "Faltas") || !strings.Contains(viewTab1, "11") || !strings.Contains(viewTab1, "13") {
		t.Errorf("expected fouls row")
	}
	if !strings.Contains(viewTab1, "Córners") || !strings.Contains(viewTab1, "7") || !strings.Contains(viewTab1, "3") {
		t.Errorf("expected corners row")
	}
	if !strings.Contains(viewTab1, "Fueras de juego") || !strings.Contains(viewTab1, "2") || !strings.Contains(viewTab1, "1") {
		t.Errorf("expected offsides row")
	}
	if !strings.Contains(viewTab1, "Atajadas") || !strings.Contains(viewTab1, "4") || !strings.Contains(viewTab1, "2") {
		t.Errorf("expected saves row")
	}

	// --- 3. SWITCH TO TAB 2: ALINEACIONES ---
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = newM.(Model)
	if m.modalTab != 2 {
		t.Fatalf("expected modalTab 2 after right arrow, got %d", m.modalTab)
	}
	viewTab2 := m.View()

	// Verify active pill in Tab 2
	if !strings.Contains(viewTab2, "[ ● Alineaciones ]") {
		t.Errorf("expected active pill [ ● Alineaciones ], got:\n%s", viewTab2)
	}

	// Verify formations badge
	if !strings.Contains(viewTab2, "Barcelona (4-3-3)") || !strings.Contains(viewTab2, "Real Madrid (4-2-3-1)") {
		t.Errorf("expected formations badge in Tab 2, got:\n%s", viewTab2)
	}

	// Verify starters with jerseys and position abbreviations
	if !strings.Contains(viewTab2, "1  M. ter Stegen (GK)") || !strings.Contains(viewTab2, "1  T. Courtois (GK)") {
		t.Errorf("expected goalkeeper starters in Tab 2, got:\n%s", viewTab2)
	}
	if !strings.Contains(viewTab2, "R. Lewandowski (FW)") || !strings.Contains(viewTab2, "Vinícius Jr (FW)") {
		t.Errorf("expected forward starters in Tab 2")
	}

	// --- 4. DIRECT NUMERIC TAB SWITCHING ---
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = newM.(Model)
	if m.modalTab != 0 {
		t.Errorf("expected modalTab 0 on '1', got %d", m.modalTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = newM.(Model)
	if m.modalTab != 2 {
		t.Errorf("expected modalTab 2 on '3', got %d", m.modalTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)
	if m.modalTab != 1 {
		t.Errorf("expected modalTab 1 on '2', got %d", m.modalTab)
	}

	// --- 5. BACKWARD TAB NAVIGATION WITH 'h', 'left', 'shift+tab' ---
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.modalTab != 0 {
		t.Errorf("expected modalTab 0 on 'h', got %d", m.modalTab)
	}

	// Wrap around backwards from 0 to 2
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = newM.(Model)
	if m.modalTab != 2 {
		t.Errorf("expected modalTab 2 after wrap-around left, got %d", m.modalTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = newM.(Model)
	if m.modalTab != 1 {
		t.Errorf("expected modalTab 1 on shift+tab, got %d", m.modalTab)
	}

	// --- 6. ESC RESETS MODAL TAB TO 0 ---
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Errorf("expected StateBrowsing after Esc, got %d", m.state)
	}
	if m.modalTab != 0 {
		t.Errorf("expected modalTab reset to 0 after Esc, got %d", m.modalTab)
	}
}

func TestModel_Modal_CursorGatingAndFallbacks(t *testing.T) {
	cli := &mockClient{
		stats:   nil, // stats loading / nil
		lineups: nil, // lineups nil
	}
	cat := &mockCatalog{
		channels: []model.Channel{
			{Name: "Channel 1", DisplayName: "Channel 1"},
			{Name: "Channel 2", DisplayName: "Channel 2"},
		},
	}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Enter league and load match
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches: []model.Match{
			{
				ID:       "m-unstarted",
				Title:    "Team A vs Team B",
				HomeTeam: model.Competitor{DisplayName: "Team A"},
				AwayTeam: model.Competitor{DisplayName: "Team B"},
				Status:   model.MatchStatus{State: "pre"},
			},
		},
	})
	m = newM.(Model)

	// Switch to TabPartidos
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)

	// Enter match modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect, got %d", m.state)
	}

	// Tab 0 cursor navigation
	if m.channelCursor != 0 {
		t.Errorf("expected initial channelCursor 0")
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newM.(Model)
	if m.channelCursor != 1 {
		t.Errorf("expected channelCursor 1 on down key in Tab 0, got %d", m.channelCursor)
	}

	// Switch to Tab 1 (Estadísticas) - stats are nil -> verify loading text
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)
	if m.modalTab != 1 {
		t.Fatalf("expected modalTab 1")
	}
	viewTab1 := m.View()
	if !strings.Contains(viewTab1, "Consultando estadísticas del partido...") {
		t.Errorf("expected stats loading text, got:\n%s", viewTab1)
	}

	// When on Tab 1, pressing down/up ('j'/'k') should NOT change channelCursor
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.channelCursor != 1 {
		t.Errorf("expected channelCursor to remain 1 on 'k' in Tab 1, got %d", m.channelCursor)
	}

	// Switch to Tab 2 (Alineaciones) - lineups are nil -> verify fallback text
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = newM.(Model)
	if m.modalTab != 2 {
		t.Fatalf("expected modalTab 2")
	}
	viewTab2 := m.View()
	if !strings.Contains(viewTab2, "Alineaciones no disponibles aún para este encuentro.") {
		t.Errorf("expected lineups unavailable notice, got:\n%s", viewTab2)
	}

	// In Tab 2, pressing Enter should NOT launch playback
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if cmd != nil {
		t.Errorf("expected nil cmd on Enter in Tab 2")
	}
	if m.state != StateChannelSelect {
		t.Errorf("expected state to remain StateChannelSelect, got %d", m.state)
	}
}

func TestModel_ChannelModal_SanitizedAndAligned(t *testing.T) {
	channels := []model.Channel{
		{
			DisplayName: "  · Win+ Fútbol (1080p Directo)   [Recomendado]",
			Name:        "Win+ Fútbol",
			Resolution:  "1080p",
			BitrateMode: "max",
			Recommended: true,
		},
		{
			DisplayName: "  · Win Sports HD (720p)",
			Name:        "Win Sports HD",
			Resolution:  "720p",
			BitrateMode: "min",
			Recommended: false,
		},
		{
			DisplayName: "Caracol TV HD",
			Name:        "Caracol TV HD",
			Resolution:  "1080p",
			BitrateMode: "max",
			Recommended: false,
		},
	}

	match := model.Match{
		ID:        "m-col",
		Title:     "Millonarios vs Santa Fe",
		LeagueKey: "colombia",
		Status:    model.MatchStatus{State: "in", Description: "1T 25'"},
	}

	out := renderChannelModal(match, channels, 0, 0, false, "")

	// 1. Must NOT contain bullet indentations "·" preceding channel names
	if strings.Contains(out, "· Win+ Fútbol") || strings.Contains(out, "· Win Sports") {
		t.Errorf("channel modal still contains bullet indentation:\n%s", out)
	}

	// 2. Must NOT contain truncated mid-string suffixes or raw paren suffixes in channel names
	if strings.Contains(out, "Directo") || strings.Contains(out, "Fluido") {
		t.Errorf("channel modal still contains parenthesized suffixes:\n%s", out)
	}

	// 3. Must contain clean channel names
	if !strings.Contains(out, "Win+ Fútbol") {
		t.Errorf("expected clean name 'Win+ Fútbol' in modal")
	}
	if !strings.Contains(out, "Win Sports HD") {
		t.Errorf("expected clean name 'Win Sports HD' in modal")
	}
	if !strings.Contains(out, "Caracol TV HD") {
		t.Errorf("expected clean name 'Caracol TV HD' in modal")
	}

	// 4. Must contain formatted badges
	if !strings.Contains(out, "[1080p · max]") || !strings.Contains(out, "[720p · min]") {
		t.Errorf("expected resolution badges in modal")
	}
	if !strings.Contains(out, "★ Recomendado") {
		t.Errorf("expected recommended badge in modal")
	}
}

func TestModel_WindowResize_TooSmall(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)

	// Default size (80x24) should not trigger the warning
	if strings.Contains(m.View(), "Terminal window is too small") {
		t.Fatalf("did not expect warning at 80x24")
	}

	// Width too small (<60)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 59, Height: 20})
	m = updated.(Model)
	expectedWarning := "Terminal window is too small (min 60x15). Please resize."
	if m.View() != expectedWarning {
		t.Errorf("expected %q, got %q", expectedWarning, m.View())
	}

	// Height too small (<15)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	m = updated.(Model)
	if m.View() != expectedWarning {
		t.Errorf("expected %q, got %q", expectedWarning, m.View())
	}

	// Both too small
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = updated.(Model)
	if m.View() != expectedWarning {
		t.Errorf("expected %q, got %q", expectedWarning, m.View())
	}

	// Restored to valid size (60x15)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	m = updated.(Model)
	if strings.Contains(m.View(), "Terminal window is too small") {
		t.Errorf("did not expect warning at 60x15")
	}
}

func TestModel_NoEmoji(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}

	mDefault := NewModel(client, cat, p)
	viewDefault := mDefault.View()
	if !strings.Contains(viewDefault, "🏆") && !strings.Contains(viewDefault, "⚽") {
		t.Errorf("expected emojis in default view")
	}

	mNoEmoji := NewModel(client, cat, p).WithNoEmoji(true)
	viewNoEmoji := mNoEmoji.View()
	if strings.Contains(viewNoEmoji, "🏆") {
		t.Errorf("did not expect trophy emoji in no-emoji view:\n%s", viewNoEmoji)
	}
}

func TestModel_FastSyncIntervals(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)

	// 1. Outside browsing (StateMainMenu) -> 30s
	if interval := m.nextSyncInterval(); interval != 30*time.Second {
		t.Errorf("expected 30s in StateMainMenu, got %v", interval)
	}

	m.state = StateBrowsing

	// 2. No matches -> 30s
	m.matches = nil
	if interval := m.nextSyncInterval(); interval != 30*time.Second {
		t.Errorf("expected 30s for empty matches, got %v", interval)
	}

	// 3. Live match -> 5s
	m.matches = []model.Match{
		{
			ID:     "1",
			Status: model.MatchStatus{State: "in"},
		},
	}
	if interval := m.nextSyncInterval(); interval != 5*time.Second {
		t.Errorf("expected 5s when match is live, got %v", interval)
	}

	// 4. Upcoming soon match (within 15 minutes) -> 15s
	m.matches = []model.Match{
		{
			ID:        "2",
			StartTime: time.Now().Add(5 * time.Minute),
			Status:    model.MatchStatus{State: "pre"},
		},
	}
	if interval := m.nextSyncInterval(); interval != 15*time.Second {
		t.Errorf("expected 15s for upcoming soon match, got %v", interval)
	}

	// 5. General browsing / far in future / finished -> 30s
	m.matches = []model.Match{
		{
			ID:        "3",
			StartTime: time.Now().Add(3 * time.Hour),
			Status:    model.MatchStatus{State: "pre"},
		},
		{
			ID:        "4",
			StartTime: time.Now().Add(-3 * time.Hour),
			Status:    model.MatchStatus{State: "post", Completed: true},
		},
	}
	if interval := m.nextSyncInterval(); interval != 30*time.Second {
		t.Errorf("expected 30s for general browsing, got %v", interval)
	}
}

func TestModel_OfflineBadgeRendering(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StateBrowsing

	cachedTime := time.Date(2026, 10, 16, 14, 30, 0, 0, time.Local)
	m.matches = []model.Match{
		{
			ID:        "101",
			Title:     "Arsenal vs Chelsea",
			IsOffline: true,
			CachedAt:  cachedTime,
			Status:    model.MatchStatus{State: "post", Description: "Final"},
		},
	}

	view := m.View()
	expectedBadge := "[OFFLINE · cached at 14:30]"
	if !strings.Contains(view, expectedBadge) {
		t.Errorf("expected %q in view, got:\n%s", expectedBadge, view)
	}
}

func TestModel_LiveBadgeRendering(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StateBrowsing

	m.matches = []model.Match{
		{
			ID:        "102",
			Title:     "Liverpool vs Man City",
			IsOffline: false,
			Status:    model.MatchStatus{State: "in", Description: "1T 30'"},
		},
	}

	view := m.View()
	expectedBadge := "[LIVE · 5s sync]"
	if !strings.Contains(view, expectedBadge) {
		t.Errorf("expected %q in view, got:\n%s", expectedBadge, view)
	}
}

func TestModel_AutoSyncTickMsg_LiveMatchModal(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StateChannelSelect
	m.matchCursor = 0
	m.matches = []model.Match{
		{
			ID:        "live-1",
			Title:     "Junior vs Nacional",
			Status:    model.MatchStatus{State: "in"},
			LeagueKey: "colombia",
		},
	}

	_, cmd := m.Update(autoSyncTickMsg(time.Now()))
	if cmd == nil {
		t.Fatalf("expected batch commands on autoSyncTickMsg, got nil")
	}
}

func TestModel_LeagueTabs_NavigationAndRendering(t *testing.T) {
	client := &mockClient{
		matches: []model.Match{
			{
				ID:        "m1",
				Title:     "Real Madrid vs Barcelona",
				HomeTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "2"},
				AwayTeam:  model.Competitor{DisplayName: "Barcelona", Score: "1"},
				TimeStr:   "16:00",
				Status:    model.MatchStatus{State: "post", Completed: true},
				LeagueKey: "laliga",
			},
			{
				ID:        "m2",
				Title:     "Atlético Madrid vs Sevilla",
				HomeTeam:  model.Competitor{DisplayName: "Atlético Madrid"},
				AwayTeam:  model.Competitor{DisplayName: "Sevilla"},
				TimeStr:   "18:30",
				Status:    model.MatchStatus{State: "pre"},
				LeagueKey: "laliga",
			},
		},
		standings: &model.LeagueStandings{
			LeagueKey: "laliga",
			Name:      "LaLiga",
			Entries: []model.StandingsEntry{
				{Rank: 1, TeamName: "Real Madrid", GamesPlayed: "10", Wins: "8", Ties: "1", Losses: "1", PointDiff: "+15", Points: "25"},
			},
		},
		statistics: &model.LeagueStatistics{
			LeagueKey: "laliga",
			Scorers: []model.ScorerEntry{
				{Rank: 1, PlayerName: "Kylian Mbappé", TeamName: "Real Madrid", Value: "8"},
			},
			Assists: []model.ScorerEntry{
				{Rank: 1, PlayerName: "Vinícius Jr", TeamName: "Real Madrid", Value: "5"},
			},
		},
	}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StateBrowsing
	m.matches = client.matches
	m.standings = client.standings
	m.statistics = client.statistics

	// Verify Tab 1 (Resumen) is default when browsing
	if m.leagueTab != TabResumen {
		t.Errorf("expected TabResumen as default, got %d", m.leagueTab)
	}
	viewResumen := m.View()
	if !strings.Contains(viewResumen, "Líder de Liga") {
		t.Errorf("expected 'Líder de Liga' highlight in TabResumen view, got:\n%s", viewResumen)
	}
	if !strings.Contains(viewResumen, "RESULTADOS RECIENTES") {
		t.Errorf("expected 'RESULTADOS RECIENTES' in TabResumen view")
	}

	// Press '2' -> Tab 2 (Partidos)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(Model)
	if m.leagueTab != TabPartidos {
		t.Errorf("expected TabPartidos after key '2', got %d", m.leagueTab)
	}
	viewPartidos := m.View()
	if !strings.Contains(viewPartidos, "PARTIDOS DE HOY") && !strings.Contains(viewPartidos, "PRÓXIMOS PARTIDOS") {
		t.Errorf("expected 'PARTIDOS DE HOY' or 'PRÓXIMOS PARTIDOS' in TabPartidos view, got:\n%s", viewPartidos)
	}

	// Press '3' -> Tab 3 (Posiciones)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = newM.(Model)
	if m.leagueTab != TabPosiciones {
		t.Errorf("expected TabPosiciones after key '3', got %d", m.leagueTab)
	}
	viewPosiciones := m.View()
	if !strings.Contains(viewPosiciones, "TABLA DE POSICIONES") {
		t.Errorf("expected 'TABLA DE POSICIONES' in TabPosiciones view, got:\n%s", viewPosiciones)
	}
	if !strings.Contains(viewPosiciones, "Real Madrid") {
		t.Errorf("expected team in standings view")
	}

	// Press '4' -> Tab 4 (Goleadores)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m = newM.(Model)
	if m.leagueTab != TabGoleadores {
		t.Errorf("expected TabGoleadores after key '4', got %d", m.leagueTab)
	}
	viewGoleadores := m.View()
	if !strings.Contains(viewGoleadores, "MÁXIMOS GOLEADORES") {
		t.Errorf("expected 'MÁXIMOS GOLEADORES' in TabGoleadores view, got:\n%s", viewGoleadores)
	}
	if !strings.Contains(viewGoleadores, "Kylian Mbappé") {
		t.Errorf("expected scorer in goleadores view")
	}
	if !strings.Contains(viewGoleadores, "MÁXIMOS ASISTENTES") {
		t.Errorf("expected 'MÁXIMOS ASISTENTES' in goleadores view")
	}

	// Cycle forward with 'l' key: TabGoleadores -> TabResumen
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = newM.(Model)
	if m.leagueTab != TabResumen {
		t.Errorf("expected TabResumen after 'l' key cycle, got %d", m.leagueTab)
	}

	// Cycle backward with 'h' key: TabResumen -> TabGoleadores
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.leagueTab != TabGoleadores {
		t.Errorf("expected TabGoleadores after 'h' key cycle, got %d", m.leagueTab)
	}
}

func TestModel_StartupWarmup_Animation(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p).WithInitialState(StateStartup)

	if m.state != StateStartup {
		t.Fatalf("expected initial state StateStartup, got %d", m.state)
	}

	// Verify renderStartup contains ANSI logo and initializing message
	startupView := m.View()
	if !strings.Contains(startupView, "Cargando Kickoff...") {
		t.Errorf("expected 'Cargando Kickoff...' in startup view, got:\n%s", startupView)
	}

	// Advance progress through ticks
	for i := 0; i < 9; i++ {
		newM, cmd := m.Update(startupTickMsg(time.Now()))
		m = newM.(Model)
		if m.state != StateStartup {
			t.Fatalf("expected state StateStartup at step %d, got %d", i, m.state)
		}
		if cmd == nil {
			t.Fatalf("expected startup tick command at step %d", i)
		}
	}

	// 10th tick reaches 100% and transitions to StateMainMenu
	newM, _ := m.Update(startupTickMsg(time.Now()))
	m = newM.(Model)
	if m.state != StateMainMenu {
		t.Errorf("expected transition to StateMainMenu after 100%% warmup, got %d", m.state)
	}
}

func TestModel_ConnectingStream_PulseAnimation(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StatePlaying
	m.connecting = true
	m.streamFeed = "ESPN 1 HD"
	m.bitrate = "auto • 1080p"

	view := m.View()
	if !strings.Contains(view, "Conectando con ESPN 1 HD") {
		t.Errorf("expected connecting message in view, got:\n%s", view)
	}
	if !strings.Contains(view, "📡") {
		t.Errorf("expected radar antenna emoji in connecting view")
	}
	if !strings.Contains(view, "1080p") {
		t.Errorf("expected bitrate/resolution indicator in connecting view")
	}

	// Pulse tick advances waves
	newM, cmd := m.Update(connectingPulseTickMsg(time.Now()))
	m = newM.(Model)
	if m.pulseStep != 1 {
		t.Errorf("expected pulseStep 1, got %d", m.pulseStep)
	}
	if cmd == nil {
		t.Errorf("expected connectingPulseTickCmd returned")
	}
}

func TestModel_SmoothBar_Fractions(t *testing.T) {
	// Test renderSmoothBar block characters
	bar0 := renderSmoothBar(0.0, 10)
	if strings.Contains(bar0, "█") {
		t.Errorf("expected empty bar for 0.0 fraction")
	}

	barHalf := renderSmoothBar(0.5, 10)
	if !strings.HasPrefix(barHalf, "█████") {
		t.Errorf("expected 5 full blocks for 0.5 fraction in 10 slots, got %q", barHalf)
	}

	barFull := renderSmoothBar(1.0, 10)
	if barFull != "██████████" {
		t.Errorf("expected 10 full blocks for 1.0 fraction, got %q", barFull)
	}

	// Test renderPossessionBar contains styled brackets and numbers
	posView := renderPossessionBar("65%", "35%", 60)
	if !strings.Contains(posView, "65%") || !strings.Contains(posView, "35%") {
		t.Errorf("expected possession numbers in posView, got:\n%s", posView)
	}
}

func TestModel_TabResumen_SymmetricalCards_Scroll_And_NvimNav(t *testing.T) {
	client := &mockClient{}
	cat := &mockCatalog{}
	p := &mockPlayer{}
	m := NewModel(client, cat, p)
	m.state = StateBrowsing
	m.height = 24 // Standard height -> windowSize = 3

	// Setup 6 completed past matches
	for i := 1; i <= 6; i++ {
		m.pastMatches = append(m.pastMatches, model.Match{
			ID:        fmt.Sprintf("past-%d", i),
			Title:     fmt.Sprintf("Team %d vs Rival %d", i, i),
			StartTime: time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC),
			Venue:     fmt.Sprintf("Estadio %d", i),
			HomeTeam:  model.Competitor{DisplayName: fmt.Sprintf("Team %d", i), Score: fmt.Sprintf("%d", i)},
			AwayTeam:  model.Competitor{DisplayName: fmt.Sprintf("Rival %d", i), Score: fmt.Sprintf("%d", i-1)},
			Status:    model.MatchStatus{State: "post", Completed: true, Description: "Final"},
		})
	}

	// 1. Initial State: Must be TabResumen
	if m.leagueTab != TabResumen {
		t.Fatalf("expected TabResumen as default, got %d", m.leagueTab)
	}
	if m.resumenCursor != 0 {
		t.Fatalf("expected initial resumenCursor 0, got %d", m.resumenCursor)
	}

	viewResumen := m.View()
	// Must contain symmetric cards with teams, scores and FINAL badge
	if !strings.Contains(viewResumen, "Team 1") || !strings.Contains(viewResumen, "[FINAL]") {
		t.Errorf("expected Team 1 and [FINAL] in symmetric card, got:\n%s", viewResumen)
	}
	// Must have scroll down indicator since there are 6 matches and windowSize is 3
	if !strings.Contains(viewResumen, "más abajo") {
		t.Errorf("expected down scroll indicator 'más abajo' in TabResumen, got:\n%s", viewResumen)
	}
	// Must NOT contain PRÓXIMOS ENCUENTROS
	if strings.Contains(viewResumen, "PRÓXIMOS ENCUENTROS") {
		t.Errorf("TabResumen should NOT contain 'PRÓXIMOS ENCUENTROS'")
	}

	// 2. Navigate down with 'j' (Neovim key)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.resumenCursor != 1 {
		t.Fatalf("expected resumenCursor 1 after 'j', got %d", m.resumenCursor)
	}

	// Navigate down to item 3 to scroll the window
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.resumenCursor != 3 {
		t.Fatalf("expected resumenCursor 3, got %d", m.resumenCursor)
	}

	viewScrolled := m.View()
	// Now it should have up scroll indicator
	if !strings.Contains(viewScrolled, "anteriores") {
		t.Errorf("expected up scroll indicator 'anteriores' after scrolling down, got:\n%s", viewScrolled)
	}

	// 3. Navigate up with 'k'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.resumenCursor != 2 {
		t.Fatalf("expected resumenCursor 2 after 'k', got %d", m.resumenCursor)
	}

	// 4. Press Enter on past match -> Opens modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.state != StateChannelSelect {
		t.Fatalf("expected StateChannelSelect after Enter on past match, got %d", m.state)
	}

	// Press Esc to close modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Fatalf("expected StateBrowsing after Esc, got %d", m.state)
	}

	// 5. Test Neovim tab switching ('l' to go right, 'h' to go left)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = newM.(Model)
	if m.leagueTab != TabPartidos {
		t.Fatalf("expected TabPartidos after 'l', got %d", m.leagueTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = newM.(Model)
	if m.leagueTab != TabPosiciones {
		t.Fatalf("expected TabPosiciones after Right arrow, got %d", m.leagueTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = newM.(Model)
	if m.leagueTab != TabPartidos {
		t.Fatalf("expected TabPartidos after Left arrow, got %d", m.leagueTab)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.leagueTab != TabResumen {
		t.Fatalf("expected TabResumen after 'h', got %d", m.leagueTab)
	}
}


