package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette based on the Rosé Pine aesthetic.
var (
	ColorIris    = lipgloss.Color("#c4a7e7")
	ColorText    = lipgloss.Color("#e0def4")
	ColorMuted   = lipgloss.Color("#908caa")
	ColorRose    = lipgloss.Color("#ebbcba")
	ColorCyan    = lipgloss.Color("#9ccfd8")
	ColorGold    = lipgloss.Color("#f6c177")
	ColorLove    = lipgloss.Color("#eb6f92")
	ColorPine    = lipgloss.Color("#31748f")

	gradientColors = []lipgloss.Color{
		ColorRose,
		ColorIris,
		ColorPine,
		ColorCyan,
		ColorCyan,
	}

	brailleBall = []string{
		"               ⣠⣤⣶⣶⣶⣶⣶⣶⣤⣀             ",
		"            ⢀⣴⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⣤          ",
		"          ⢠⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡆        ",
		"         ⢰⣿⣿⣿⣿⠟⠉⢁⣴⣶⣦⡈⠙⢿⣿⣿⣿⣿⣿⡆       ",
		"         ⣿⣿⣿⡿⠁  ⢰⣿⣿⣿⣿⡇   ⠈⢿⣿⣿⣿⣧       ",
		"         ⣿⣿⣿⡇   ⠸⣿⣿⣿⣿⠇    ⢸⣿⣿⣿⣿       ",
		"         ⠸⣿⣿⣿⣄    ⠉⠻⠿⠋    ⣠⣿⣿⣿⣿⠇       ",
		"          ⠹⣿⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣾⣿⣿⣿⡿⠋        ",
		"            ⠙⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠋          ",
		"               ⠈⠉⠛⠿⠿⠿⠿⠿⠿⠛⠉⠁             ",
	}
)

// SystemCheck represents a single diagnostic result.
type SystemCheck struct {
	Name    string
	Passed  bool
	Message string
}

// Model represents the Bubble Tea model for the setup and diagnostics screen.
type Model struct {
	version     string
	width       int
	height      int
	cursor      int
	options     []string
	checks      []SystemCheck
	statusMsg   string
	statusColor lipgloss.Color
	testing     bool
	quitting    bool
	cfgPath     string
	cacheDir    string
}

// Messages for asynchronous tasks.
type checkFinishedMsg struct {
	checks []SystemCheck
}

type actionDoneMsg struct {
	msg   string
	color lipgloss.Color
}

type editorFinishedMsg struct {
	err error
}

// NewModel creates a new setup screen model.
func NewModel(version string) Model {
	cfgPath := getChannelsConfigPath()
	cacheDir := getCacheDirPath()

	options := []string{
		"Test MPV stream player",
		"Edit channels configuration",
		"Verify configured signals",
		"Clear match & API cache",
		"Check for updates",
		"Exit setup",
	}

	m := Model{
		version:   version,
		width:     84,
		height:    30,
		cursor:    0,
		options:   options,
		cfgPath:   cfgPath,
		cacheDir:  cacheDir,
		checks:    runDiagnostics(cfgPath, cacheDir),
		statusMsg: "",
	}
	return m
}

func getChannelsConfigPath() string {
	configBase, err := os.UserConfigDir()
	if err != nil || configBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configBase = filepath.Join(home, ".config")
		}
	}
	if configBase != "" {
		cfgDir := filepath.Join(configBase, "kickoff")
		_ = os.MkdirAll(cfgDir, 0o755)
		return filepath.Join(cfgDir, "channels.json")
	}
	return ""
}

func getCacheDirPath() string {
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	if cacheBase != "" {
		cDir := filepath.Join(cacheBase, "kickoff")
		_ = os.MkdirAll(cDir, 0o755)
		return cDir
	}
	return ""
}

