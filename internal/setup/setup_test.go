package setup

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/updater"
)

func TestSetup_NewModel_And_Diagnostics(t *testing.T) {
	m := NewModel("v0.2.4")
	if len(m.options) != 8 {
		t.Fatalf("expected 8 options, got %d", len(m.options))
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
	m := NewModel("v0.2.4")

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
	m := NewModel("v0.2.4")
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
	m := NewModel("v0.2.4")
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
	if !strings.Contains(view, "Kickoff v0.2.4") {
		t.Errorf("expected view to contain version v0.2.4")
	}

	// 4. Verify Configuration & Setup header
	if !strings.Contains(view, "Configuration & Setup") {
		t.Errorf("expected view to contain 'Configuration & Setup'")
	}

	// 5. Verify Menu Options and Active Minimalist Cursor (> )
	if !strings.Contains(view, "> Install dependencies & setup") {
		t.Errorf("expected view to show active cursor on first option")
	}

	// 6. Verify Footer Keybindings Hint
	if !strings.Contains(view, "enter: select") || !strings.Contains(view, "q/esc: exit") {
		t.Errorf("expected footer keybindings in view")
	}

	// 7. Strictly NO Emojis in setup view
	disallowed := []string{"⚽", "✨", "🎉", "🚀", "📡", "⚙️", "📦", "🩺", "🗑️"}
	for _, bad := range disallowed {
		if strings.Contains(view, bad) {
			t.Errorf("found disallowed emoji %q in setup view", bad)
		}
	}
}

func TestSetup_Doctor_View(t *testing.T) {
	m := NewModel("v0.2.4")
	m.width = 84
	m.height = 35
	m.state = stateDoctor

	view := m.View()

	if !strings.Contains(view, "System Doctor (Diagnostics)") {
		t.Errorf("expected view to contain 'System Doctor (Diagnostics)'")
	}
	if !strings.Contains(view, "mpv:") {
		t.Errorf("expected doctor view to contain mpv check")
	}
	if !strings.Contains(view, "[t] Test stream playback") {
		t.Errorf("expected doctor view to contain stream playback action")
	}

	// Return to menu on Esc
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.state != stateMenu {
		t.Errorf("expected state to return to stateMenu on Esc, got %v", m.state)
	}
}

func TestSetup_Installing_View(t *testing.T) {
	m := NewModel("v0.2.4")
	m.width = 84
	m.height = 35
	m.state = stateInstalling
	m.installSteps[0].Status = "done"
	m.installSteps[0].Detail = "linux/amd64 detected"
	m.installSteps[1].Status = "running"

	view := m.View()

	if !strings.Contains(view, "Installing Kickoff Dependencies") {
		t.Errorf("expected view to contain 'Installing Kickoff Dependencies'")
	}
	if !strings.Contains(view, "[ok]") {
		t.Errorf("expected view to contain [ok] badge for completed step")
	}
	if !strings.Contains(view, "linux/amd64 detected") {
		t.Errorf("expected view to contain detail for completed step")
	}
}

func TestSetup_UpdatePrompt_View(t *testing.T) {
	m := NewModel("v0.2.3")
	m.width = 84
	m.height = 35
	m.state = stateUpdatePrompt
	m.latestRelease = &updater.ReleaseInfo{
		TagName: "v0.2.4",
	}

	view := m.View()

	if !strings.Contains(view, "Software Update Available") {
		t.Errorf("expected view to contain 'Software Update Available'")
	}
	if !strings.Contains(view, "v0.2.3") || !strings.Contains(view, "v0.2.4") {
		t.Errorf("expected view to display current and latest versions")
	}
	if !strings.Contains(view, "> [y] Yes, update now") {
		t.Errorf("expected view to display update choice")
	}
}

func TestSetup_UninstallPrompt_View(t *testing.T) {
	m := NewModel("v0.2.4")
	m.width = 84
	m.height = 35
	m.state = stateUninstallPrompt

	view := m.View()

	if !strings.Contains(view, "Uninstall Kickoff") {
		t.Errorf("expected view to contain 'Uninstall Kickoff'")
	}
	if !strings.Contains(view, "Are you sure you want to proceed?") {
		t.Errorf("expected view to ask for confirmation")
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

func TestSetup_Installer_InitStartsInstallation(t *testing.T) {
	m := startInstaller("v0.2.4", true)
	if m.state != stateInstalling {
		t.Fatalf("expected installer model in stateInstalling, got %v", m.state)
	}
	if len(m.installSteps) == 0 || m.installSteps[0].Status != "running" {
		t.Fatalf("expected first install step to be running")
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("expected Init to return the initial installation command")
	}

	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected Init to return a batch command")
	}

	foundFirstStep := false
	for _, batchCmd := range batch {
		if batchCmd == nil {
			continue
		}
		if msg, ok := batchCmd().(installStepMsg); ok {
			if msg.stepIndex != 0 {
				t.Errorf("expected initial install command to run step 0, got %d", msg.stepIndex)
			}
			if msg.nextStep != 1 {
				t.Errorf("expected initial step to queue step 1, got %d", msg.nextStep)
			}
			foundFirstStep = true
		}
	}
	if !foundFirstStep {
		t.Fatalf("expected Init command batch to contain the first install step result")
	}
}

func TestSetup_Installing_AbortConfirmation(t *testing.T) {
	m := NewModel("v0.2.4")
	m.state = stateInstalling
	m.installComplete = false
	m.installSteps[0].Status = "running"

	// ctrl+c asks for confirmation instead of being ignored.
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = newM.(Model)
	if !m.abortPrompt {
		t.Fatalf("expected ctrl+c to open the abort confirmation prompt")
	}
	if m.state != stateInstalling {
		t.Errorf("expected to stay in stateInstalling while confirming, got %v", m.state)
	}
	if cmd != nil {
		t.Errorf("expected no command while waiting for confirmation")
	}

	// esc cancels the confirmation and keeps the installation running.
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(Model)
	if m.abortPrompt {
		t.Errorf("expected esc to dismiss the abort confirmation prompt")
	}
	if m.state != stateInstalling {
		t.Errorf("expected to stay in stateInstalling after dismissing prompt, got %v", m.state)
	}

	// q opens the confirmation again and y aborts back to the menu.
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = newM.(Model)
	if !m.abortPrompt {
		t.Fatalf("expected q to open the abort confirmation prompt")
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(Model)
	if m.abortPrompt {
		t.Errorf("expected confirmation prompt to close after aborting")
	}
	if m.state != stateMenu {
		t.Errorf("expected abort to return to stateMenu, got %v", m.state)
	}
	if !m.installCancelled {
		t.Errorf("expected installation to be marked as cancelled")
	}

	// Late installStepMsg results must not resume the cancelled pipeline.
	newM, _ = m.Update(installStepMsg{stepIndex: 0, success: true, nextStep: 1})
	m = newM.(Model)
	if m.installSteps[0].Status != "running" {
		t.Errorf("expected cancelled installation to ignore late step results, got %q", m.installSteps[0].Status)
	}
}

func TestSetup_Installing_AbortPrompt_View(t *testing.T) {
	m := NewModel("v0.2.4")
	m.width = 84
	m.height = 35
	m.state = stateInstalling
	m.abortPrompt = true

	view := m.View()
	if !strings.Contains(view, "Abort installation?") {
		t.Errorf("expected installing view to show the abort confirmation")
	}
}
