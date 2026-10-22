package tvguide

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/fsutil"
	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	DefaultCacheTTL = 4 * time.Hour

	// minMatchScore is the MatchScore a candidate must reach to be considered.
	// MatchScore only returns this value (or the full-title 10) when BOTH teams
	// are identified or the full fixture title is matched; a single-team hit
	// scores zero, so a lone team name can never resolve a channel.
	minMatchScore = 5

	// matchWindowMinutes is how long a fixture is assumed to be on air from
	// kickoff. A guide programme must overlap that window to be eligible.
	matchWindowMinutes = 120

	// defaultEntryMinutes is the assumed programme length when the guide entry
	// has a start time but no usable end time.
	defaultEntryMinutes = 120
)

// Service coordinates fetching, caching, and matching of TV programming.
type Service struct {
	crawler    Crawler
	cacheDir   string
	cacheTTL   time.Duration
	now        func() time.Time
	loc        *time.Location // display zone; source schedules use Bogota
	targets    []ChannelTarget
	httpClient *http.Client
	baseURL    string

	// autoRefresh opts in to the startup disk load and background crawl. It is
	// explicit so tests never depend on how the binary was invoked.
	autoRefresh bool

	mu         sync.RWMutex
	guides     []ChannelGuide
	loadedDate string
}

// Option configures Service behavior.
type Option func(*Service)

// WithCrawler sets a custom crawler (useful for testing).
func WithCrawler(c Crawler) Option {
	return func(s *Service) {
		s.crawler = c
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		s.httpClient = client
	}
}

// WithBaseURL sets the base URL for the TV guide.
func WithBaseURL(url string) Option {
	return func(s *Service) {
		s.baseURL = url
	}
}

// WithCacheDir configures the directory where daily guide caches are stored.
func WithCacheDir(dir string) Option {
	return func(s *Service) {
		s.cacheDir = dir
	}
}

// WithCacheTTL configures the freshness TTL for the guide cache.
func WithCacheTTL(ttl time.Duration) Option {
	return func(s *Service) {
		s.cacheTTL = ttl
	}
}

// WithNow overrides the current time function (for deterministic testing).
func WithNow(nowFunc func() time.Time) Option {
	return func(s *Service) {
		s.now = nowFunc
	}
}

// WithTimezone overrides the display timezone held by the service. Resolution
// itself is anchored to locale.BogotaLocation, so this option does not change
// which guide entry matches a fixture; the view layer owns display conversion.
func WithTimezone(loc *time.Location) Option {
	return func(s *Service) {
		s.loc = loc
	}
}

// WithAutoRefresh enables the startup disk cache load and the background crawl
// when the cache is cold. Long-lived callers opt in; tests and short-lived tools
// stay offline and deterministic.
func WithAutoRefresh(enabled bool) Option {
	return func(s *Service) {
		s.autoRefresh = enabled
	}
}

// WithTargets sets custom channel targets.
func WithTargets(targets []ChannelTarget) Option {
	return func(s *Service) {
		s.targets = targets
	}
}

// NewService creates a new tvguide.Service and loads fresh cache from disk.
// If cache is cold or expired and not running in unit test mode, it initiates a background refresh.
func NewService(opts ...Option) *Service {
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheBase = filepath.Join(home, ".cache")
		} else {
			cacheBase = os.TempDir()
		}
	}
	defaultCache := filepath.Join(cacheBase, "kickoff")

	// Retain the display timezone on the service so callers can model the user's
	// zone. Source schedules and cache days are anchored to Bogota instead, so
	// the display choice never influences which guide or programme matches.
	loc := locale.Location()

	s := &Service{
		cacheDir: defaultCache,
		cacheTTL: DefaultCacheTTL,
		loc:      loc,
		now:      time.Now,
		targets:  DefaultTargets,
		baseURL:  DefaultBaseURL,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.crawler == nil {
		s.crawler = NewCrawler(s.httpClient, s.baseURL)
	}

	// 1. Attempt a zero-delay load from disk: always for an explicitly
	// configured cache directory, otherwise only when auto refresh is enabled.
	var loaded bool
	if s.cacheDir != defaultCache || s.autoRefresh {
		loaded = s.loadCache()
	}

	// 2. If the cache is not fresh, refresh in the background. Never block the
	// constructor on the network.
	if !loaded && s.autoRefresh {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.Refresh(ctx)
		}()
	}

	return s
}

func (s *Service) cacheFilePath(dateStr string) string {
	return filepath.Join(s.cacheDir, fmt.Sprintf("tvguide_%s.json", dateStr))
}

