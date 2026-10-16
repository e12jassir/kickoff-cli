package ui

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// View renders the terminal user interface according to model state.
func (m Model) View() string {
	if m.width < 60 || m.height < 15 {
		return "Terminal window is too small (min 60x15). Please resize."
	}
	if m.state == StateStartup {
		return renderStartup(m)
	}
	if m.state == StateMainMenu {
		return renderMainMenu(m)
	}
	return renderBrowsing(m)
}

func renderStartup(m Model) string {
	logo := AsciiLogoStyle.Render(AsciiLogo)
	subtitle := SubtitleStyle.Render("Terminal Live Football • Cargando Competiciones...")

	loader := fmt.Sprintf("%s Cargando Kickoff...", m.spinner.View())
	loaderStyled := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(loader)
	hint := FooterHelpStyle.Render("Presiona cualquier tecla para continuar...")

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo,
		subtitle,
		"",
		loaderStyled,
		"",
		hint,
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

func renderMainMenu(m Model) string {
	logo := AsciiLogoStyle.Render(AsciiLogo)
	subtitle := SubtitleStyle.Render("Terminal Live Football")

	var lines []string

	for catIdx, cat := range m.categories {
		indices := m.categoryIndices[cat]
		isExpanded := m.expandedCats[cat]
		isCatSelected := m.categoryMode && (catIdx == m.categoryCursor)

		// Minimal aesthetic indicator for accordion category
		dot := "○"
		if isExpanded {
			dot = "●"
		}
		if m.noEmoji {
			if isExpanded {
				dot = "[-]"
			} else {
				dot = "[+]"
			}
		}

		catPrefix := "  "
		if isCatSelected {
			catPrefix = "› "
		}

		catText := fmt.Sprintf("%s%s %s (%d)", catPrefix, dot, strings.ToUpper(cat), len(indices))
		if isCatSelected {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(catText))
		} else {
			lines = append(lines, CategoryHeaderStyle.Render(catText))
		}

		if isExpanded {
			for _, lIdx := range indices {
				if lIdx < 0 || lIdx >= len(m.leagues) {
					continue
				}
				league := m.leagues[lIdx]
				isLeagueSelected := (!m.categoryMode || catIdx == m.categoryCursor) && (lIdx == m.leagueCursor)

				prefix := "     "
				if isLeagueSelected {
					prefix = "   › "
				}

				emoji := league.Emoji
				if m.noEmoji {
					emoji = "-"
				} else if emoji == "" {
					emoji = "⚽"
				}

				itemText := fmt.Sprintf("%s%s  %s", prefix, emoji, league.Name)
				if isLeagueSelected {
					lines = append(lines, MenuItemSelected.Render(itemText))
				} else {
					lines = append(lines, MenuItemNormal.Render(itemText))
				}
			}
		}
	}

	menuBox := MenuBoxStyle.Render(strings.Join(lines, "\n"))

	footerText := "↑/k ↓/j: navegar  •  enter/space: desplegar/entrar  •  ←/→: plegar/desplegar  •  q: salir"
	if m.updateAvailable != "" {
		footerText = fmt.Sprintf("↑/k ↓/j: navegar  •  enter: entrar  •  u: actualizar (%s)  •  q: salir", m.updateAvailable)
	}
	footer := FooterHelpStyle.Render(footerText)

	var elements []string
	elements = append(elements, logo, subtitle)
	if m.updateAvailable != "" {
		icon := "✨"
		if m.noEmoji {
			icon = "*"
		}
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF88")).
			Bold(true).
			Render(fmt.Sprintf("%s ¡Actualización disponible (%s)! Presiona [u] para actualizar", icon, m.updateAvailable))
		elements = append(elements, "", banner)
	}
	if m.statusMsg != "" {
		elements = append(elements, "", StatusBarSuccess.Render(m.statusMsg))
	}
	if m.errorMsg != "" {
		elements = append(elements, "", StatusBarError.Render(m.errorMsg))
	}
	elements = append(elements, "", menuBox, "", footer)

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

func renderBrowsing(m Model) string {
	curr := m.CurrentLeague()
	var headerText string
	if m.noEmoji {
		headerText = fmt.Sprintf("KICKOFF · %s", curr.Name)
	} else if curr.Emoji == "" {
		headerText = fmt.Sprintf("⚽ KICKOFF · %s", curr.Name)
	} else {
		headerText = fmt.Sprintf("⚽ KICKOFF · %s %s", curr.Emoji, curr.Name)
	}

	var statusBadge string
	if len(m.matches) > 0 && m.matches[0].IsOffline {
		cachedTimeStr := "--:--"
		if !m.matches[0].CachedAt.IsZero() {
			cachedTimeStr = m.matches[0].CachedAt.Format("15:04")
		}
		statusBadge = OfflineBadgeStyle.Render(fmt.Sprintf("[OFFLINE · cached at %s]", cachedTimeStr))
	} else {
		hasLive := false
		for _, match := range m.matches {
			if match.Status.State == "in" {
				hasLive = true
				break
			}
		}
		if hasLive {
			statusBadge = LiveBadgeStyle.Render("[LIVE · 5s sync]")
		}
	}

	if statusBadge != "" {
		headerText += "  " + statusBadge
	}
	header := AppTitleStyle.Render(headerText)

	// Top View Tabs: [1 Resumen] [2 Partidos] [3 Posiciones] [4 Goleadores]
	tabLabels := []string{"1 Resumen", "2 Partidos", "3 Posiciones", "4 Goleadores"}
	renderedTabs := make([]string, 4)
	for i, lbl := range tabLabels {
		if LeagueTab(i) == m.leagueTab {
			renderedTabs[i] = LeagueTabActive.Render(fmt.Sprintf("[● %s]", lbl))
		} else {
			renderedTabs[i] = LeagueTabInactive.Render(fmt.Sprintf("○ %s", lbl))
		}
	}
	tabsRow := lipgloss.JoinHorizontal(lipgloss.Center, renderedTabs[0], "   ", renderedTabs[1], "   ", renderedTabs[2], "   ", renderedTabs[3])

	var body string
	footer := FooterHelpStyle.Render("h/l o ←/→: pestaña  •  j/k o ↑/↓: navegar  •  enter: detalles  •  esc: ligas  •  r: recargar  •  q: salir")

	// Connecting stream pulse overlay check
	if m.state == StatePlaying && m.connecting {
		pulseWaves := []string{"📡 ∿", "📡 ∿ ∿", "📡 ∿ ∿ ∿", "📡 ∿ ∿ ∿ ∿"}
		wave := pulseWaves[m.pulseStep%len(pulseWaves)]
		feedName := m.streamFeed
		if feedName == "" {
			feedName = "Canal"
		}
		bitrateInfo := m.bitrate
		if bitrateInfo == "" {
			bitrateInfo = "HLS Stream"
		}
		pulseText := fmt.Sprintf("%s Conectando con %s... %s\nBitrate: %s", m.spinner.View(), feedName, wave, bitrateInfo)
		connectingBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#38BDF8")).
			Padding(1, 3).
			Width(62).
			Align(lipgloss.Center).
			Render(ConnectingPulseStyle.Render(pulseText))
		body = connectingBox
		footer = FooterHelpStyle.Render("Conectando stream de video...  •  esc: volver  •  q: salir")
	} else {
		switch m.state {
		case StateLoading:
			body = fmt.Sprintf("  %s %s", m.spinner.View(), m.loadingMsg)
			footer = FooterHelpStyle.Render("esc: volver  •  q: salir")

		case StateError:
			body = StatusBarError.Render(fmt.Sprintf("  ✖ Error: %s", m.errorMsg))
			footer = FooterHelpStyle.Render("r: recargar  •  esc: volver  •  q: salir")

		case StateChannelSelect:
			selMatch, _ := m.CurrentSelectedMatch()
			modalBox := ModalBoxStyle
			modalWidth := 60
			if m.modalTab == 2 && m.width >= 80 {
				modalWidth = 74
				modalBox = modalBox.Width(modalWidth + 6)
			}
			modalContent := renderChannelModal(selMatch, m.channels, m.channelCursor, m.modalTab, m.livePulse, m.spinner.View(), m.IsDetailsLoaded(selMatch.ID), m.noEmoji, m.lineupTeam, modalWidth)
			body = modalBox.Render(modalContent)
			footer = FooterHelpStyle.Render("h/l o ←/→: pestaña/equipo  •  j/k o ↑/↓: canal  •  enter: reproducir  •  esc: volver")

		default: // StateBrowsing, StatePlaying
			switch m.leagueTab {
			case TabResumen:
				body = renderTabResumen(m, curr)
			case TabPosiciones:
				body = renderTabPosiciones(m, curr)
			case TabGoleadores:
				body = renderTabGoleadores(m, curr)
			default: // TabPartidos
				body = renderTabPartidos(m, curr)
			}
		}
	}

	var elements []string
	elements = append(elements, header, tabsRow, "")
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

func renderTabPartidos(m Model, curr model.League) string {
	if len(m.matches) == 0 {
		emptyPrefix := "⚽ "
		if m.noEmoji {
			emptyPrefix = ""
		}
		emptyMsg := fmt.Sprintf("%sNo hay próximos partidos programados para %s.", emptyPrefix, curr.Name)
		return MatchBoxStyle.Render(EmptyStateStyle.Render(emptyMsg))
	}

	todayStr := time.Now().Format("2006-01-02")
	isTodayMatchday := false
	for _, match := range m.matches {
		if match.StartTime.Format("2006-01-02") == todayStr {
			isTodayMatchday = true
			break
		}
	}

	headerText := fmt.Sprintf("PARTIDOS DE HOY (%d)", len(m.matches))
	if !isTodayMatchday {
		firstDate := ""
		if len(m.matches) > 0 && !m.matches[0].StartTime.IsZero() {
			firstDate = fmt.Sprintf(" · %s", m.matches[0].StartTime.Format("02/01"))
		}
		headerText = fmt.Sprintf("PRÓXIMOS PARTIDOS%s (%d)", firstDate, len(m.matches))
	}

	cursor := m.matchCursor
	if cursor >= len(m.matches) {
		cursor = len(m.matches) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	headerText += fmt.Sprintf("  •  [%d/%d]", cursor+1, len(m.matches))

	var lines []string
	lines = append(lines, CategoryHeaderStyle.Render(headerText))
	lines = append(lines, "")

	// Sliding window for scroll support
	windowSize := 4
	if m.height >= 36 {
		windowSize = 6
	} else if m.height >= 28 {
		windowSize = 5
	}
	if windowSize > len(m.matches) {
		windowSize = len(m.matches)
	}

	start := 0
	if cursor >= windowSize {
		start = cursor - windowSize + 1
	}
	end := start + windowSize
	if end > len(m.matches) {
		end = len(m.matches)
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		upScroll := lipgloss.NewStyle().Foreground(ColorMuted).Width(56).Align(lipgloss.Center).Render(fmt.Sprintf("▲  %d partidos anteriores  ▲", start))
		lines = append(lines, upScroll, "")
	}

	for i := start; i < end; i++ {
		match := m.matches[i]
		isSelected := i == cursor
		itemLines := renderMatchItem(match, isSelected, m.livePulse)
		lines = append(lines, itemLines...)
		if i < end-1 {
			lines = append(lines, "")
		}
	}

	if end < len(m.matches) {
		remaining := len(m.matches) - end
		downScroll := lipgloss.NewStyle().Foreground(ColorMuted).Width(56).Align(lipgloss.Center).Render(fmt.Sprintf("▼  %d partidos más abajo  ▼", remaining))
		lines = append(lines, "", downScroll)
	}

	return MatchBoxStyle.Render(strings.Join(lines, "\n"))
}

func renderTabResumen(m Model, curr model.League) string {
	var lines []string

	// 1. League Leader Highlight from Standings if available
	if m.standings != nil && len(m.standings.Entries) > 0 {
		leader := m.standings.Entries[0]
		icon := "🏆"
		if m.noEmoji {
			icon = "[1]"
		}
		leaderText := fmt.Sprintf("%s Líder de Liga: %s (%s pts • %s PJ • DG %s)", icon, leader.TeamName, leader.Points, leader.GamesPlayed, leader.PointDiff)
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(leaderText))
		lines = append(lines, "")
	}

	// 2. Past Match Results (FT)
	pastMatches := m.ResumenMatches()

	if len(pastMatches) == 0 {
		lines = append(lines, CategoryHeaderStyle.Render("RESULTADOS RECIENTES / JORNADA PASADA (0)"))
		lines = append(lines, EmptyStateStyle.Render("No hay partidos finalizados recientemente."))
		return MatchBoxStyle.Render(strings.Join(lines, "\n"))
	}

	cursor := m.resumenCursor
	if cursor >= len(pastMatches) {
		cursor = len(pastMatches) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	headerText := fmt.Sprintf("RESULTADOS RECIENTES (%d)  •  [%d/%d]", len(pastMatches), cursor+1, len(pastMatches))
	lines = append(lines, CategoryHeaderStyle.Render(headerText))
	lines = append(lines, "")

	// Sliding window for scroll support
	windowSize := 3
	if m.height >= 36 {
		windowSize = 5
	} else if m.height >= 28 {
		windowSize = 4
	}
	if windowSize > len(pastMatches) {
		windowSize = len(pastMatches)
	}

	start := 0
	if cursor >= windowSize {
		start = cursor - windowSize + 1
	}
	end := start + windowSize
	if end > len(pastMatches) {
		end = len(pastMatches)
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		upScroll := lipgloss.NewStyle().Foreground(ColorMuted).Width(56).Align(lipgloss.Center).Render(fmt.Sprintf("▲  %d partidos anteriores  ▲", start))
		lines = append(lines, upScroll)
	}

	for i := start; i < end; i++ {
		pm := pastMatches[i]
		isSelected := i == cursor
		lines = append(lines, renderResumenCard(pm, isSelected))
	}

	if end < len(pastMatches) {
		remaining := len(pastMatches) - end
		downScroll := lipgloss.NewStyle().Foreground(ColorMuted).Width(56).Align(lipgloss.Center).Render(fmt.Sprintf("▼  %d partidos más abajo  ▼", remaining))
		lines = append(lines, downScroll)
	}

	return MatchBoxStyle.Render(strings.Join(lines, "\n"))
}

func renderResumenCard(pm model.Match, isSelected bool) string {
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(56)

	if isSelected {
		cardStyle = cardStyle.BorderForeground(ColorBorderSelected)
	}

	dateStr := "--/--"
	if !pm.StartTime.IsZero() {
		dateStr = pm.StartTime.Format("02/01")
	}

	prefix := "  "
	if isSelected {
		prefix = "› "
	}
	dateCol := MatchTimeStyle.Render(fmt.Sprintf("%s%s", prefix, dateStr))

	homeName := pm.HomeTeam.DisplayName
	if homeName == "" {
		homeName = "Local"
	}
	homeTrunc := truncateModalText(homeName, 14)
	homeStyled := lipgloss.NewStyle().Width(14).Align(lipgloss.Right).Foreground(ColorTextMuted).Render(homeTrunc)
	if isSelected {
		homeStyled = lipgloss.NewStyle().Width(14).Align(lipgloss.Right).Foreground(ColorHighlight).Bold(true).Render(homeTrunc)
	}

	homeScore := pm.HomeTeam.Score
	awayScore := pm.AwayTeam.Score
	if homeScore == "" && awayScore == "" {
		homeScore = "0"
		awayScore = "0"
	}
	scoreStr := fmt.Sprintf(" %s - %s ", homeScore, awayScore)
	scoreStyled := ScoreStyle.Width(7).Align(lipgloss.Center).Render(scoreStr)

	awayName := pm.AwayTeam.DisplayName
	if awayName == "" {
		awayName = "Visitante"
	}
	awayTrunc := truncateModalText(awayName, 14)
	awayStyled := lipgloss.NewStyle().Width(14).Align(lipgloss.Left).Foreground(ColorTextMuted).Render(awayTrunc)
	if isSelected {
		awayStyled = lipgloss.NewStyle().Width(14).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true).Render(awayTrunc)
	}

	statusBadge := StatusPostStyle.Render("[FINAL]")

	line1 := fmt.Sprintf("%s %s%s%s %s", dateCol, homeStyled, scoreStyled, awayStyled, statusBadge)

	venue := pm.Venue
	if venue == "" {
		venue = "Estadio Principal"
	}
	venueTrunc := truncateModalText(venue, 26)
	venueStyled := VenueStyle.Render(venueTrunc)

	var line2 string
	if isSelected {
		hintStyled := lipgloss.NewStyle().Foreground(ColorCategory).Render("Enter: detalles")
		line2 = fmt.Sprintf("        %-26s   %s", venueStyled, hintStyled)
	} else {
		line2 = fmt.Sprintf("        %s", venueStyled)
	}

	return cardStyle.Render(line1 + "\n" + line2)
}

func renderTabPosiciones(m Model, curr model.League) string {
	if m.standings == nil {
		if m.standingsErr != nil {
			return MatchBoxStyle.Render(StatusBarError.Render("No se pudieron cargar las posiciones de la liga."))
		}
		return MatchBoxStyle.Render(fmt.Sprintf("  %s Cargando tabla de posiciones...", m.spinner.View()))
	}

	var lines []string
	var displayEntries []model.StandingsEntry
	headerTitle := "TABLA DE POSICIONES · " + curr.Name

	if len(m.standings.Groups) > 0 {
		grpIdx := m.selectedGroup
		if grpIdx < 0 || grpIdx >= len(m.standings.Groups) {
			grpIdx = 0
		}
		currentGrp := m.standings.Groups[grpIdx]
		displayEntries = currentGrp.Entries

		groupNav := fmt.Sprintf("◄ %s (%d/%d) ►  [ [ / ] : grupo ]", currentGrp.Name, grpIdx+1, len(m.standings.Groups))
		headerTitle = fmt.Sprintf("POSICIONES · %s", lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(groupNav))
	} else {
		displayEntries = m.standings.Entries
	}

	if len(displayEntries) == 0 {
		return MatchBoxStyle.Render(EmptyStateStyle.Render("Tabla de posiciones no disponible para esta competición."))
	}

	lines = append(lines, CategoryHeaderStyle.Render(headerTitle))
	lines = append(lines, "")

	// Table header: #  Equipo               PJ  G   E   P   DG  PTS
	hdr := fmt.Sprintf(" %-2s  %-22s  %3s %3s %3s %3s %4s  %3s", "#", "Equipo", "PJ", "G", "E", "P", "DG", "PTS")
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

	for _, e := range displayEntries {
		rankStr := fmt.Sprintf("%d", e.Rank)
		teamCol := truncateModalText(e.TeamName, 22)
		row := fmt.Sprintf(" %-2s  %-22s  %3s %3s %3s %3s %4s  %3s", rankStr, teamCol, e.GamesPlayed, e.Wins, e.Ties, e.Losses, e.PointDiff, e.Points)
		if e.Rank <= 4 {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(row))
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(row))
		}
	}

	return MatchBoxStyle.Render(strings.Join(lines, "\n"))
}

