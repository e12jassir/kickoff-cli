package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
)

const (
	defaultBaseURL  = "https://site.api.espn.com/apis/site/v2/sports/soccer"
	defaultCacheTTL = 300 * time.Second
	defaultTimeout  = 10 * time.Second
	userAgent       = "Mozilla/5.0"
)

// Raw ESPN response structures for defensive unmarshaling.
type rawResponse struct {
	Events []rawEvent `json:"events"`
}

type rawEvent struct {
	ID           string           `json:"id"`
	Date         string           `json:"date"`
	Competitions []rawCompetition `json:"competitions"`
	Status       *rawStatus       `json:"status"`
}

type rawCompetition struct {
	ID          string          `json:"id"`
	Venue       *rawVenue       `json:"venue"`
	Competitors []rawCompetitor `json:"competitors"`
}

type rawVenue struct {
	FullName string `json:"fullName"`
}

type rawCompetitor struct {
	ID       string   `json:"id"`
	HomeAway string   `json:"homeAway"`
	Score    string   `json:"score"`
	Team     *rawTeam `json:"team"`
}

type rawTeam struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
}

type rawStatus struct {
	Type *rawStatusType `json:"type"`
}

type rawStatusType struct {
	Description string `json:"description"`
	Detail      string `json:"detail"`
	State       string `json:"state"`
	Completed   bool   `json:"completed"`
}

type cachedData struct {
	CachedAt time.Time     `json:"cached_at"`
	Matches  []model.Match `json:"matches"`
}

// Client implements model.Client for fetching ESPN soccer fixtures.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cacheDir   string
	cacheTTL   time.Duration
	loc        *time.Location
	now        func() time.Time
}

// Option configures the ESPN Client.
type Option func(*Client)

// WithBaseURL overrides the ESPN base URL (useful for testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient overrides the internal HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithCacheDir overrides the cache directory.
func WithCacheDir(dir string) Option {
	return func(c *Client) {
		c.cacheDir = dir
	}
}

// WithCacheTTL overrides the cache expiration duration.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.cacheTTL = ttl
	}
}

// WithTimezone overrides the target timezone for filtering current-day matches.
func WithTimezone(loc *time.Location) Option {
	return func(c *Client) {
		c.loc = loc
	}
}

// WithNow overrides the current time supplier (useful for deterministic tests).
func WithNow(nowFunc func() time.Time) Option {
	return func(c *Client) {
		c.now = nowFunc
	}
}

// NewClient initializes a new ESPN client with sensible defaults.
func NewClient(opts ...Option) *Client {
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cacheBase == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheBase = filepath.Join(home, ".cache")
		} else {
			cacheBase = os.TempDir()
		}
	}
	defaultCache := filepath.Join(cacheBase, "kickoff")

	// Default timezone UTC-5 (America/Bogota / Colombia)
	loc := time.FixedZone("America/Bogota", -5*3600)

	c := &Client{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cacheDir: defaultCache,
		cacheTTL: defaultCacheTTL,
		loc:      loc,
		now:      time.Now,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// LeagueCode maps a league key to the ESPN league identifier.
func LeagueCode(leagueKey string) string {
	switch strings.ToLower(leagueKey) {
	case "laliga":
		return "esp.1"
	case "premier":
		return "eng.1"
	case "seriea":
		return "ita.1"
	case "bundesliga":
		return "ger.1"
	case "ligue1":
		return "fra.1"
	case "champions":
		return "uefa.champions"
	case "libertadores":
		return "conmebol.libertadores"
	case "argentina":
		return "arg.1"
	case "colombia":
		return "col.1"
	case "brasil":
		return "bra.1"
	default:
		// Check DefaultLeagues
		for _, l := range model.DefaultLeagues {
			if strings.EqualFold(l.Key, leagueKey) || strings.EqualFold(l.Code, leagueKey) {
				return l.Code
			}
		}
		return leagueKey
	}
}

func (c *Client) cacheFilePath(leagueKey string) string {
	cleanKey := strings.ReplaceAll(leagueKey, "/", "_")
	return filepath.Join(c.cacheDir, fmt.Sprintf("matches_%s.json", cleanKey))
}

func (c *Client) getCached(leagueKey string) ([]model.Match, bool) {
	cachePath := c.cacheFilePath(leagueKey)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var cached cachedData
	if err := json.Unmarshal(data, &cached); err != nil {
		_ = os.Remove(cachePath)
		return nil, false
	}

	if c.now().Sub(cached.CachedAt) > c.cacheTTL {
		return nil, false
	}

	return cached.Matches, true
}

func (c *Client) setCache(leagueKey string, matches []model.Match) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	cachePath := c.cacheFilePath(leagueKey)
	cached := cachedData{
		CachedAt: c.now(),
		Matches:  matches,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath, data, 0o644)
}

