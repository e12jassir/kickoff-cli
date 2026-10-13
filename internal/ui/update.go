package ui

import (
	"context"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/e12jassir/kickoff/internal/model"
)

// Async messages for Bubble Tea event loop.
type matchesLoadedMsg struct {
	leagueKey string
	matches   []model.Match
	err       error
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

func pulseTickCmd() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(t time.Time) tea.Msg {
		return pulseTickMsg(t)
	})
}

func fetchMatchesCmd(client model.Client, leagueKey string) tea.Cmd {
	return func() tea.Msg {
		matches, err := client.FetchMatches(context.Background(), leagueKey)
		return matchesLoadedMsg{
			leagueKey: leagueKey,
			matches:   matches,
			err:       err,
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

	case pulseTickMsg:
		m.livePulse = !m.livePulse
		return m, pulseTickCmd()

	case matchesLoadedMsg:
		curr := m.CurrentLeague()
		if msg.leagueKey == curr.Key {
			if msg.err != nil {
				m.state = StateError
				m.errorMsg = msg.err.Error()
				m.statusMsg = ""
			} else {
				m.state = StateBrowsing
				// Sort matches chronologically by StartTime
				sort.Slice(msg.matches, func(i, j int) bool {
					return msg.matches[i].StartTime.Before(msg.matches[j].StartTime)
				})
				m.matches = msg.matches
				m.matchCursor = 0
				m.errorMsg = ""
			}
		}
		return m, nil

	case playbackStartedMsg:
		if msg.playbackID == m.playbackID {
			m.statusMsg = "✓ Reproduciendo: " + msg.channel.Name
			m.errorMsg = ""
			return m, waitForPlaybackCmd(m.player, msg.playbackID)
		}
		return m, nil

	case playbackFinishedMsg:
		if msg.playbackID == m.playbackID {
			m.statusMsg = ""
		}
		return m, nil

	case playbackErrMsg:
		m.errorMsg = "Error de reproducción: " + msg.err.Error()
		m.statusMsg = ""
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
		case StateMainMenu:
			switch key {
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
					m.state = StateLoading
					m.matches = nil
					m.matchCursor = 0
					m.channels = nil
					m.channelCursor = 0
					m.errorMsg = ""
					league := m.CurrentLeague()
					m.loadingMsg = "Consultando partidos de hoy (" + league.Name + ")..."
					return m, tea.Batch(m.spinner.Tick, fetchMatchesCmd(m.client, league.Key))
				}
			}

		case StateBrowsing, StatePlaying:
			switch key {
			case "up", "k":
				if m.matchCursor > 0 {
					m.matchCursor--
				}
			case "down", "j":
				if m.matchCursor < len(m.matches)-1 {
					m.matchCursor++
				}
			case "enter":
				if len(m.matches) > 0 && m.matchCursor < len(m.matches) {
					selectedMatch := m.matches[m.matchCursor]
					m.channels = m.catalog.GetChannels(selectedMatch)
					m.channelCursor = 0
					m.state = StateChannelSelect
				}
			case "esc", "backspace", "h":
				m.state = StateMainMenu
				m.matches = nil
				m.matchCursor = 0
				m.channels = nil
				m.channelCursor = 0
				m.statusMsg = ""
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando partidos (" + league.Name + ")..."
				m.errorMsg = ""
				return m, tea.Batch(m.spinner.Tick, fetchMatchesCmd(m.client, league.Key))
			}

		case StateChannelSelect:
			switch key {
			case "up", "k":
				if m.channelCursor > 0 {
					m.channelCursor--
				}
			case "down", "j":
				if m.channelCursor < len(m.channels)-1 {
					m.channelCursor++
				}
			case "enter":
				if len(m.channels) > 0 && m.channelCursor < len(m.channels) {
					selectedChannel := m.channels[m.channelCursor]
					selectedMatch := m.matches[m.matchCursor]
					m.state = StatePlaying
					m.statusMsg = "Conectando con " + selectedChannel.Name + "..."
					m.errorMsg = ""
					m.playbackID++
					req := model.PlayRequest{
						Channel:     selectedChannel,
						Match:       selectedMatch,
						BitrateMode: selectedChannel.BitrateMode,
					}
					return m, tea.Batch(m.spinner.Tick, playStreamCmd(m.player, req, m.playbackID))
				}
			case "esc", "backspace":
				m.state = StateBrowsing
				m.channels = nil
				m.channelCursor = 0
				return m, nil
			}

		case StateLoading, StateError:
			switch key {
			case "esc", "backspace", "h":
				m.state = StateMainMenu
				m.matches = nil
				m.matchCursor = 0
				m.channels = nil
				m.channelCursor = 0
				m.statusMsg = ""
				m.errorMsg = ""
				return m, nil
			case "r":
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando partidos (" + league.Name + ")..."
				m.errorMsg = ""
				return m, tea.Batch(m.spinner.Tick, fetchMatchesCmd(m.client, league.Key))
			}
		}
	}

	return m, nil
}