func renderTabGoleadores(m Model, curr model.League) string {
	if m.statistics == nil {
		if m.statisticsErr != nil {
			return MatchBoxStyle.Render(StatusBarError.Render("No se pudieron cargar las estadísticas de goleadores."))
		}
		return MatchBoxStyle.Render(fmt.Sprintf("  %s Cargando máximos goleadores y asistentes...", m.spinner.View()))
	}

	var lines []string
	lines = append(lines, CategoryHeaderStyle.Render("MÁXIMOS GOLEADORES (TOP 5)"))
	lines = append(lines, "")

	if len(m.statistics.Scorers) == 0 {
		lines = append(lines, EmptyStateStyle.Render("Estadísticas de goles no disponibles aún."))
	} else {
		hdr := fmt.Sprintf(" %-2s  %-24s  %-18s  %4s", "#", "Jugador", "Equipo", "Goles")
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

		limit := 5
		if len(m.statistics.Scorers) < limit {
			limit = len(m.statistics.Scorers)
		}
		for i := 0; i < limit; i++ {
			s := m.statistics.Scorers[i]
			row := fmt.Sprintf(" %-2d  %-24s  %-18s  %4s", i+1, truncateModalText(s.PlayerName, 24), truncateModalText(s.TeamName, 18), s.Value)
			if i < 3 {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(row))
			} else {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(row))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, CategoryHeaderStyle.Render("MÁXIMOS ASISTENTES (TOP 5)"))
	lines = append(lines, "")

	if len(m.statistics.Assists) == 0 {
		lines = append(lines, EmptyStateStyle.Render("Estadísticas de asistencias no disponibles aún."))
	} else {
		hdr := fmt.Sprintf(" %-2s  %-24s  %-18s  %4s", "#", "Jugador", "Equipo", "Asist.")
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

		limit := 5
		if len(m.statistics.Assists) < limit {
			limit = len(m.statistics.Assists)
		}
		for i := 0; i < limit; i++ {
			a := m.statistics.Assists[i]
			row := fmt.Sprintf(" %-2d  %-24s  %-18s  %4s", i+1, truncateModalText(a.PlayerName, 24), truncateModalText(a.TeamName, 18), a.Value)
			if i < 3 {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(row))
			} else {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(row))
			}
		}
	}

	return MatchBoxStyle.Render(strings.Join(lines, "\n"))
}

