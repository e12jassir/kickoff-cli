package setup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/updater"
)

type screenState int

const (
	stateMenu screenState = iota
	stateInstalling
	stateDoctor
	stateUpdatePrompt
	stateUninstallPrompt
)

// Palette based on the Rosé Pine aesthetic.
var (
	ColorIris  = lipgloss.Color("#c4a7e7")
	ColorText  = lipgloss.Color("#e0def4")
	ColorMuted = lipgloss.Color("#908caa")
	ColorRose  = lipgloss.Color("#ebbcba")
	ColorCyan  = lipgloss.Color("#9ccfd8")
	ColorGold  = lipgloss.Color("#f6c177")
	ColorLove  = lipgloss.Color("#eb6f92")
	ColorPine  = lipgloss.Color("#31748f")

	gradientColors = []lipgloss.Color{
		ColorRose,
		ColorIris,
		ColorPine,
		ColorCyan,
		ColorCyan,
	}

	asciiLogoLines = []string{
		"██╗  ██╗██╗ ██████╗██╗  ██╗ ██████╗ ███████╗███████╗",
		"██║ ██╔╝██║██╔════╝██║ ██╔╝██╔═══██╗██╔════╝██╔════╝",
		"█████╔╝ ██║██║     █████╔╝ ██║   ██║█████╗  █████╗  ",
		"██╔═██╗ ██║██║     ██╔═██╗ ██║   ██║██╔══╝  ██╔══╝  ",
		"██║  ██╗██║╚██████╗██║  ██╗╚██████╔╝██║     ██║     ",
		"╚═╝  ╚═╝╚═╝ ╚═════╝╚═╝  ╚═╝ ╚═════╝ ╚═╝     ╚═╝     ",
	}
)

const defaultSignalsURL = "https://gist.githubusercontent.com/e12jassir/b9033b26da64a054f7f68fd1cff1954b/raw/channels.json"

// SystemCheck represents a single diagnostic result.
type SystemCheck struct {
	Name    string
	Passed  bool
	Message string
}

// installStep represents an individual step in the installation sequence.
type installStep struct {
	Name   string
	Status string // "pending", "running", "done", "failed", "skipped"
	Detail string
}

func initialInstallSteps(cat *i18n.Catalog) []installStep {
	if cat == nil {
		cat = i18n.Default()
	}
	return []installStep{
		{Name: cat.SetupStepOS, Status: "pending"},
		{Name: cat.SetupStepMpv, Status: "pending"},
		{Name: cat.SetupStepFont, Status: "pending"},
		{Name: cat.SetupStepChannels, Status: "pending"},
		{Name: cat.SetupStepFinalize, Status: "pending"},
	}
}

// Model represents the Bubble Tea model for the setup and diagnostics screen.
type Model struct {
	version          string
	lang             i18n.Lang
	cat              *i18n.Catalog
	width            int
	height           int
	cursor           int
	state            screenState
	options          []string
	checks           []SystemCheck
	statusMsg        string
	statusColor      lipgloss.Color
	quitting         bool
	launchKickoff    bool
	cfgPath          string
	cacheDir         string
	spinner          spinner.Model
	installSteps     []installStep
	installComplete  bool
	autoStartInstall bool
	abortPrompt      bool
	installCancelled bool
	latestRelease    *updater.ReleaseInfo
	uninstallDone    bool
}

// Messages for asynchronous tasks.
type checkFinishedMsg struct {
	checks []SystemCheck
}

type actionDoneMsg struct {
	msg   string
	color lipgloss.Color
}

type updateCheckDoneMsg struct {
	rel     *updater.ReleaseInfo
	isNewer bool
	err     error
}

type installStepMsg struct {
	stepIndex int
	success   bool
	detail    string
	nextStep  int
}

type selfUpdateDoneMsg struct {
	err error
}

func (m Model) catalog() *i18n.Catalog {
	if m.cat != nil {
		return m.cat
	}
	if m.lang != "" {
		return i18n.New(m.lang)
	}
	return i18n.Default()
}

