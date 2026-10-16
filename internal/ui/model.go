package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// UIState represents the current view mode of the TUI.
type UIState int

const (
	StateStartup UIState = iota
	StateMainMenu
	StateLoading
	StateBrowsing
	StateChannelSelect
	StatePlaying
	StateError
)

// LeagueTab represents the 4 views per league.
type LeagueTab int

const (
	TabResumen LeagueTab = iota
	TabPartidos
	TabPosiciones
	TabGoleadores
)

// Model is the main Bubble Tea model for kickoff.
type Model struct {
	client  model.Client
	catalog model.Resolver
	player  model.Controller

	state        UIState
	leagues      []model.League
	leagueCursor int
	activeTab    int
	leagueTab    LeagueTab // 0: Resumen, 1: Partidos, 2: Posiciones, 3: Goleadores

	matches     []model.Match
	matchCursor int

	standings     *model.LeagueStandings
	standingsErr  error
	selectedGroup int

	statistics    *model.LeagueStatistics
	statisticsErr error

	channels      []model.Channel
	channelCursor int
	modalTab      int

	livePulse  bool
	pulseStep  int
	playbackID int64
	connecting bool
	streamFeed string
	bitrate    string

	// Startup animation progress (0-100)
	startupProgress int

	spinner    spinner.Model
	loadingMsg string
	statusMsg  string
	errorMsg   string

	width  int
	height int

	version         string
	updateAvailable string
	noEmoji         bool
}

// NewModel constructs a new UI model with defaults and dependencies.
func NewModel(client model.Client, catalog model.Resolver, player model.Controller, leagues ...model.League) Model {
	if len(leagues) == 0 {
		leagues = model.DefaultLeagues
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = AppTitleStyle

	return Model{
		client:          client,
		catalog:         catalog,
		player:          player,
		state:           StateMainMenu,
		leagues:         leagues,
		leagueCursor:    0,
		activeTab:       0,
		leagueTab:       TabPartidos,
		matches:         nil,
		matchCursor:     0,
		channels:        nil,
		channelCursor:   0,
		modalTab:        0,
		livePulse:       true,
		pulseStep:       0,
		startupProgress: 0,
		spinner:         sp,
		loadingMsg:      "",
		statusMsg:       "",
		errorMsg:        "",
		width:           80,
		height:          24,
		noEmoji:         false,
	}
}

// WithVersion sets the app version on the model.
func (m Model) WithVersion(v string) Model {
	m.version = v
	return m
}

// WithNoEmoji sets whether emoji characters are replaced with clean ASCII alternatives.
func (m Model) WithNoEmoji(disabled bool) Model {
	m.noEmoji = disabled
	return m
}

// WithInitialState overrides the default initial state (e.g. StateMainMenu for testing).
func (m Model) WithInitialState(st UIState) Model {
	m.state = st
	return m
}

// CurrentLeague returns the currently selected league.
func (m Model) CurrentLeague() model.League {
	if len(m.leagues) == 0 {
		return model.League{Key: "champions", Name: "UEFA Champions League", Code: "uefa.champions", Emoji: "🏆", Category: "Torneos Internacionales"}
	}
	if m.activeTab < 0 || m.activeTab >= len(m.leagues) {
		return m.leagues[0]
	}
	return m.leagues[m.activeTab]
}

// Init begins spinner tick, pulse ticker, startup animation, and automatic smart live-sync ticker.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		pulseTickCmd(),
		startupTickCmd(),
		autoSyncTickCmd(15 * time.Second),
	}
	if m.version != "" {
		cmds = append(cmds, checkUpdateCmd(m.version))
	}
	return tea.Batch(cmds...)
}
