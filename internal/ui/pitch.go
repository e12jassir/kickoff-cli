package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

var knownShortNames = map[string]string{
	"LEWANDOWSKI": "LEW",
	"YAMAL":       "YAM",
	"TER STEGEN":  "STEG",
	"STEGEN":      "STEG",
	"PEDRI":       "PED",
	"VINÍCIUS":    "VINI",
	"VINICIUS":    "VINI",
	"MBAPPÉ":      "MBAP",
	"MBAPPE":      "MBAP",
	"RÜDIGER":     "RÜD",
	"RUDIGER":     "RÜD",
	"MODRIĆ":      "MOD",
	"MODRIC":      "MOD",
	"COURTOIS":    "COUR",
	"BELLINGHAM":  "BEL",
	"VALVERDE":    "VAL",
	"CARVAJAL":    "CARV",
	"MILITÃO":     "MIL",
	"MILITAO":     "MIL",
	"TCHOUAMÉNI":  "TCH",
	"TCHOUAMENI":  "TCH",
	"RODRYGO":     "ROD",
	"KOUNDÉ":      "KOU",
	"KOUNDE":      "KOU",
	"ARAÚJO":      "ARA",
	"ARAUJO":      "ARA",
	"CUBARSÍ":     "CUB",
	"CUBARSI":     "CUB",
	"BALDE":       "BAL",
	"DE JONG":     "JONG",
	"GAVI":        "GAVI",
	"RAPHINHA":    "RAPH",
	"HAALAND":     "HAAL",
	"DE BRUYNE":   "KDB",
	"SALAH":       "SAL",
	"MESSI":       "MES",
	"RONALDO":     "CR7",
}

// GenerateShortName returns a 3-5 uppercase letter short name for display on pitch mini-cards.
func GenerateShortName(fullName string) string {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "JUG"
	}

	clean := fullName
	for _, suffix := range []string{" Jr.", " Jr", " II", " III", " Sr.", " Sr"} {
		if strings.HasSuffix(strings.ToLower(clean), strings.ToLower(suffix)) {
			clean = strings.TrimSpace(clean[:len(clean)-len(suffix)])
		}
	}

	upper := strings.ToUpper(clean)
	words := strings.Fields(upper)
	if len(words) == 0 {
		return "JUG"
	}

	// Multi-word check (e.g. TER STEGEN)
	if len(words) > 1 {
		lastTwo := words[len(words)-2] + " " + words[len(words)-1]
		if alias, ok := knownShortNames[lastTwo]; ok {
			return alias
		}
	}

	// Word-by-word reverse check
	for i := len(words) - 1; i >= 0; i-- {
		w := strings.Trim(words[i], ".")
		if alias, ok := knownShortNames[w]; ok {
			return alias
		}
	}

	// Fallback: extract surname or significant word
	target := words[len(words)-1]
	if len(words) > 1 && len([]rune(strings.Trim(target, "."))) <= 2 {
		target = words[0]
	}

	runes := []rune(strings.Trim(target, "."))
	if len(runes) >= 4 {
		return string(runes[:4])
	}
	if len(runes) == 3 {
		return string(runes)
	}
	if len(runes) == 2 {
		return string(runes)
	}
	return upper
}

// ParseFormation parses formation strings like "4-3-3", "4-2-3-1", "3-5-2", "4-4-2", "5-3-2", "3-4-3".
// Returns the counts of outfield lines (e.g. [4, 3, 3] for "4-3-3") and true if recognized.
func ParseFormation(f string) ([]int, bool) {
	f = strings.TrimSpace(f)
	if f == "" {
		return nil, false
	}
	// Normalize dashes
	f = strings.ReplaceAll(f, "‐", "-")
	f = strings.ReplaceAll(f, "–", "-")
	f = strings.ReplaceAll(f, "—", "-")

	tokens := strings.Split(f, "-")
	if len(tokens) < 2 {
		return nil, false
	}

	var nums []int
	sum := 0
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		n, err := strconv.Atoi(tok)
		if err != nil || n <= 0 {
			return nil, false
		}
		nums = append(nums, n)
		sum += n
	}

	// If sum == 10, outfield players without GK
	if sum == 10 {
		return nums, true
	}
	// If sum == 11 and starts with 1 (e.g. 1-4-3-3), strip GK
	if sum == 11 && nums[0] == 1 {
		return nums[1:], true
	}

	return nums, true
}

