package ui

import (
	"context"
	"os"
	"os/exec"
	"sort"
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
}

type matchStatsLoadedMsg struct {
	matchID    string
	stats      *model.MatchStats
	lineups    *model.MatchLineups
	referee    string
	attendance int
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
}

type statisticsLoadedMsg struct {
	leagueKey  string
	statistics *model.LeagueStatistics
	err        error
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

func fetchMatchesCmd(client model.Client, leagueKey string) tea.Cmd {
	return func() tea.Msg {
		matches, err := client.FetchMatches(context.Background(), leagueKey)
		return matchesLoadedMsg{
			leagueKey: leagueKey,
			matches:   matches,
			err:       err,
			isSilent:  false,
		}
	}
}

func forceRefreshMatchesCmd(client model.Client, leagueKey string, isSilent bool) tea.Cmd {
	return func() tea.Msg {
		ctx := model.WithForceRefresh(context.Background())
		matches, err := client.FetchMatches(ctx, leagueKey)
		return matchesLoadedMsg{
			leagueKey: leagueKey,
			matches:   matches,
			err:       err,
			isSilent:  isSilent,
		}
	}
}

func fetchMatchStatsCmd(client model.Client, leagueKey, eventID string) tea.Cmd {
	return func() tea.Msg {
		details, _ := client.FetchMatchDetails(context.Background(), leagueKey, eventID)
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
		details, _ := client.FetchMatchDetails(ctx, leagueKey, eventID)
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

func fetchStandingsCmd(client model.Client, leagueKey string) tea.Cmd {
	return func() tea.Msg {
		st, err := client.FetchStandings(context.Background(), leagueKey)
		return standingsLoadedMsg{
			leagueKey: leagueKey,
			standings: st,
			err:       err,
		}
	}
}

func fetchStatisticsCmd(client model.Client, leagueKey string) tea.Cmd {
	return func() tea.Msg {
		stat, err := client.FetchStatistics(context.Background(), leagueKey)
		return statisticsLoadedMsg{
			leagueKey:  leagueKey,
			statistics: stat,
			err:        err,
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
		cmds := []tea.Cmd{autoSyncTickCmd(m.nextSyncInterval())}
		if m.state == StateBrowsing || m.state == StateChannelSelect || m.state == StatePlaying {
			league := m.CurrentLeague()
			cmds = append(cmds, forceRefreshMatchesCmd(m.client, league.Key, true))

			if m.state == StateChannelSelect && len(m.matches) > 0 && m.matchCursor < len(m.matches) {
				selMatch := m.matches[m.matchCursor]
				if selMatch.ID != "" && selMatch.Status.State == "in" {
					cmds = append(cmds, forceFetchMatchStatsCmd(m.client, league.Key, selMatch.ID))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case matchesLoadedMsg:
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
				if len(m.matches) > 0 && m.matchCursor >= 0 && m.matchCursor < len(m.matches) {
					prevSelectedID = m.matches[m.matchCursor].ID
				}

				if !msg.isSilent {
					m.state = StateBrowsing
				}

				sort.Slice(msg.matches, func(i, j int) bool {
					return msg.matches[i].StartTime.Before(msg.matches[j].StartTime)
				})

				if len(m.matches) > 0 {
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
					for i := range msg.matches {
						if s, ok := statsMap[msg.matches[i].ID]; ok {
							msg.matches[i].Stats = s
						}
						if l, ok := lineupsMap[msg.matches[i].ID]; ok {
							msg.matches[i].Lineups = l
						}
					}
				}

				m.matches = msg.matches

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
				} else {
					m.matchCursor = 0
				}
				m.errorMsg = ""
			}
		}
		return m, nil

	case matchStatsLoadedMsg:
		for i := range m.matches {
			if m.matches[i].ID == msg.matchID {
				m.matches[i].Stats = msg.stats
				m.matches[i].Lineups = msg.lineups
				if msg.referee != "" {
					m.matches[i].Referee = msg.referee
				}
				if msg.attendance > 0 {
					m.matches[i].Attendance = msg.attendance
				}
				break
			}
		}
		return m, nil

	case playbackStartedMsg:
		if msg.playbackID == m.playbackID {
			m.connecting = false
			m.statusMsg = "✓ Reproduciendo: " + msg.channel.Name
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
		case "ctrl+c", "q":
			if m.player != nil {
				_ = m.player.Stop()
			}
			return m, tea.Quit
		}

		switch m.state {
		case StateStartup:
			// Any key dismisses startup animation directly to MainMenu
			m.state = StateMainMenu
			return m, nil

		case StateMainMenu:
			switch key {
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
			case "up", "k":
				if len(m.leagues) > 0 {
					m.leagueCursor = (m.leagueCursor - 1 + len(m.leagues)) % len(m.leagues)
				}
			case "down", "j":
				if len(m.leagues) > 0 {
					m.leagueCursor = (m.leagueCursor + 1) % len(m.leagues)
				}
			case "enter":
				if len(m.leagues) > 0 {
					m.activeTab = m.leagueCursor
					m.leagueTab = TabPartidos
					m.state = StateLoading
					m.matches = nil
					m.matchCursor = 0
					m.standings = nil
					m.statistics = nil
					m.channels = nil
					m.channelCursor = 0
					m.modalTab = 0
					m.errorMsg = ""
					league := m.CurrentLeague()
					m.loadingMsg = "Consultando partidos de hoy (" + league.Name + ")..."
					return m, tea.Batch(
						m.spinner.Tick,
						fetchMatchesCmd(m.client, league.Key),
						fetchStandingsCmd(m.client, league.Key),
						fetchStatisticsCmd(m.client, league.Key),
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
					return m, fetchStandingsCmd(m.client, m.CurrentLeague().Key)
				}
				return m, nil
			case "4":
				m.leagueTab = TabGoleadores
				if m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, m.CurrentLeague().Key)
				}
				return m, nil
			case "tab":
				m.leagueTab = (m.leagueTab + 1) % 4
				league := m.CurrentLeague()
				if m.leagueTab == TabPosiciones && m.standings == nil && m.standingsErr == nil {
					return m, fetchStandingsCmd(m.client, league.Key)
				}
				if m.leagueTab == TabGoleadores && m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, league.Key)
				}
				return m, nil
			case "shift+tab":
				m.leagueTab = (m.leagueTab - 1 + 4) % 4
				league := m.CurrentLeague()
				if m.leagueTab == TabPosiciones && m.standings == nil && m.standingsErr == nil {
					return m, fetchStandingsCmd(m.client, league.Key)
				}
				if m.leagueTab == TabGoleadores && m.statistics == nil && m.statisticsErr == nil {
					return m, fetchStatisticsCmd(m.client, league.Key)
				}
				return m, nil
			case "up", "k":
				if m.leagueTab == TabPartidos && m.matchCursor > 0 {
					m.matchCursor--
				}
			case "down", "j":
				if m.leagueTab == TabPartidos && m.matchCursor < len(m.matches)-1 {
					m.matchCursor++
				}
			case "enter":
				if m.leagueTab == TabPartidos && len(m.matches) > 0 && m.matchCursor < len(m.matches) {
					selectedMatch := m.matches[m.matchCursor]
					m.channels = m.catalog.GetChannels(selectedMatch)
					m.channelCursor = 0
					m.modalTab = 0
					m.state = StateChannelSelect
					if (selectedMatch.Stats == nil || selectedMatch.Lineups == nil) && selectedMatch.ID != "" {
						return m, fetchMatchStatsCmd(m.client, m.CurrentLeague().Key, selectedMatch.ID)
					}
				}
			case "esc", "backspace", "h":
				m.state = StateMainMenu
				m.matches = nil
				m.matchCursor = 0
				m.standings = nil
				m.statistics = nil
				m.channels = nil
				m.channelCursor = 0
				m.modalTab = 0
				m.statusMsg = ""
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando datos (" + league.Name + ")..."
				m.errorMsg = ""
				return m, tea.Batch(
					m.spinner.Tick,
					forceRefreshMatchesCmd(m.client, league.Key, false),
					fetchStandingsCmd(m.client, league.Key),
					fetchStatisticsCmd(m.client, league.Key),
				)
			}

		case StateChannelSelect:
			switch key {
			case "left", "h":
				m.modalTab = (m.modalTab - 1 + 3) % 3
			case "right", "l", "tab":
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
					selectedChannel := m.channels[m.channelCursor]
					selectedMatch := m.matches[m.matchCursor]
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
			case "esc", "backspace":
				m.state = StateBrowsing
				m.channels = nil
				m.channelCursor = 0
				m.modalTab = 0
				return m, nil
			}

		case StateLoading, StateError:
			switch key {
			case "esc", "backspace", "h":
				m.state = StateMainMenu
				m.matches = nil
				m.matchCursor = 0
				m.standings = nil
				m.statistics = nil
				m.channels = nil
				m.channelCursor = 0
				m.modalTab = 0
				m.statusMsg = ""
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando datos (" + league.Name + ")..."
				m.errorMsg = ""
				return m, tea.Batch(
					m.spinner.Tick,
					forceRefreshMatchesCmd(m.client, league.Key, false),
					fetchStandingsCmd(m.client, league.Key),
					fetchStatisticsCmd(m.client, league.Key),
				)
			}
		}
	}

	return m, nil
}