func (c *Client) invalidateCache(leagueKey string) {
	cachePath := c.cacheFilePath(leagueKey)
	_ = os.Remove(cachePath)
}

// FetchMatches queries ESPN scoreboard API, defensive parses fixtures, and caches results.
func (c *Client) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	// Check TTL cache first
	if matches, found := c.getCached(leagueKey); found {
		return matches, nil
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/scoreboard", c.baseURL, leagueCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.invalidateCache(leagueKey)
		return nil, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.invalidateCache(leagueKey)
		return nil, fmt.Errorf("upstream espn returned status code %d", resp.StatusCode)
	}

	var raw rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		c.invalidateCache(leagueKey)
		return nil, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, err)
	}

	now := c.now().In(c.loc)
	todayStr := now.Format("2006-01-02")

	matches := make([]model.Match, 0, len(raw.Events))
	for _, ev := range raw.Events {
		if ev.Date == "" || len(ev.Competitions) == 0 {
			continue
		}

		matchTime, err := parseESPNTime(ev.Date)
		if err != nil {
			continue
		}

		localMatchTime := matchTime.In(c.loc)
		if localMatchTime.Format("2006-01-02") != todayStr {
			continue
		}

		comp := ev.Competitions[0]
		if len(comp.Competitors) == 0 {
			continue
		}

		var homeComp, awayComp model.Competitor
		hasHome, hasAway := false, false

		for _, competitor := range comp.Competitors {
			var name, dispName, logo string
			if competitor.Team != nil {
				name = competitor.Team.Name
				dispName = competitor.Team.DisplayName
				logo = competitor.Team.Logo
			}
			if dispName == "" {
				dispName = name
			}
			if dispName == "" {
				dispName = "Unknown"
			}

			cModel := model.Competitor{
				ID:          competitor.ID,
				Name:        name,
				DisplayName: dispName,
				Logo:        logo,
				Score:       competitor.Score,
			}

			if strings.EqualFold(competitor.HomeAway, "home") {
				cModel.IsHome = true
				homeComp = cModel
				hasHome = true
			} else {
				cModel.IsHome = false
				awayComp = cModel
				hasAway = true
			}
		}

		// Ensure we have at least partial competitor names
		if !hasHome {
			homeComp = model.Competitor{DisplayName: "Local", IsHome: true}
		}
		if !hasAway {
			awayComp = model.Competitor{DisplayName: "Visitante", IsHome: false}
		}

		venueName := "Estadio Principal"
		if comp.Venue != nil && comp.Venue.FullName != "" {
			venueName = comp.Venue.FullName
		}

		status := model.MatchStatus{
			Description: "Programado",
			State:       "pre",
		}
		if ev.Status != nil && ev.Status.Type != nil {
			if ev.Status.Type.Description != "" {
				status.Description = ev.Status.Type.Description
			}
			status.Detail = ev.Status.Type.Detail
			status.State = ev.Status.Type.State
			status.Completed = ev.Status.Type.Completed
		}

		match := model.Match{
			ID:        ev.ID,
			Title:     fmt.Sprintf("%s vs %s", homeComp.DisplayName, awayComp.DisplayName),
			HomeTeam:  homeComp,
			AwayTeam:  awayComp,
			StartTime: localMatchTime,
			TimeStr:   localMatchTime.Format("15:04"),
			Venue:     venueName,
			Status:    status,
			LeagueKey: leagueKey,
		}

		matches = append(matches, match)
	}

	// Sort matches chronologically by StartTime
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].StartTime.Before(matches[j].StartTime)
	})

	c.setCache(leagueKey, matches)
	return matches, nil
}

func parseESPNTime(s string) (time.Time, error) {
	// Try standard RFC3339 formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04-0700",
		"2006-01-02T15:04:05-0700",
	}

	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}
