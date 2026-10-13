package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff/internal/model"
)

// View renders the terminal user interface according to model state.
func (m Model) View() string {
	if m.state == StateMainMenu {
		return renderMainMenu(m)
	}
	return renderBrowsing(m)
}

func renderMainMenu(m Model) string {
	logo := AsciiLogoStyle.Render(AsciiLogo)
	subtitle := SubtitleStyle.Render("Terminal Live Football")

	var lines []string
	var lastCategory string
	for i, league := range m.leagues {
		cat := strings.ToUpper(league.Category)
		if cat != "" && cat != lastCategory {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, CategoryHeaderStyle.Render(cat))
			lastCategory = cat
		}

		prefix := "   "
		if i == m.leagueCursor {
			prefix = " › "
		}

		emoji := league.Emoji
		if emoji == "" {
			emoji = "⚽"
		}

		itemText := fmt.Sprintf("%s%s  %s", prefix, emoji, league.Name)
		if i == m.leagueCursor {
			lines = append(lines, MenuItemSelected.Render(itemText))
		} else {
			lines = append(lines, MenuItemNormal.Render(itemText))
		}
	}

	menuBox := MenuBoxStyle.Render(strings.Join(lines, "\n"))
	footer := FooterHelpStyle.Render("↑/k  ↓/j: navegar  •  enter: entrar  •  q: salir")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		logo,
		subtitle,
		"",
		menuBox,
		"",
		footer,
	)

	w := m.width
	h := m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}

func renderBrowsing(m Model) string {
	curr := m.CurrentLeague()
	headerText := fmt.Sprintf("⚽ KICKOFF · %s %s", curr.Emoji, curr.Name)
	if curr.Emoji == "" {
		headerText = fmt.Sprintf("⚽ KICKOFF · %s", curr.Name)
	}
	header := AppTitleStyle.Render(headerText)

	var body string
	footer := FooterHelpStyle.Render("↑/k  ↓/j: navegar  •  enter: ver señales  •  esc: volver  •  r: recargar  •  q: salir")

	switch m.state {
	case StateLoading:
		body = fmt.Sprintf("  %s %s", m.spinner.View(), m.loadingMsg)
		footer = FooterHelpStyle.Render("esc: volver  •  q: salir")

	case StateError:
		body = StatusBarError.Render(fmt.Sprintf("  ✖ Error: %s", m.errorMsg))
		footer = FooterHelpStyle.Render("r: recargar  •  esc: volver  •  q: salir")

	case StateChannelSelect:
		matchTitle := ""
		if m.matchCursor < len(m.matches) {
			matchTitle = m.matches[m.matchCursor].Title
		}
		modalContent := renderChannelModal(matchTitle, m.channels, m.channelCursor)
		body = ModalBoxStyle.Render(modalContent)
		footer = FooterHelpStyle.Render("↑/k  ↓/j: navegar  •  enter: reproducir  •  esc: volver  •  q: salir")

	default: // StateBrowsing, StatePlaying
		if len(m.matches) == 0 {
			emptyMsg := fmt.Sprintf("⚽ No hay partidos programados hoy para %s.", curr.Name)
			body = MenuBoxStyle.Render(EmptyStateStyle.Render(emptyMsg))
		} else {
			var matchCards []string
			for i, match := range m.matches {
				cardContent := renderMatchCard(match)
				if i == m.matchCursor {
					matchCards = append(matchCards, MatchCardSelected.Render(cardContent))
				} else {
					matchCards = append(matchCards, MatchCardNormal.Render(cardContent))
				}
			}
			body = lipgloss.JoinVertical(lipgloss.Center, matchCards...)
		}
	}

	var elements []string
	elements = append(elements, header, "")
	if m.statusMsg != "" {
		elements = append(elements, StatusBarSuccess.Render(m.statusMsg), "")
	}
	if m.errorMsg != "" && m.state != StateError {
		elements = append(elements, StatusBarError.Render(m.errorMsg), "")
	}
	elements = append(elements, body, "", footer)
	content := lipgloss.JoinVertical(lipgloss.Center, elements...)

	w := m.width
	h := m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
}

func renderMatchCard(match model.Match) string {
	var card strings.Builder

	timeStr := match.TimeStr
	if timeStr == "" {
		timeStr = "--:--"
	}

	homeName := match.HomeTeam.DisplayName
	if homeName == "" {
		homeName = "Local"
	}
	awayName := match.AwayTeam.DisplayName
	if awayName == "" {
		awayName = "Visitante"
	}

	statusDesc := match.Status.Description
	if statusDesc == "" {
		statusDesc = "Programado"
	}

	var statusStyled string
	switch match.Status.State {
	case "in":
		statusStyled = StatusLiveStyle.Render("[" + statusDesc + "]")
	case "post":
		statusStyled = StatusPostStyle.Render("[" + statusDesc + "]")
	default:
		statusStyled = StatusPreStyle.Render("[" + statusDesc + "]")
	}

	scoreHome := match.HomeTeam.Score
	scoreAway := match.AwayTeam.Score

	vsSection := " vs "
	if scoreHome != "" && scoreAway != "" {
		vsSection = fmt.Sprintf(" %s - %s ", ScoreStyle.Render(scoreHome), ScoreStyle.Render(scoreAway))
	}

	line1 := fmt.Sprintf("%s   %s%s%s   %s",
		MatchTimeStyle.Render(timeStr),
		TeamNameStyle.Render(homeName),
		vsSection,
		TeamNameStyle.Render(awayName),
		statusStyled,
	)

	card.WriteString(line1)
	if match.Venue != "" {
		card.WriteString("\n")
		card.WriteString(VenueStyle.Render("Estadio: " + match.Venue))
	}

	return card.String()
}

func renderChannelModal(matchTitle string, channels []model.Channel, selectedIdx int) string {
	var modal strings.Builder

	title := fmt.Sprintf("📡 Señales de transmisión: %s", matchTitle)
	modal.WriteString(ModalTitleStyle.Render(title))
	modal.WriteString("\n")
	modal.WriteString(VenueStyle.Render("Enter: reproducir  •  Esc: volver"))
	modal.WriteString("\n\n")

	for i, ch := range channels {
		var item string
		prefix := "   "
		if i == selectedIdx {
			prefix = " › "
		}

		item = fmt.Sprintf("%s%s", prefix, ch.DisplayName)
		if i == selectedIdx {
			modal.WriteString(ChannelItemSelected.Render(item))
		} else {
			modal.WriteString(ChannelItemNormal.Render(item))
		}
		modal.WriteString("\n")
	}

	return modal.String()
}
