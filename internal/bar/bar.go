package bar

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// Format represents an output target format for the status bar.
type Format string

const (
	FormatPlain    Format = "plain"
	FormatStarship Format = "starship"
	FormatPrompt   Format = "prompt" // alias for starship
	FormatTmux     Format = "tmux"
	FormatWaybar   Format = "waybar"
	FormatJSON     Format = "json"
)

// Options specifies configuration for formatting and filtering fixtures.
type Options struct {
	Format  Format
	League  string
	Team    string
	NoEmoji bool
}

// WaybarOutput represents the Waybar JSON communication protocol.
type WaybarOutput struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
	Alt     string `json:"alt"`
}

// JSONMatch represents a match entry in the structured JSON output.
type JSONMatch struct {
	ID        string   `json:"id"`
	League    string   `json:"league"`
	Home      string   `json:"home"`
	Away      string   `json:"away"`
	HomeScore string   `json:"homeScore"`
	AwayScore string   `json:"awayScore"`
	Status    string   `json:"status"`
	State     string   `json:"state"`
	Venue     string   `json:"venue"`
	Channels  []string `json:"channels,omitempty"`
	Formatted string   `json:"formatted"`
}

// JSONOutput represents structured status output for raycast, xbar, and scripting.
type JSONOutput struct {
	Live    bool        `json:"live"`
	Count   int         `json:"count"`
	Matches []JSONMatch `json:"matches"`
}

var knownTeamCodes = map[string]string{
	"real madrid":         "RMA",
	"fc barcelona":        "BAR",
	"barcelona":           "BAR",
	"atlético madrid":     "ATM",
	"atletico madrid":     "ATM",
	"atlético de madrid":  "ATM",
	"athletic club":       "ATH",
	"athletic bilbao":     "ATH",
	"sevilla":             "SEV",
	"sevilla fc":          "SEV",
	"real betis":          "BET",
	"betis":               "BET",
	"real sociedad":       "RSO",
	"villarreal":          "VIL",
	"villarreal cf":       "VIL",
	"valencia":            "VAL",
	"valencia cf":         "VAL",
	"celta de vigo":       "CEL",
	"celta vigo":          "CEL",
	"celta":               "CEL",
	"girona":              "GIR",
	"girona fc":           "GIR",
	"rcd mallorca":        "MLL",
	"mallorca":            "MLL",
	"osasuna":             "OSA",
	"ca osasuna":          "OSA",
	"getafe":              "GET",
	"getafe cf":           "GET",
	"rayo vallecano":      "RAY",
	"rcd espanyol":        "ESP",
	"espanyol":            "ESP",
	"deportivo alavés":    "ALA",
	"alaves":              "ALA",
	"alavés":              "ALA",
	"las palmas":          "LPA",
	"ud las palmas":       "LPA",
	"cd leganés":          "LEG",
	"leganés":             "LEG",
	"real valladolid":     "VLL",
	"valladolid":          "VLL",
	"manchester city":     "MCI",
	"man city":            "MCI",
	"manchester united":   "MUN",
	"man united":          "MUN",
	"man utd":             "MUN",
	"liverpool":           "LIV",
	"liverpool fc":        "LIV",
	"arsenal":             "ARS",
	"arsenal fc":          "ARS",
	"chelsea":             "CHE",
	"chelsea fc":          "CHE",
	"tottenham hotspur":   "TOT",
	"tottenham":           "TOT",
	"spurs":               "TOT",
	"aston villa":         "AVL",
	"newcastle united":    "NEW",
	"newcastle":           "NEW",
	"west ham united":     "WHU",
	"west ham":            "WHU",
	"bayern münchen":      "BAY",
	"bayern munich":       "BAY",
	"fc bayern":           "BAY",
	"borussia dortmund":   "BVB",
	"dortmund":            "BVB",
	"bayer leverkusen":    "B04",
	"leverkusen":          "B04",
	"rb leipzig":          "RBL",
	"paris saint-germain": "PSG",
	"paris sg":            "PSG",
	"psg":                 "PSG",
	"olympique marseille": "OM",
	"marseille":           "OM",
	"as monaco":           "ASM",
	"monaco":              "ASM",
	"juventus":            "JUV",
	"inter milan":         "INT",
	"inter de milan":      "INT",
	"inter":               "INT",
	"internazionale":      "INT",
	"ac milan":            "MIL",
	"milan":               "MIL",
	"napoli":              "NAP",
	"ssc napoli":          "NAP",
	"roma":                "ROM",
	"as roma":             "ROM",
	"lazio":               "LAZ",
	"ss lazio":            "LAZ",
	"boca juniors":        "BOC",
	"river plate":         "RIV",
	"flamengo":            "FLA",
	"palmeiras":           "PAL",
}