// loadCache loads today's guide from disk if present and within TTL. The day is
// the source calendar day in Colombia, not the display day: the crawled guide
// lists the Colombian schedule.
func (s *Service) loadCache() bool {
	todayStr := s.now().In(locale.BogotaLocation()).Format("2006-01-02")
	filePath := s.cacheFilePath(todayStr)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	var cache GuideCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}

	if cache.Date != todayStr {
		return false
	}

	if s.now().Sub(cache.CachedAt) > s.cacheTTL {
		return false
	}

	if len(cache.Channels) == 0 {
		return false
	}

	s.mu.Lock()
	s.guides = cache.Channels
	s.loadedDate = cache.Date
	s.mu.Unlock()

	return true
}

// loadAnyCache loads today's cache file regardless of TTL (used for graceful offline fallback).
func (s *Service) loadAnyCache() bool {
	todayStr := s.now().In(locale.BogotaLocation()).Format("2006-01-02")
	filePath := s.cacheFilePath(todayStr)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	var cache GuideCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return false
	}

	if len(cache.Channels) == 0 {
		return false
	}

	s.mu.Lock()
	s.guides = cache.Channels
	s.loadedDate = cache.Date
	s.mu.Unlock()

	return true
}

// saveCache serializes the crawled channel guides to disk.
func (s *Service) saveCache(guides []ChannelGuide) error {
	if len(guides) == 0 {
		return nil
	}

	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return err
	}

	todayStr := s.now().In(locale.BogotaLocation()).Format("2006-01-02")
	filePath := s.cacheFilePath(todayStr)

	cache := GuideCache{
		Date:     todayStr,
		CachedAt: s.now(),
		Channels: guides,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return fsutil.AtomicWriteFile(filePath, data, 0o644)
}

// Refresh performs a network crawl across all targets and persists to cache.
// If network fails or times out, it logs/falls back gracefully without panic.
func (s *Service) Refresh(ctx context.Context) error {
	guides, err := s.crawler.Crawl(ctx, s.targets)
	if err != nil || len(guides) == 0 {
		// Graceful offline fallback: try loading any available cache from disk
		s.loadAnyCache()
		return err
	}

	s.mu.Lock()
	s.guides = guides
	s.loadedDate = s.now().In(locale.BogotaLocation()).Format("2006-01-02")
	s.mu.Unlock()

	_ = s.saveCache(guides)
	return nil
}

// SetGuides directly injects channel guides (useful for testing).
func (s *Service) SetGuides(guides []ChannelGuide) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guides = guides
	s.loadedDate = s.now().In(locale.BogotaLocation()).Format("2006-01-02")
}

// Guides returns a snapshot of currently loaded channel guides.
func (s *Service) Guides() []ChannelGuide {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make([]ChannelGuide, len(s.guides))
	copy(copied, s.guides)
	return copied
}

func parseClockMinutes(tStr string) (int, bool) {
	tStr = strings.TrimSpace(strings.ToUpper(tStr))
	if tStr == "" {
		return 0, false
	}

	if strings.Contains(tStr, ":") && !strings.Contains(tStr, "AM") && !strings.Contains(tStr, "PM") {
		parts := strings.Split(tStr, ":")
		if len(parts) >= 2 {
			var h, m int
			if _, err := fmt.Sscanf(parts[0], "%d", &h); err == nil {
				if _, err := fmt.Sscanf(parts[1], "%d", &m); err == nil {
					return h*60 + m, true
				}
			}
		}
	}

	t, err := time.Parse("3:04 PM", tStr)
	if err == nil {
		return t.Hour()*60 + t.Minute(), true
	}
	t, err = time.Parse("15:04", tStr)
	if err == nil {
		return t.Hour()*60 + t.Minute(), true
	}

	return 0, false
}

// intervalsOverlap reports whether two half-open minute intervals on a 24h clock
// intersect, tolerating midnight wrap-around. Interval endpoints may extend past
// 1440; every interval is compared against shifted copies of the other.
func intervalsOverlap(s1, e1, s2, e2 int) bool {
	for _, shiftA := range [...]int{-1440, 0, 1440} {
		a1, a2 := s1+shiftA, e1+shiftA
		for _, shiftB := range [...]int{-1440, 0, 1440} {
			b1, b2 := s2+shiftB, e2+shiftB
			if a1 < b2 && b1 < a2 {
				return true
			}
		}
	}
	return false
}

