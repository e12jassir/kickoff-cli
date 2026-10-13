package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff/internal/model"
)

type mockClient struct {
	matches []model.Match
	err     error
}

func (m *mockClient) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	return m.matches, m.err
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

	if m.state != StateMainMenu {
		t.Fatalf("expected initial state StateMainMenu, got %d", m.state)
	}
	if m.leagueCursor != 0 {
		t.Fatalf("expected initial leagueCursor 0, got %d", m.leagueCursor)
	}

	// Move down with 'j'
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.leagueCursor != 1 {
		t.Errorf("expected leagueCursor 1 after 'j', got %d", m.leagueCursor)
	}

	// Move up with 'k'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.leagueCursor != 0 {
		t.Errorf("expected leagueCursor 0 after 'k', got %d", m.leagueCursor)
	}

	// Wrap around up from 0 to last
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = newM.(Model)
	if m.leagueCursor != len(m.leagues)-1 {
		t.Errorf("expected leagueCursor %d after wrap-up, got %d", len(m.leagues)-1, m.leagueCursor)
	}

	// Wrap around down from last to 0
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newM.(Model)
	if m.leagueCursor != 0 {
		t.Errorf("expected leagueCursor 0 after wrap-down, got %d", m.leagueCursor)
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

	// Test 'h' key returns to main menu as well
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, _ = m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   []model.Match{{ID: "m2"}},
	})
	m = newM.(Model)

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.state != StateMainMenu {
		t.Fatalf("expected state StateMainMenu after 'h', got %d", m.state)
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
	if !strings.Contains(viewStr, "Este partido ya finalizó") {
		t.Errorf("expected modal to inform match is finished, got:\n%s", viewStr)
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
	if !strings.Contains(viewStr, "No hay partidos programados") {
		t.Errorf("expected empty state message, got:\n%s", viewStr)
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