// matchPlayerEvent checks if a player matches a match event's player name.
func matchPlayerEvent(playerName, eventPlayerName string) bool {
	p := strings.ToLower(strings.TrimSpace(playerName))
	e := strings.ToLower(strings.TrimSpace(eventPlayerName))
	if p == "" || e == "" {
		return false
	}
	if p == e {
		return true
	}
	for _, suf := range []string{" jr.", " jr", " sr.", " sr"} {
		p = strings.TrimSuffix(p, suf)
		e = strings.TrimSuffix(e, suf)
	}
	pWords := strings.Fields(p)
	eWords := strings.Fields(e)
	if len(pWords) > 0 && len(eWords) > 0 {
		pLast := strings.Trim(pWords[len(pWords)-1], ".")
		eLast := strings.Trim(eWords[len(eWords)-1], ".")
		if pLast == eLast && len(pLast) >= 3 {
			return true
		}
	}
	return strings.Contains(p, e) || strings.Contains(e, p)
}

// PlayerEventBadge returns event badge string for a player:
// Goal: ⚽ (or G), Yellow: 🟨 (or A), Red: 🟥 (or R), Penalty Goal: ⚽P (or GP).
func PlayerEventBadge(player model.Player, events []model.MatchEvent, noEmoji bool) string {
	hasRed := false
	hasPenaltyGoal := false
	hasGoal := false
	hasYellow := false

	for _, ev := range events {
		if !matchPlayerEvent(player.DisplayName, ev.PlayerName) {
			continue
		}
		t := strings.ToLower(ev.Type)
		if ev.IsRedCard || strings.Contains(t, "red") {
			hasRed = true
		}
		if (ev.IsPenalty && ev.IsScoring) || (ev.IsScoring && strings.Contains(t, "pen")) {
			hasPenaltyGoal = true
		} else if ev.IsScoring || strings.Contains(t, "goal") {
			hasGoal = true
		}
		if ev.IsYellow || strings.Contains(t, "yellow") {
			hasYellow = true
		}
	}

	if hasRed {
		if noEmoji {
			return "R"
		}
		return "🟥"
	}
	if hasPenaltyGoal {
		if noEmoji {
			return "GP"
		}
		return "⚽P"
	}
	if hasGoal {
		if noEmoji {
			return "G"
		}
		return "⚽"
	}
	if hasYellow {
		if noEmoji {
			return "A"
		}
		return "🟨"
	}
	return ""
}

// RenderMiniCard formats a player mini-card of width 8-10 chars.
// Returns 3 lines: top border, inner content, bottom border.
func RenderMiniCard(p model.Player, badge string, cardWidth int, noEmoji bool) []string {
	if cardWidth < 8 {
		cardWidth = 8
	}
	if cardWidth > 10 {
		cardWidth = 10
	}
	innerWidth := cardWidth - 2

	dashes := strings.Repeat("─", innerWidth)
	top := "┌" + dashes + "┐"
	bottom := "└" + dashes + "┘"

	jerseyStr := "#"
	if p.Jersey != "" {
		jerseyStr = "#" + p.Jersey
	}
	shortName := GenerateShortName(p.DisplayName)

	bw := lipgloss.Width(badge)
	rem := innerWidth - bw
	if rem < 0 {
		rem = 0
		badge = ""
		bw = 0
	}

	var innerText string
	lead := jerseyStr + " " + shortName
	if lipgloss.Width(lead) <= rem {
		padding := strings.Repeat(" ", rem-lipgloss.Width(lead))
		innerText = lead + padding + badge
	} else {
		leadNoSpace := jerseyStr + shortName
		if lipgloss.Width(leadNoSpace) <= rem {
			padding := strings.Repeat(" ", rem-lipgloss.Width(leadNoSpace))
			innerText = leadNoSpace + padding + badge
		} else {
			// Shorten name to fit
			availForName := rem - lipgloss.Width(jerseyStr)
			if availForName > 0 {
				r := []rune(shortName)
				if len(r) > availForName {
					r = r[:availForName]
				}
				leadTrunc := jerseyStr + string(r)
				padding := strings.Repeat(" ", rem-lipgloss.Width(leadTrunc))
				innerText = leadTrunc + padding + badge
			} else {
				padding := strings.Repeat(" ", rem)
				innerText = padding + badge
			}
		}
	}

	// Pad if needed
	for lipgloss.Width(innerText) < innerWidth {
		innerText += " "
	}

	// Style inner line
	middle := "│" + innerText + "│"
	return []string{top, middle, bottom}
}