func renderMatchItem(match model.Match, isSelected bool, livePulse bool) []string {
	timeStr := match.TimeStr
	if timeStr == "" {
		timeStr = "--:--"
	}
	todayStr := time.Now().Format("2006-01-02")
	if !match.StartTime.IsZero() && match.StartTime.Format("2006-01-02") != todayStr {
		timeStr = fmt.Sprintf("[%s] %s", match.StartTime.Format("02/01"), timeStr)
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

func renderChannelModal(match model.Match, channels []model.Channel, selectedIdx int, modalTab int, livePulse bool, spinnerStr string, detailsLoaded bool, optArgs ...any) string {
	noEmoji := false
	lineupTeam := 0
	modalContentWidth := 60

	if len(optArgs) > 0 {
		if b, ok := optArgs[0].(bool); ok {
			noEmoji = b
		}
	}
	if len(optArgs) > 1 {
		if t, ok := optArgs[1].(int); ok {
			lineupTeam = t
		}
	}
	if len(optArgs) > 2 {
		if w, ok := optArgs[2].(int); ok && w > 0 {
			modalContentWidth = w
		}
	}
	var modal strings.Builder

	homeName := match.HomeTeam.DisplayName
	if homeName == "" {
		homeName = "Local"
	}
	awayName := match.AwayTeam.DisplayName
	if awayName == "" {
		awayName = "Visitante"
	}

	// 1. Header line 1: Status • Stadium
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

	headerLine1 := fmt.Sprintf("%s  •  %s", statusPart, venueStyled)
	modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Render(headerLine1) + "\n")

	// 1. Header line 2: Scoreboard Duel
	var duel string
	if match.HomeTeam.Score != "" && match.AwayTeam.Score != "" {
		duel = fmt.Sprintf("%s   %s   %s",
			ScoreboardTeams.Render(homeName),
			ScoreStyle.Render(fmt.Sprintf("%s ─ %s", match.HomeTeam.Score, match.AwayTeam.Score)),
			ScoreboardTeams.Render(awayName),
		)
	} else {
		duel = fmt.Sprintf("%s   %s   %s",
			ScoreboardTeams.Render(homeName),
			StatusPreStyle.Render("─ vs ─"),
			ScoreboardTeams.Render(awayName),
		)
	}
	modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Render(duel) + "\n\n")

	// 2. Interactive Segmented Pill Tabs
	tabNames := []string{"Resumen", "Estadísticas", "Alineaciones"}
	pills := make([]string, 3)
	for i := 0; i < 3; i++ {
		if i == modalTab {
			pills[i] = ActiveTabPillStyle.Render("[ ● " + tabNames[i] + " ]")
		} else {
			pills[i] = InactiveTabPillStyle.Render("○ " + tabNames[i])
		}
	}
	tabsRow := lipgloss.JoinHorizontal(lipgloss.Center, pills[0], "     ", pills[1], "     ", pills[2])
	modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Render(tabsRow) + "\n\n")

	// 3. Tab contents
	switch modalTab {
	case 0:
		// Tab 0: Resumen
		if len(match.Events) > 0 {
			tlStr := renderTimeline(match.Events, match.HomeTeam.ID, modalContentWidth)
			modal.WriteString(tlStr)
			modal.WriteString("\n")
		}

		modal.WriteString(ModalDivider.Render("────────────────────────────────────────────────────────────") + "\n\n")

		if match.Status.Completed || strings.ToLower(match.Status.State) == "post" {
			finishedBadge := lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render("[ ⏹ Transmisión finalizada • No hay señales activas ]")
			modal.WriteString(finishedBadge + "\n")
		} else if len(channels) == 0 {
			emptyNoticeText := "[ ⚙️ Sin señales activas • Configura tus canales en ~/.config/kickoff/channels.json ]"
			if noEmoji {
				emptyNoticeText = "[ Sin señales activas • Configura tus canales en ~/.config/kickoff/channels.json ]"
			}
			emptyNotice := lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render(emptyNoticeText)
			modal.WriteString(emptyNotice + "\n")
		} else {
			modal.WriteString(ModalSectionHeader.Render("SEÑALES DE TRANSMISIÓN") + "\n\n")
			for i, ch := range channels {
				prefix := "   "
				if i == selectedIdx {
					prefix = " › "
				}

				cleanName := cleanChannelName(ch.DisplayName)
				if cleanName == "" {
					cleanName = cleanChannelName(ch.Name)
				}
				if cleanName == "" {
					cleanName = "Canal"
				}

				nameCol := lipgloss.NewStyle().Width(22).MaxWidth(22).Render(truncateModalText(cleanName, 22))

				var prefixAndName string
				if i == selectedIdx {
					prefixAndName = ChannelItemSelected.Render(prefix + nameCol)
				} else {
					prefixAndName = ChannelItemNormal.Render(prefix + nameCol)
				}

				res := ch.Resolution
				if res == "" {
					res = "720p"
				}
				bitrate := ch.BitrateMode
				if bitrate == "" {
					bitrate = "auto"
				}
				badgeText := fmt.Sprintf("[%s · %s]", res, bitrate)
				badgeCol := ResolutionBadge.Width(14).MaxWidth(14).Render(badgeText)

				recBadge := ""
				if ch.Recommended {
					recBadge = "  " + RecommendedBadge.Render("★ Recomendado")
				}

				itemLine := fmt.Sprintf("%s  %s%s", prefixAndName, badgeCol, recBadge)
				modal.WriteString(itemLine + "\n")
			}
		}

		if match.Referee != "" {
			refereeText := fmt.Sprintf("👤 Árbitro: %s", match.Referee)
			if noEmoji {
				refereeText = fmt.Sprintf("Árbitro: %s", match.Referee)
			}
			modal.WriteString("\n" + lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render(refereeText) + "\n")
		}

	case 1:
		// Tab 1: Estadísticas
		if isStatsEmpty(match.Stats) {
			isPre := strings.ToLower(strings.TrimSpace(match.Status.State)) == "pre"
			if isPre || detailsLoaded || match.ID == "" {
				emptyText := "[ Estadísticas no disponibles para este encuentro ]"
				modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render(emptyText) + "\n")
			} else {
				loadingText := "Consultando estadísticas del partido..."
				if spinnerStr != "" {
					loadingText = fmt.Sprintf("%s  Consultando estadísticas del partido...", spinnerStr)
				}
				modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render(loadingText) + "\n")
			}
		} else {
			barLine := renderPossessionBar(match.Stats.HomePossession, match.Stats.AwayPossession, modalContentWidth)
			modal.WriteString(barLine + "\n")
			modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorCategory).Bold(true).Render("Posesión") + "\n\n")

			homeShots := match.Stats.HomeShots
			if match.Stats.HomeShotsOnGoal != "" {
				homeShots = fmt.Sprintf("%s (%s)", match.Stats.HomeShots, match.Stats.HomeShotsOnGoal)
			}
			awayShots := match.Stats.AwayShots
			if match.Stats.AwayShotsOnGoal != "" {
				awayShots = fmt.Sprintf("%s (%s)", match.Stats.AwayShots, match.Stats.AwayShotsOnGoal)
			}
			modal.WriteString(renderStatRow(homeShots, "Tiros (Al arco)", awayShots) + "\n")

			homePasses := match.Stats.HomePasses
			if match.Stats.HomePassPct != "" {
				homePasses = fmt.Sprintf("%s (%s)", match.Stats.HomePasses, match.Stats.HomePassPct)
			}
			awayPasses := match.Stats.AwayPasses
			if match.Stats.AwayPassPct != "" {
				awayPasses = fmt.Sprintf("%s (%s)", match.Stats.AwayPasses, match.Stats.AwayPassPct)
			}
			modal.WriteString(renderStatRow(homePasses, "Pases (Precisión)", awayPasses) + "\n")

			modal.WriteString(renderStatRow(match.Stats.HomeFouls, "Faltas", match.Stats.AwayFouls) + "\n")
			modal.WriteString(renderStatRow(match.Stats.HomeCorners, "Córners", match.Stats.AwayCorners) + "\n")
			modal.WriteString(renderStatRow(match.Stats.HomeOffsides, "Fueras de juego", match.Stats.AwayOffsides) + "\n")
			modal.WriteString(renderStatRow(match.Stats.HomeSaves, "Atajadas", match.Stats.AwaySaves) + "\n")
		}

	case 2:
		// Tab 2: Alineaciones
		modal.WriteString(RenderTacticalPitch(match, lineupTeam, modalContentWidth, noEmoji) + "\n")
	}

	return modal.String()
}