// ShortTeam returns a concise 3-4 letter uppercase representation for a team.
func ShortTeam(c model.Competitor) string {
	if abbr := strings.TrimSpace(c.Abbreviation); abbr != "" {
		return strings.ToUpper(abbr)
	}

	name := strings.TrimSpace(c.DisplayName)
	if name == "" {
		name = strings.TrimSpace(c.Name)
	}
	if name == "" {
		return "UNK"
	}

	// If already a short uppercase acronym (e.g. RMA, BAR, MCI, PSG)
	if len(name) >= 2 && len(name) <= 4 && name == strings.ToUpper(name) && !strings.Contains(name, " ") {
		return name
	}

	norm := strings.ToLower(name)
	if code, ok := knownTeamCodes[norm]; ok {
		return code
	}

	// Try with c.Name if different
	if c.Name != "" && c.Name != c.DisplayName {
		normName := strings.ToLower(strings.TrimSpace(c.Name))
		if code, ok := knownTeamCodes[normName]; ok {
			return code
		}
		if len(c.Name) >= 2 && len(c.Name) <= 4 && c.Name == strings.ToUpper(c.Name) && !strings.Contains(c.Name, " ") {
			return c.Name
		}
	}

	// Remove common prefixes and clean words
	words := strings.Fields(name)
	var filtered []string
	prefixes := map[string]bool{
		"fc": true, "cf": true, "cd": true, "sc": true, "ac": true, "ud": true,
		"ca": true, "as": true, "ss": true, "rcd": true, "afc": true, "club": true,
		"deportivo": true, "real": true,
	}

	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,-"))
		if !prefixes[clean] && clean != "" {
			filtered = append(filtered, w)
		}
	}

	if len(filtered) == 0 {
		filtered = words
	}

	if len(filtered) >= 3 {
		res := ""
		for _, w := range filtered[:3] {
			r := []rune(w)
			if len(r) > 0 {
				res += string(r[0])
			}
		}
		return strings.ToUpper(res)
	}

	if len(filtered) == 2 {
		r0 := []rune(filtered[0])
		r1 := []rune(filtered[1])
		if len(r0) > 0 && len(r1) >= 2 {
			return strings.ToUpper(string(r0[0]) + string(r1[0:2]))
		}
		if len(r0) >= 2 && len(r1) > 0 {
			return strings.ToUpper(string(r0[0:2]) + string(r1[0]))
		}
	}

	runes := []rune(filtered[0])
	if len(runes) >= 3 {
		return strings.ToUpper(string(runes[:3]))
	}

	return strings.ToUpper(filtered[0])
}

