// Package parrilla fetches and parses the Gol Caracol daily broadcast schedule
// (the "parrilla") published by Noticias Caracol. It is the authoritative
// per-match source of which channel airs each football match in Colombia.
//
// The parser is deliberately schema-free: the daily article is pasted from
// Google Docs, so cells carry huge inline styles, the schedule can be split
// across more than one <table>, and the header wording drifts between
// editions. ParseHTML therefore collects every <td> cell in document order,
// strips markup and collapses whitespace, then groups cells positionally in
// chunks of four (Hora | Equipos | Liga/Competiciones | Canales).
package parrilla

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ErrNoSchedule is returned when the fetched document contains no table cells
// at all, which indicates the page structure was not recognized.
var ErrNoSchedule = errors.New("parrilla: no schedule cells found in document")

// Entry is a single match row from the daily parrilla.
type Entry struct {
	Competition string   // "La Liga EA Sports"
	HomeTeam    string   // "Espanyol"
	AwayTeam    string   // "Atlético Madrid"
	Raw         string   // "Espanyol vs. Atlético Madrid"
	StartLocal  string   // "7:00 a.m." exactly as published (Colombia time)
	StartMins   int      // minutes since midnight; -1 when unparseable
	Channels    []string // canonical display names; empty slice means TBD
}

// Snapshot is the parsed parrilla for a single day.
type Snapshot struct {
	Date    string // source date, YYYY-MM-DD
	Entries []Entry
}

// Fetcher retrieves the parrilla snapshot for a given calendar date.
type Fetcher interface {
	Fetch(ctx context.Context, date time.Time) (Snapshot, error)
}

const (
	defaultBaseURL   = "https://www.noticiascaracol.com/golcaracol"
	articleSlug      = "partidos-en-vivo-hoy"
	articleSeriesTag = "so35"
)

// spanishMonths holds the lowercase Spanish month names used in the URL slug.
var spanishMonths = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// BuildURL returns the deterministic Gol Caracol article URL for a date, using
// the default base URL. The day has no leading zero, the month is the lowercase
// Spanish name and the constant series tag is appended.
func BuildURL(date time.Time) string {
	return buildURL(defaultBaseURL, date)
}

// buildURL is BuildURL with a configurable base URL, used by the fetcher for
// testability.
func buildURL(baseURL string, date time.Time) string {
	year, month, day := date.Date()
	base := strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/%s-%d-de-%s-del-%d-horarios-y-programacion-por-tv-%s",
		base, articleSlug, day, spanishMonths[int(month)-1], year, articleSeriesTag)
}

var (
	// reCell matches a single table cell. It is intentionally bounded by the
	// closing </td> so it never scans the whole document.
	reCell = regexp.MustCompile(`(?is)<td\b[^>]*>(.*?)</td>`)
	// reTag strips any remaining markup inside a cell.
	reTag = regexp.MustCompile(`(?s)<[^>]*>`)
	// reClock extracts the H:MM portion of a published time.
	reClock = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
	// reFixtureSeparator splits "Home vs. Away" on "vs." or "vs".
	reFixtureSeparator = regexp.MustCompile(`(?i)\s+vs\.?\s+`)
)

// ParseHTML parses a Gol Caracol article body for the given date (YYYY-MM-DD).
// It performs no network access and is fully unit-testable.
func ParseHTML(date string, body []byte) (Snapshot, error) {
	cells := extractCells(body)
	if len(cells) == 0 {
		return Snapshot{}, ErrNoSchedule
	}

	snap := Snapshot{Date: date, Entries: make([]Entry, 0, len(cells)/4)}
	for i := 0; i+4 <= len(cells); i += 4 {
		hora := cells[i]
		fixture := cells[i+1]
		competition := cells[i+2]
		channels := cells[i+3]

		startMins := parseStartMinutes(hora)

		// Skip the header row and any layout cells: a real match row either
		// carries a parseable time or a "vs." fixture separator. Never key on
		// the drifting header text.
		if startMins < 0 && !reFixtureSeparator.MatchString(fixture) {
			continue
		}
		if strings.TrimSpace(fixture) == "" {
			continue
		}

		home, away := splitFixture(fixture)
		snap.Entries = append(snap.Entries, Entry{
			Competition: competition,
			HomeTeam:    home,
			AwayTeam:    away,
			Raw:         fixture,
			StartLocal:  hora,
			StartMins:   startMins,
			Channels:    parseChannels(channels),
		})
	}

	return snap, nil
}

// extractCells returns every <td> cell text in document order, with tags
// stripped, HTML entities decoded and whitespace collapsed. Because it keys on
// <td> and not on <table>, a schedule split across multiple tables is captured
// whole.
func extractCells(body []byte) []string {
	matches := reCell.FindAllSubmatch(body, -1)
	cells := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		cells = append(cells, cleanCell(m[1]))
	}
	return cells
}

// cleanCell strips markup, decodes entities and collapses whitespace.
func cleanCell(raw []byte) string {
	s := reTag.ReplaceAllString(string(raw), " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// splitFixture splits "Home vs. Away" into its two teams. When no separator is
// present the whole fixture is returned as home with an empty away team.
func splitFixture(fixture string) (home, away string) {
	if loc := reFixtureSeparator.FindStringIndex(fixture); loc != nil {
		return strings.TrimSpace(fixture[:loc[0]]), strings.TrimSpace(fixture[loc[1]:])
	}
	return strings.TrimSpace(fixture), ""
}

// parseChannels splits a comma-separated channel cell into canonical display
// names, de-duplicating by canonical identity. An empty or whitespace-only cell
// yields a non-nil empty slice, representing the honest "por definir" state.
func parseChannels(cell string) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]bool)
	for _, part := range strings.Split(cell, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		display := part
		dedupKey := normalizeChannelName(part)
		if canonical, ok := CanonicalDisplay(part); ok {
			display = canonical
			if key, _, found := CanonicalChannel(part); found {
				dedupKey = key
			}
		}
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		out = append(out, display)
	}
	return out
}

// parseStartMinutes converts a published Colombian time such as "6:30 a.m." or
// "9:30 p.m." into minutes since midnight. It returns -1 when the value cannot
// be interpreted.
func parseStartMinutes(hora string) int {
	clock := reClock.FindStringSubmatch(hora)
	if clock == nil {
		return -1
	}
	var hour, minute int
	if _, err := fmt.Sscanf(clock[1], "%d", &hour); err != nil {
		return -1
	}
	if _, err := fmt.Sscanf(clock[2], "%d", &minute); err != nil {
		return -1
	}
	if minute < 0 || minute > 59 || hour > 23 {
		return -1
	}

	// "a.m." does not contain the substring "am" (the dots intervene), so the
	// meridiem markers are matched with and without periods explicitly.
	lowered := strings.ToLower(hora)
	if strings.Contains(lowered, "p.m") || strings.Contains(lowered, "pm") {
		if hour != 12 {
			hour += 12
		}
	} else if strings.Contains(lowered, "a.m") || strings.Contains(lowered, "am") {
		if hour == 12 {
			hour = 0
		}
	}

	if hour < 0 || hour > 23 {
		return -1
	}
	return hour*60 + minute
}

// NewGolCaracol returns a Fetcher for the Gol Caracol daily parrilla using the
// default base URL and a browser User-Agent. Options allow overriding the HTTP
// client and base URL for tests.
func NewGolCaracol(opts ...Option) Fetcher {
	g := &golCaracol{
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: browserUserAgent,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}