// groupStartersByPosition groups starters into GK, DF, MF, FW slices.
func groupStartersByPosition(starters []model.Player) (gks, dfs, mfs, fws []model.Player) {
	var others []model.Player
	hasAnyPos := false
	for _, p := range starters {
		pos := strings.ToUpper(strings.TrimSpace(p.Position))
		switch {
		case strings.HasPrefix(pos, "G"):
			gks = append(gks, p)
			hasAnyPos = true
		case strings.HasPrefix(pos, "D") || pos == "CB" || pos == "LB" || pos == "RB" || pos == "LWB" || pos == "RWB":
			dfs = append(dfs, p)
			hasAnyPos = true
		case strings.HasPrefix(pos, "M") || pos == "CM" || pos == "CDM" || pos == "CAM" || pos == "LM" || pos == "RM":
			mfs = append(mfs, p)
			hasAnyPos = true
		case strings.HasPrefix(pos, "F") || strings.HasPrefix(pos, "A") || pos == "ST" || pos == "CF" || pos == "LW" || pos == "RW":
			fws = append(fws, p)
			hasAnyPos = true
		default:
			others = append(others, p)
		}
	}

	if !hasAnyPos {
		return nil, nil, nil, nil
	}

	// If no GK identified and others exist, take first
	if len(gks) == 0 && len(others) > 0 {
		gks = append(gks, others[0])
		others = others[1:]
	}
	// Distribute unassigned players
	for _, o := range others {
		if len(dfs) < 4 {
			dfs = append(dfs, o)
		} else if len(mfs) < 3 {
			mfs = append(mfs, o)
		} else {
			fws = append(fws, o)
		}
	}
	return
}

// buildPitchRows builds the tactical pitch lines from starters and formation.
// Rows are ordered from TOP to BOTTOM: Forwards at top, Midfielders, Center Circle, Defenders, GK at bottom.
func buildPitchRows(lineup model.TeamLineup, events []model.MatchEvent, interiorWidth int, noEmoji bool) (lines [][]model.Player, formationLabel string) {
	starters := lineup.Starters
	if len(starters) == 0 {
		return nil, "4-3-3"
	}

	outfieldCounts, ok := ParseFormation(lineup.Formation)
	if ok && len(outfieldCounts) >= 2 {
		formationLabel = lineup.Formation
		// Partition starters: GK (1) + outfield lines
		lines = append(lines, []model.Player{starters[0]})
		idx := 1
		for _, cnt := range outfieldCounts {
			end := idx + cnt
			if end > len(starters) {
				end = len(starters)
			}
			if idx < len(starters) {
				lines = append(lines, starters[idx:end])
			} else {
				lines = append(lines, nil)
			}
			idx = end
		}
		// If extra players exist, append to last line
		if idx < len(starters) && len(lines) > 0 {
			lines[len(lines)-1] = append(lines[len(lines)-1], starters[idx:]...)
		}
		return lines, formationLabel
	}

	// Unrecognized or empty formation: group by position
	gks, dfs, mfs, fws := groupStartersByPosition(starters)
	if len(gks) > 0 || len(dfs) > 0 || len(mfs) > 0 || len(fws) > 0 {
		formationLabel = fmt.Sprintf("%d-%d-%d", len(dfs), len(mfs), len(fws))
		if len(dfs) == 0 && len(mfs) == 0 && len(fws) == 0 {
			formationLabel = "4-3-3"
		}
		lines = [][]model.Player{gks, dfs, mfs, fws}
		return lines, formationLabel
	}

	// Default 4-3-3 fallback
	formationLabel = "4-3-3"
	gk := starters[:1]
	dEnd := min(5, len(starters))
	mEnd := min(8, len(starters))
	df := starters[1:dEnd]
	mf := starters[dEnd:mEnd]
	fw := starters[mEnd:]
	lines = [][]model.Player{gk, df, mf, fw}
	return lines, formationLabel
}

