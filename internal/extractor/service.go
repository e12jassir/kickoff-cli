package extractor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/fsutil"
	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

const (
	defaultHTTPTimeout = 8 * time.Second
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
)

var (
	// Regex patterns for agenda parsing
	reArticleTag = regexp.MustCompile(`(?si)<article\b[^>]*>(.*?)</article>`)
	reLiTag      = regexp.MustCompile(`(?si)<li\b[^>]*>(.*?)</li>`)
	reTrTag      = regexp.MustCompile(`(?si)<tr\b[^>]*>(.*?)</tr>`)
	reDivTag     = regexp.MustCompile(`(?si)<div\b[^>]*class=["'][^"']*(?:partido|event|match)[^"']*["'][^>]*>(.*?)</div>`)
	reTableTag   = regexp.MustCompile(`(?si)<table\b[^>]*class=["'][^"']*(?:partido|event|match)[^"']*["'][^>]*>(.*?)</table>`)
	reLinkTag    = regexp.MustCompile(`(?si)<a\b[^>]*?href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	reAnyTag     = regexp.MustCompile(`(?s)<[^>]*>`)
	reTimeStr    = regexp.MustCompile(`\b([01]?\d|2[0-3]):([0-5]\d)\b`)
)

// Verified alive 2026-10-20: agenda18 (structured JSON, richest UCL
// coverage incl. Disney/ESPN per-signal embeds), rusticotvplus, pelotalibretv and
// futbol-librehd. librefutboltv.com now 302s to a survey-scam redirect and
// futbollibre.lol hangs, so both were removed: they only burned up to 8s
// of connect timeout each before the working fallback answered.
var DefaultKnownDomains = []string{
	"https://agenda18.com/agenda.json",
	"https://rusticotvplus.org/agenda.php",
	"https://rusticotvplus.org",
	"https://pelotalibretv.net",
	"https://pelotalive.com",
	"https://pelotalibre.cc",
	"https://futbol-librehd.com",
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
	cacheDir     string
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

// WithCacheDir configures custom disk cache directory for offline resilience.
func WithCacheDir(dir string) Option {
	return func(s *Service) {
		if dir != "" {
			s.cacheDir = dir
		}
	}
}

// NewService instantiates a new dynamic match extractor service.
func NewService(opts ...Option) *Service {
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheBase = filepath.Join(home, ".cache")
		} else {
			cacheBase = os.TempDir()
		}
	}
	s := &Service{
		client:      &http.Client{Timeout: defaultHTTPTimeout},
		baseDomains: DefaultKnownDomains,
		userAgent:   defaultUserAgent,
		cacheTTL:    15 * time.Minute,
		cacheDir:    filepath.Join(cacheBase, "kickoff"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) diskCachePath() string {
	todayStr := time.Now().In(locale.BogotaLocation()).Format("2006-01-02")
	return filepath.Join(s.cacheDir, fmt.Sprintf("extractor_agenda_%s.json", todayStr))
}

func (s *Service) saveDiskCache(events []ExtractedEvent) {
	if len(events) == 0 || s.cacheDir == "" {
		return
	}
	data, err := json.Marshal(events)
	if err != nil {
		return
	}
	_ = fsutil.AtomicWriteFile(s.diskCachePath(), data, 0o644)
}

func (s *Service) loadDiskCache() ([]ExtractedEvent, error) {
	if s.cacheDir == "" {
		return nil, fmt.Errorf("no cache dir")
	}
	data, err := os.ReadFile(s.diskCachePath())
	if err != nil {
		return nil, err
	}
	var events []ExtractedEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FetchAgenda scrapes the agenda from mirror domains with fallback, context cancellation,
// and resilient stale-while-revalidate disk persistence.
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
domainLoop:
	for _, domain := range s.baseDomains {
		select {
		case <-ctx.Done():
			break domainLoop
		default:
		}

		domainCtx, domainCancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		events, err := s.fetchAgendaFromDomain(domainCtx, domain)
		domainCancel()

		if err == nil && len(events) > 0 {
			s.cachedAgenda = events
			s.lastFetch = time.Now()
			go s.saveDiskCache(events)
			return events, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	// Resilient fallback (stale-while-revalidate):
	// 1. If we have memory cache (even if stale), use it when upstream is failing.
	if len(s.cachedAgenda) > 0 {
		return s.cachedAgenda, nil
	}
	// 2. If we have today's disk cache from earlier, recover from disk.
	if diskEvents, err := s.loadDiskCache(); err == nil && len(diskEvents) > 0 {
		s.cachedAgenda = diskEvents
		s.lastFetch = time.Now()
		return diskEvents, nil
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
	// Read-only HTTP body: a Close error cannot change the bytes already read.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, baseDomain)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8MB limit
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	cleanBase := strings.Split(baseDomain, "?")[0]
	if strings.Contains(contentType, "application/json") || strings.HasSuffix(cleanBase, ".json") {
		return ParseAgendaJSON(bodyBytes, baseDomain)
	}

	return ParseAgendaHTML(string(bodyBytes), baseDomain)
}

// ParseAgendaHTML parses raw HTML from live match agenda portals.
func ParseAgendaHTML(rawHTML, baseDomain string) ([]ExtractedEvent, error) {
	var events []ExtractedEvent
	loc := locale.Location()
	now := time.Now().In(loc)

	var itemContents []string
	for _, m := range reArticleTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reLiTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reTrTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reDivTag.FindAllStringSubmatch(rawHTML, -1) {
		itemContents = append(itemContents, m[1])
	}
	for _, m := range reTableTag.FindAllStringSubmatch(rawHTML, -1) {
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
				playerURL := ""
				if idx := strings.Index(href, "r="); idx != -1 {
					rVal := href[idx+2:]
					if endIdx := strings.Index(rVal, "&"); endIdx != -1 {
						rVal = rVal[:endIdx]
					}
					if decoded := decodeBase64URL(rVal); decoded != "" {
						playerURL = decoded
					}
				}
				if playerURL == "" {
					playerURL = resolveURL(href, baseDomain)
				}
				events = append(events, ExtractedEvent{
					HomeTeam:     home,
					AwayTeam:     away,
					RawTitle:     text,
					StartTime:    now,
					SourceDomain: baseDomain,
					Streams: []ExtractedStream{
						{
							ChannelName: "Direct",
							PlayerURL:   playerURL,
							Resolution:  "1080p",
						},
					},
				})
			}
		}
		return events, nil
	}

	eventIndices := make(map[string]int)

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

			playerURL := ""
			// Support /embed/eventos.html?r=... or r= parameter in links
			if idx := strings.Index(href, "r="); idx != -1 {
				rVal := href[idx+2:]
				if endIdx := strings.Index(rVal, "&"); endIdx != -1 {
					rVal = rVal[:endIdx]
				}
				if decoded := decodeBase64URL(rVal); decoded != "" {
					playerURL = decoded
				}
			}
			if playerURL == "" {
				playerURL = resolveURL(href, baseDomain)
			}

			res := "1080p"
			lowerText := strings.ToLower(linkText)
			if strings.Contains(lowerText, "720p") {
				res = "720p"
			} else if strings.Contains(lowerText, "4k") || strings.Contains(lowerText, "uhd") {
				res = "4K"
			}

			streams = append(streams, ExtractedStream{
				ChannelName: linkText,
				PlayerURL:   playerURL,
				Resolution:  res,
			})
		}

		startTime := now
		if timeStr != "" {
			if t, err := time.ParseInLocation("15:04", strings.TrimSpace(timeStr), loc); err == nil {
				startTime = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			}
		}

		eventKey := strings.ToLower(home) + "|" + strings.ToLower(away)
		if existingIdx, exists := eventIndices[eventKey]; exists {
			// Merge streams, avoiding duplicates
			for _, st := range streams {
				alreadyHas := false
				for _, cur := range events[existingIdx].Streams {
					if cur.PlayerURL == st.PlayerURL && cur.ChannelName == st.ChannelName {
						alreadyHas = true
						break
					}
				}
				if !alreadyHas {
					events[existingIdx].Streams = append(events[existingIdx].Streams, st)
				}
			}
			if events[existingIdx].TimeStr == "" && timeStr != "" {
				events[existingIdx].TimeStr = timeStr
				events[existingIdx].StartTime = startTime
			}
			continue
		}

		eventIndices[eventKey] = len(events)
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

var reLeadingTime = regexp.MustCompile(`^\s*(?:[01]?\d|2[0-3]):[0-5]\d(?::[0-5]\d)?\s*`)

func cleanTeamName(name string) string {
	name = reLeadingTime.ReplaceAllString(name, "")
	if idx := strings.Index(name, ":"); idx != -1 {
		name = strings.TrimSpace(name[idx+1:])
		name = reLeadingTime.ReplaceAllString(name, "")
	}
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

type agenda18Response struct {
	Data []struct {
		Attributes struct {
			DateDiary        string `json:"date_diary"`
			DiaryHour        string `json:"diary_hour"`
			DiaryDescription string `json:"diary_description"`
			Embeds           struct {
				Data []struct {
					Attributes struct {
						EmbedName   string `json:"embed_name"`
						EmbedIframe string `json:"embed_iframe"`
						Idioma      string `json:"idioma"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"embeds"`
		} `json:"attributes"`
	} `json:"data"`
}

// ParseAgendaJSON parses structured JSON agenda feeds (e.g. agenda18.com / pelotalibre).
func ParseAgendaJSON(data []byte, baseDomain string) ([]ExtractedEvent, error) {
	var resp agenda18Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("extractor: parse json agenda: %w", err)
	}

	var events []ExtractedEvent
	loc := locale.Location()
	now := time.Now().In(loc)

	for _, item := range resp.Data {
		desc := strings.TrimSpace(item.Attributes.DiaryDescription)
		if desc == "" {
			continue
		}

		home, away := splitTeams(desc)
		if home == "" || away == "" {
			continue
		}

		timeStr := strings.TrimSpace(item.Attributes.DiaryHour)
		startTime := now
		if len(timeStr) >= 5 {
			if t, err := time.ParseInLocation("15:04", timeStr[:5], loc); err == nil {
				startTime = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			}
		}

		var streams []ExtractedStream
		for _, emb := range item.Attributes.Embeds.Data {
			name := strings.TrimSpace(emb.Attributes.EmbedName)
			if name == "" {
				name = "Direct"
			}
			iframe := strings.TrimSpace(emb.Attributes.EmbedIframe)
			if iframe == "" {
				continue
			}

			playerURL := ""
			// If iframe is an embed URL containing base64 param r:
			// e.g. /embed/eventos.html?r=aHR0cHM6Ly9sYTE4aGQuc3U...
			if idx := strings.Index(iframe, "r="); idx != -1 {
				rVal := iframe[idx+2:]
				if endIdx := strings.Index(rVal, "&"); endIdx != -1 {
					rVal = rVal[:endIdx]
				}
				if decoded := decodeBase64URL(rVal); decoded != "" {
					playerURL = decoded
				}
			}
			if playerURL == "" {
				playerURL = resolveURL(iframe, baseDomain)
			}

			streams = append(streams, ExtractedStream{
				ChannelName: name,
				PlayerURL:   playerURL,
				Resolution:  "1080p",
			})
		}

		events = append(events, ExtractedEvent{
			HomeTeam:     home,
			AwayTeam:     away,
			RawTitle:     desc,
			StartTime:    startTime,
			TimeStr:      timeStr,
			Streams:      streams,
			SourceDomain: baseDomain,
		})
	}

	return events, nil
}

func decodeBase64URL(raw string) string {
	raw = strings.TrimSpace(raw)
	if unescaped, err := url.QueryUnescape(raw); err == nil && unescaped != "" {
		raw = unescaped
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(raw); err == nil {
			s := string(b)
			if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
				return s
			}
		}
	}
	return ""
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

	resolveCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		resolveCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
	}

	events, err := s.FetchAgenda(resolveCtx)
	if err != nil {
		return nil, fmt.Errorf("extractor: fetch agenda: %w", err)
	}

	event, score := s.MatchEvent(match, events)
	if event == nil || score <= 0 || len(event.Streams) == 0 {
		return nil, nil
	}

	type indexedStream struct {
		idx    int
		stream ExtractedStream
		ok     bool
	}

	results := make([]indexedStream, len(event.Streams))
	var wg sync.WaitGroup

	for i, st := range event.Streams {
		wg.Add(1)
		go func(idx int, target ExtractedStream) {
			defer wg.Done()
			select {
			case <-resolveCtx.Done():
				return
			default:
			}

			if target.StreamURL == "" {
				if target.PlayerURL == "" {
					return
				}
				if err := s.ResolveStream(resolveCtx, &target); err != nil {
					return
				}
			}

			if target.StreamURL != "" {
				// ELIMINATE THE DESTRUCTIVE PROBE DISCARD:
				// Never discard a stream just because ProbeStream fails or times out.
				// If target.StreamURL is resolved, keep it.
				// If ProbeStream succeeds, set target.Alive = true.
				// If it fails or times out, keep target.Alive = false, but DO NOT DROP THE STREAM.
				if !target.Alive {
					if s.ProbeStream(resolveCtx, &target) {
						target.Alive = true
					} else {
						target.Alive = false
					}
				}
				results[idx] = indexedStream{idx: idx, stream: target, ok: true}
			}
		}(i, st)
	}

	wg.Wait()

	var liveStreams []ExtractedStream
	for _, res := range results {
		if res.ok {
			liveStreams = append(liveStreams, res.stream)
		}
	}

	return liveStreams, nil
}