// FormatClock extracts and formats the current match clock / minute or status.
func FormatClock(status model.MatchStatus) string {
	switch strings.ToUpper(strings.TrimSpace(status.Code)) {
	case "STATUS_HALFTIME":
		return "HT"
	case "STATUS_FULL_TIME", "STATUS_FINAL":
		return "FT"
	case "STATUS_SCHEDULED":
		// The not-yet-started rendering produced below for a pre-match.
		return "PRE"
	case "STATUS_IN_PROGRESS":
		if clock := formatDetailClock(status); clock != "" {
			return clock
		}
		return "LIVE"
	}

	if status.Completed || status.State == "post" {
		return "FT"
	}

	if clock := formatDetailClock(status); clock != "" {
		return clock
	}

	if status.State == "in" {
		return "LIVE"
	}
	return "PRE"
}

// formatDetailClock renders the feed's detail or description for a match that
// has already started, returning "" when the feed carries no usable text.
func formatDetailClock(status model.MatchStatus) string {
	detail := strings.TrimSpace(status.Detail)
	if detail != "" {
		if detail == "Halftime" || detail == "Half Time" || detail == "HT" {
			return "HT"
		}
		if isMinute(detail) {
			return detail + "'"
		}
		return detail
	}

	desc := strings.TrimSpace(status.Description)
	if desc != "" {
		if strings.EqualFold(desc, "halftime") || strings.EqualFold(desc, "half time") {
			return "HT"
		}
		return desc
	}

	return ""
}

