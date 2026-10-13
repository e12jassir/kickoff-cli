package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff/internal/model"
)

// View renders the terminal user interface according to model state.
func (m Model) View() string {
	var b strings.Builder

	// 1. Header: App title
	b.WriteString(AppTitleStyle.Render("⚽ KICKOFF · Terminal Live Football"))
	b.WriteString("\n")

	// 2. Tabs Row
	var tabs []string
	for i, league := range m.leagues {
		if i == m.activeTab {
			tabs = append(tabs, ActiveTabStyle.Render(league.Name))
		} else {
			tabs = append(tabs, InactiveTabStyle.Render(league.Name))
		}
	}
	b.WriteString(TabsRowStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabs...)))
	b.WriteString("\n\n")

	// 3. Body Content
	switch m.state {
	case StateLoading:
		b.WriteString(fmt.Sprintf("  %s %s\n\n", m.spinner.View(), m.loadingMsg))

	case StateError:
		b.WriteString(StatusBarError.Render(fmt.Sprintf("  ✖ Error: %s\n\n", m.errorMsg)))
		b.WriteString(FooterHelpStyle.Render("  Presiona 'r' para reintentar o cambia de liga con ←/→\n\n"))

	default:
		if len(m.matches) == 0 {
			b.WriteString(EmptyStateStyle.Render(fmt.Sprintf("⚽ No hay partidos programados hoy para %s.", m.CurrentLeague().Name)))
			b.WriteString("\n\n")
		} else {
			// Render match list
			for i, match := range m.matches {
				cardContent := renderMatchCard(match)
				if i == m.matchCursor {
					b.WriteString(MatchCardSelected.Render(cardContent))
				} else {
					b.WriteString(MatchCardNormal.Render(cardContent))
				}
				b.WriteString("\n")
			}
		}

		// Channel Modal Popup
		if m.state == StateChannelSelect && len(m.channels) > 0 {
			matchTitle := ""
			if m.matchCursor < len(m.matches) {
				matchTitle = m.matches[m.matchCursor].Title
			}
			modalContent := renderChannelModal(matchTitle, m.channels, m.channelCursor)
			b.WriteString("\n")
			b.WriteString(ModalBoxStyle.Render(modalContent))
			b.WriteString("\n")
		}
	}

	// 4. Status Bar
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(StatusBarSuccess.Render("  " + m.statusMsg))
		b.WriteString("\n")
	}
	if m.errorMsg != "" && m.state != StateError {
		b.WriteString("\n")
		b.WriteString(StatusBarError.Render("  ✖ " + m.errorMsg))
		b.WriteString("\n")
	}

	// 5. Footer Help
	b.WriteString("\n")
	b.WriteString(renderFooter(m.state))

	return b.String()
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
	modal.WriteString(VenueStyle.Render("Selecciona canal con Enter · Esc para volver"))
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

func renderFooter(state UIState) string {
	if state == StateChannelSelect {
		return FooterHelpStyle.Render(
			FooterKeyStyle.Render("↑/↓") + " Seleccionar señal  " +
				FooterKeyStyle.Render("Enter") + " Reproducir en MPV  " +
				FooterKeyStyle.Render("Esc") + " Cerrar modal  " +
				FooterKeyStyle.Render("q") + " Salir",
		)
	}

	return FooterHelpStyle.Render(
		FooterKeyStyle.Render("←/→") + " Cambiar liga  " +
			FooterKeyStyle.Render("↑/↓") + " Seleccionar partido  " +
			FooterKeyStyle.Render("Enter") + " Ver señales  " +
			FooterKeyStyle.Render("r") + " Recargar  " +
			FooterKeyStyle.Render("q") + " Salir",
	)
}
