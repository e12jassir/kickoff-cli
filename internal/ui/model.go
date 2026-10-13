package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff/internal/model"
)

// UIState represents the current view mode of the TUI.
type UIState int

const (
	StateLoading UIState = iota
	StateBrowsing
	StateChannelSelect
	StatePlaying
	StateError
)

// Model is the main Bubble Tea model for kickoff.
type Model struct {
	client  model.Client
	catalog model.Resolver
	player  model.Controller

	state     UIState
	leagues   []model.League
	activeTab int

	matches     []model.Match
	matchCursor int

	channels      []model.Channel
	channelCursor int

	spinner    spinner.Model
	loadingMsg string
	statusMsg  string
	errorMsg   string

	width  int
	height int
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
		client:        client,
		catalog:       catalog,
		player:        player,
		state:         StateLoading,
		leagues:       leagues,
		activeTab:     0,
		matches:       nil,
		matchCursor:   0,
		channels:      nil,
		channelCursor: 0,
		spinner:       sp,
		loadingMsg:    "Consultando partidos de hoy (" + leagues[0].Name + ")...",
		statusMsg:     "",
		errorMsg:      "",
		width:         80,
		height:        24,
	}
}

// CurrentLeague returns the currently selected league.
func (m Model) CurrentLeague() model.League {
	if len(m.leagues) == 0 {
		return model.League{Key: "laliga", Name: "La Liga (España)", Code: "esp.1"}
	}
	if m.activeTab < 0 || m.activeTab >= len(m.leagues) {
		return m.leagues[0]
	}
	return m.leagues[m.activeTab]
}

// Init begins spinner tick and loads fixtures for the initial league tab.
func (m Model) Init() tea.Cmd {
	league := m.CurrentLeague()
	return tea.Batch(
		m.spinner.Tick,
		fetchMatchesCmd(m.client, league.Key),
	)
}
