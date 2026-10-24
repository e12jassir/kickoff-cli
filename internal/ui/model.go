package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/bar"
	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/locale"
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

// playabilityResolver is the narrow slice of the catalog the TUI needs to enforce
// the visibility rule: a fixture whose only broadcasts cannot be played is hidden
// entirely, and a completely unconfigured catalog is surfaced as an actionable
// hint instead of an empty list.
//
// It is deliberately separate from model.Resolver rather than folded into it: the
// TUI is the only consumer of these methods, so keeping them out of the shared
// interface avoids forcing the status bar's resolver (and every test fake) to
// implement methods they never call. *catalog.Catalog satisfies this interface.
type playabilityResolver interface {
	// HasPlayableSource reports whether at least one emitted channel for the match
	// resolves to a playable stream URL.
	HasPlayableSource(match model.Match) bool
	// HasConfiguredStreams reports whether the user configured any stream source at
	// all, distinguishing "nothing playable because nothing is set up" from
	// "nothing playable for this fixture".
	HasConfiguredStreams() bool
}

// Model is the main Bubble Tea model for kickoff.
type Model struct {
	client  model.Client
	catalog model.Resolver
	player  model.Controller
	i18n    *i18n.Catalog

	state        UIState
	leagues      []model.League
	leagueCursor int
	activeTab    int
	leagueTab    LeagueTab // 0: Resumen, 1: Partidos, 2: Posiciones, 3: Goleadores

	// Accordion menu state. Every map and list is keyed by model.League.CategoryID,
	// the language-invariant category identity; categoryNames holds the localized
	// label that is only ever rendered.
	categories      []string // CategoryIDs in menu order
	categoryCursor  int
	categoryMode    bool              // true: cursor moves between categories; false: cursor moves between leagues of expanded category
	categoryNames   map[string]string // category ID -> localized display name
	categoryIndices map[string][]int  // category ID -> indices into m.leagues
	expandedCats    map[string]bool   // category ID -> whether it is expanded

	matches         []model.Match
	pastMatches     []model.Match
	matchCursor     int
	resumenCursor   int
	selectedMatchID string
	detailsLoaded   map[string]bool

	// noStreamsConfigured records that the catalog has no stream source at all, so
	// the upcoming-matches list was (correctly) filtered down to nothing. It drives
	// an actionable configuration hint instead of a silent empty list.
	noStreamsConfigured bool

	// loadGen is the generation of the currently displayed view data. Every
	// destructive transition bumps it so results from a previous view can be
	// identified and discarded instead of mutating the new state.
	loadGen uint64

	standings     *model.LeagueStandings
	standingsErr  error
	selectedGroup int

	statistics    *model.LeagueStatistics
	statisticsErr error

	channels        []model.Channel
	channelCursor   int
	channelsLoading bool
	modalTab        int

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

	// clock and loc define the wall clock the view renders against. Both are
	// injectable so day boundaries can be tested without depending on the
	// machine's timezone or the real clock.
	clock func() time.Time
	loc   *time.Location

	// goalTracker and notifier raise desktop notifications whenever a background
	// refresh observes a new goal, even when no match is open. The tracker primes
	// its baseline on the first observation so opening Kickoff never replays
	// goals that were already scored.
	goalTracker *bar.ScoreTracker
	notifier    bar.Notifier
}

// unknownCategoryID groups leagues that carry no CategoryID. It is a stable
// identity, so such leagues stay in one group regardless of display language.
const unknownCategoryID = "other"

// unknownCategoryName is the fallback label rendered for unknownCategoryID.
const unknownCategoryName = "Otras Competiciones"

// buildCategories derives the accordion menu structure from the league list.
// CategoryID is the group identity and Category only its label, so translating a
// label never changes grouping, ordering, or cursor state.
func buildCategories(leagues []model.League) (ids []string, names map[string]string, indices map[string][]int) {
	names = make(map[string]string)
	indices = make(map[string][]int)
	seen := make(map[string]bool)

	for i, league := range leagues {
		catID := league.CategoryID
		if catID == "" {
			catID = unknownCategoryID
		}
		if !seen[catID] {
			seen[catID] = true
			ids = append(ids, catID)
			label := league.Category
			if label == "" {
				label = catID
				if catID == unknownCategoryID {
					label = unknownCategoryName
				}
			}
			names[catID] = label
		}
		indices[catID] = append(indices[catID], i)
	}

	return ids, names, indices
}

// NewModel constructs a new UI model with defaults and dependencies.
func NewModel(client model.Client, catalog model.Resolver, player model.Controller, leagues ...model.League) Model {
	if len(leagues) == 0 {
		leagues = model.DefaultLeagues
	}

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = AppTitleStyle

	categories, categoryNames, categoryIndices := buildCategories(leagues)

	expandedCats := make(map[string]bool)
	// All categories start collapsed by default for a clean minimal layout

	return Model{
		client:          client,
		catalog:         catalog,
		player:          player,
		i18n:            i18n.Default(),
		state:           StateMainMenu,
		leagues:         leagues,
		leagueCursor:    0,
		categories:      categories,
		categoryCursor:  0,
		categoryMode:    true,
		categoryNames:   categoryNames,
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
		clock:           time.Now,
		loc:             locale.Location(),
		goalTracker:     bar.NewScoreTracker(),
		notifier:        bar.NewNotifier(),
	}
}

// currentCategoryID returns the CategoryID under the category cursor.
func (m Model) currentCategoryID() (string, bool) {
	if m.categoryCursor < 0 || m.categoryCursor >= len(m.categories) {
		return "", false
	}
	return m.categories[m.categoryCursor], true
}

// categoryName returns the localized label for a category ID. It falls back to
// the ID itself so an unknown category still renders something meaningful.
func (m Model) categoryName(catID string) string {
	if name := m.categoryNames[catID]; name != "" {
		return name
	}
	return catID
}

// WithClock overrides the wall clock and display timezone used by the view.
// Tests use it to pin day boundaries; production keeps time.Now and the locale
// resolver.
func (m Model) WithClock(now func() time.Time, loc *time.Location) Model {
	if now != nil {
		m.clock = now
	}
	if loc != nil {
		m.loc = loc
	}
	return m
}

// WithI18n overrides the localization catalog used by the view.
func (m Model) WithI18n(c *i18n.Catalog) Model {
	if c != nil {
		m.i18n = c
	}
	return m
}

func (m Model) cat() *i18n.Catalog {
	if m.i18n == nil {
		return i18n.Default()
	}
	return m.i18n
}

// displayNow returns the current instant in the user's display timezone.
func (m Model) displayNow() time.Time {
	now := time.Now
	if m.clock != nil {
		now = m.clock
	}
	loc := m.loc
	if loc == nil {
		loc = locale.Location()
	}
	return now().In(loc)
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
		return model.League{Key: "champions", Name: "UEFA Champions League", Code: "uefa.champions", Category: "Torneos Internacionales", CategoryID: "international"}
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
