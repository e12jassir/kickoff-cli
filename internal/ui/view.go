package ui

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

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
	subtitle := SubtitleStyle.Render("Terminal Live Football • Initializing Engine")

	// Bouncing ball progress bar: 30 chars wide
	const barWidth = 30
	ballPos := (m.startupProgress * (barWidth - 1)) / 100
	if ballPos >= barWidth {
		ballPos = barWidth - 1
	}

	var track strings.Builder
	for i := 0; i < barWidth; i++ {
		if i == ballPos {
			if m.noEmoji {
				track.WriteString("O")
			} else {
				track.WriteString("⚽")
			}
		} else {
			track.WriteString("─")
		}
	}

	progressText := fmt.Sprintf("[%s] %d%%", track.String(), m.startupProgress)
	progressStyled := lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(progressText)
	hint := FooterHelpStyle.Render("Press any key to skip warmup...")

	content := lipgloss.JoinVertical(lipgloss.Center,
		logo,
		subtitle,
		"",
		progressStyled,
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
		if m.noEmoji {
			emoji = "-"
		} else if emoji == "" {
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

	footerText := "↑/k  ↓/j: navegar  •  enter: entrar  •  q: salir"
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
	footer := FooterHelpStyle.Render("1-4/tab: cambiar vista  •  ↑/↓: navegar  •  enter: ver partido  •  esc: ligas  •  r: recargar  •  q: salir")

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
			var selMatch model.Match
			if len(m.matches) > 0 && m.matchCursor < len(m.matches) {
				selMatch = m.matches[m.matchCursor]
			}
			modalContent := renderChannelModal(selMatch, m.channels, m.channelCursor, m.modalTab, m.livePulse, m.spinner.View(), m.noEmoji)
			body = ModalBoxStyle.Render(modalContent)
			footer = FooterHelpStyle.Render("←/→/tab: pestaña  •  ↑/↓: canal  •  enter: reproducir  •  esc: volver")

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
		emptyMsg := fmt.Sprintf("%sNo hay partidos programados hoy para %s.", emptyPrefix, curr.Name)
		return MatchBoxStyle.Render(EmptyStateStyle.Render(emptyMsg))
	}

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
	var pastMatches []model.Match
	var upcomingMatches []model.Match
	for _, mt := range m.matches {
		if mt.Status.Completed || strings.ToLower(mt.Status.State) == "post" {
			pastMatches = append(pastMatches, mt)
		} else if strings.ToLower(mt.Status.State) == "pre" {
			upcomingMatches = append(upcomingMatches, mt)
		}
	}

	lines = append(lines, CategoryHeaderStyle.Render(fmt.Sprintf("RESULTADOS RECIENTES (%d)", len(pastMatches))))
	if len(pastMatches) == 0 {
		lines = append(lines, EmptyStateStyle.Render("No hay partidos finalizados hoy."))
	} else {
		for _, pm := range pastMatches {
			score := fmt.Sprintf("%s %s - %s %s", pm.HomeTeam.DisplayName, pm.HomeTeam.Score, pm.AwayTeam.Score, pm.AwayTeam.DisplayName)
			lines = append(lines, fmt.Sprintf("  • %s  (%s)", ScoreboardTeams.Render(score), StatusPostStyle.Render("FINAL")))
		}
	}
	lines = append(lines, "")

	// 3. Next Upcoming Fixtures
	lines = append(lines, CategoryHeaderStyle.Render(fmt.Sprintf("PRÓXIMOS ENCUENTROS (%d)", len(upcomingMatches))))
	if len(upcomingMatches) == 0 {
		lines = append(lines, EmptyStateStyle.Render("No hay partidos pendientes hoy."))
	} else {
		for _, um := range upcomingMatches {
			fixture := fmt.Sprintf("%s vs %s", um.HomeTeam.DisplayName, um.AwayTeam.DisplayName)
			lines = append(lines, fmt.Sprintf("  • %s  %s  (%s)", MatchTimeStyle.Render(um.TimeStr), TeamNameStyle.Render(fixture), VenueStyle.Render(um.Venue)))
		}
	}

	return MatchBoxStyle.Render(strings.Join(lines, "\n"))
}