// renderPitchBox renders the pitch display box.
func renderPitchBox(teamName, formation string, pitchLines [][]model.Player, events []model.MatchEvent, pitchWidth int, noEmoji bool) string {
	interiorWidth := pitchWidth - 2
	if interiorWidth < 20 {
		interiorWidth = 20
		pitchWidth = 22
	}

	var sb strings.Builder

	// Top line: ┌─── PIZARRA TÁCTICA: <Equipo> (<Formación>) ───┐
	title := fmt.Sprintf(" PIZARRA TÁCTICA: %s (%s) ", teamName, formation)
	maxTitleLen := interiorWidth - 4
	if lipgloss.Width(title) > maxTitleLen {
		availName := maxTitleLen - lipgloss.Width(fmt.Sprintf(" PIZARRA TÁCTICA:  (%s) ", formation))
		if availName > 4 {
			teamName = truncateModalText(teamName, availName)
			title = fmt.Sprintf(" PIZARRA TÁCTICA: %s (%s) ", teamName, formation)
		} else {
			title = fmt.Sprintf(" TÁCTICA: %s ", formation)
		}
	}
	leftDashes := 3
	remDashes := interiorWidth - lipgloss.Width(title) - leftDashes
	if remDashes < 1 {
		remDashes = 1
	}
	topLine := "┌" + strings.Repeat("─", leftDashes) + title + strings.Repeat("─", remDashes) + "┐"
	sb.WriteString(topLine + "\n")

	if len(pitchLines) == 0 {
		emptyLine := "│" + strings.Repeat(" ", interiorWidth) + "│\n"
		sb.WriteString(emptyLine)
		notice := "Alineaciones no disponibles aún para este encuentro."
		sb.WriteString("│" + lipgloss.NewStyle().Width(interiorWidth).Align(lipgloss.Center).Foreground(ColorMuted).Italic(true).Render(notice) + "│\n")
		sb.WriteString(emptyLine)
		sb.WriteString("└" + strings.Repeat("─", interiorWidth) + "┘")
		return sb.String()
	}

	// Reverse lines so Front/Attack is at TOP and GK is at BOTTOM
	// pitchLines has [GK, Line1, Line2, ..., LineN]
	// displayRows will have [LineN (FW), ..., Line1 (DF), GK]
	displayRows := make([][]model.Player, len(pitchLines))
	for i := range pitchLines {
		displayRows[i] = pitchLines[len(pitchLines)-1-i]
	}

	// Center circle index: between attack/upper midfield and defense/lower midfield
	midIdx := len(displayRows) / 2

	for rowIdx, players := range displayRows {
		if rowIdx == midIdx {
			// Center circle line: │──────────────(   ●   )──────────────│
			circleStr := "(   ●   )"
			if noEmoji {
				circleStr = "(   o   )"
			}
			cw := lipgloss.Width(circleStr)
			dashL := (interiorWidth - cw) / 2
			dashR := interiorWidth - cw - dashL
			if dashL < 1 {
				dashL = 1
			}
			if dashR < 1 {
				dashR = 1
			}
			circleLine := "│" + strings.Repeat("─", dashL) + circleStr + strings.Repeat("─", dashR) + "│"
			sb.WriteString(circleLine + "\n")
		}

		if len(players) == 0 {
			continue
		}

		// Choose mini-card width: 10 chars, or 8 if 5+ players
		cardWidth := 10
		if len(players)*10+(len(players)-1) > interiorWidth {
			cardWidth = 8
		}

		var cardCols [][]string
		for _, p := range players {
			badge := PlayerEventBadge(p, events, noEmoji)
			cardCols = append(cardCols, RenderMiniCard(p, badge, cardWidth, noEmoji))
		}

		// Horizontal gap between cards
		gap := 1
		totalRowWidth := len(players)*cardWidth + (len(players)-1)*gap
		if totalRowWidth > interiorWidth {
			gap = 0
			totalRowWidth = len(players) * cardWidth
		}
		leftPad := (interiorWidth - totalRowWidth) / 2
		rightPad := interiorWidth - totalRowWidth - leftPad
		if leftPad < 0 {
			leftPad = 0
		}
		if rightPad < 0 {
			rightPad = 0
		}

		// Render the 3 lines of the mini-cards row
		for lineIdx := 0; lineIdx < 3; lineIdx++ {
			var rowPieces []string
			for _, cardLines := range cardCols {
				rowPieces = append(rowPieces, cardLines[lineIdx])
			}
			joinedCards := strings.Join(rowPieces, strings.Repeat(" ", gap))
			line := "│" + strings.Repeat(" ", leftPad) + joinedCards + strings.Repeat(" ", rightPad) + "│"
			sb.WriteString(line + "\n")
		}
	}

	// Bottom line: └──────────────────────────────────────────────┘
	bottomLine := "└" + strings.Repeat("─", interiorWidth) + "┘"
	sb.WriteString(bottomLine)

	return sb.String()
}

