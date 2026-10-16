package ui

import (
	"strings"
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

	// Accordion menu state
	categories      []string
	categoryCursor  int
	categoryMode    bool             // true: cursor moves between categories; false: cursor moves between leagues of expanded category
	categoryIndices map[string][]int // category name -> indices into m.leagues
	expandedCats    map[string]bool  // category name -> whether it is expanded

	matches         []model.Match
	pastMatches     []model.Match
	matchCursor     int
	resumenCursor   int
	selectedMatchID string
	detailsLoaded   map[string]bool

	standings     *model.LeagueStandings
	standingsErr  error
	selectedGroup int

	statistics    *model.LeagueStatistics
	statisticsErr error

	channels      []model.Channel
	channelCursor int
	modalTab      int
	lineupTeam    int // 0: Home, 1: Away

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

	var categories []string
	categoryIndices := make(map[string][]int)
	seenCat := make(map[string]bool)
	for i, l := range leagues {
		cat := l.Category
		if cat == "" {
			cat = "Otras Competiciones"
		}
		if !seenCat[cat] {
			seenCat[cat] = true
			categories = append(categories, cat)
		}
		categoryIndices[cat] = append(categoryIndices[cat], i)
	}

	expandedCats := make(map[string]bool)
	// All categories start collapsed by default for a clean minimal layout

	return Model{
		client:          client,
		catalog:         catalog,
		player:          player,
		state:           StateMainMenu,
		leagues:         leagues,
		leagueCursor:    0,
		categories:      categories,
		categoryCursor:  0,
		categoryMode:    true,
		categoryIndices: categoryIndices,
		expandedCats:    expandedCats,
		activeTab:       0,
		leagueTab:       TabResumen,
		matches:         nil,
		pastMatches:     nil,
		matchCursor:     0,
		resumenCursor:   0,
		selectedMatchID: "",
		detailsLoaded:   make(map[string]bool),
		channels:        nil,
		channelCursor:   0,
		modalTab:        0,
		lineupTeam:      0,
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

// ResumenMatches returns the deduplicated list of recent completed matches for TabResumen.
func (m Model) ResumenMatches() []model.Match {
	var pastMatches []model.Match
	seenIDs := make(map[string]bool)

	for _, mt := range m.pastMatches {
		id := mt.ID
		if id == "" {
			id = mt.HomeTeam.DisplayName + mt.AwayTeam.DisplayName
		}
		if !seenIDs[id] {
			seenIDs[id] = true
			pastMatches = append(pastMatches, mt)
		}
	}

	for _, mt := range m.matches {
		if mt.Status.Completed || strings.ToLower(mt.Status.State) == "post" {
			id := mt.ID
			if id == "" {
				id = mt.HomeTeam.DisplayName + mt.AwayTeam.DisplayName
			}
			if !seenIDs[id] {
				seenIDs[id] = true
				pastMatches = append(pastMatches, mt)
			}
		}
	}

	return pastMatches
}

// SelectedMatchID returns the currently anchored match ID in modal or playback.
func (m Model) SelectedMatchID() string {
	return m.selectedMatchID
}

// IsDetailsLoaded returns whether detailed stats/lineups have finished loading for the given match ID.
func (m Model) IsDetailsLoaded(matchID string) bool {
	if m.detailsLoaded == nil {
		return false
	}
	return m.detailsLoaded[matchID]
}

// WithDetailsLoaded marks match details as loaded for the specified match IDs.
func (m Model) WithDetailsLoaded(matchIDs ...string) Model {
	if m.detailsLoaded == nil {
		m.detailsLoaded = make(map[string]bool)
	}
	for _, id := range matchIDs {
		m.detailsLoaded[id] = true
	}
	return m
}

// CurrentSelectedMatch returns the currently selected match based on active modal anchor, tab, and bounds checks.
func (m Model) CurrentSelectedMatch() (model.Match, bool) {
	if (m.state == StateChannelSelect || m.state == StatePlaying) && m.selectedMatchID != "" {
		for _, mt := range m.matches {
			if mt.ID == m.selectedMatchID {
				return mt, true
			}
		}
		for _, mt := range m.pastMatches {
			if mt.ID == m.selectedMatchID {
				return mt, true
			}
		}
	}
	if m.leagueTab == TabResumen {
		pms := m.ResumenMatches()
		if len(pms) > 0 && m.resumenCursor >= 0 && m.resumenCursor < len(pms) {
			return pms[m.resumenCursor], true
		}
		return model.Match{}, false
	}
	if len(m.matches) > 0 && m.matchCursor >= 0 && m.matchCursor < len(m.matches) {
		return m.matches[m.matchCursor], true
	}
	return model.Match{}, false
}

// LineupTeam returns the currently selected team index for tactical lineups (0: Home, 1: Away).
func (m Model) LineupTeam() int {
	return m.lineupTeam
}

// ToggleLineupTeam toggles between Home (0) and Away (1) for tactical lineups.
func (m *Model) ToggleLineupTeam() {
	m.lineupTeam = 1 - m.lineupTeam
}
