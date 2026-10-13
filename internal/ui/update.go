package ui

import (
	"context"

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
	channel model.Channel
}

type playbackErrMsg struct {
	err error
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

func playStreamCmd(player model.Controller, req model.PlayRequest) tea.Cmd {
	return func() tea.Msg {
		err := player.PlayStream(context.Background(), req)
		if err != nil {
			return playbackErrMsg{err: err}
		}
		return playbackStartedMsg{channel: req.Channel}
	}
}

// Update handles incoming events and state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case matchesLoadedMsg:
		curr := m.CurrentLeague()
		if msg.leagueKey == curr.Key {
			if msg.err != nil {
				m.state = StateError
				m.errorMsg = msg.err.Error()
				m.statusMsg = ""
			} else {
				m.state = StateBrowsing
				m.matches = msg.matches
				m.matchCursor = 0
				m.errorMsg = ""
			}
		}
		return m, nil

	case playbackStartedMsg:
		m.statusMsg = "✓ Reproduciendo: " + msg.channel.Name
		m.errorMsg = ""
		return m, nil

	case playbackErrMsg:
		m.errorMsg = "Error de reproducción: " + msg.err.Error()
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.player != nil {
				_ = m.player.Stop()
			}
			return m, tea.Quit

		case "left", "h", "shift+tab":
			if m.state != StateChannelSelect {
				m.activeTab = (m.activeTab - 1 + len(m.leagues)) % len(m.leagues)
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

		case "right", "l", "tab":
			if m.state != StateChannelSelect {
				m.activeTab = (m.activeTab + 1) % len(m.leagues)
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

		case "up", "k":
			if m.state == StateChannelSelect {
				if m.channelCursor > 0 {
					m.channelCursor--
				}
			} else if m.state == StateBrowsing || m.state == StatePlaying {
				if m.matchCursor > 0 {
					m.matchCursor--
				}
			}

		case "down", "j":
			if m.state == StateChannelSelect {
				if m.channelCursor < len(m.channels)-1 {
					m.channelCursor++
				}
			} else if m.state == StateBrowsing || m.state == StatePlaying {
				if m.matchCursor < len(m.matches)-1 {
					m.matchCursor++
				}
			}

		case "enter":
			if m.state == StateBrowsing || m.state == StatePlaying {
				if len(m.matches) > 0 && m.matchCursor < len(m.matches) {
					selectedMatch := m.matches[m.matchCursor]
					m.channels = m.catalog.GetChannels(selectedMatch)
					m.channelCursor = 0
					if len(m.channels) > 0 {
						m.state = StateChannelSelect
					}
				}
			} else if m.state == StateChannelSelect {
				if len(m.channels) > 0 && m.channelCursor < len(m.channels) {
					selectedChannel := m.channels[m.channelCursor]
					selectedMatch := m.matches[m.matchCursor]
					m.state = StatePlaying
					m.statusMsg = "Conectando con " + selectedChannel.Name + "..."
					m.errorMsg = ""
					req := model.PlayRequest{
						Channel:     selectedChannel,
						Match:       selectedMatch,
						BitrateMode: selectedChannel.BitrateMode,
					}
					return m, tea.Batch(m.spinner.Tick, playStreamCmd(m.player, req))
				}
			}

		case "esc":
			if m.state == StateChannelSelect {
				m.state = StateBrowsing
				m.channels = nil
				m.channelCursor = 0
				return m, nil
			}

		case "r":
			if m.state != StateChannelSelect {
				m.state = StateLoading
				league := m.CurrentLeague()
				m.loadingMsg = "Recargando partidos (" + league.Name + ")..."
				m.errorMsg = ""
				return m, tea.Batch(m.spinner.Tick, fetchMatchesCmd(m.client, league.Key))
			}
		}
	}

	return m, tea.Batch(cmds...)
}