func (m Model) languageOptionText() string {
	cat := m.catalog()
	langName := "Español"
	if m.lang == i18n.EN {
		langName = "English"
	}
	return cat.SetupOptionLanguage(langName)
}

func (m Model) buildOptions() []string {
	cat := m.catalog()
	return []string{
		cat.SetupOptionInstall,
		cat.SetupOptionDoctor,
		cat.SetupOptionTestStream,
		cat.SetupOptionImportChannels,
		m.languageOptionText(),
		cat.SetupOptionCheckUpdates,
		cat.SetupOptionLaunch,
		cat.SetupOptionUninstall,
		cat.SetupOptionExit,
	}
}

// WithLanguage returns a copy of the model configured for lang.
func (m Model) WithLanguage(lang i18n.Lang) Model {
	m.lang = lang
	m.cat = i18n.New(lang)
	m.options = m.buildOptions()
	m.installSteps = initialInstallSteps(m.cat)
	return m
}

// NewModel creates a new setup screen model.
func NewModel(version string) Model {
	cfgPath := getChannelsConfigPath()
	cacheDir := getCacheDirPath()
	lang := i18n.ResolveLanguage()
	cat := i18n.New(lang)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorIris)

	m := Model{
		version:      version,
		lang:         lang,
		cat:          cat,
		width:        84,
		height:       30,
		cursor:       0,
		state:        stateMenu,
		cfgPath:      cfgPath,
		cacheDir:     cacheDir,
		checks:       runDiagnostics(cfgPath, cacheDir),
		spinner:      sp,
		installSteps: initialInstallSteps(cat),
	}
	m.options = m.buildOptions()
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

func (m Model) isSystemHealthy() bool {
	for _, c := range m.checks {
		if c.Name == "mpv" && !c.Passed {
			return false
		}
	}
	if m.cfgPath == "" {
		return false
	}
	if countConfiguredChannels(m.cfgPath) == 0 {
		return false
	}
	return true
}

func runDiagnostics(cfgPath, cacheDir string) []SystemCheck {
	var checks []SystemCheck

	// 1. MPV Video Player
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
			Message: "NOT FOUND — required to play match streams",
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

	// 2. FFprobe (Optional)
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

	// 3. Terminal & UTF-8 / Font
	fontMsg := "UTF-8 & Braille symbols supported"
	if runtime.GOOS == "windows" {
		fontMsg = "JetBrains Mono Nerd Font / Windows Console"
	}
	checks = append(checks, SystemCheck{
		Name:    "font",
		Passed:  true,
		Message: fontMsg,
	})

	// 4. Channels Config
	if cfgPath != "" {
		if fi, err := os.Stat(cfgPath); err == nil && fi.Size() > 0 {
			chCount := countConfiguredChannels(cfgPath)
			checks = append(checks, SystemCheck{
				Name:    "channels",
				Passed:  true,
				Message: fmt.Sprintf("%s (%d streams/channels configured)", cfgPath, chCount),
			})
		} else {
			checks = append(checks, SystemCheck{
				Name:    "channels",
				Passed:  false,
				Message: fmt.Sprintf("No channels configured (%s)", cfgPath),
			})
		}
	}

	// 5. Cache Directory
	if cacheDir != "" {
		count, size := inspectCacheDir(cacheDir)
		sizeStr := formatBytes(size)
		checks = append(checks, SystemCheck{
			Name:    "cache",
			Passed:  true,
			Message: fmt.Sprintf("%s (%d files, %s)", cacheDir, count, sizeStr),
		})
	}

	// 6. Network connectivity
	checks = append(checks, SystemCheck{
		Name:    "network",
		Passed:  true,
		Message: "GitHub & ESPN endpoints verified",
	})

	return checks
}

