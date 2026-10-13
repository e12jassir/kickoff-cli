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
	footer := FooterHelpStyle.Render("↑/k  ↓/j: navegar  •  enter: abrir partido  •  esc: ligas  •  r: recargar  •  q: salir")

	switch m.state {
	case StateLoading:
		body = fmt.Sprintf("  %s %s", m.spinner.View(), m.loadingMsg)
		footer = FooterHelpStyle.Render("esc: volver  •  q: salir")

	case StateError:
		body = StatusBarError.Render(fmt.Sprintf("  ✖ Error: %s", m.errorMsg))
		footer = FooterHelpStyle.Render("r: recargar  •  esc: volver  •  q: salir")

	case StateChannelSelect:
		var selMatch model.Match
		if len(m.matches) > 0 && m.matchCursor < len(m.matches) {
			selMatch = m.matches[m.matchCursor]
		}
		modalContent := renderChannelModal(selMatch, m.channels, m.channelCursor, m.livePulse)
		body = ModalBoxStyle.Render(modalContent)
		if len(m.channels) > 0 {
			footer = FooterHelpStyle.Render("↑/k  ↓/j: navegar  •  enter: reproducir  •  esc: volver  •  q: salir")
		} else {
			footer = FooterHelpStyle.Render("esc: volver  •  q: salir")
		}

	default: // StateBrowsing, StatePlaying
		if len(m.matches) == 0 {
			emptyMsg := fmt.Sprintf("⚽ No hay partidos programados hoy para %s.", curr.Name)
			body = MatchBoxStyle.Render(EmptyStateStyle.Render(emptyMsg))
		} else {
			var lines []string
			lines = append(lines, CategoryHeaderStyle.Render(fmt.Sprintf("PARTIDOS DE HOY (%d)", len(m.matches))))
			lines = append(lines, "")

			for i, match := range m.matches {
				isSelected := i == m.matchCursor
				itemLines := renderMatchItem(match, isSelected, m.livePulse)
				lines = append(lines, itemLines...)
				if i < len(m.matches)-1 {
					lines = append(lines, "")
				}
			}

			body = MatchBoxStyle.Render(strings.Join(lines, "\n"))
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

func renderMatchItem(match model.Match, isSelected bool, livePulse bool) []string {
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

	cursorPrefix := "   "
	if isSelected {
		cursorPrefix = " › "
	}

	timeStyled := MatchTimeStyle.Render(timeStr)
	vsStyled := ScoreboardTeams.Render(fmt.Sprintf("%s vs %s", homeName, awayName))
	if !isSelected {
		vsStyled = TeamNameStyle.Render(fmt.Sprintf("%s vs %s", homeName, awayName))
	}

	line1 := fmt.Sprintf("%s%s  %s", cursorPrefix, timeStyled, vsStyled)

	// Line 2: Details / Score / Venue
	venue := match.Venue
	if venue == "" {
		venue = "Estadio Principal"
	}

	var line2 string
	indent := "         "

	switch match.Status.State {
	case "in":
		dot := LiveDotDim.Render("●")
		if livePulse {
			dot = LiveDotBright.Render("●")
		}
		statusText := match.Status.Description
		if match.Status.Detail != "" {
			statusText = match.Status.Detail
		}
		liveLabel := LiveTextStyle.Render(fmt.Sprintf("%s %s", dot, statusText))
		scoreLabel := ScoreStyle.Render(fmt.Sprintf("%s - %s", match.HomeTeam.Score, match.AwayTeam.Score))
		line2 = fmt.Sprintf("%s%s  •  %s  •  %s", indent, liveLabel, scoreLabel, VenueStyle.Render(venue))

	case "post":
		scoreLabel := ScoreStyle.Render(fmt.Sprintf("%s - %s", match.HomeTeam.Score, match.AwayTeam.Score))
		line2 = fmt.Sprintf("%s%s  •  %s  •  %s", indent, StatusPostStyle.Render("FIN"), scoreLabel, VenueStyle.Render(venue))

	default: // "pre"
		line2 = fmt.Sprintf("%s%s  •  %s", indent, StatusPreStyle.Render("Por jugar"), VenueStyle.Render(venue))
	}

	return []string{line1, line2}
}

func renderChannelModal(match model.Match, channels []model.Channel, selectedIdx int, livePulse bool) string {
	var modal strings.Builder

	homeName := match.HomeTeam.DisplayName
	if homeName == "" {
		homeName = "Local"
	}
	awayName := match.AwayTeam.DisplayName
	if awayName == "" {
		awayName = "Visitante"
	}

	// 1. Header row: Status and Stadium
	var statusPart string
	switch match.Status.State {
	case "in":
		dot := LiveDotDim.Render("●")
		if livePulse {
			dot = LiveDotBright.Render("●")
		}
		statusText := match.Status.Description
		if match.Status.Detail != "" {
			statusText = match.Status.Detail
		}
		statusPart = fmt.Sprintf("%s %s", dot, LiveTextStyle.Render("EN VIVO "+statusText))
	case "post":
		statusPart = StatusPostStyle.Render("FINALIZADO")
	default:
		statusPart = StatusPreStyle.Render(fmt.Sprintf("%s (Hoy)", match.TimeStr))
	}

	venue := match.Venue
	if venue == "" {
		venue = "Estadio Principal"
	}
	venueStyled := VenueStyle.Render(venue)

	headerRow := fmt.Sprintf("%s    %s", statusPart, venueStyled)
	modal.WriteString(headerRow)
	modal.WriteString("\n\n")

	// 2. Scoreboard Duelo
	var duel string
	if match.HomeTeam.Score != "" && match.AwayTeam.Score != "" {
		duel = fmt.Sprintf("   %s   %s   %s",
			ScoreboardTeams.Render(homeName),
			ScoreStyle.Render(fmt.Sprintf("%s ─ %s", match.HomeTeam.Score, match.AwayTeam.Score)),
			ScoreboardTeams.Render(awayName),
		)
	} else {
		duel = fmt.Sprintf("   %s   %s   %s",
			ScoreboardTeams.Render(homeName),
			StatusPreStyle.Render("─ vs ─"),
			ScoreboardTeams.Render(awayName),
		)
	}
	modal.WriteString(duel)
	modal.WriteString("\n\n")

	// Events (Goals, Cards)
	if len(match.Events) > 0 {
		var homeEvents, awayEvents []model.MatchEvent
		for _, ev := range match.Events {
			if ev.TeamID == match.HomeTeam.ID {
				homeEvents = append(homeEvents, ev)
			} else {
				awayEvents = append(awayEvents, ev)
			}
		}

		var homeLines, awayLines []string
		for _, ev := range homeEvents {
			homeLines = append(homeLines, formatModalEvent(ev))
		}
		for _, ev := range awayEvents {
			awayLines = append(awayLines, formatModalEvent(ev))
		}

		maxLines := len(homeLines)
		if len(awayLines) > maxLines {
			maxLines = len(awayLines)
		}

		colStyle := lipgloss.NewStyle().Width(26)
		for i := 0; i < maxLines; i++ {
			hText := ""
			if i < len(homeLines) {
				hText = homeLines[i]
			}
			aText := ""
			if i < len(awayLines) {
				aText = awayLines[i]
			}
			modal.WriteString(fmt.Sprintf("   %s  %s\n", colStyle.Render(truncateModalText(hText, 26)), colStyle.Render(truncateModalText(aText, 26))))
		}
		modal.WriteString("\n")
	}

	// Divider
	modal.WriteString(ModalDivider.Render("──────────────────────────────────────────────────────────") + "\n")

	// Match Statistics
	if match.Stats != nil {
		modal.WriteString(ModalSectionHeader.Render("ESTADÍSTICAS DEL PARTIDO") + "\n\n")

		homeShots := match.Stats.HomeShots
		if match.Stats.HomeShotsOnGoal != "" {
			homeShots = fmt.Sprintf("%s (%s)", match.Stats.HomeShots, match.Stats.HomeShotsOnGoal)
		}
		awayShots := match.Stats.AwayShots
		if match.Stats.AwayShotsOnGoal != "" {
			awayShots = fmt.Sprintf("%s (%s)", match.Stats.AwayShots, match.Stats.AwayShotsOnGoal)
		}

		modal.WriteString(renderStatRow(match.Stats.HomePossession, "Posesión", match.Stats.AwayPossession) + "\n")
		modal.WriteString(renderStatRow(homeShots, "Tiros (Al arco)", awayShots) + "\n")
		modal.WriteString(renderStatRow(match.Stats.HomeCorners, "Córners", match.Stats.AwayCorners) + "\n")
		modal.WriteString(renderStatRow(match.Stats.HomeFouls, "Faltas", match.Stats.AwayFouls) + "\n")
		modal.WriteString("\n")

		modal.WriteString(ModalDivider.Render("──────────────────────────────────────────────────────────") + "\n")
	}

	// Section Title
	modal.WriteString(ModalSectionHeader.Render("SEÑALES DE TRANSMISIÓN"))
	modal.WriteString("\n\n")

	// 5. Clean Channel items (Zero cheesy emojis) or empty state
	if len(channels) == 0 {
		var emptyNotice string
		if match.Status.Completed || strings.ToLower(match.Status.State) == "post" {
			emptyNotice = "Este partido ya finalizó. No hay transmisiones en vivo disponibles."
		} else {
			emptyNotice = "Las señales estarán disponibles cerca de la hora del partido."
		}
		modal.WriteString("   " + VenueStyle.Render(emptyNotice) + "\n")
	} else {
		for i, ch := range channels {
			prefix := "   "
			if i == selectedIdx {
				prefix = " › "
			}

			item := fmt.Sprintf("%s%s", prefix, ch.DisplayName)
			if i == selectedIdx {
				modal.WriteString(ChannelItemSelected.Render(item))
			} else {
				modal.WriteString(ChannelItemNormal.Render(item))
			}
			modal.WriteString("\n")
		}
	}

	return modal.String()
}

func formatModalEvent(ev model.MatchEvent) string {
	clock := strings.TrimSpace(ev.Clock)
	clockStr := ""
	if clock != "" {
		clockStr = " " + clock
	}

	pName := strings.TrimSpace(ev.PlayerName)
	if pName == "" {
		pName = "Jugador"
	}

	if ev.IsRedCard {
		return fmt.Sprintf("🟥 %s%s", pName, clockStr)
	}
	if ev.IsYellow {
		return fmt.Sprintf("🟨 %s%s", pName, clockStr)
	}

	tag := ""
	if ev.IsOwnGoal {
		tag = " (AG)"
	} else if ev.IsPenalty {
		tag = " (pen.)"
	}

	return fmt.Sprintf("⚽ %s%s%s", pName, tag, clockStr)
}

func truncateModalText(s string, maxLen int) string {
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen-1]) + "…"
	}
	return s
}

func renderStatRow(leftVal, label, rightVal string) string {
	leftStyle := lipgloss.NewStyle().Width(14).Align(lipgloss.Right).Foreground(ColorHighlight).Bold(true)
	labelStyle := lipgloss.NewStyle().Width(24).Align(lipgloss.Center).Foreground(ColorCategory)
	rightStyle := lipgloss.NewStyle().Width(14).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true)

	return fmt.Sprintf("   %s %s %s", leftStyle.Render(leftVal), labelStyle.Render(label), rightStyle.Render(rightVal))
}
