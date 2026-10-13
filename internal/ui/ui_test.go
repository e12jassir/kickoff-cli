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

func TestModel_TabNavigation(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	if m.activeTab != 0 {
		t.Fatalf("expected initial activeTab 0, got %d", m.activeTab)
	}

	// Move right
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = newM.(Model)
	if m.activeTab != 1 {
		t.Errorf("expected activeTab 1 after 'l', got %d", m.activeTab)
	}
	if cmd == nil {
		t.Errorf("expected fetch cmd when tab changes")
	}

	// Move left
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.activeTab != 0 {
		t.Errorf("expected activeTab 0 after 'h', got %d", m.activeTab)
	}

	// Move left from 0 (wrap around)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.activeTab != len(m.leagues)-1 {
		t.Errorf("expected wrap around to last tab, got %d", m.activeTab)
	}
}

func TestModel_MatchesLoaded_And_Modal(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{
		channels: []model.Channel{
			{Name: "ESPN 1 HD (720p)", DisplayName: "ESPN 1 HD (720p)", URL: "http://example.com/1.m3u8", BitrateMode: "min"},
			{Name: "ESPN 1 HD (1080p)", DisplayName: "ESPN 1 HD (1080p)", URL: "http://example.com/2.m3u8", BitrateMode: "max"},
		},
	}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Simulate matches loaded
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

	newM, _ := m.Update(matchesLoadedMsg{
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

	// Move down
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
		t.Fatalf("expected 2 channels in modal, got %d", len(m.channels))
	}

	// Verify view contains modal
	viewStr := m.View()
	if !strings.Contains(viewStr, "Señales de transmisión") {
		t.Errorf("expected view to contain modal title, got:\n%s", viewStr)
	}

	// Press Esc to close modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != StateBrowsing {
		t.Fatalf("expected StateBrowsing after Esc, got %d", m.state)
	}

	// Reopen modal and trigger play
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	newM, playCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if playCmd == nil {
		t.Fatalf("expected playCmd on channel select")
	}
	if !strings.Contains(m.statusMsg, "Conectando con") {
		t.Errorf("expected statusMsg connecting, got %s", m.statusMsg)
	}

	// Simulate playback started msg
	newM, _ = m.Update(playbackStartedMsg{channel: m.channels[0]})
	m = newM.(Model)
	if !strings.Contains(m.statusMsg, "Reproduciendo") {
		t.Errorf("expected statusMsg playing, got %s", m.statusMsg)
	}
}

func TestModel_EmptyMatches_And_Error(t *testing.T) {
	cli := &mockClient{}
	cat := &mockCatalog{}
	pla := &mockPlayer{}

	m := NewModel(cli, cat, pla)

	// Simulate empty matches loaded
	newM, _ := m.Update(matchesLoadedMsg{
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
