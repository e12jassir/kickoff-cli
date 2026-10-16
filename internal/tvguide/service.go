package tvguide

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	DefaultCacheTTL = 4 * time.Hour
)

// Service coordinates fetching, caching, and matching of TV programming.
type Service struct {
	crawler    Crawler
	cacheDir   string
	cacheTTL   time.Duration
	now        func() time.Time
	loc        *time.Location
	targets    []ChannelTarget
	httpClient *http.Client
	baseURL    string

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

// WithTimezone overrides the local timezone.
func WithTimezone(loc *time.Location) Option {
	return func(s *Service) {
		s.loc = loc
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

	loc := time.FixedZone("America/Bogota", -5*3600)

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

	// 1. Attempt zero-delay load from disk cache (in production or when custom cacheDir is configured)
	var loaded bool
	if flag.Lookup("test.v") == nil || s.cacheDir != defaultCache {
		loaded = s.loadCache()
	}

	// 2. If cache is not fresh and not executing unit tests, kick off background crawl
	if !loaded && flag.Lookup("test.v") == nil {
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

// loadCache loads today's guide from disk if present and within TTL.
func (s *Service) loadCache() bool {
	todayStr := s.now().In(s.loc).Format("2006-01-02")
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
	todayStr := s.now().In(s.loc).Format("2006-01-02")
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

	todayStr := s.now().In(s.loc).Format("2006-01-02")
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

	return os.WriteFile(filePath, data, 0o644)
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
	s.loadedDate = s.now().In(s.loc).Format("2006-01-02")
	s.mu.Unlock()

	_ = s.saveCache(guides)
	return nil
}

// SetGuides directly injects channel guides (useful for testing).
func (s *Service) SetGuides(guides []ChannelGuide) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guides = guides
	s.loadedDate = s.now().In(s.loc).Format("2006-01-02")
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

// ResolveMatch matches an ESPN match against today's TV guide listings.
// If multiple channels match, the entry with the highest confidence score and
// temporal alignment to the match start time is selected, discarding distant replays.
func (s *Service) ResolveMatch(match model.Match) (ChannelTarget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.guides) == 0 {
		return ChannelTarget{}, false
	}

	hasMatchTime := !match.StartTime.IsZero()
	matchMins := 0
	if hasMatchTime {
		localTime := match.StartTime.In(s.loc)
		matchMins = localTime.Hour()*60 + localTime.Minute()
	}

	bestScore := 0
	var bestTarget ChannelTarget
	found := false

	for _, guide := range s.guides {
		if len(guide.Entries) > 0 {
			for _, entry := range guide.Entries {
				matched, baseScore := MatchScore(entry.Title, match)
				if !matched {
					continue
				}

				effectiveScore := baseScore

				if hasMatchTime && entry.StartTime != "" {
					entryMins, ok := parseClockMinutes(entry.StartTime)
					if ok {
						diff := entryMins - matchMins
						if diff < 0 {
							diff = -diff
						}
						if diff > 12*60 {
							diff = 24*60 - diff
						}

						// Disqualify replays scheduled far away (>2.5h) when not live now
						if diff > 150 && !entry.IsLiveNow {
							continue
						}

						if diff <= 60 {
							effectiveScore += 6
						} else if diff <= 120 {
							effectiveScore += 3
						}
					}
				}

				if entry.IsLiveNow {
					effectiveScore += 4
				}

				if effectiveScore > bestScore {
					bestScore = effectiveScore
					bestTarget = ChannelTarget{
						Name:      guide.Name,
						Slug:      guide.Slug,
						StreamURL: guide.StreamURL,
					}
					found = true
				}
			}
			continue
		}

		// Fallback for flat program titles without time metadata
		for _, prog := range guide.Programs {
			matched, score := MatchScore(prog, match)
			if matched && score > bestScore {
				bestScore = score
				bestTarget = ChannelTarget{
					Name:      guide.Name,
					Slug:      guide.Slug,
					StreamURL: guide.StreamURL,
				}
				found = true
				if score >= 10 {
					break
				}
			}
		}
	}

	if found && bestScore >= 2 {
		return bestTarget, true
	}

	return ChannelTarget{}, false
}
