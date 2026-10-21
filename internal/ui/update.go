package ui

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/updater"
)

// Async messages for Bubble Tea event loop.
type updateAvailableMsg struct {
	tagName string
}

type updateFinishedMsg struct {
	err error
}

func checkUpdateCmd(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rel, newer, err := updater.CheckForUpdate(ctx, "", currentVersion)
		if err == nil && newer && rel != nil {
			return updateAvailableMsg{tagName: rel.TagName}
		}
		return nil
	}
}

type matchesLoadedMsg struct {
	leagueKey string
	matches   []model.Match
	err       error
	isSilent  bool
	gen       uint64
}

type matchStatsLoadedMsg struct {
	matchID    string
	stats      *model.MatchStats
	lineups    *model.MatchLineups
	referee    string
	attendance int
	err        error
}

type playbackStartedMsg struct {
	playbackID int64
	channel    model.Channel
}

type playbackErrMsg struct {
	err error
}

type playbackFinishedMsg struct {
	playbackID int64
}

type pulseTickMsg time.Time

type startupTickMsg time.Time

type connectingPulseTickMsg time.Time

type autoSyncTickMsg time.Time

type standingsLoadedMsg struct {
	leagueKey string
	standings *model.LeagueStandings
	err       error
	gen       uint64
}

type statisticsLoadedMsg struct {
	leagueKey  string
	statistics *model.LeagueStatistics
	err        error
	gen        uint64
}

type webChannelsLoadedMsg struct {
	matchID  string
	channels []model.Channel
}

// channelsResolvedMsg carries the catalog channel lookup for a match. The lookup
// runs inside a command so a slow catalog can never stall the event loop.
type channelsResolvedMsg struct {
	matchID  string
	gen      uint64
	channels []model.Channel
}

func resolveChannelsCmd(catalog model.Resolver, match model.Match, gen uint64) tea.Cmd {
	return func() tea.Msg {
		return channelsResolvedMsg{
			matchID:  match.ID,
			gen:      gen,
			channels: catalog.GetChannels(match),
		}
	}
}

type webStreamsResolver interface {
	ResolveWebChannels(ctx context.Context, match model.Match) []model.Channel
}

type prewarmResolver interface {
	PrewarmMatches(ctx context.Context, matches []model.Match)
}

type peekCachedResolver interface {
	PeekCachedChannels(match model.Match) []model.Channel
}

func prewarmMatchesCmd(catalog model.Resolver, matches []model.Match) tea.Cmd {
	return func() tea.Msg {
		if pw, ok := catalog.(prewarmResolver); ok && len(matches) > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			pw.PrewarmMatches(ctx, matches)
		}
		return nil
	}
}

func isTopLeague(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "uefa.champions", "champions", "eng.1", "premier", "esp.1", "laliga", "col.1", "colombia", "ita.1", "seriea", "ger.1", "bundesliga", "conmebol.libertadores", "libertadores":
		return true
	default:
		return false
	}
}

func fetchWebChannelsCmd(catalog model.Resolver, match model.Match) tea.Cmd {
	return func() tea.Msg {
		if res, ok := catalog.(webStreamsResolver); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			chs := res.ResolveWebChannels(ctx, match)
			return webChannelsLoadedMsg{
				matchID:  match.ID,
				channels: chs,
			}
		}
		return webChannelsLoadedMsg{matchID: match.ID}
	}
}

func pulseTickCmd() tea.Cmd {
	return tea.Tick(800*time.Millisecond, func(t time.Time) tea.Msg {
		return pulseTickMsg(t)
	})
}

func startupTickCmd() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
		return startupTickMsg(t)
	})
}

func connectingPulseTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return connectingPulseTickMsg(t)
	})
}

func autoSyncTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return autoSyncTickMsg(t)
	})
}

// autoSyncCommands builds the work for one auto-sync tick. The matches fetch runs
// in every state, the main menu included, so goal notifications keep arriving
// while the user has not opened any match. Stats are only refreshed when a live
// match is selected in the channel picker, because that is the only view that
// renders them.
func (m Model) autoSyncCommands() []tea.Cmd {
	cmds := []tea.Cmd{autoSyncTickCmd(m.nextSyncInterval())}
	if m.client == nil {
		return cmds
	}

	league := m.CurrentLeague()
	cmds = append(cmds, forceRefreshMatchesCmd(m.client, league.Key, true, m.loadGen))

	if m.state == StateChannelSelect {
		if selMatch, ok := m.CurrentSelectedMatch(); ok && selMatch.ID != "" && selMatch.Status.State == "in" {
			cmds = append(cmds, forceFetchMatchStatsCmd(m.client, league.Key, selMatch.ID))
		}
	}
	return cmds
}