// programmeOverlapsMatch reports whether a guide programme overlaps the match
// window starting at matchStartMins. It returns hasTime=false when the entry
// carries no usable start time, in which case timing must not be used to
// disqualify the candidate. Timing can only disqualify here; it never promotes a
// candidate (there are no timing bonuses).
func programmeOverlapsMatch(entry ProgramEntry, matchStartMins int) (overlaps, hasTime bool) {
	start, ok := parseClockMinutes(entry.StartTime)
	if !ok {
		return false, false
	}

	dur := defaultEntryMinutes
	if end, ok := parseClockMinutes(entry.EndTime); ok {
		dur = end - start
		if dur <= 0 {
			dur += 24 * 60
		}
	}

	return intervalsOverlap(start, start+dur, matchStartMins, matchStartMins+matchWindowMinutes), true
}

// ResolveMatch matches an ESPN match against today's TV guide listings.
//
// Acceptance is deliberately strict:
//   - A candidate is eligible only when MatchScore reports BOTH teams
//     identified or a full fixture-title match (score >= minMatchScore). A
//     single team name never qualifies. MatchScore also applies a symmetric
//     squad-qualifier gate first: the fixture title and the programme title must
//     carry the same squad qualifier (or none), so a senior fixture never
//     resolves against a youth, women's or reserve broadcast of the same clubs.
//   - Timing is a gate, not a score. When the match and programme both carry a
//     start time, the programme must overlap the match window; a programme that
//     does not overlap is disqualified. Timing never adds points, so a perfect
//     time slot can never lift a weak candidate above a correct one.
//   - If two different channels share the top score, the result is ambiguous
//     and the call declines rather than picking one. Unverified behaviour is
//     strictly better than a wrong channel. Ties are counted per canonical
//     channel identity, not per raw slug: GatoTV publishes some signals under
//     more than one slug (caracol_television / caracol_colombia), and comparing
//     slugs would make a single channel look ambiguous. Two genuinely different
//     channels still decline.
func (s *Service) ResolveMatch(match model.Match) (ChannelTarget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.guides) == 0 {
		return ChannelTarget{}, false
	}

	hasMatchTime := !match.StartTime.IsZero()
	matchMins := 0
	if hasMatchTime {
		// Guide entries publish Colombian wall-clock times, so the kickoff must be
		// read in the source zone regardless of the user's display timezone.
		sourceTime := match.StartTime.In(locale.BogotaLocation())
		matchMins = sourceTime.Hour()*60 + sourceTime.Minute()
	}

	bestScore := 0
	bestIdentities := make(map[string]struct{})
	var bestTarget ChannelTarget

	// consider records an eligible candidate. Ties across distinct channels are
	// tracked by canonical identity so an ambiguous result can be declined at the
	// end. The guide index is the last-resort identity: a guide with neither name
	// nor slug cannot be proven equal to another, and the index keeps repeated
	// entries of the same guide on one identity.
	consider := func(guide ChannelGuide, index, score int) {
		if score < minMatchScore {
			return
		}
		target := ChannelTarget{
			Name:      guide.Name,
			Slug:      guide.Slug,
			StreamURL: guide.StreamURL,
		}
		identity := guideChannelIdentity(guide)
		if identity == "" {
			identity = fmt.Sprintf("\x00guide-%d", index)
		}
		switch {
		case score > bestScore:
			bestScore = score
			bestIdentities = map[string]struct{}{identity: {}}
			bestTarget = target
		case score == bestScore:
			// Same signal under another slug: record the identity only and keep
			// the first target, so the chosen slug is deterministic.
			bestIdentities[identity] = struct{}{}
		}
	}

	for i, guide := range s.guides {
		if len(guide.Entries) > 0 {
			for _, entry := range guide.Entries {
				matched, baseScore := MatchScore(entry.Title, match)
				if !matched {
					continue
				}

				// Time gate: a programme that does not overlap the match window
				// is disqualified. If it carries no usable time, it cannot be
				// disqualified on timing and remains eligible.
				if hasMatchTime && entry.StartTime != "" {
					overlaps, hasTime := programmeOverlapsMatch(entry, matchMins)
					if hasTime && !overlaps {
						continue
					}
				}

				consider(guide, i, baseScore)
			}
			continue
		}

		// Fallback for flat program titles without time metadata. There is no
		// time to gate on, so eligibility rests entirely on MatchScore.
		for _, prog := range guide.Programs {
			matched, score := MatchScore(prog, match)
			if matched {
				consider(guide, i, score)
			}
		}
	}

	if bestScore < minMatchScore || len(bestIdentities) > 1 {
		return ChannelTarget{}, false
	}

	return bestTarget, true
}