// renderBenchBox renders the "SUPLENTES Y DETALLES" panel.
func renderBenchBox(match model.Match, lineup model.TeamLineup, events []model.MatchEvent, team model.Competitor, benchWidth int, noEmoji bool) string {
	interiorWidth := benchWidth - 2
	if interiorWidth < 18 {
		interiorWidth = 18
		benchWidth = 20
	}

	var sb strings.Builder

	// Top border
	title := " SUPLENTES Y DETALLES "
	if lipgloss.Width(title) > interiorWidth-2 {
		title = " DETALLES "
	}
	leftDashes := 2
	remDashes := interiorWidth - lipgloss.Width(title) - leftDashes
	if remDashes < 1 {
		remDashes = 1
	}
	sb.WriteString("┌" + strings.Repeat("─", leftDashes) + title + strings.Repeat("─", remDashes) + "┐\n")

	renderInteriorLine := func(text string, style lipgloss.Style) {
		trunc := truncateModalText(text, interiorWidth-2)
		styled := style.Width(interiorWidth - 2).Render(trunc)
		line := "│ " + styled + " │"
		sb.WriteString(line + "\n")
	}

	// Venue & Referee
	if match.Venue != "" {
		vText := "🏟️ " + match.Venue
		if noEmoji {
			vText = "Estadio: " + match.Venue
		}
		renderInteriorLine(vText, VenueStyle)
	}
	if match.Referee != "" {
		rText := "👤 " + match.Referee
		if noEmoji {
			rText = "Árbitro: " + match.Referee
		}
		renderInteriorLine(rText, lipgloss.NewStyle().Foreground(ColorMuted))
	}

	// Key Match Events for this team
	var teamEvents []model.MatchEvent
	for _, ev := range events {
		if ev.TeamID != "" && team.ID != "" {
			if ev.TeamID == team.ID {
				teamEvents = append(teamEvents, ev)
				continue
			}
		}
		// Match by player name in starters or bench
		matched := false
		for _, p := range lineup.Starters {
			if matchPlayerEvent(p.DisplayName, ev.PlayerName) {
				matched = true
				break
			}
		}
		if !matched {
			for _, p := range lineup.Bench {
				if matchPlayerEvent(p.DisplayName, ev.PlayerName) {
					matched = true
					break
				}
			}
		}
		if matched {
			teamEvents = append(teamEvents, ev)
		}
	}

	if len(teamEvents) > 0 {
		renderInteriorLine("EVENTOS:", CategoryHeaderStyle)
		for _, ev := range teamEvents {
			evBadge := "⚽"
			if ev.IsRedCard || strings.Contains(strings.ToLower(ev.Type), "red") {
				evBadge = "🟥"
				if noEmoji {
					evBadge = "[R]"
				}
			} else if (ev.IsPenalty && ev.IsScoring) || (ev.IsScoring && strings.Contains(strings.ToLower(ev.Type), "pen")) {
				evBadge = "⚽P"
				if noEmoji {
					evBadge = "[GP]"
				}
			} else if ev.IsScoring || strings.Contains(strings.ToLower(ev.Type), "goal") {
				evBadge = "⚽"
				if noEmoji {
					evBadge = "[G]"
				}
			} else if ev.IsYellow || strings.Contains(strings.ToLower(ev.Type), "yellow") {
				evBadge = "🟨"
				if noEmoji {
					evBadge = "[A]"
				}
			}
			clk := ev.Clock
			if clk == "" && ev.ClockValue > 0 {
				clk = fmt.Sprintf("%.0f'", ev.ClockValue/60)
			}
			evText := fmt.Sprintf("%s %s %s", evBadge, clk, ev.PlayerName)
			renderInteriorLine(evText, lipgloss.NewStyle().Foreground(ColorHighlight))
		}
	}

	// Bench (Substitutes)
	renderInteriorLine("SUPLENTES:", CategoryHeaderStyle)
	if len(lineup.Bench) == 0 {
		renderInteriorLine("Sin suplentes", lipgloss.NewStyle().Foreground(ColorMuted).Italic(true))
	} else {
		for _, b := range lineup.Bench {
			jStr := ""
			if b.Jersey != "" {
				jStr = "#" + b.Jersey + " "
			}
			name := b.DisplayName
			if name == "" {
				name = "Jugador"
			}
			benchText := jStr + name
			renderInteriorLine(benchText, lipgloss.NewStyle().Foreground(ColorTextMuted))
		}
	}

	// Bottom border
	sb.WriteString("└" + strings.Repeat("─", interiorWidth) + "┘")

	return sb.String()
}