// nextSyncInterval dynamically computes the polling frequency based on match states.
func (m Model) nextSyncInterval() time.Duration {
	if m.state != StateBrowsing && m.state != StateChannelSelect && m.state != StatePlaying {
		return 30 * time.Second
	}
	if len(m.matches) == 0 {
		return 30 * time.Second
	}

	hasLive := false
	hasUpcomingSoon := false
	now := time.Now()

	for _, match := range m.matches {
		if match.Status.State == "in" {
			hasLive = true
			break
		}
		diff := match.StartTime.Sub(now)
		if match.Status.State == "pre" && diff <= 15*time.Minute && diff >= -15*time.Minute {
			hasUpcomingSoon = true
		}
	}

	if hasLive {
		return 5 * time.Second
	}
	if hasUpcomingSoon {
		return 15 * time.Second
	}
	return 30 * time.Second
}

func fetchMatchesCmd(client model.Client, leagueKey string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		matches, err := client.FetchMatches(context.Background(), leagueKey)
		return matchesLoadedMsg{
			leagueKey: leagueKey,
			matches:   matches,
			err:       err,
			isSilent:  false,
			gen:       gen,
		}
	}
}

func forceRefreshMatchesCmd(client model.Client, leagueKey string, isSilent bool, gen uint64) tea.Cmd {
	return func() tea.Msg {
		ctx := model.WithForceRefresh(context.Background())
		matches, err := client.FetchMatches(ctx, leagueKey)
		return matchesLoadedMsg{
			leagueKey: leagueKey,
			matches:   matches,
			err:       err,
			isSilent:  isSilent,
			gen:       gen,
		}
	}
}

func fetchMatchStatsCmd(client model.Client, leagueKey, eventID string) tea.Cmd {
	return func() tea.Msg {
		details, err := client.FetchMatchDetails(context.Background(), leagueKey, eventID)
		if err != nil {
			return matchStatsLoadedMsg{matchID: eventID, err: err}
		}
		if details != nil {
			return matchStatsLoadedMsg{
				matchID:    eventID,
				stats:      details.Stats,
				lineups:    details.Lineups,
				referee:    details.Referee,
				attendance: details.Attendance,
			}
		}
		return matchStatsLoadedMsg{matchID: eventID}
	}
}

func forceFetchMatchStatsCmd(client model.Client, leagueKey, eventID string) tea.Cmd {
	return func() tea.Msg {
		ctx := model.WithForceRefresh(context.Background())
		details, err := client.FetchMatchDetails(ctx, leagueKey, eventID)
		if err != nil {
			return matchStatsLoadedMsg{matchID: eventID, err: err}
		}
		if details != nil {
			return matchStatsLoadedMsg{
				matchID:    eventID,
				stats:      details.Stats,
				lineups:    details.Lineups,
				referee:    details.Referee,
				attendance: details.Attendance,
			}
		}
		return matchStatsLoadedMsg{matchID: eventID}
	}
}

func fetchStandingsCmd(client model.Client, leagueKey string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		st, err := client.FetchStandings(context.Background(), leagueKey)
		return standingsLoadedMsg{
			leagueKey: leagueKey,
			standings: st,
			err:       err,
			gen:       gen,
		}
	}
}

func fetchStatisticsCmd(client model.Client, leagueKey string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		stat, err := client.FetchStatistics(context.Background(), leagueKey)
		return statisticsLoadedMsg{
			leagueKey:  leagueKey,
			statistics: stat,
			err:        err,
			gen:        gen,
		}
	}
}

func waitForPlaybackCmd(player model.Controller, playbackID int64) tea.Cmd {
	return func() tea.Msg {
		if player != nil {
			_ = player.Wait()
		}
		return playbackFinishedMsg{playbackID: playbackID}
	}
}

func playStreamCmd(player model.Controller, req model.PlayRequest, playbackID int64) tea.Cmd {
	return func() tea.Msg {
		err := player.PlayStream(context.Background(), req)
		if err != nil {
			return playbackErrMsg{err: err}
		}
		return playbackStartedMsg{playbackID: playbackID, channel: req.Channel}
	}
}