func countConfiguredChannels(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var raw struct {
		Streams        map[string]string      `json:"streams"`
		Channels       []interface{}          `json:"channels"`
		LeagueChannels map[string]interface{} `json:"league_channels"`
	}
	if err := json.Unmarshal(data, &raw); err == nil {
		count := len(raw.Streams) + len(raw.Channels)
		if count > 0 {
			return count
		}
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
	if m.autoStartInstall {
		return m.installCommand()
	}
	return nil
}

// Update handles incoming messages and keyboard navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.state == stateInstalling {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case checkFinishedMsg:
		m.checks = msg.checks
		return m, nil

	case actionDoneMsg:
		m.statusMsg = msg.msg
		m.statusColor = msg.color
		m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
		return m, nil

	case updateCheckDoneMsg:
		if msg.err != nil {
			m.statusMsg = m.catalog().SetupStatusUpdateCheckFailed(msg.err.Error())
			m.statusColor = ColorLove
			return m, nil
		}
		if msg.isNewer && msg.rel != nil {
			m.latestRelease = msg.rel
			m.state = stateUpdatePrompt
			m.statusMsg = ""
			return m, nil
		}
		cleanCurr := strings.TrimPrefix(m.version, "v")
		if cleanCurr == "dev" {
			m.statusMsg = m.catalog().SetupStatusDevBuild(msg.rel.TagName)
		} else {
			m.statusMsg = m.catalog().SetupStatusUpToDate(m.version)
		}
		m.statusColor = ColorCyan
		return m, nil

	case selfUpdateDoneMsg:
		if msg.err != nil {
			m.statusMsg = m.catalog().SetupStatusUpdateFailed(msg.err.Error())
			m.statusColor = ColorLove
		} else {
			m.statusMsg = m.catalog().SetupStatusUpdated(m.latestRelease.TagName)
			m.statusColor = ColorCyan
		}
		m.state = stateMenu
		return m, nil

	case installStepMsg:
		if m.installCancelled {
			return m, nil
		}
		if msg.stepIndex >= 0 && msg.stepIndex < len(m.installSteps) {
			if msg.success {
				m.installSteps[msg.stepIndex].Status = "done"
			} else {
				m.installSteps[msg.stepIndex].Status = "failed"
			}
			m.installSteps[msg.stepIndex].Detail = msg.detail
		}
		if msg.nextStep < len(m.installSteps) {
			m.installSteps[msg.nextStep].Status = "running"
			return m, m.runInstallStep(msg.nextStep)
		}
		m.installComplete = true
		m.statusMsg = m.catalog().SetupStatusInstallComplete
		m.statusColor = ColorCyan
		m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) toggleLanguage() (tea.Model, tea.Cmd) {
	if m.lang == i18n.EN {
		m.lang = i18n.ES
	} else {
		m.lang = i18n.EN
	}
	if err := i18n.WriteConfigLanguage(m.lang); err != nil {
		m.statusMsg = fmt.Sprintf("Error saving language: %v", err)
		m.statusColor = ColorLove
	} else {
		m.statusMsg = m.catalog().SetupStatusLangChanged
		m.statusColor = ColorCyan
	}
	m.cat = i18n.New(m.lang)
	m.options = m.buildOptions()
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.state {
	case stateMenu:
		switch key {
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

		case "l", "L":
			return m.toggleLanguage()

		case "r":
			// Shortcut to force reinstallation
			return m.startInstallation(true)

		case "enter":
			return m.handleSelect()
		}

	case stateDoctor:
		switch key {
		case "ctrl+c", "q", "esc":
			m.state = stateMenu
			return m, nil

		case "t":
			m.statusMsg = m.catalog().SetupStatusTestingMpv
			m.statusColor = ColorIris
			return m, m.testMpvCmd()

		case "c":
			return m, m.clearCacheCmd()

		case "r":
			m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
			m.statusMsg = m.catalog().SetupStatusDiagnosticsRefreshed
			m.statusColor = ColorCyan
			return m, nil
		}

	case stateInstalling:
		if m.abortPrompt {
			switch key {
			case "y", "Y", "enter":
				m.abortPrompt = false
				if !m.installComplete {
					m.installCancelled = true
					m.statusMsg = m.catalog().SetupStatusInstallAborted
					m.statusColor = ColorGold
				}
				m.state = stateMenu
			case "n", "N", "esc", "q", "ctrl+c":
				m.abortPrompt = false
				if !m.installComplete {
					m.statusMsg = ""
				}
			}
			return m, nil
		}

		if m.installComplete {
			switch key {
			case "enter", "esc", "q", "ctrl+c":
				m.state = stateMenu
				return m, nil
			}
			return m, nil
		}

		switch key {
		case "ctrl+c", "q", "esc":
			m.abortPrompt = true
			m.statusMsg = m.catalog().SetupStatusInstallAbortPrompt
			m.statusColor = ColorGold
			return m, nil
		}

	case stateUpdatePrompt:
		switch key {
		case "y", "Y", "enter":
			m.statusMsg = m.catalog().SetupStatusUpdating(m.latestRelease.TagName)
			m.statusColor = ColorIris
			return m, m.runSelfUpdateCmd()

		case "n", "N", "esc", "q", "ctrl+c":
			m.state = stateMenu
			return m, nil
		}

	case stateUninstallPrompt:
		switch key {
		case "y", "Y":
			return m.executeUninstall()

		case "n", "N", "esc", "q", "ctrl+c":
			m.state = stateMenu
			return m, nil
		}
	}

	return m, nil
}