// RenderTacticalPitch renders a tactical football pitch with modular mini-cards and bench details.
// teamIdx: 0 for Home, 1 for Away.
func RenderTacticalPitch(match model.Match, teamIdx int, width int, noEmoji bool) string {
	if teamIdx != 1 {
		teamIdx = 0
	}

	homeName := match.HomeTeam.DisplayName
	if homeName == "" {
		homeName = match.HomeTeam.Name
	}
	if homeName == "" {
		homeName = "Local"
	}

	awayName := match.AwayTeam.DisplayName
	if awayName == "" {
		awayName = match.AwayTeam.Name
	}
	if awayName == "" {
		awayName = "Visitante"
	}

	var homeLineup, awayLineup model.TeamLineup
	if match.Lineups != nil {
		homeLineup = match.Lineups.Home
		awayLineup = match.Lineups.Away
	}

	homeForm := homeLineup.Formation
	if homeForm == "" {
		_, homeForm = buildPitchRows(homeLineup, match.Events, 48, noEmoji)
	}
	awayForm := awayLineup.Formation
	if awayForm == "" {
		_, awayForm = buildPitchRows(awayLineup, match.Events, 48, noEmoji)
	}

	// 1. Team Selector Pills
	homePillText := fmt.Sprintf("%s (%s)", homeName, homeForm)
	awayPillText := fmt.Sprintf("%s (%s)", awayName, awayForm)

	var homePill, awayPill string
	if teamIdx == 0 {
		homePill = ActiveTabPillStyle.Render("[ ● " + homePillText + " ]")
		awayPill = InactiveTabPillStyle.Render("○ " + awayPillText)
	} else {
		homePill = InactiveTabPillStyle.Render("○ " + homePillText)
		awayPill = ActiveTabPillStyle.Render("[ ● " + awayPillText + " ]")
	}

	pillsRow := lipgloss.JoinHorizontal(lipgloss.Center, homePill, "   ", awayPill)
	hint := lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render("(Tab o ←/→ para alternar)")
	topSection := lipgloss.JoinVertical(lipgloss.Center, pillsRow, hint)

	// If lineups are nil or both teams have no starters, show fallback notice
	if match.Lineups == nil || (len(homeLineup.Starters) == 0 && len(awayLineup.Starters) == 0) {
		notice := "Alineaciones no disponibles aún para este encuentro."
		noticeStyled := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Foreground(ColorMuted).Italic(true).Render(notice)
		return lipgloss.JoinVertical(lipgloss.Center, topSection, "", noticeStyled)
	}

	// Determine active team data
	var activeTeam model.Competitor
	var activeLineup model.TeamLineup
	var activeName, activeFormation string
	if teamIdx == 0 {
		activeTeam = match.HomeTeam
		activeLineup = homeLineup
		activeName = homeName
		activeFormation = homeForm
	} else {
		activeTeam = match.AwayTeam
		activeLineup = awayLineup
		activeName = awayName
		activeFormation = awayForm
	}

	// 2. Responsive Layout: Side-by-Side (width >= 72) vs Stacked (width < 72)
	var body string
	if width >= 72 {
		pitchWidth := 46
		benchWidth := width - pitchWidth - 1
		if benchWidth > 32 {
			benchWidth = 32
			pitchWidth = width - benchWidth - 1
		}

		pitchLines, derivedForm := buildPitchRows(activeLineup, match.Events, pitchWidth-2, noEmoji)
		if activeFormation == "" {
			activeFormation = derivedForm
		}

		pitchBox := renderPitchBox(activeName, activeFormation, pitchLines, match.Events, pitchWidth, noEmoji)
		benchBox := renderBenchBox(match, activeLineup, match.Events, activeTeam, benchWidth, noEmoji)

		// Harmonize height so both boxes line up cleanly
		pLines := strings.Split(pitchBox, "\n")
		bLines := strings.Split(benchBox, "\n")
		if len(pLines) > len(bLines) {
			diff := len(pLines) - len(bLines)
			emptyBenchLine := "│" + strings.Repeat(" ", benchWidth-2) + "│"
			// Insert empty lines right before bottom border
			newBLines := append([]string{}, bLines[:len(bLines)-1]...)
			for k := 0; k < diff; k++ {
				newBLines = append(newBLines, emptyBenchLine)
			}
			newBLines = append(newBLines, bLines[len(bLines)-1])
			benchBox = strings.Join(newBLines, "\n")
		} else if len(bLines) > len(pLines) {
			diff := len(bLines) - len(pLines)
			emptyPitchLine := "│" + strings.Repeat(" ", pitchWidth-2) + "│"
			newPLines := append([]string{}, pLines[:len(pLines)-1]...)
			for k := 0; k < diff; k++ {
				newPLines = append(newPLines, emptyPitchLine)
			}
			newPLines = append(newPLines, pLines[len(pLines)-1])
			pitchBox = strings.Join(newPLines, "\n")
		}

		body = lipgloss.JoinHorizontal(lipgloss.Top, pitchBox, " ", benchBox)
	} else {
		// Stacked layout
		pitchWidth := width
		if pitchWidth > 64 {
			pitchWidth = 64
		}
		if pitchWidth < 40 {
			pitchWidth = 40
		}
		benchWidth := pitchWidth

		pitchLines, derivedForm := buildPitchRows(activeLineup, match.Events, pitchWidth-2, noEmoji)
		if activeFormation == "" {
			activeFormation = derivedForm
		}

		pitchBox := renderPitchBox(activeName, activeFormation, pitchLines, match.Events, pitchWidth, noEmoji)
		benchBox := renderBenchBox(match, activeLineup, match.Events, activeTeam, benchWidth, noEmoji)

		body = lipgloss.JoinVertical(lipgloss.Left, pitchBox, "", benchBox)
	}

	return lipgloss.JoinVertical(lipgloss.Center, topSection, "", body)
}