// Update handles incoming events and state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case startupTickMsg:
		if m.state == StateStartup {
			m.startupProgress += 10
			if m.startupProgress >= 100 {
				m.state = StateMainMenu
				return m, nil
			}
			return m, startupTickCmd()
		}
		return m, nil

	case connectingPulseTickMsg:
		if m.connecting && m.state == StatePlaying {
			m.pulseStep = (m.pulseStep + 1) % 4
			return m, connectingPulseTickCmd()
		}
		return m, nil

	case pulseTickMsg:
		m.livePulse = !m.livePulse
		m.pulseStep = (m.pulseStep + 1) % 4
		return m, pulseTickCmd()

	case standingsLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		curr := m.CurrentLeague()
		if msg.leagueKey == curr.Key {
			m.standings = msg.standings
			m.standingsErr = msg.err
			if m.leagueTab == TabPosiciones && m.state == StateLoading {
				m.state = StateBrowsing
			}
		}
		return m, nil

	case statisticsLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		curr := m.CurrentLeague()
		if msg.leagueKey == curr.Key {
			m.statistics = msg.statistics
			m.statisticsErr = msg.err
			if m.leagueTab == TabGoleadores && m.state == StateLoading {
				m.state = StateBrowsing
			}
		}
		return m, nil

	case autoSyncTickMsg:
		return m, tea.Batch(m.autoSyncCommands()...)

	case matchesLoadedMsg:
		if msg.gen != m.loadGen {
			return m, nil
		}
		curr := m.CurrentLeague()
		if msg.leagueKey == curr.Key {
			if msg.err != nil {
				if msg.isSilent {
					return m, nil
				}
				m.state = StateError
				m.errorMsg = msg.err.Error()
				m.statusMsg = ""
			} else {
				var prevSelectedID string
				if (m.state == StateChannelSelect || m.state == StatePlaying) && m.selectedMatchID != "" {
					prevSelectedID = m.selectedMatchID
				} else if m.leagueTab == TabResumen {
					pms := m.ResumenMatches()
					if len(pms) > 0 && m.resumenCursor >= 0 && m.resumenCursor < len(pms) {
						prevSelectedID = pms[m.resumenCursor].ID
					}
				} else if len(m.matches) > 0 && m.matchCursor >= 0 && m.matchCursor < len(m.matches) {
					prevSelectedID = m.matches[m.matchCursor].ID
				}

				if !msg.isSilent {
					m.state = StateBrowsing
				}

				sort.Slice(msg.matches, func(i, j int) bool {
					return msg.matches[i].StartTime.Before(msg.matches[j].StartTime)
				})

				statsMap := make(map[string]*model.MatchStats)
				lineupsMap := make(map[string]*model.MatchLineups)
				for _, prev := range m.matches {
					if prev.Stats != nil {
						statsMap[prev.ID] = prev.Stats
					}
					if prev.Lineups != nil {
						lineupsMap[prev.ID] = prev.Lineups
					}
				}
				for _, prev := range m.pastMatches {
					if prev.Stats != nil {
						statsMap[prev.ID] = prev.Stats
					}
					if prev.Lineups != nil {
						lineupsMap[prev.ID] = prev.Lineups
					}
				}
				for i := range msg.matches {
					if s, ok := statsMap[msg.matches[i].ID]; ok {
						msg.matches[i].Stats = s
					}
					if l, ok := lineupsMap[msg.matches[i].ID]; ok {
						msg.matches[i].Lineups = l
					}
				}

				// Fixtures stay visible so users can track fixtures, schedules, and scores.
				// Dynamic extractors and live channels resolve when playback is requested.
				resolver, canFilter := m.catalog.(playabilityResolver)
				m.noStreamsConfigured = canFilter && !resolver.HasConfiguredStreams()

				var activeMatches []model.Match
				var pastMatches []model.Match

				for _, match := range msg.matches {
					isCompleted := match.Status.Completed || strings.ToLower(match.Status.State) == "post"
					if isCompleted {
						// Completed fixtures are history: they stay in Resumen
						// regardless, and GetChannels never emits a live source for them.
						pastMatches = append(pastMatches, match)
						continue
					}
					activeMatches = append(activeMatches, match)
				}

				m.matches = activeMatches
				m.pastMatches = pastMatches

				if msg.isSilent && prevSelectedID != "" {
					foundIdx := -1
					for i, mt := range m.matches {
						if mt.ID == prevSelectedID {
							foundIdx = i
							break
						}
					}
					if foundIdx != -1 {
						m.matchCursor = foundIdx
					} else if m.matchCursor >= len(m.matches) && len(m.matches) > 0 {
						m.matchCursor = len(m.matches) - 1
					}

					pms := m.ResumenMatches()
					foundResumenIdx := -1
					for i, mt := range pms {
						if mt.ID == prevSelectedID {
							foundResumenIdx = i
							break
						}
					}
					if foundResumenIdx != -1 {
						m.resumenCursor = foundResumenIdx
					} else if m.resumenCursor >= len(pms) && len(pms) > 0 {
						m.resumenCursor = len(pms) - 1
					}
				} else if !msg.isSilent {
					m.matchCursor = 0
					m.resumenCursor = 0
				}
				m.errorMsg = ""
				notifyCmd := m.detectGoalNotifications()
				if isTopLeague(msg.leagueKey) && len(activeMatches) > 0 {
					return m, tea.Batch(prewarmMatchesCmd(m.catalog, activeMatches), notifyCmd)
				}
				return m, notifyCmd
			}
		}
		return m, nil

	case matchStatsLoadedMsg:
		if msg.err != nil {
			// Keep whatever is already on screen and leave the details pending so
			// the user (or the live auto-sync) can retry.
			return m, nil
		}
		if m.detailsLoaded == nil {
			m.detailsLoaded = make(map[string]bool)
		}
		if msg.matchID != "" {
			m.detailsLoaded[msg.matchID] = true
		}
		for i := range m.matches {
			if m.matches[i].ID == msg.matchID {
				if msg.stats != nil {
					m.matches[i].Stats = msg.stats
				}
				if msg.lineups != nil {
					m.matches[i].Lineups = msg.lineups
				}
				if msg.referee != "" {
					m.matches[i].Referee = msg.referee
				}
				if msg.attendance > 0 {
					m.matches[i].Attendance = msg.attendance
				}
				break
			}
		}
		for i := range m.pastMatches {
			if m.pastMatches[i].ID == msg.matchID {
				if msg.stats != nil {
					m.pastMatches[i].Stats = msg.stats
				}
				if msg.lineups != nil {
					m.pastMatches[i].Lineups = msg.lineups
				}
				if msg.referee != "" {
					m.pastMatches[i].Referee = msg.referee
				}
				if msg.attendance > 0 {
					m.pastMatches[i].Attendance = msg.attendance
				}
				break
			}
		}
		return m, nil

	case channelsResolvedMsg:
		if (m.state != StateChannelSelect && m.state != StatePlaying) || msg.matchID == "" || msg.matchID != m.selectedMatchID || msg.gen != m.loadGen {
			return m, nil
		}
		// If we already resolved channels from catalog, turn off the loading indicator.
		// If catalog resolved 0 channels and a web resolver is running in parallel, keep
		// channelsLoading true so the user sees the spinner instead of a false "No signals" banner.
		_, hasWeb := m.catalog.(webStreamsResolver)
		if len(msg.channels) > 0 || !hasWeb {
			m.channelsLoading = false
		}
		seen := make(map[string]bool, len(msg.channels)+len(m.channels))
		merged := make([]model.Channel, 0, len(msg.channels)+len(m.channels))
		addChannels := func(chans []model.Channel) {
			for _, ch := range chans {
				key := ch.URL
				if key == "" {
					key = ch.Name
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				merged = append(merged, ch)
			}
		}
		// Catalog channels keep priority over web enrichment regardless of which
		// async result arrives first.
		addChannels(msg.channels)
		addChannels(m.channels)
		m.channels = merged
		for i := range m.channels {
			m.channels[i].Recommended = (i == 0)
		}
		return m, nil

	case webChannelsLoadedMsg:
		if (m.state == StateChannelSelect || m.state == StatePlaying) && msg.matchID != "" && msg.matchID == m.selectedMatchID {
			m.channelsLoading = false
			if len(msg.channels) > 0 {
				seen := make(map[string]bool)
				for _, ch := range m.channels {
					seen[ch.URL] = true
				}
				for _, ch := range msg.channels {
					if !seen[ch.URL] {
						seen[ch.URL] = true
						m.channels = append(m.channels, ch)
					}
				}
				for i := range m.channels {
					m.channels[i].Recommended = (i == 0)
				}
			}
		}
		return m, nil

	case playbackStartedMsg:
		if msg.playbackID == m.playbackID {
			m.connecting = false
			m.statusMsg = "✓ Reproduciendo: " + msg.channel.Name + " • x: parar señal"
			m.errorMsg = ""
			return m, waitForPlaybackCmd(m.player, msg.playbackID)
		}
		return m, nil

	case playbackFinishedMsg:
		if msg.playbackID == m.playbackID {
			m.connecting = false
			m.statusMsg = ""
		}
		return m, nil

	case playbackErrMsg:
		m.connecting = false
		m.errorMsg = "Error de reproducción: " + msg.err.Error()
		m.statusMsg = ""
		return m, nil

	case updateAvailableMsg:
		m.updateAvailable = msg.tagName
		return m, nil

	case updateFinishedMsg:
		if msg.err != nil {
			m.errorMsg = "Error al actualizar: " + msg.err.Error()
		} else {
			m.statusMsg = "✓ kickoff actualizado a " + m.updateAvailable + ". ¡Reinicia la app!"
			m.updateAvailable = ""
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			if m.player != nil {
				_ = m.player.Stop()
			}
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		}

		switch m.state {
		case StateStartup:
			// Any key dismisses startup animation directly to MainMenu
			m.state = StateMainMenu
			return m, nil

		case StateMainMenu:
			switch key {
			case "x":
				if m.player != nil {
					_ = m.player.Stop()
				}
				m.playbackID++
				m.connecting = false
				m.statusMsg = "⏹ Señal detenida"
				return m, nil
			case "u":
				if m.updateAvailable != "" {
					exe, err := os.Executable()
					if err == nil {
						c := exec.Command(exe, "update")
						c.Stdin = os.Stdin
						c.Stdout = os.Stdout
						c.Stderr = os.Stderr
						return m, tea.ExecProcess(c, func(err error) tea.Msg {
							return updateFinishedMsg{err: err}
						})
					}
				}
			case "esc":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					if !m.categoryMode {
						m.categoryMode = true
						return m, nil
					} else if m.expandedCats[currCat] {
						m.expandedCats[currCat] = false
						return m, nil
					}
				}
			case "left", "h":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					if !m.categoryMode {
						m.categoryMode = true
					} else if m.expandedCats[currCat] {
						m.expandedCats[currCat] = false
					}
				}
			case "right", "l":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					if !m.expandedCats[currCat] {
						m.expandedCats[currCat] = true
					}
					m.categoryMode = false
					if indices := m.categoryIndices[currCat]; len(indices) > 0 {
						m.leagueCursor = indices[0]
					}
				}
			case "up", "k":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					indices := m.categoryIndices[currCat]
					if !m.categoryMode && m.expandedCats[currCat] && len(indices) > 0 {
						// Inside expanded category: check current league position
						pos := -1
						for idx, lIdx := range indices {
							if lIdx == m.leagueCursor {
								pos = idx
								break
							}
						}
						if pos > 0 {
							m.leagueCursor = indices[pos-1]
						} else {
							// Return to category header
							m.categoryMode = true
						}
					} else {
						// Moving between categories
						m.categoryMode = true
						m.categoryCursor = (m.categoryCursor - 1 + len(m.categories)) % len(m.categories)
						prevCat := m.categories[m.categoryCursor]
						if m.expandedCats[prevCat] && len(m.categoryIndices[prevCat]) > 0 {
							m.leagueCursor = m.categoryIndices[prevCat][0]
						}
					}
				}
			case "down", "j":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					indices := m.categoryIndices[currCat]
					if m.categoryMode {
						if m.expandedCats[currCat] && len(indices) > 0 {
							// Enter expanded category first item
							m.categoryMode = false
							m.leagueCursor = indices[0]
						} else {
							// Move to next category
							m.categoryCursor = (m.categoryCursor + 1) % len(m.categories)
							nextCat := m.categories[m.categoryCursor]
							if m.expandedCats[nextCat] && len(m.categoryIndices[nextCat]) > 0 {
								m.leagueCursor = m.categoryIndices[nextCat][0]
							}
						}
					} else {
						// Already inside expanded category leagues
						pos := -1
						for idx, lIdx := range indices {
							if lIdx == m.leagueCursor {
								pos = idx
								break
							}
						}
						if pos >= 0 && pos < len(indices)-1 {
							m.leagueCursor = indices[pos+1]
						} else {
							// At last league in category, move to next category header
							m.categoryMode = true
							m.categoryCursor = (m.categoryCursor + 1) % len(m.categories)
							nextCat := m.categories[m.categoryCursor]
							if m.expandedCats[nextCat] && len(m.categoryIndices[nextCat]) > 0 {
								m.leagueCursor = m.categoryIndices[nextCat][0]
							}
						}
					}
				}
			case "space":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					m.expandedCats[currCat] = !m.expandedCats[currCat]
					if m.expandedCats[currCat] {
						m.categoryMode = false
						if len(m.categoryIndices[currCat]) > 0 {
							m.leagueCursor = m.categoryIndices[currCat][0]
						}
					} else {
						m.categoryMode = true
					}
				}
			case "enter":
				if len(m.categories) > 0 {
					currCat := m.categories[m.categoryCursor]
					indices := m.categoryIndices[currCat]
					if m.categoryMode {
						if !m.expandedCats[currCat] {
							m.expandedCats[currCat] = true
							if len(indices) > 0 {
								m.leagueCursor = indices[0]
							}
							return m, nil
						}
						m.categoryMode = false
						if len(indices) > 0 {
							m.leagueCursor = indices[0]
						}
						return m, nil
					}
				}

				if len(m.leagues) > 0 {
					m.activeTab = m.leagueCursor
					m.leagueTab = TabResumen
					m.state = StateLoading
					m.matches = nil
					m.pastMatches = nil
					m.matchCursor = 0
					m.resumenCursor = 0
					m.selectedMatchID = ""
					m.detailsLoaded = make(map[string]bool)
					m.standings = nil
					m.standingsErr = nil
					m.selectedGroup = 0
					m.statistics = nil
					m.statisticsErr = nil
					m.channels = nil
					m.channelsLoading = false
					m.channelCursor = 0
					m.modalTab = 0
					m.errorMsg = ""
					m.loadGen++
					gen := m.loadGen
					league := m.CurrentLeague()
					m.loadingMsg = "Consultando partidos de hoy (" + league.Name + ")..."
					return m, tea.Batch(
						m.spinner.Tick,
						fetchMatchesCmd(m.client, league.Key, gen),
						fetchStandingsCmd(m.client, league.Key, gen),
						fetchStatisticsCmd(m.client, league.Key, gen),
					)
				}
			}

		case StateBrowsing, StatePlaying:
			switch key {
			case "1":
				m.leagueTab = TabResumen
				return m, nil
			case "2":
				m.leagueTab = TabPartidos
				return m, nil
			case "3":
				m.leagueTab = TabPosiciones
				if m.standings == nil && m.standingsErr == nil {
					return m, fetchStandingsCmd(m.client, m.CurrentLeague().Key, m.loadGen)
				}
				return m, nil
			case "4":
				m.leagueTab = TabGoleadores
				if m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, m.CurrentLeague().Key, m.loadGen)
				}
				return m, nil
			case "right", "l", "tab":
				m.leagueTab = (m.leagueTab + 1) % 4
				league := m.CurrentLeague()
				if m.leagueTab == TabPosiciones && m.standings == nil && m.standingsErr == nil {
					return m, fetchStandingsCmd(m.client, league.Key, m.loadGen)
				}
				if m.leagueTab == TabGoleadores && m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, league.Key, m.loadGen)
				}
				return m, nil
			case "left", "h", "shift+tab":
				m.leagueTab = (m.leagueTab - 1 + 4) % 4
				league := m.CurrentLeague()
				if m.leagueTab == TabPosiciones && m.standings == nil && m.standingsErr == nil {
					return m, fetchStandingsCmd(m.client, league.Key, m.loadGen)
				}
				if m.leagueTab == TabGoleadores && m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, league.Key, m.loadGen)
				}
				return m, nil
			case "[", "p":
				if m.leagueTab == TabPosiciones && m.standings != nil && len(m.standings.Groups) > 0 {
					numGrps := len(m.standings.Groups)
					m.selectedGroup = (m.selectedGroup - 1 + numGrps) % numGrps
					return m, nil
				}
			case "]", "n":
				if m.leagueTab == TabPosiciones && m.standings != nil && len(m.standings.Groups) > 0 {
					numGrps := len(m.standings.Groups)
					m.selectedGroup = (m.selectedGroup + 1) % numGrps
					return m, nil
				}
			case "up", "k":
				if m.leagueTab == TabResumen {
					if m.resumenCursor > 0 {
						m.resumenCursor--
					}
				} else if m.leagueTab == TabPartidos {
					if m.matchCursor > 0 {
						m.matchCursor--
					}
				}
				return m, nil
			case "down", "j":
				if m.leagueTab == TabResumen {
					pms := m.ResumenMatches()
					if m.resumenCursor < len(pms)-1 {
						m.resumenCursor++
					}
				} else if m.leagueTab == TabPartidos {
					if m.matchCursor < len(m.matches)-1 {
						m.matchCursor++
					}
				}
				return m, nil
			case "enter":
				if m.leagueTab == TabResumen {
					pms := m.ResumenMatches()
					if len(pms) > 0 && m.resumenCursor < len(pms) {
						selectedMatch := pms[m.resumenCursor]
						m.selectedMatchID = selectedMatch.ID
						var preChs []model.Channel
						if peeker, ok := m.catalog.(peekCachedResolver); ok {
							preChs = peeker.PeekCachedChannels(selectedMatch)
						}
						if len(preChs) > 0 {
							m.channels = preChs
							m.channelsLoading = false
						} else {
							m.channels = nil
							m.channelsLoading = true
						}
						m.channelCursor = 0
						m.modalTab = 0
						m.state = StateChannelSelect
						cmds := []tea.Cmd{resolveChannelsCmd(m.catalog, selectedMatch, m.loadGen)}
						if (selectedMatch.Stats == nil || selectedMatch.Lineups == nil) && selectedMatch.ID != "" && !m.detailsLoaded[selectedMatch.ID] {
							cmds = append(cmds, fetchMatchStatsCmd(m.client, m.CurrentLeague().Key, selectedMatch.ID))
						}
						if _, ok := m.catalog.(webStreamsResolver); ok && len(preChs) == 0 {
							cmds = append(cmds, fetchWebChannelsCmd(m.catalog, selectedMatch))
						}
						if len(cmds) == 0 {
							return m, nil
						}
						if len(cmds) == 1 {
							return m, cmds[0]
						}
						return m, tea.Batch(cmds...)
					}
				} else if m.leagueTab == TabPartidos {
					if len(m.matches) > 0 && m.matchCursor < len(m.matches) {
						selectedMatch := m.matches[m.matchCursor]
						m.selectedMatchID = selectedMatch.ID
						var preChs []model.Channel
						if peeker, ok := m.catalog.(peekCachedResolver); ok {
							preChs = peeker.PeekCachedChannels(selectedMatch)
						}
						if len(preChs) > 0 {
							m.channels = preChs
							m.channelsLoading = false
						} else {
							m.channels = nil
							m.channelsLoading = true
						}
						m.channelCursor = 0
						m.modalTab = 0
						m.state = StateChannelSelect
						cmds := []tea.Cmd{resolveChannelsCmd(m.catalog, selectedMatch, m.loadGen)}
						if (selectedMatch.Stats == nil || selectedMatch.Lineups == nil) && selectedMatch.ID != "" && !m.detailsLoaded[selectedMatch.ID] {
							cmds = append(cmds, fetchMatchStatsCmd(m.client, m.CurrentLeague().Key, selectedMatch.ID))
						}
						if _, ok := m.catalog.(webStreamsResolver); ok && len(preChs) == 0 {
							cmds = append(cmds, fetchWebChannelsCmd(m.catalog, selectedMatch))
						}
						if len(cmds) == 0 {
							return m, nil
						}
						if len(cmds) == 1 {
							return m, cmds[0]
						}
						return m, tea.Batch(cmds...)
					}
				}
			case "x":
				if m.player != nil {
					_ = m.player.Stop()
				}
				m.playbackID++
				m.connecting = false
				m.statusMsg = "⏹ Señal detenida"
				return m, nil
			case "esc", "backspace":
				if m.connecting {
					if m.player != nil {
						_ = m.player.Stop()
					}
					m.playbackID++
					m.connecting = false
				}
				m.selectedMatchID = ""
				m.state = StateMainMenu
				m.loadGen++
				m.matches = nil
				m.pastMatches = nil
				m.matchCursor = 0
				m.resumenCursor = 0
				m.standings = nil
				m.statistics = nil
				m.channels = nil
				m.channelsLoading = false
				m.channelCursor = 0
				m.modalTab = 0
				if !strings.Contains(m.statusMsg, "Reproduciendo") {
					m.statusMsg = ""
				}
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				m.selectedMatchID = ""
				m.detailsLoaded = make(map[string]bool)
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando datos (" + league.Name + ")..."
				m.errorMsg = ""
				m.loadGen++
				gen := m.loadGen
				return m, tea.Batch(
					m.spinner.Tick,
					forceRefreshMatchesCmd(m.client, league.Key, false, gen),
					fetchStandingsCmd(m.client, league.Key, gen),
					fetchStatisticsCmd(m.client, league.Key, gen),
				)
			}

		case StateChannelSelect:
			switch key {
			case "left", "h":
				m.modalTab = (m.modalTab - 1 + 3) % 3
			case "right", "l":
				m.modalTab = (m.modalTab + 1) % 3
			case "tab":
				m.modalTab = (m.modalTab + 1) % 3
			case "shift+tab":
				m.modalTab = (m.modalTab - 1 + 3) % 3
			case "1":
				m.modalTab = 0
			case "2":
				m.modalTab = 1
			case "3":
				m.modalTab = 2
			case "up", "k":
				if m.modalTab == 0 && m.channelCursor > 0 {
					m.channelCursor--
				}
			case "down", "j":
				if m.modalTab == 0 && m.channelCursor < len(m.channels)-1 {
					m.channelCursor++
				}
			case "enter":
				if m.modalTab == 0 && len(m.channels) > 0 && m.channelCursor < len(m.channels) {
					selectedMatch, ok := m.CurrentSelectedMatch()
					if !ok {
						return m, nil
					}
					selectedChannel := m.channels[m.channelCursor]
					m.state = StatePlaying
					m.connecting = true
					m.pulseStep = 0
					m.streamFeed = selectedChannel.Name
					bitrateStr := selectedChannel.BitrateMode
					if bitrateStr == "" {
						bitrateStr = "auto"
					}
					if selectedChannel.Resolution != "" {
						bitrateStr += " • " + selectedChannel.Resolution
					}
					m.bitrate = bitrateStr
					m.statusMsg = "Conectando con " + selectedChannel.Name + "..."
					m.errorMsg = ""
					m.playbackID++
					req := model.PlayRequest{
						Channel:     selectedChannel,
						Match:       selectedMatch,
						BitrateMode: selectedChannel.BitrateMode,
					}
					return m, tea.Batch(
						m.spinner.Tick,
						connectingPulseTickCmd(),
						playStreamCmd(m.player, req, m.playbackID),
					)
				}
			case "r":
				selectedMatch, ok := m.CurrentSelectedMatch()
				if !ok || selectedMatch.ID == "" {
					return m, nil
				}
				return m, tea.Batch(
					m.spinner.Tick,
					forceFetchMatchStatsCmd(m.client, m.CurrentLeague().Key, selectedMatch.ID),
				)
			case "x":
				if m.player != nil {
					_ = m.player.Stop()
				}
				m.playbackID++
				m.connecting = false
				m.statusMsg = "⏹ Señal detenida"
				return m, nil
			case "esc", "backspace":
				m.state = StateBrowsing
				m.selectedMatchID = ""
				m.channels = nil
				m.channelsLoading = false
				m.channelCursor = 0
				m.modalTab = 0
				return m, nil
			}

		case StateLoading, StateError:
			switch key {
			case "x":
				if m.player != nil {
					_ = m.player.Stop()
				}
				m.playbackID++
				m.connecting = false
				m.statusMsg = "⏹ Señal detenida"
				return m, nil
			case "esc", "backspace", "h":
				if m.connecting && m.player != nil {
					_ = m.player.Stop()
					m.playbackID++
					m.connecting = false
				}
				m.selectedMatchID = ""
				m.state = StateMainMenu
				m.loadGen++
				m.matches = nil
				m.pastMatches = nil
				m.matchCursor = 0
				m.resumenCursor = 0
				m.standings = nil
				m.statistics = nil
				m.channels = nil
				m.channelsLoading = false
				m.channelCursor = 0
				m.modalTab = 0
				if !strings.Contains(m.statusMsg, "Reproduciendo") {
					m.statusMsg = ""
				}
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando datos (" + league.Name + ")..."
				m.errorMsg = ""
				m.loadGen++
				gen := m.loadGen
				return m, tea.Batch(
					m.spinner.Tick,
					forceRefreshMatchesCmd(m.client, league.Key, false, gen),
					fetchStandingsCmd(m.client, league.Key, gen),
					fetchStatisticsCmd(m.client, league.Key, gen),
				)
			}
		}
	}

	return m, nil
}