// renderModalContent renders the tactical pitch modal tab content.
func renderModalContent(match model.Match, lineupTeam int, modalContentWidth int, noEmoji bool) string {
	return RenderTacticalPitch(match, lineupTeam, modalContentWidth, noEmoji)
}

func renderPossessionBar(homeStr, awayStr string, totalWidth int) string {
	parseVal := func(s string) float64 {
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
		var v float64
		if _, err := fmt.Sscanf(s, "%f", &v); err == nil {
			return v
		}
		return 50
	}

	hVal := parseVal(homeStr)
	aVal := parseVal(awayStr)
	total := hVal + aVal
	if total <= 0 {
		total = 100
		hVal = 50
		aVal = 50
	}

	const barSlots = 20
	// Calculate smooth fractional fill for home and away
	hFraction := hVal / total
	if hFraction < 0.05 && hVal > 0 {
		hFraction = 0.05
	} else if hFraction > 0.95 && aVal > 0 {
		hFraction = 0.95
	}

	totalUnits := int(math.Round(hFraction * float64(barSlots*8)))
	fullBlocks := totalUnits / 8
	rem := totalUnits % 8
	if fullBlocks > barSlots {
		fullBlocks = barSlots
	}

	hBlocks := strings.Repeat("█", fullBlocks)
	partial := ""
	usedSlots := fullBlocks
	fractions := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	if usedSlots < barSlots && rem > 0 {
		partial = fractions[rem]
		usedSlots++
	}
	aSlots := barSlots - usedSlots
	if aSlots < 0 {
		aSlots = 0
	}
	aBlocks := strings.Repeat("░", aSlots)

	leftNum := lipgloss.NewStyle().Width(5).Align(lipgloss.Right).Foreground(ColorHighlight).Bold(true).Render(homeStr)
	rightNum := lipgloss.NewStyle().Width(5).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true).Render(awayStr)
	bracketOpen := lipgloss.NewStyle().Foreground(ColorBorder).Render("[")
	bracketClose := lipgloss.NewStyle().Foreground(ColorBorder).Render("]")
	hStyled := lipgloss.NewStyle().Foreground(ColorHighlight).Render(hBlocks + partial)
	aStyled := lipgloss.NewStyle().Foreground(ColorMuted).Render(aBlocks)

	barRow := fmt.Sprintf("%s  %s%s%s%s  %s", leftNum, bracketOpen, hStyled, aStyled, bracketClose, rightNum)
	return lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center).Render(barRow)
}