// isMinute reports whether s is a bare minute number such as "63".
func isMinute(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// LeagueName returns human-readable competition name from a key or code.
func LeagueName(leagueKey string) string {
	for _, l := range model.DefaultLeagues {
		if strings.EqualFold(l.Key, leagueKey) || strings.EqualFold(l.Code, leagueKey) {
			return l.Name
		}
	}
	if leagueKey != "" {
		return leagueKey
	}
	return "Soccer"
}

// IsLive returns true if match is in progress.
func IsLive(m model.Match) bool {
	return m.Status.State == "in"
}

// IsUpcoming returns true if match has not started yet.
func IsUpcoming(m model.Match) bool {
	return m.Status.State == "pre" && !m.Status.Completed
}

// IsFinished returns true if match is finished.
func IsFinished(m model.Match) bool {
	return m.Status.Completed || m.Status.State == "post"
}

// FormatMatchSingle formats a single match for plain, starship, or tmux.
func FormatMatchSingle(m model.Match, format Format, noEmoji bool) string {
	icon := "⚽"
	if noEmoji {
		icon = "[*]"
	}

	home := ShortTeam(m.HomeTeam)
	away := ShortTeam(m.AwayTeam)
	homeScore := m.HomeTeam.Score
	if homeScore == "" {
		homeScore = "0"
	}
	awayScore := m.AwayTeam.Score
	if awayScore == "" {
		awayScore = "0"
	}

	switch format {
	case FormatStarship, FormatPrompt:
		if IsLive(m) {
			clock := FormatClock(m.Status)
			return fmt.Sprintf("%s %s %s-%s %s %s", icon, home, homeScore, awayScore, away, clock)
		}
		if IsUpcoming(m) {
			timeStr := m.TimeStr
			if timeStr == "" && !m.StartTime.IsZero() {
				timeStr = m.StartTime.Format("15:04")
			}
			if timeStr == "" {
				timeStr = "TBD"
			}
			return fmt.Sprintf("%s %s vs %s %s", icon, home, away, timeStr)
		}
		return fmt.Sprintf("%s %s %s-%s %s FT", icon, home, homeScore, awayScore, away)

	case FormatTmux:
		if IsLive(m) {
			clock := FormatClock(m.Status)
			return fmt.Sprintf("#[fg=green,bold]%s %s %s-%s %s #[fg=yellow](%s)#[default]", icon, home, homeScore, awayScore, away, clock)
		}
		if IsUpcoming(m) {
			timeStr := m.TimeStr
			if timeStr == "" && !m.StartTime.IsZero() {
				timeStr = m.StartTime.Format("15:04")
			}
			if timeStr == "" {
				timeStr = "TBD"
			}
			return fmt.Sprintf("#[fg=blue,bold]%s %s vs %s #[fg=yellow](%s)#[default]", icon, home, away, timeStr)
		}
		return fmt.Sprintf("#[fg=white,bold]%s %s %s-%s %s #[fg=yellow](FT)#[default]", icon, home, homeScore, awayScore, away)

	default: // FormatPlain
		if IsLive(m) {
			clock := FormatClock(m.Status)
			return fmt.Sprintf("%s %s %s-%s %s (%s)", icon, home, homeScore, awayScore, away, clock)
		}
		if IsUpcoming(m) {
			timeStr := m.TimeStr
			if timeStr == "" && !m.StartTime.IsZero() {
				timeStr = m.StartTime.Format("15:04")
			}
			if timeStr == "" {
				timeStr = "TBD"
			}
			return fmt.Sprintf("%s %s vs %s (%s)", icon, home, away, timeStr)
		}
		return fmt.Sprintf("%s %s %s-%s %s (FT)", icon, home, homeScore, awayScore, away)
	}
}

// BuildTooltip constructs rich tooltip details for Waybar.
func BuildTooltip(matches []model.Match, resolver model.Resolver) string {
	if len(matches) == 0 {
		return "No matches"
	}

	var blocks []string
	for _, m := range matches {
		league := LeagueName(m.LeagueKey)
		home := m.HomeTeam.DisplayName
		if home == "" {
			home = m.HomeTeam.Name
		}
		away := m.AwayTeam.DisplayName
		if away == "" {
			away = m.AwayTeam.Name
		}

		var lineScore string
		if IsLive(m) {
			clock := FormatClock(m.Status)
			homeScore := m.HomeTeam.Score
			if homeScore == "" {
				homeScore = "0"
			}
			awayScore := m.AwayTeam.Score
			if awayScore == "" {
				awayScore = "0"
			}
			lineScore = fmt.Sprintf("%s %s - %s %s (%s)", home, homeScore, awayScore, away, clock)
		} else if IsUpcoming(m) {
			timeStr := m.TimeStr
			if timeStr == "" && !m.StartTime.IsZero() {
				timeStr = m.StartTime.Format("15:04")
			}
			if timeStr == "" {
				timeStr = "TBD"
			}
			lineScore = fmt.Sprintf("%s vs %s (%s)", home, away, timeStr)
		} else {
			homeScore := m.HomeTeam.Score
			if homeScore == "" {
				homeScore = "0"
			}
			awayScore := m.AwayTeam.Score
			if awayScore == "" {
				awayScore = "0"
			}
			lineScore = fmt.Sprintf("%s %s - %s %s (Final)", home, homeScore, awayScore, away)
		}

		venue := m.Venue
		if venue == "" {
			venue = "Estadio Principal"
		}

		block := fmt.Sprintf("%s: %s\nVenue: %s", league, lineScore, venue)

		var channelNames []string
		if resolver != nil {
			channels := resolver.GetChannels(m)
			for _, ch := range channels {
				name := ch.DisplayName
				if name == "" {
					name = ch.Name
				}
				if name != "" {
					channelNames = append(channelNames, name)
				}
			}
		}
		if len(channelNames) == 0 && len(m.Broadcasts) > 0 {
			channelNames = m.Broadcasts
		}

		if len(channelNames) > 0 {
			block += fmt.Sprintf("\nChannels: %s", strings.Join(channelNames, ", "))
		}

		blocks = append(blocks, block)
	}

	return strings.Join(blocks, "\n\n")
}

// FormatMatches formats a list of matches according to the chosen Options.
func FormatMatches(matches []model.Match, opts Options, resolver model.Resolver) (string, error) {
	var liveMatches []model.Match
	var upcomingMatches []model.Match
	var finishedMatches []model.Match

	for _, m := range matches {
		if IsLive(m) {
			liveMatches = append(liveMatches, m)
		} else if IsUpcoming(m) {
			upcomingMatches = append(upcomingMatches, m)
		} else {
			finishedMatches = append(finishedMatches, m)
		}
	}

	hasLive := len(liveMatches) > 0

	switch opts.Format {
	case FormatJSON:
		out := JSONOutput{
			Live:    hasLive,
			Count:   len(matches),
			Matches: make([]JSONMatch, 0, len(matches)),
		}
		for _, m := range matches {
			var channels []string
			if resolver != nil {
				for _, ch := range resolver.GetChannels(m) {
					cName := ch.DisplayName
					if cName == "" {
						cName = ch.Name
					}
					if cName != "" {
						channels = append(channels, cName)
					}
				}
			}
			if len(channels) == 0 && len(m.Broadcasts) > 0 {
				channels = m.Broadcasts
			}

			statusStr := FormatClock(m.Status)
			if IsUpcoming(m) {
				timeStr := m.TimeStr
				if timeStr == "" && !m.StartTime.IsZero() {
					timeStr = m.StartTime.Format("15:04")
				}
				statusStr = timeStr
			}

			out.Matches = append(out.Matches, JSONMatch{
				ID:        m.ID,
				League:    m.LeagueKey,
				Home:      m.HomeTeam.DisplayName,
				Away:      m.AwayTeam.DisplayName,
				HomeScore: m.HomeTeam.Score,
				AwayScore: m.AwayTeam.Score,
				Status:    statusStr,
				State:     m.Status.State,
				Venue:     m.Venue,
				Channels:  channels,
				Formatted: FormatMatchSingle(m, FormatPlain, opts.NoEmoji),
			})
		}
		data, err := json.Marshal(out)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case FormatWaybar:
		var class string
		var text string

		if hasLive {
			class = "live"
			var parts []string
			for _, m := range liveMatches {
				parts = append(parts, FormatMatchSingle(m, FormatPlain, opts.NoEmoji))
			}
			text = strings.Join(parts, " | ")
		} else if len(upcomingMatches) > 0 {
			class = "upcoming"
			text = FormatMatchSingle(upcomingMatches[0], FormatPlain, opts.NoEmoji)
		} else if len(finishedMatches) > 0 {
			class = "idle"
			text = FormatMatchSingle(finishedMatches[0], FormatPlain, opts.NoEmoji)
		} else {
			class = "idle"
			text = ""
		}

		tooltip := BuildTooltip(matches, resolver)
		wOut := WaybarOutput{
			Text:    text,
			Tooltip: tooltip,
			Class:   class,
			Alt:     class,
		}
		data, err := json.Marshal(wOut)
		if err != nil {
			return "", err
		}
		return string(data), nil

	case FormatStarship, FormatPrompt:
		if hasLive {
			return FormatMatchSingle(liveMatches[0], opts.Format, opts.NoEmoji), nil
		}
		if len(upcomingMatches) > 0 {
			return FormatMatchSingle(upcomingMatches[0], opts.Format, opts.NoEmoji), nil
		}
		if len(finishedMatches) > 0 {
			return FormatMatchSingle(finishedMatches[0], opts.Format, opts.NoEmoji), nil
		}
		return "", nil

	case FormatTmux:
		if hasLive {
			var parts []string
			for _, m := range liveMatches {
				parts = append(parts, FormatMatchSingle(m, FormatTmux, opts.NoEmoji))
			}
			return strings.Join(parts, " | "), nil
		}
		if len(upcomingMatches) > 0 {
			return FormatMatchSingle(upcomingMatches[0], FormatTmux, opts.NoEmoji), nil
		}
		if len(finishedMatches) > 0 {
			return FormatMatchSingle(finishedMatches[0], FormatTmux, opts.NoEmoji), nil
		}
		return "", nil

	default: // FormatPlain
		if hasLive {
			var parts []string
			for _, m := range liveMatches {
				parts = append(parts, FormatMatchSingle(m, FormatPlain, opts.NoEmoji))
			}
			return strings.Join(parts, " | "), nil
		}
		if len(upcomingMatches) > 0 {
			return FormatMatchSingle(upcomingMatches[0], FormatPlain, opts.NoEmoji), nil
		}
		if len(finishedMatches) > 0 {
			return FormatMatchSingle(finishedMatches[0], FormatPlain, opts.NoEmoji), nil
		}
		return "", nil
	}
}

// FetchAndFilter retrieves matches from model.Client and applies league and team filters.
func FetchAndFilter(ctx context.Context, client model.Client, opts Options) ([]model.Match, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}

	var allMatches []model.Match

	if opts.League != "" {
		matches, err := client.FetchMatches(ctx, opts.League)
		if err != nil {
			return nil, err
		}
		allMatches = matches
	} else {
		var (
			mu        sync.Mutex
			wg        sync.WaitGroup
			fetchErr  error
			collected []model.Match
		)

		// This semaphore bounds in-process work (goroutines and cache reads),
		// not the network: the ESPN client's HTTP transport enforces the
		// process-wide request budget, so the total in flight stays at its cap
		// even when every league here starts at once.
		sem := make(chan struct{}, 6)
		for _, l := range model.DefaultLeagues {
			wg.Add(1)
			go func(leagueKey string) {
				defer wg.Done()
				select {
				case <-ctx.Done():
					return
				case sem <- struct{}{}:
					defer func() { <-sem }()
				}

				m, err := client.FetchMatches(ctx, leagueKey)
				if err != nil {
					mu.Lock()
					if fetchErr == nil {
						fetchErr = err
					}
					mu.Unlock()
					return
				}

				mu.Lock()
				collected = append(collected, m...)
				mu.Unlock()
			}(l.Key)
		}
		wg.Wait()

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if len(collected) == 0 && fetchErr != nil {
			return nil, fetchErr
		}
		allMatches = collected
	}

	// Deduplicate matches by ID
	seen := make(map[string]bool)
	var unique []model.Match
	for _, m := range allMatches {
		if m.ID != "" {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
		}
		unique = append(unique, m)
	}

	// Filter by team substring if specified
	var filtered []model.Match
	teamQuery := strings.ToLower(strings.TrimSpace(opts.Team))
	for _, m := range unique {
		if teamQuery != "" {
			hName := strings.ToLower(m.HomeTeam.Name)
			hDisp := strings.ToLower(m.HomeTeam.DisplayName)
			aName := strings.ToLower(m.AwayTeam.Name)
			aDisp := strings.ToLower(m.AwayTeam.DisplayName)
			title := strings.ToLower(m.Title)

			if !strings.Contains(hName, teamQuery) &&
				!strings.Contains(hDisp, teamQuery) &&
				!strings.Contains(aName, teamQuery) &&
				!strings.Contains(aDisp, teamQuery) &&
				!strings.Contains(title, teamQuery) {
				continue
			}
		}
		filtered = append(filtered, m)
	}

	// Deterministic sort:
	// 1. Live matches first ("in")
	// 2. Upcoming matches ("pre") sorted by StartTime ascending
	// 3. Finished matches ("post" / completed) sorted by StartTime descending
	sort.SliceStable(filtered, func(i, j int) bool {
		mI, mJ := filtered[i], filtered[j]
		rank := func(m model.Match) int {
			if IsLive(m) {
				return 0
			}
			if IsUpcoming(m) {
				return 1
			}
			return 2
		}
		rI, rJ := rank(mI), rank(mJ)
		if rI != rJ {
			return rI < rJ
		}
		if rI == 1 { // upcoming: earliest first
			return mI.StartTime.Before(mJ.StartTime)
		}
		if rI == 2 { // finished: latest first
			return mI.StartTime.After(mJ.StartTime)
		}
		return mI.StartTime.Before(mJ.StartTime)
	})

	return filtered, nil
}
