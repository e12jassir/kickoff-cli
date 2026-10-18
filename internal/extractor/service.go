package extractor

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

const (
	defaultHTTPTimeout = 8 * time.Second
	defaultUserAgent   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

var (
	// Regex patterns for agenda parsing
	reLiTag   = regexp.MustCompile(`(?si)<li\b[^>]*>(.*?)</li>`)
	reTrTag   = regexp.MustCompile(`(?si)<tr\b[^>]*>(.*?)</tr>`)
	reDivTag  = regexp.MustCompile(`(?si)<div\b[^>]*class=["'][^"']*(?:partido|event|match)[^"']*["'][^>]*>(.*?)</div>`)
	reLinkTag = regexp.MustCompile(`(?si)<a\b[^>]*?href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	reAnyTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	reTimeStr = regexp.MustCompile(`\b([01]?\d|2[0-3]):([0-5]\d)\b`)
)

// DefaultKnownDomains contains active mirror domains known to publish live match agendas.
var DefaultKnownDomains = []string{
	"https://librefutboltv.com",
	"https://pelotalibre.org",
	"https://futbollibre.lol",
}

// Service coordinates scraping of agenda portals and manifest resolution.
type Service struct {
	mu           sync.RWMutex
	client       *http.Client
	baseDomains  []string
	userAgent    string
	cacheTTL     time.Duration
	cachedAgenda []ExtractedEvent
	lastFetch    time.Time
}

// Option configures the extractor Service.
type Option func(*Service)

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		if client != nil {
			s.client = client
		}
	}
}

// WithBaseDomains configures custom base mirror domains.
func WithBaseDomains(domains ...string) Option {
	return func(s *Service) {
		if len(domains) > 0 {
			s.baseDomains = domains
		}
	}
}

// WithUserAgent overrides default browser User-Agent.
func WithUserAgent(ua string) Option {
	return func(s *Service) {
		if ua != "" {
			s.userAgent = ua
		}
	}
}

// WithCacheTTL configures the in-memory agenda cache duration.
func WithCacheTTL(d time.Duration) Option {
	return func(s *Service) {
		s.cacheTTL = d
	}
}

// NewService instantiates a new dynamic match extractor service.
func NewService(opts ...Option) *Service {
	s := &Service{
		client:      &http.Client{Timeout: defaultHTTPTimeout},
		baseDomains: DefaultKnownDomains,
		userAgent:   defaultUserAgent,
		cacheTTL:    5 * time.Minute,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// FetchAgenda scrapes the agenda from mirror domains with fallback and context cancellation.
func (s *Service) FetchAgenda(ctx context.Context) ([]ExtractedEvent, error) {
	s.mu.RLock()
	if len(s.cachedAgenda) > 0 && time.Since(s.lastFetch) < s.cacheTTL {
		cached := s.cachedAgenda
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check under write lock
	if len(s.cachedAgenda) > 0 && time.Since(s.lastFetch) < s.cacheTTL {
		return s.cachedAgenda, nil
	}

	var lastErr error
	for _, domain := range s.baseDomains {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		events, err := s.fetchAgendaFromDomain(ctx, domain)
		if err == nil && len(events) > 0 {
			s.cachedAgenda = events
			s.lastFetch = time.Now()
			return events, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("extractor: all mirror domains failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("extractor: no matches found in any configured domain")
}

func (s *Service) fetchAgendaFromDomain(ctx context.Context, baseDomain string) ([]ExtractedEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseDomain, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-419,es;q=0.9,en;q=0.5")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch domain %s: %w", baseDomain, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, baseDomain)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8MB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return ParseAgendaHTML(string(bodyBytes), baseDomain)
}

// ParseAgendaHTML parses raw HTML from live match agenda portals.
func ParseAgendaHTML(rawHTML, baseDomain string) ([]ExtractedEvent, error) {
	var events []ExtractedEvent
	loc := locale.Location()
	now := time.Now().In(loc)

	var itemContents []string
	for _, m := range reLiTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reTrTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reDivTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}

	if len(itemContents) == 0 {
		// Fallback: look for direct <a> tags that contain match titles
		aMatches := reLinkTag.FindAllStringSubmatch(rawHTML, -1)
		for _, am := range aMatches {
			href := am[1]
			text := cleanHTMLText(am[2])
			if !strings.Contains(strings.ToLower(text), "vs") && !strings.Contains(strings.ToLower(text), " - ") {
				continue
			}
			home, away := splitTeams(text)
			if home != "" && away != "" {
				fullURL := resolveURL(href, baseDomain)
				events = append(events, ExtractedEvent{
					HomeTeam:     home,
					AwayTeam:     away,
					RawTitle:     text,
					StartTime:    now,
					SourceDomain: baseDomain,
					Streams: []ExtractedStream{
						{
							ChannelName: "Direct",
							PlayerURL:   fullURL,
							Resolution:  "1080p",
						},
					},
				})
			}
		}
		return events, nil
	}

	for _, blockHTML := range itemContents {
		plainText := cleanHTMLText(blockHTML)
		lower := strings.ToLower(plainText)

		if !strings.Contains(lower, "vs") && !strings.Contains(lower, " - ") {
			continue
		}

		// Look for time HH:MM
		timeStr := ""
		if tMatch := reTimeStr.FindString(plainText); tMatch != "" {
			timeStr = tMatch
		}

		// Find title
		rawTitle := ""
		for _, line := range strings.Split(plainText, "\n") {
			line = strings.TrimSpace(line)
			lLower := strings.ToLower(line)
			if strings.Contains(lLower, "vs") || strings.Contains(lLower, " - ") {
				rawTitle = line
				break
			}
		}
		if rawTitle == "" {
			rawTitle = plainText
		}

		home, away := splitTeams(rawTitle)
		if home == "" || away == "" {
			continue
		}

		// Extract player/stream links
		var streams []ExtractedStream
		linkMatches := reLinkTag.FindAllStringSubmatch(blockHTML, -1)
		for _, lm := range linkMatches {
			href := strings.TrimSpace(lm[1])
			if href == "" || href == "#" || strings.HasPrefix(href, "javascript:") {
				continue
			}
			linkText := cleanHTMLText(lm[2])
			if linkText == "" {
				linkText = "Stream"
			}
			fullURL := resolveURL(href, baseDomain)
			streams = append(streams, ExtractedStream{
				ChannelName: linkText,
				PlayerURL:   fullURL,
				Resolution:  "1080p",
			})
		}

		startTime := now
		if timeStr != "" {
			if t, err := time.ParseInLocation("15:04", strings.TrimSpace(timeStr), loc); err == nil {
				startTime = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			}
		}

		events = append(events, ExtractedEvent{
			HomeTeam:     home,
			AwayTeam:     away,
			RawTitle:     rawTitle,
			StartTime:    startTime,
			TimeStr:      timeStr,
			Streams:      streams,
			SourceDomain: baseDomain,
		})
	}

	return events, nil
}

func cleanHTMLText(s string) string {
	// Replace block-level end tags or breaks with newlines
	s = regexp.MustCompile(`(?i)<(br|p|div|li|tr|span)[^>]*>`).ReplaceAllString(s, "\n")
	s = reAnyTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}

// splitTeams splits a fixture line into Home and Away team names.
func splitTeams(raw string) (string, string) {
	lower := strings.ToLower(raw)
	separators := []string{" vs. ", " vs ", " v ", " - "}
	for _, sep := range separators {
		idx := strings.Index(lower, sep)
		if idx != -1 {
			home := strings.TrimSpace(raw[:idx])
			away := strings.TrimSpace(raw[idx+len(sep):])
			home = cleanTeamName(home)
			away = cleanTeamName(away)
			if home != "" && away != "" {
				return home, away
			}
		}
	}
	return "", ""
}

func cleanTeamName(name string) string {
	idx := strings.Index(name, "(")
	if idx != -1 {
		name = name[:idx]
	}
	idx = strings.Index(name, "[")
	if idx != -1 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

// MatchEvent pairs an official ESPN match with the most accurate extracted event using tvguide's MatchScore algorithm.
func (s *Service) MatchEvent(match model.Match, events []ExtractedEvent) (*ExtractedEvent, int) {
	bestScore := 0
	var bestEvent *ExtractedEvent

	for i := range events {
		event := &events[i]
		candidateTitle := event.RawTitle
		if candidateTitle == "" {
			candidateTitle = fmt.Sprintf("%s vs %s", event.HomeTeam, event.AwayTeam)
		}

		matched, score := tvguide.MatchScore(candidateTitle, match)
		if matched && score > bestScore {
			bestScore = score
			bestEvent = event
		}
	}

	return bestEvent, bestScore
}

// ResolveStream implements the Extractor interface by delegating to ResolveManifest.
func (s *Service) ResolveStream(ctx context.Context, stream *ExtractedStream) error {
	return s.ResolveManifest(ctx, stream)
}

// ProbeStream implements the Extractor interface by delegating to ProbeHLS.
func (s *Service) ProbeStream(ctx context.Context, stream *ExtractedStream) bool {
	return s.ProbeHLS(ctx, stream)
}

// ResolveMatchStreams coordinates agenda fetching, event matching, manifest resolution, and probe verification.
func (s *Service) ResolveMatchStreams(ctx context.Context, match model.Match) ([]ExtractedStream, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	events, err := s.FetchAgenda(ctx)
	if err != nil {
		return nil, fmt.Errorf("extractor: fetch agenda: %w", err)
	}

	event, score := s.MatchEvent(match, events)
	if event == nil || score <= 0 || len(event.Streams) == 0 {
		return nil, nil
	}

	var liveStreams []ExtractedStream
	for _, st := range event.Streams {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		streamCopy := st
		if streamCopy.StreamURL == "" {
			if streamCopy.PlayerURL == "" {
				continue
			}
			if err := s.ResolveStream(ctx, &streamCopy); err != nil {
				continue
			}
		}

		if !streamCopy.Alive {
			if !s.ProbeStream(ctx, &streamCopy) || !streamCopy.Alive {
				continue
			}
		}

		liveStreams = append(liveStreams, streamCopy)
	}

	return liveStreams, nil
}