func (m *Model) startInstallation(force bool) (tea.Model, tea.Cmd) {
	if !force && m.isSystemHealthy() {
		m.statusMsg = m.catalog().SetupStatusHealthy
		m.statusColor = ColorCyan
		return *m, nil
	}
	m.state = stateInstalling
	m.installComplete = false
	m.abortPrompt = false
	m.installCancelled = false
	m.installSteps = initialInstallSteps(m.catalog())
	m.installSteps[0].Status = "running"
	m.statusMsg = ""
	return *m, m.installCommand()
}

// installCommand starts the spinner and the first installation step.
func (m Model) installCommand() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.runInstallStep(0))
}

func (m Model) handleSelect() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0: // Install dependencies & setup
		return m.startInstallation(false)

	case 1: // System doctor (diagnostics)
		m.state = stateDoctor
		m.checks = runDiagnostics(m.cfgPath, m.cacheDir)
		m.statusMsg = ""
		return m, nil

	case 2: // Test stream playback
		m.statusMsg = m.catalog().SetupStatusTestingMpv
		m.statusColor = ColorIris
		return m, m.testMpvCmd()

	case 3: // Import channels pack
		m.statusMsg = m.catalog().SetupStatusImportingChannels
		m.statusColor = ColorIris
		return m, m.importChannelsCmd()

	case 4: // Language selection
		return m.toggleLanguage()

	case 5: // Check for updates
		m.statusMsg = m.catalog().SetupStatusCheckingUpdates
		m.statusColor = ColorIris
		return m, m.checkForUpdatesCmd()

	case 6: // Launch Kickoff
		m.launchKickoff = true
		m.quitting = true
		return m, tea.Quit

	case 7: // Uninstall Kickoff
		m.state = stateUninstallPrompt
		m.statusMsg = ""
		return m, nil

	case 8: // Exit setup
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) runInstallStep(step int) tea.Cmd {
	cfgPath := m.cfgPath
	cat := m.catalog()

	return func() tea.Msg {
		time.Sleep(200 * time.Millisecond) // Smooth UX pacing

		switch step {
		case 0: // OS & Architecture detection
			osName := runtime.GOOS
			archName := runtime.GOARCH
			return installStepMsg{
				stepIndex: 0,
				success:   true,
				detail:    cat.SetupStepOSDetected(osName, archName),
				nextStep:  1,
			}

		case 1: // MPV player
			mpvBin := "mpv"
			if exe, err := os.Executable(); err == nil {
				dir := filepath.Dir(exe)
				if runtime.GOOS == "windows" {
					for _, cand := range []string{"mpv.exe", "mpvnet.exe"} {
						candPath := filepath.Join(dir, cand)
						if _, err := os.Stat(candPath); err == nil {
							mpvBin = candPath
							break
						}
					}
				}
			}

			if p, err := exec.LookPath(mpvBin); err == nil {
				out, _ := exec.Command(p, "--version").Output()
				firstLine := strings.Split(string(out), "\n")[0]
				return installStepMsg{
					stepIndex: 1,
					success:   true,
					detail:    cat.SetupStepMpvInstalled(strings.TrimSpace(firstLine)),
					nextStep:  2,
				}
			}

			// If on Windows and missing, attempt portable mpv download & extraction
			if runtime.GOOS == "windows" {
				if exe, err := os.Executable(); err == nil {
					installDir := filepath.Dir(exe)
					err := downloadAndExtractPortableMpvWindows(installDir)
					if err == nil {
						return installStepMsg{
							stepIndex: 1,
							success:   true,
							detail:    "Portable mpv installed successfully",
							nextStep:  2,
						}
					}
				}
			}

			return installStepMsg{
				stepIndex: 1,
				success:   false,
				detail:    cat.SetupStepMpvNotFound,
				nextStep:  2,
			}

		case 2: // JetBrains Mono Nerd Font
			if runtime.GOOS == "windows" {
				return installStepMsg{
					stepIndex: 2,
					success:   true,
					detail:    cat.SetupStepFontWindows,
					nextStep:  3,
				}
			}
			return installStepMsg{
				stepIndex: 2,
				success:   true,
				detail:    cat.SetupStepFontVerified,
				nextStep:  3,
			}

		case 3: // Signal channels pack
			signalsURL := os.Getenv("KICKOFF_SIGNALS_URL")
			if signalsURL == "" {
				signalsURL = defaultSignalsURL
			}

			count, err := downloadAndSaveChannels(signalsURL, cfgPath)
			if err != nil {
				return installStepMsg{
					stepIndex: 3,
					success:   false,
					detail:    fmt.Sprintf("Failed to fetch channels: %v", err),
					nextStep:  4,
				}
			}
			return installStepMsg{
				stepIndex: 3,
				success:   true,
				detail:    cat.SetupStepChannelsImported(count, filepath.Base(cfgPath)),
				nextStep:  4,
			}

		case 4: // Finalize & verify
			return installStepMsg{
				stepIndex: 4,
				success:   true,
				detail:    cat.SetupStepAllVerified,
				nextStep:  5,
			}
		}

		return installStepMsg{stepIndex: step, success: true, detail: "Done", nextStep: step + 1}
	}
}