// truncateModalText shortens s to at most maxLen runes.
//
// When truncation is required, the last visible rune is replaced by an
// ellipsis "…", so the result keeps exactly maxLen runes. A non-positive
// maxLen returns an empty string instead of panicking on a negative slice.
func truncateModalText(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen-1]) + "…"
	}
	return s
}

var (
	reBracketTag   = regexp.MustCompile(`\[.*?\]`)
	reParenSuffix  = regexp.MustCompile(`\s*\(.*?\)`)
	reBulletPrefix = regexp.MustCompile(`^[\s·•\-\*]+`)
)

func cleanChannelName(raw string) string {
	s := strings.TrimSpace(raw)
	s = reBracketTag.ReplaceAllString(s, "")
	s = reParenSuffix.ReplaceAllString(s, "")
	s = reBulletPrefix.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func renderStatRow(leftVal, label, rightVal string) string {
	leftStyle := lipgloss.NewStyle().Width(18).Align(lipgloss.Right).Foreground(ColorHighlight).Bold(true)
	labelStyle := lipgloss.NewStyle().Width(24).Align(lipgloss.Center).Foreground(ColorCategory)
	rightStyle := lipgloss.NewStyle().Width(18).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true)

	return fmt.Sprintf("%s%s%s", leftStyle.Render(leftVal), labelStyle.Render(label), rightStyle.Render(rightVal))
}