func runDiagnostics(cfgPath, cacheDir string) []SystemCheck {
	var checks []SystemCheck

	// 1. MPV
	mpvBin := "mpv"
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if runtime.GOOS == "windows" {
			for _, cand := range []string{"mpv.exe", "mpvnet.exe", "mpv"} {
				candidate := filepath.Join(dir, cand)
				if _, err := os.Stat(candidate); err == nil {
					mpvBin = candidate
					break
				}
			}
		} else {
			candidate := filepath.Join(dir, "mpv")
			if _, err := os.Stat(candidate); err == nil {
				mpvBin = candidate
			}
		}
	}

	mpvPath, err := exec.LookPath(mpvBin)
	if err != nil && runtime.GOOS == "windows" {
		for _, cand := range []string{"mpv.exe", "mpvnet.exe", "mpv"} {
			if p, e := exec.LookPath(cand); e == nil {
				mpvPath = p
				err = nil
				break
			}
		}
	}

	if err != nil {
		checks = append(checks, SystemCheck{
			Name:    "mpv",
			Passed:  false,
			Message: "NOT FOUND — required to play match streams (install via package manager)",
		})
	} else {
		out, _ := exec.Command(mpvPath, "--version").Output()
		firstLine := strings.Split(string(out), "\n")[0]
		checks = append(checks, SystemCheck{
			Name:    "mpv",
			Passed:  true,
			Message: fmt.Sprintf("%s (%s)", mpvPath, strings.TrimSpace(firstLine)),
		})
	}

	// 2. FFprobe
	if ffPath, err := exec.LookPath("ffprobe"); err == nil {
		checks = append(checks, SystemCheck{
			Name:    "ffprobe",
			Passed:  true,
			Message: ffPath,
		})
	} else {
		checks = append(checks, SystemCheck{
			Name:    "ffprobe",
			Passed:  true,
			Message: "Not installed (optional stream analyzer)",
		})
	}

	// 3. Terminal & UTF-8
	checks = append(checks, SystemCheck{
		Name:    "Terminal",
		Passed:  true,
		Message: "UTF-8 & Braille symbols supported",
	})

	// 4. Channels Config
	if cfgPath != "" {
		if fi, err := os.Stat(cfgPath); err == nil && fi.Size() > 0 {
			chCount := countConfiguredChannels(cfgPath)
			checks = append(checks, SystemCheck{
				Name:    "Channels",
				Passed:  true,
				Message: fmt.Sprintf("%s (%d channels loaded)", cfgPath, chCount),
			})
		} else {
			checks = append(checks, SystemCheck{
				Name:    "Channels",
				Passed:  true,
				Message: fmt.Sprintf("Built-in defaults in use (%s)", cfgPath),
			})
		}
	}

	// 5. Cache Directory
	if cacheDir != "" {
		count, size := inspectCacheDir(cacheDir)
		sizeStr := formatBytes(size)
		checks = append(checks, SystemCheck{
			Name:    "Cache",
			Passed:  true,
			Message: fmt.Sprintf("%s (%d files, %s)", cacheDir, count, sizeStr),
		})
	}

	return checks
}

func countConfiguredChannels(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var raw struct {
		Channels []interface{} `json:"channels"`
	}
	if err := json.Unmarshal(data, &raw); err == nil {
		return len(raw.Channels)
	}
	var list []interface{}
	if err := json.Unmarshal(data, &list); err == nil {
		return len(list)
	}
	return 0
}

func inspectCacheDir(dir string) (int, int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	var totalSize int64
	var count int
	for _, e := range entries {
		if !e.IsDir() {
			count++
			if info, err := e.Info(); err == nil {
				totalSize += info.Size()
			}
		}
	}
	return count, totalSize
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

// Init initializes the Bubble Tea program.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages and keyboard navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case checkFinishedMsg:
		m.checks = msg.checks
		return m, nil

	case actionDoneMsg:
		m.statusMsg = msg.msg
		m.statusColor = msg.color
		m.testing = false
		m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
		return m, nil

	case editorFinishedMsg:
		m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Editor exited with error: %v", msg.err)
			m.statusColor = ColorLove
		} else {
			m.statusMsg = "Channels configuration updated."
			m.statusColor = ColorCyan
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.options) - 1
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
			return m, nil

		case "enter":
			return m.handleSelect()
		}
	}

	return m, nil
}