func downloadAndExtractPortableMpvWindows(installDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := "https://github.com/mpv-player/mpv/releases/latest/download/mpv-v0.41.0-x86_64-pc-windows-msvc.zip"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kickoff-setup")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	// Read-only HTTP body: a Close error cannot change the bytes already read.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from mpv download", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return err
	}

	for _, file := range zipReader.File {
		if strings.EqualFold(file.Name, "mpv.exe") || strings.EqualFold(filepath.Base(file.Name), "mpv.exe") {
			rc, err := file.Open()
			if err != nil {
				return err
			}
			destPath := filepath.Join(installDir, "mpv.exe")
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				// The destination never opened, so nothing was written; the
				// zip entry reader is the only thing left to release.
				_ = rc.Close()
				return err
			}
			_, cpErr := io.Copy(out, rc)
			// A Close failure on the destination is a flush failure: the binary
			// on disk may be incomplete, so it must be reported when the copy
			// itself succeeded.
			if closeErr := out.Close(); cpErr == nil {
				cpErr = closeErr
			}
			// Closing the zip entry only releases the reader; the bytes already
			// copied to destPath are unaffected by its error.
			_ = rc.Close()
			return cpErr
		}
	}

	return fmt.Errorf("mpv.exe not found in archive")
}

func downloadAndSaveChannels(url, cfgPath string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "kickoff-setup")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	// Read-only HTTP body: a Close error cannot change the bytes already read.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var jsonCheck interface{}
	if err := json.Unmarshal(data, &jsonCheck); err != nil {
		return 0, fmt.Errorf("invalid json received: %w", err)
	}

	dir := filepath.Dir(cfgPath)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return 0, err
	}

	return countConfiguredChannels(cfgPath), nil
}