func renderTimeline(events []model.MatchEvent, homeTeamID string, modalContentWidth int) string {
	if len(events) == 0 {
		return ""
	}

	sortedEvents := make([]model.MatchEvent, len(events))
	copy(sortedEvents, events)

	parseClockMinute := func(s string) int {
		s = strings.TrimSuffix(strings.TrimSpace(s), "'")
		var m int
		if idx := strings.Index(s, "+"); idx != -1 {
			_, _ = fmt.Sscanf(s[:idx], "%d", &m)
			var extra int
			_, _ = fmt.Sscanf(s[idx+1:], "%d", &extra)
			return m*60 + extra
		}
		_, _ = fmt.Sscanf(s, "%d", &m)
		return m * 60
	}

	sort.SliceStable(sortedEvents, func(i, j int) bool {
		ci := sortedEvents[i].ClockValue
		if ci <= 0 {
			ci = float64(parseClockMinute(sortedEvents[i].Clock))
		}
		cj := sortedEvents[j].ClockValue
		if cj <= 0 {
			cj = float64(parseClockMinute(sortedEvents[j].Clock))
		}
		return ci < cj
	})

	var b strings.Builder
	htShown := false
	spineStyle := lipgloss.NewStyle().Foreground(ColorBorderSelected)

	formatClock5 := func(clock string) string {
		c := strings.TrimSpace(clock)
		switch len(c) {
		case 2:
			return fmt.Sprintf("  %s ", c)
		case 3:
			return fmt.Sprintf(" %s ", c)
		case 4:
			return fmt.Sprintf(" %s", c)
		case 5:
			return c
		default:
			if len(c) > 5 {
				return c[:5]
			}
			return fmt.Sprintf("%-5s", c)
		}
	}

	for _, ev := range sortedEvents {
		sec := ev.ClockValue
		if sec <= 0 {
			sec = float64(parseClockMinute(ev.Clock))
		}
		if !htShown && sec > 45*60 {
			leftBlank := strings.Repeat(" ", 24)
			etText := " " + StatusPostStyle.Render("E.T") + " "
			axis := spineStyle.Render("  │") + etText + spineStyle.Render("│  ")
			rightBlank := strings.Repeat(" ", 25)
			b.WriteString(fmt.Sprintf("%s%s%s\n", leftBlank, axis, rightBlank))
			htShown = true
		}

		clockBadge := strings.TrimSpace(ev.Clock)
		if clockBadge == "" {
			clockBadge = "--'"
		}
		clkFmt := formatClock5(clockBadge)

		icon := "⚽"
		if ev.IsRedCard {
			icon = "🟥"
		} else if ev.IsYellow {
			icon = "🟨"
		}

		pName := strings.TrimSpace(ev.PlayerName)
		if pName == "" || strings.EqualFold(pName, "jugador") {
			if ev.IsYellow || ev.IsRedCard {
				pName = "Cuerpo técnico"
			} else if ev.IsOwnGoal {
				pName = "Autogol"
			} else {
				pName = "Gol"
			}
		}
		if ev.IsPenalty {
			pName += " (pen.)"
		} else if ev.IsOwnGoal {
			pName += " (AG)"
		}

		isHome := ev.TeamID == homeTeamID
		if isHome {
			eventText := fmt.Sprintf("%s %s ", truncateModalText(pName, 19), icon)
			leftStyled := lipgloss.NewStyle().Width(24).Align(lipgloss.Right).Foreground(ColorText).Render(eventText)
			axis := spineStyle.Render("──┤") + clkFmt + spineStyle.Render("│  ")
			rightBlank := strings.Repeat(" ", 25)
			b.WriteString(fmt.Sprintf("%s%s%s\n", leftStyled, axis, rightBlank))
		} else {
			leftBlank := strings.Repeat(" ", 24)
			axis := spineStyle.Render("  │") + clkFmt + spineStyle.Render("├──")
			eventText := fmt.Sprintf(" %s %s", icon, truncateModalText(pName, 19))
			rightStyled := lipgloss.NewStyle().Width(25).Align(lipgloss.Left).Foreground(ColorText).Render(eventText)
			b.WriteString(fmt.Sprintf("%s%s%s\n", leftBlank, axis, rightStyled))
		}
	}
	return b.String()
}

func isStatsEmpty(stats *model.MatchStats) bool {
	if stats == nil {
		return true
	}
	return stats.HomePossession == "" && stats.AwayPossession == "" &&
		stats.HomeShots == "" && stats.AwayShots == "" &&
		stats.HomePasses == "" && stats.AwayPasses == "" &&
		stats.HomeCorners == "" && stats.AwayCorners == "" &&
		stats.HomeFouls == "" && stats.AwayFouls == ""
}