func renderTabPosiciones(m Model, curr model.League) string {
	if m.standings == nil {
		if m.standingsErr != nil {
			return MatchBoxStyle.Render(StatusBarError.Render("No se pudieron cargar las posiciones de la liga."))
		}
		return MatchBoxStyle.Render(fmt.Sprintf("  %s Cargando tabla de posiciones...", m.spinner.View()))
	}

	if len(m.standings.Entries) == 0 {
		return MatchBoxStyle.Render(EmptyStateStyle.Render("Tabla de posiciones no disponible para esta competición."))
	}

	var lines []string
	lines = append(lines, CategoryHeaderStyle.Render("TABLA DE POSICIONES · "+curr.Name))
	lines = append(lines, "")

	// Table header: #  Equipo               PJ  G   E   P   DG  PTS
	hdr := fmt.Sprintf(" %-2s  %-22s  %3s %3s %3s %3s %4s  %3s", "#", "Equipo", "PJ", "G", "E", "P", "DG", "PTS")
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

	for _, e := range m.standings.Entries {
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
	lines = append(lines, CategoryHeaderStyle.Render("MÁXIMOS GOLEADORES"))
	lines = append(lines, "")

	if len(m.statistics.Scorers) == 0 {
		lines = append(lines, EmptyStateStyle.Render("Estadísticas de goles no disponibles aún."))
	} else {
		hdr := fmt.Sprintf(" %-2s  %-24s  %-18s  %4s", "#", "Jugador", "Equipo", "Goles")
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

		limit := 10
		if len(m.statistics.Scorers) < limit {
			limit = len(m.statistics.Scorers)
		}
		for i := 0; i < limit; i++ {
			s := m.statistics.Scorers[i]
			row := fmt.Sprintf(" %-2d  %-24s  %-18s  %4s", s.Rank, truncateModalText(s.PlayerName, 24), truncateModalText(s.TeamName, 18), s.Value)
			if i < 3 {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorHighlight).Bold(true).Render(row))
			} else {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(row))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, CategoryHeaderStyle.Render("MÁXIMOS ASISTENTES"))
	lines = append(lines, "")

	if len(m.statistics.Assists) == 0 {
		lines = append(lines, EmptyStateStyle.Render("Estadísticas de asistencias no disponibles aún."))
	} else {
		hdr := fmt.Sprintf(" %-2s  %-24s  %-18s  %4s", "#", "Jugador", "Equipo", "Asist.")
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorCategory).Bold(true).Render(hdr))
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorBorder).Render("────────────────────────────────────────────────────────────"))

		limit := 10
		if len(m.statistics.Assists) < limit {
			limit = len(m.statistics.Assists)
		}
		for i := 0; i < limit; i++ {
			a := m.statistics.Assists[i]
			row := fmt.Sprintf(" %-2d  %-24s  %-18s  %4s", a.Rank, truncateModalText(a.PlayerName, 24), truncateModalText(a.TeamName, 18), a.Value)
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

func renderChannelModal(match model.Match, channels []model.Channel, selectedIdx int, modalTab int, livePulse bool, spinnerStr string, noEmojiOpt ...bool) string {
	noEmoji := len(noEmojiOpt) > 0 && noEmojiOpt[0]
	const modalContentWidth = 60
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
		if match.Stats == nil {
			loadingText := "Consultando estadísticas del partido..."
			if spinnerStr != "" {
				loadingText = fmt.Sprintf("%s  Consultando estadísticas del partido...", spinnerStr)
			}
			modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Render(loadingText) + "\n")
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
		if match.Lineups == nil || (len(match.Lineups.Home.Starters) == 0 && len(match.Lineups.Away.Starters) == 0) {
			notice := "Alineaciones no disponibles aún para este encuentro."
			modal.WriteString(lipgloss.NewStyle().Width(modalContentWidth).Align(lipgloss.Center).Foreground(ColorMuted).Italic(true).Render(notice) + "\n")
		} else {
			homeFormationText := homeName
			if match.Lineups.Home.Formation != "" {
				homeFormationText = fmt.Sprintf("%s (%s)", homeName, match.Lineups.Home.Formation)
			}
			awayFormationText := awayName
			if match.Lineups.Away.Formation != "" {
				awayFormationText = fmt.Sprintf("%s (%s)", awayName, match.Lineups.Away.Formation)
			}

			headerLeft := lipgloss.NewStyle().Width(28).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true).Render(truncateModalText(homeFormationText, 28))
			headerRight := lipgloss.NewStyle().Width(28).Align(lipgloss.Left).Foreground(ColorHighlight).Bold(true).Render(truncateModalText(awayFormationText, 28))
			modal.WriteString(fmt.Sprintf("%s    %s\n\n", headerLeft, headerRight))

			homeStarters := match.Lineups.Home.Starters
			awayStarters := match.Lineups.Away.Starters
			maxStarters := len(homeStarters)
			if len(awayStarters) > maxStarters {
				maxStarters = len(awayStarters)
			}
			if maxStarters > 11 {
				maxStarters = 11
			}

			formatPlayerItem := func(p model.Player) string {
				jerseyStr := "   "
				if p.Jersey != "" {
					jerseyStr = fmt.Sprintf("%-2s ", p.Jersey)
				}
				posStr := ""
				if p.Position != "" {
					posStr = fmt.Sprintf(" (%s)", p.Position)
				}
				nameStr := p.DisplayName
				if nameStr == "" {
					nameStr = "Jugador"
				}
				return fmt.Sprintf("%s%s%s", jerseyStr, nameStr, posStr)
			}

			colLeft := lipgloss.NewStyle().Width(28).Align(lipgloss.Left).Foreground(ColorTextMuted)
			colRight := lipgloss.NewStyle().Width(28).Align(lipgloss.Left).Foreground(ColorTextMuted)

			for i := 0; i < maxStarters; i++ {
				hText := ""
				if i < len(homeStarters) {
					hText = formatPlayerItem(homeStarters[i])
				}
				aText := ""
				if i < len(awayStarters) {
					aText = formatPlayerItem(awayStarters[i])
				}
				modal.WriteString(fmt.Sprintf("%s    %s\n", colLeft.Render(truncateModalText(hText, 28)), colRight.Render(truncateModalText(aText, 28))))
			}

			// Coach / DT display
			homeCoach := match.HomeTeamCoach()
			awayCoach := match.AwayTeamCoach()
			if homeCoach == "" && match.Lineups != nil {
				homeCoach = match.Lineups.HomeCoach
				if homeCoach == "" {
					homeCoach = match.Lineups.Home.Coach
				}
			}
			if awayCoach == "" && match.Lineups != nil {
				awayCoach = match.Lineups.AwayCoach
				if awayCoach == "" {
					awayCoach = match.Lineups.Away.Coach
				}
			}
			if homeCoach != "" || awayCoach != "" {
				modal.WriteString("\n")
				hCoachText := ""
				if homeCoach != "" {
					hCoachText = "👔 DT: " + homeCoach
					if noEmoji {
						hCoachText = "DT: " + homeCoach
					}
				}
				aCoachText := ""
				if awayCoach != "" {
					aCoachText = "👔 DT: " + awayCoach
					if noEmoji {
						aCoachText = "DT: " + awayCoach
					}
				}
				coachStyle := lipgloss.NewStyle().Width(28).Align(lipgloss.Left).Foreground(ColorCategory).Italic(true)
				modal.WriteString(fmt.Sprintf("%s    %s\n", coachStyle.Render(truncateModalText(hCoachText, 28)), coachStyle.Render(truncateModalText(aCoachText, 28))))
			}
		}
	}

	return modal.String()
}

// renderSmoothBar creates a fractional bar fill using 1/8th blocks (▏▎▍▌▋▊▉█).
func renderSmoothBar(fraction float64, totalSlots int) string {
	if fraction <= 0 {
		return strings.Repeat(" ", totalSlots)
	}
	if fraction >= 1 {
		return strings.Repeat("█", totalSlots)
	}
	totalUnits := fraction * float64(totalSlots*8)
	fullBlocks := int(totalUnits) / 8
	rem := int(totalUnits) % 8

	var b strings.Builder
	if fullBlocks > totalSlots {
		fullBlocks = totalSlots
	}
	b.WriteString(strings.Repeat("█", fullBlocks))

	if fullBlocks < totalSlots && rem > 0 {
		fractions := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
		b.WriteString(fractions[rem])
		fullBlocks++
	}
	remaining := totalSlots - fullBlocks
	if remaining > 0 {
		b.WriteString(strings.Repeat(" ", remaining))
	}
	return b.String()
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