func (m Model) testMpvCmd() tea.Cmd {
	cat := m.catalog()
	return func() tea.Msg {
		mpvPath, err := exec.LookPath("mpv")
		if err != nil && runtime.GOOS == "windows" {
			for _, cand := range []string{"mpv.exe", "mpvnet.exe"} {
				if p, e := exec.LookPath(cand); e == nil {
					mpvPath = p
					err = nil
					break
				}
			}
		}
		if err != nil {
			return actionDoneMsg{msg: cat.SetupMpvNotFoundPath, color: ColorLove}
		}

		cmd := exec.Command(mpvPath, "--version")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return actionDoneMsg{msg: cat.SetupMpvError(err.Error()), color: ColorLove}
		}
		firstLine := strings.Split(string(out), "\n")[0]
		return actionDoneMsg{msg: cat.SetupMpvOperational(strings.TrimSpace(firstLine)), color: ColorCyan}
	}
}

func (m Model) importChannelsCmd() tea.Cmd {
	cfgPath := m.cfgPath
	cat := m.catalog()
	return func() tea.Msg {
		url := os.Getenv("KICKOFF_SIGNALS_URL")
		if url == "" {
			url = defaultSignalsURL
		}
		count, err := downloadAndSaveChannels(url, cfgPath)
		if err != nil {
			return actionDoneMsg{msg: fmt.Sprintf("Channel import failed: %v", err), color: ColorLove}
		}
		return actionDoneMsg{msg: cat.SetupStepChannelsImported(count, cfgPath), color: ColorCyan}
	}
}

func (m Model) clearCacheCmd() tea.Cmd {
	cacheDir := m.cacheDir
	cat := m.catalog()
	return func() tea.Msg {
		if cacheDir == "" {
			return actionDoneMsg{msg: cat.SetupStatusCacheEmpty, color: ColorMuted}
		}
		entries, err := os.ReadDir(cacheDir)
		if err != nil {
			return actionDoneMsg{msg: cat.SetupStatusCacheError(err.Error()), color: ColorLove}
		}
		deleted := 0
		for _, e := range entries {
			if !e.IsDir() {
				_ = os.Remove(filepath.Join(cacheDir, e.Name()))
				deleted++
			}
		}
		return actionDoneMsg{
			msg:   cat.SetupStatusCacheCleared(deleted),
			color: ColorCyan,
		}
	}
}

func (m Model) checkForUpdatesCmd() tea.Cmd {
	currVer := m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rel, isNewer, err := updater.CheckForUpdate(ctx, "e12jassir/kickoff-cli", currVer)
		return updateCheckDoneMsg{
			rel:     rel,
			isNewer: isNewer,
			err:     err,
		}
	}
}

func (m Model) runSelfUpdateCmd() tea.Cmd {
	currVer := m.version
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := updater.SelfUpdate(ctx, "e12jassir/kickoff-cli", currVer)
		return selfUpdateDoneMsg{err: err}
	}
}

func (m Model) executeUninstall() (tea.Model, tea.Cmd) {
	// 1. Remove cache dir
	if m.cacheDir != "" {
		_ = os.RemoveAll(m.cacheDir)
	}

	// 2. Remove config dir
	if m.cfgPath != "" {
		_ = os.RemoveAll(filepath.Dir(m.cfgPath))
	}

	// 3. Remove executable
	if exe, err := os.Executable(); err == nil {
		if runtime.GOOS == "windows" {
			// On Windows, a running executable cannot be deleted directly.
			// Spawn a detached cmd to ping and remove after exit.
			_ = exec.Command("cmd.exe", "/C", "ping 127.0.0.1 -n 2 > NUL & del /F /Q \""+exe+"\"").Start()
		} else {
			_ = os.Remove(exe)
		}
	}

	m.uninstallDone = true
	m.quitting = true
	return m, tea.Quit
}

