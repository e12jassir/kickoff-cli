package setup

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSetup_NewModel_And_Diagnostics(t *testing.T) {
	m := NewModel("v0.2.2")
	if len(m.options) != 6 {
		t.Fatalf("expected 6 options, got %d", len(m.options))
	}
	if len(m.checks) == 0 {
		t.Fatalf("expected system checks to be populated")
	}

	foundMpv := false
	for _, c := range m.checks {
		if c.Name == "mpv" {
			foundMpv = true
			break
		}
	}
	if !foundMpv {
		t.Errorf("expected mpv check in diagnostics")
	}
}

func TestSetup_Navigation_CursorWrap(t *testing.T) {
	m := NewModel("v0.2.2")

	// Initial cursor is 0
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0, got %d", m.cursor)
	}

	// Move up -> wraps to last
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = newM.(Model)
	if m.cursor != len(m.options)-1 {
		t.Errorf("expected cursor to wrap to %d, got %d", len(m.options)-1, m.cursor)
	}

	// Move down -> wraps to 0
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newM.(Model)
	if m.cursor != 0 {
		t.Errorf("expected cursor to wrap back to 0, got %d", m.cursor)
	}

	// Move down with 'j'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 with 'j', got %d", m.cursor)
	}

	// Move up with 'k'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 with 'k', got %d", m.cursor)
	}
}

func TestSetup_QuitKey(t *testing.T) {
	m := NewModel("v0.2.2")
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = newM.(Model)
	if !m.quitting {
		t.Errorf("expected model quitting flag to be true on 'q'")
	}
	if cmd == nil {
		t.Errorf("expected quit cmd on 'q'")
	}
}

func TestSetup_View_Rendering(t *testing.T) {
	m := NewModel("v0.2.2")
	m.width = 84
	m.height = 35

	view := m.View()

	// 1. Verify Double Border elements
	if !strings.Contains(view, "╔") || !strings.Contains(view, "╗") || !strings.Contains(view, "╚") || !strings.Contains(view, "╝") {
		t.Errorf("expected view to contain double borders (╔═╗ ╚═╝)")
	}

	// 2. Verify Kickoff ANSI Shadow ASCII logo
	if !strings.Contains(view, "██╗  ██╗██╗ ██████╗██╗  ██╗ ██████╗ ███████╗███████╗") {
		t.Errorf("expected view to contain Kickoff ANSI Shadow ASCII logo")
	}

	// 3. Verify Version & Title
	if !strings.Contains(view, "Kickoff v0.2.2") {
		t.Errorf("expected view to contain version v0.2.2")
	}

	// 4. Verify System Diagnostics
	if !strings.Contains(view, "System Diagnostics") {
		t.Errorf("expected view to contain 'System Diagnostics'")
	}
	if !strings.Contains(view, "mpv:") {
		t.Errorf("expected view to contain mpv check")
	}

	// 5. Verify Menu Options and Active Cursor
	if !strings.Contains(view, "▸ Test MPV stream player") {
		t.Errorf("expected view to show active cursor on first option")
	}

	// 6. Verify Footer Keybindings Hint
	if !strings.Contains(view, "enter: select") || !strings.Contains(view, "q/esc: exit") {
		t.Errorf("expected footer keybindings in view")
	}

	// 7. Strictly NO Emojis in setup view
	disallowed := []string{"⚽", "✨", "🎉", "🚀", "📡", "⚙️"}
	for _, bad := range disallowed {
		if strings.Contains(view, bad) {
			t.Errorf("found disallowed emoji %q in setup view", bad)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{500, "500 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}

	for _, tc := range tests {
		if got := formatBytes(tc.bytes); got != tc.expected {
			t.Errorf("formatBytes(%d) = %q, expected %q", tc.bytes, got, tc.expected)
		}
	}
}