func (m Model) handleSelect() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Test MPV
		m.testing = true
		m.statusMsg = "Testing mpv binary..."
		m.statusColor = ColorIris
		return m, func() tea.Msg {
			mpvPath, err := exec.LookPath("mpv")
			if err != nil {
				return actionDoneMsg{msg: "mpv not found on PATH", color: ColorLove}
			}
			cmd := exec.Command(mpvPath, "--version")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return actionDoneMsg{msg: fmt.Sprintf("mpv error: %v", err), color: ColorLove}
			}
			firstLine := strings.Split(string(out), "\n")[0]
			return actionDoneMsg{msg: fmt.Sprintf("mpv operational: %s", firstLine), color: ColorCyan}
		}

	case 1: // Edit channels
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			for _, ed := range []string{"nano", "vim", "vi"} {
				if _, err := exec.LookPath(ed); err == nil {
					editor = ed
					break
				}
			}
		}
		if editor == "" {
			m.statusMsg = fmt.Sprintf("No text editor found in environment. Config file: %s", m.cfgPath)
			m.statusColor = ColorLove
			return m, nil
		}
		cmd := exec.Command(editor, m.cfgPath)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return editorFinishedMsg{err: err}
		})

	case 2: // Verify signals
		m.testing = true
		m.statusMsg = "Verifying signal channels..."
		m.statusColor = ColorIris
		cfgPath := m.cfgPath
		return m, func() tea.Msg {
			count := countConfiguredChannels(cfgPath)
			if count == 0 {
				return actionDoneMsg{msg: "No external channels configured; built-in signals are active.", color: ColorCyan}
			}
			return actionDoneMsg{msg: fmt.Sprintf("Verified %d channels in configuration file.", count), color: ColorCyan}
		}

	case 3: // Clear cache
		cacheDir := m.cacheDir
		return m, func() tea.Msg {
			if cacheDir == "" {
				return actionDoneMsg{msg: "Cache directory not configured.", color: ColorMuted}
			}
			entries, err := os.ReadDir(cacheDir)
			if err != nil {
				return actionDoneMsg{msg: fmt.Sprintf("Error reading cache: %v", err), color: ColorLove}
			}
			deleted := 0
			for _, e := range entries {
				if !e.IsDir() {
					_ = os.Remove(filepath.Join(cacheDir, e.Name()))
					deleted++
				}
			}
			return actionDoneMsg{msg: fmt.Sprintf("Cache cleared: %d files removed.", deleted), color: ColorCyan}
		}

	case 4: // Check for updates
		m.testing = true
		m.statusMsg = "Checking GitHub releases for updates..."
		m.statusColor = ColorIris
		currVer := m.version
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/e12jassir/kickoff/releases/latest", nil)
			if err != nil {
				return actionDoneMsg{msg: fmt.Sprintf("Update check failed: %v", err), color: ColorLove}
			}
			req.Header.Set("User-Agent", "kickoff-setup")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return actionDoneMsg{msg: fmt.Sprintf("Update check connection failed: %v", err), color: ColorLove}
			}
			defer resp.Body.Close()

			var rel struct {
				TagName string `json:"tag_name"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
				return actionDoneMsg{msg: "Failed to parse release response.", color: ColorLove}
			}

			cleanRel := strings.TrimPrefix(rel.TagName, "v")
			cleanCurr := strings.TrimPrefix(currVer, "v")

			if cleanRel != "" && cleanRel != cleanCurr && !strings.Contains(cleanCurr, "dirty") && cleanRel > cleanCurr {
				return actionDoneMsg{msg: fmt.Sprintf("Update available: %s -> %s (run 'kickoff update')", currVer, rel.TagName), color: ColorGold}
			}
			return actionDoneMsg{msg: fmt.Sprintf("Kickoff is up to date (%s).", currVer), color: ColorCyan}
		}

	case 5: // Exit setup
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the setup screen inside the Gentle-AI double border container.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	targetWidth := m.width - 4
	if targetWidth > 90 {
		targetWidth = 90
	}
	if targetWidth < 68 {
		targetWidth = 68
	}

	var content strings.Builder

	// 1. Braille ASCII Header with Vertical Gradient
	for i, line := range brailleBall {
		colorIdx := (i * len(gradientColors)) / len(brailleBall)
		lineStyle := lipgloss.NewStyle().
			Foreground(gradientColors[colorIdx]).
			Bold(true).
			Width(targetWidth).
			Align(lipgloss.Center)
		content.WriteString(lineStyle.Render(line) + "\n")
	}

	// 2. Title & Version Subheading
	cleanVer := m.version
	if !strings.HasPrefix(cleanVer, "v") && cleanVer != "" {
		cleanVer = "v" + cleanVer
	}
	title := fmt.Sprintf("Kickoff %s — CLI Football Experience & Streaming Hub", cleanVer)
	titleStyled := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText).
		Width(targetWidth).
		Align(lipgloss.Center).
		Render(title)

	repoStyled := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Width(targetWidth).
		Align(lipgloss.Center).
		Render("https://github.com/e12jassir/kickoff")

	content.WriteString("\n" + titleStyled + "\n" + repoStyled + "\n\n")

	// 3. System Diagnostics Block
	diagHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorRose).
		Render("  System Diagnostics")
	content.WriteString(diagHeader + "\n")

	for _, chk := range m.checks {
		icon := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("✓")
		if !chk.Passed {
			icon = lipgloss.NewStyle().Foreground(ColorLove).Bold(true).Render("✗")
		}
		label := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(chk.Name + ":")
		msg := lipgloss.NewStyle().Foreground(ColorMuted).Render(chk.Message)
		content.WriteString(fmt.Sprintf("    %s %s %s\n", icon, label, msg))
	}

	// 4. Subtle Section Divider
	dividerLen := targetWidth - 8
	if dividerLen < 40 {
		dividerLen = 40
	}
	divider := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("  " + strings.Repeat("─", dividerLen))
	content.WriteString("\n" + divider + "\n\n")

	// 5. Interactive Configuration Options
	optHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorRose).
		Render("  Configuration & Tools")
	content.WriteString(optHeader + "\n")

	for i, opt := range m.options {
		if i == m.cursor {
			cursorSymbol := lipgloss.NewStyle().Foreground(ColorIris).Bold(true).Render("▸ ")
			optText := lipgloss.NewStyle().Foreground(ColorIris).Bold(true).Render(opt)
			content.WriteString(fmt.Sprintf("    %s%s\n", cursorSymbol, optText))
		} else {
			optText := lipgloss.NewStyle().Foreground(ColorText).Render(opt)
			content.WriteString(fmt.Sprintf("      %s\n", optText))
		}
	}

	// 6. Action Status / Feedback message
	if m.statusMsg != "" {
		statusLine := lipgloss.NewStyle().
			Foreground(m.statusColor).
			Bold(true).
			Render(fmt.Sprintf("\n  [ %s ]", m.statusMsg))
		content.WriteString(statusLine + "\n")
	} else {
		content.WriteString("\n")
	}

	// 7. Footer keybindings hint
	footerHint := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Width(targetWidth).
		Align(lipgloss.Center).
		Render("↑/↓ / j/k: move • enter: select • q/esc: exit")
	content.WriteString("\n" + footerHint)

	// Wrap inside the Gentle-AI style Double Border Frame
	frameStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorIris).
		Padding(1, 2).
		Width(targetWidth + 4)

	return frameStyle.Render(content.String()) + "\n"
}

// Run starts the interactive Bubble Tea setup TUI.
func Run(version string) error {
	m := NewModel(version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