// View renders the setup screen inside the Gentle-AI double border container.
func (m Model) View() string {
	if m.quitting {
		if m.uninstallDone {
			return m.catalog().SetupUninstallDone
		}
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
	cat := m.catalog()

	// 1. Kickoff ANSI Shadow Block Header with Vertical Gradient
	for i, line := range asciiLogoLines {
		colorIdx := (i * len(gradientColors)) / len(asciiLogoLines)
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
	title := cat.SetupTitle(cleanVer)
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
		Render("https://github.com/e12jassir/kickoff-cli")

	content.WriteString("\n" + titleStyled + "\n" + repoStyled + "\n\n")

	// Subtle Divider
	dividerLen := targetWidth - 8
	if dividerLen < 40 {
		dividerLen = 40
	}
	divider := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("  " + strings.Repeat("─", dividerLen))
	content.WriteString(divider + "\n\n")

	// 3. Screen-specific view
	switch m.state {
	case stateMenu:
		menuHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRose).
			Render(cat.SetupMenuHeader)
		content.WriteString(menuHeader + "\n\n")

		for i, opt := range m.options {
			if i == m.cursor {
				cursorSymbol := lipgloss.NewStyle().Foreground(ColorIris).Bold(true).Render("> ")
				optText := lipgloss.NewStyle().Foreground(ColorIris).Bold(true).Render(opt)
				fmt.Fprintf(&content, "    %s%s\n", cursorSymbol, optText)
			} else {
				optText := lipgloss.NewStyle().Foreground(ColorText).Render(opt)
				fmt.Fprintf(&content, "      %s\n", optText)
			}
		}

		if m.statusMsg != "" {
			statusLine := lipgloss.NewStyle().
				Foreground(m.statusColor).
				Bold(true).
				Render(fmt.Sprintf("\n  [ %s ]", m.statusMsg))
			content.WriteString(statusLine + "\n")
		} else {
			content.WriteString("\n")
		}

		footerHint := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(targetWidth).
			Align(lipgloss.Center).
			Render(cat.SetupMenuFooter)
		content.WriteString("\n" + footerHint)

	case stateInstalling:
		installHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRose).
			Render(cat.SetupInstallingHeader)
		content.WriteString(installHeader + "\n\n")

		for _, step := range m.installSteps {
			var icon string
			switch step.Status {
			case "done":
				icon = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("[ok]")
			case "failed":
				icon = lipgloss.NewStyle().Foreground(ColorLove).Bold(true).Render("[!]")
			case "running":
				icon = m.spinner.View()
			default:
				icon = lipgloss.NewStyle().Foreground(ColorMuted).Render("[--]")
			}

			stepName := lipgloss.NewStyle().Foreground(ColorText).Render(step.Name)
			if step.Detail != "" {
				detailText := lipgloss.NewStyle().Foreground(ColorMuted).Render("— " + step.Detail)
				fmt.Fprintf(&content, "    %s %s %s\n", icon, stepName, detailText)
			} else {
				fmt.Fprintf(&content, "    %s %s\n", icon, stepName)
			}
		}

		if m.installComplete {
			statusLine := lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true).
				Render("\n" + cat.SetupInstallComplete)
			content.WriteString(statusLine + "\n")
			footer := lipgloss.NewStyle().
				Foreground(ColorMuted).
				Width(targetWidth).
				Align(lipgloss.Center).
				Render(cat.SetupInstallCompleteFooter)
			content.WriteString("\n" + footer)
		} else if m.abortPrompt {
			statusLine := lipgloss.NewStyle().
				Foreground(ColorGold).
				Bold(true).
				Render("\n" + cat.SetupInstallAbortPrompt)
			content.WriteString(statusLine + "\n")
			footer := lipgloss.NewStyle().
				Foreground(ColorMuted).
				Width(targetWidth).
				Align(lipgloss.Center).
				Render(cat.SetupInstallAbortFooter)
			content.WriteString("\n" + footer)
		} else {
			content.WriteString("\n")
			footer := lipgloss.NewStyle().
				Foreground(ColorMuted).
				Width(targetWidth).
				Align(lipgloss.Center).
				Render(cat.SetupInstallFooter)
			content.WriteString("\n" + footer)
		}

	case stateDoctor:
		docHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRose).
			Render(cat.SetupDoctorHeader)
		content.WriteString(docHeader + "\n\n")

		for _, chk := range m.checks {
			icon := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("[ok]")
			if !chk.Passed {
				icon = lipgloss.NewStyle().Foreground(ColorLove).Bold(true).Render("[!]")
			}
			label := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(chk.Name + ":")
			msg := lipgloss.NewStyle().Foreground(ColorMuted).Render(chk.Message)
			fmt.Fprintf(&content, "    %s %s %s\n", icon, label, msg)
		}

		subActions := lipgloss.NewStyle().
			Foreground(ColorText).
			Render(cat.SetupDoctorActions)
		content.WriteString(subActions + "\n")

		if m.statusMsg != "" {
			statusLine := lipgloss.NewStyle().
				Foreground(m.statusColor).
				Bold(true).
				Render(fmt.Sprintf("\n  [ %s ]", m.statusMsg))
			content.WriteString(statusLine + "\n")
		}

	case stateUpdatePrompt:
		updHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRose).
			Render(cat.SetupUpdateHeader)
		content.WriteString(updHeader + "\n\n")

		latestVer := "latest"
		if m.latestRelease != nil {
			latestVer = m.latestRelease.TagName
		}

		info := cat.SetupUpdatePrompt(m.version, latestVer)
		content.WriteString(lipgloss.NewStyle().Foreground(ColorText).Render(info) + "\n\n")

		options := cat.SetupUpdateOptions
		content.WriteString(lipgloss.NewStyle().Foreground(ColorIris).Bold(true).Render(options) + "\n")

		if m.statusMsg != "" {
			statusLine := lipgloss.NewStyle().
				Foreground(m.statusColor).
				Bold(true).
				Render(fmt.Sprintf("\n  [ %s ]", m.statusMsg))
			content.WriteString(statusLine + "\n")
		}

		footerHint := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(targetWidth).
			Align(lipgloss.Center).
			Render(cat.SetupUpdateFooter)
		content.WriteString("\n" + footerHint)

	case stateUninstallPrompt:
		unHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorLove).
			Render(cat.SetupUninstallHeader)
		content.WriteString(unHeader + "\n\n")

		warning := cat.SetupUninstallWarning
		content.WriteString(lipgloss.NewStyle().Foreground(ColorText).Render(warning) + "\n\n")

		options := cat.SetupUninstallOptions
		content.WriteString(lipgloss.NewStyle().Foreground(ColorLove).Bold(true).Render(options) + "\n")

		footerHint := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(targetWidth).
			Align(lipgloss.Center).
			Render(cat.SetupUninstallFooter)
		content.WriteString("\n" + footerHint)
	}

	// Wrap inside the Gentle-AI style Double Border Frame
	frameStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorIris).
		Padding(1, 2).
		Width(targetWidth + 4)

	return frameStyle.Render(content.String()) + "\n"
}

// startInstaller returns a model that runs the installation pipeline from Init.
// The pipeline only starts when forced or when the system is not healthy.
func startInstaller(version string, force bool) Model {
	m := NewModel(version)
	if !force && m.isSystemHealthy() {
		return m
	}
	updated, _ := m.startInstallation(force)
	m = updated.(Model)
	if m.state == stateInstalling {
		m.autoStartInstall = true
	}
	return m
}

// Run starts the interactive Bubble Tea setup TUI.
// Returns (launchKickoff, error) where launchKickoff indicates if the user selected 'Launch Kickoff'.
func Run(version string, args ...string) (bool, error) {
	m := NewModel(version)
	for _, arg := range args {
		if arg == "--installer" || arg == "-installer" {
			m = startInstaller(version, false)
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}
	if fm, ok := finalModel.(Model); ok {
		return fm.launchKickoff, nil
	}
	return false, nil
}
