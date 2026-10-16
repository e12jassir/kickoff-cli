package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	defaultBaseURL  = "https://site.api.espn.com/apis/site/v2/sports/soccer"
	defaultCacheTTL = 30 * time.Second
	liveCacheTTL    = 5 * time.Second
	defaultTimeout  = 10 * time.Second
	userAgent       = "kickoff-cli/1.0 (+https://github.com/e12jassir/kickoff-cli)"
)

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
	ID            string            `json:"id"`
	Venue         *rawVenue         `json:"venue"`
	Competitors   []rawCompetitor   `json:"competitors"`
	Details       []rawDetail       `json:"details"`
	Attendance    int               `json:"attendance"`
	Officials     []rawOfficial     `json:"officials"`
	Broadcasts    []rawBroadcast    `json:"broadcasts"`
	GeoBroadcasts []rawGeoBroadcast `json:"geoBroadcasts"`
}

type rawBroadcast struct {
	Market string   `json:"market"`
	Names  []string `json:"names"`
}

type rawGeoBroadcast struct {
	Media *struct {
		ShortName string `json:"shortName"`
	} `json:"media"`
}

type rawDetail struct {
	Type             *rawDetailType  `json:"type"`
	Clock            *rawDetailClock `json:"clock"`
	Team             *rawDetailTeam  `json:"team"`
	ScoreValue       int             `json:"scoreValue"`
	ScoringPlay      bool            `json:"scoringPlay"`
	RedCard          bool            `json:"redCard"`
	YellowCard       bool            `json:"yellowCard"`
	PenaltyKick      bool            `json:"penaltyKick"`
	OwnGoal          bool            `json:"ownGoal"`
	AthletesInvolved []rawAthlete    `json:"athletesInvolved"`
}

type rawDetailType struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type rawDetailClock struct {
	Value        float64 `json:"value"`
	DisplayValue string  `json:"displayValue"`
}

type rawDetailTeam struct {
	ID string `json:"id"`
}

type rawAthlete struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	ShortName   string `json:"shortName"`
}

type rawSummaryResponse struct {
	Boxscore *rawBoxscore `json:"boxscore"`
	Rosters  []rawRoster  `json:"rosters"`
	GameInfo *rawGameInfo `json:"gameInfo"`
}

type rawGameInfo struct {
	Attendance int           `json:"attendance"`
	Officials  []rawOfficial `json:"officials"`
	Coaches    []rawCoach    `json:"coaches"`
}

type rawCoach struct {
	FirstName   string      `json:"firstName"`
	LastName    string      `json:"lastName"`
	DisplayName string      `json:"displayName"`
	Athlete     *rawAthlete `json:"athlete"`
}

type rawOfficial struct {
	DisplayName string `json:"displayName"`
}

type rawRoster struct {
	Team      *rawTeam        `json:"team"`
	Formation string          `json:"formation"`
	Roster    []rawRosterItem `json:"roster"`
	Coaches   []rawCoach      `json:"coaches"`
}

type rawRosterItem struct {
	Starter  bool        `json:"starter"`
	Jersey   string      `json:"jersey"`
	Position *rawPos     `json:"position"`
	Athlete  *rawAthlete `json:"athlete"`
	Role     string      `json:"role"`
}

type rawPos struct {
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
}

type rawBoxscore struct {
	Teams []rawBoxscoreTeam `json:"teams"`
}

type rawBoxscoreTeam struct {
	Team       *rawTeam           `json:"team"`
	HomeAway   string             `json:"homeAway"`
	Statistics []rawStatisticItem `json:"statistics"`
}

type rawStatisticItem struct {
	Name         string  `json:"name"`
	DisplayValue string  `json:"displayValue"`
	Value        float64 `json:"value"`
}

type cachedSummary struct {
	CachedAt   time.Time           `json:"cached_at"`
	Stats      *model.MatchStats   `json:"stats"`
	Lineups    *model.MatchLineups `json:"lineups"`
	Referee    string              `json:"referee"`
	Attendance int                 `json:"attendance"`
	HomeCoach  string              `json:"home_coach,omitempty"`
	AwayCoach  string              `json:"away_coach,omitempty"`
}

type cachedStandings struct {
	CachedAt  time.Time              `json:"cached_at"`
	Standings *model.LeagueStandings `json:"standings"`
}

type cachedStatistics struct {
	CachedAt   time.Time               `json:"cached_at"`
	Statistics *model.LeagueStatistics `json:"statistics"`
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
	baseURL      string
	httpClient   *http.Client
	cacheDir     string
	cacheTTL     time.Duration
	loc          *time.Location
	now          func() time.Time
	retryBackoff func() time.Duration
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(url, "/")
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func WithCacheDir(dir string) Option {
	return func(c *Client) {
		c.cacheDir = dir
	}
}

func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.cacheTTL = ttl
	}
}

func WithTimezone(loc *time.Location) Option {
	return func(c *Client) {
		c.loc = loc
	}
}

func WithNow(nowFunc func() time.Time) Option {
	return func(c *Client) {
		c.now = nowFunc
	}
}

func WithRetryBackoff(fn func() time.Duration) Option {
	return func(c *Client) {
		c.retryBackoff = fn
	}
}

func NewClient(opts ...Option) *Client {
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

	c := &Client{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cacheDir: defaultCache,
		cacheTTL: defaultCacheTTL,
		loc:      loc,
		now:      time.Now,
		retryBackoff: func() time.Duration {
			return time.Duration(200+rand.IntN(301)) * time.Millisecond
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// doWithRetry executes HTTP request with transient retry and backoff + jitter.
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	const maxRetries = 2
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		clonedReq := req.Clone(req.Context())
		resp, err = c.httpClient.Do(clonedReq)
		if err == nil {
			return resp, nil
		}

		if req.Context().Err() != nil {
			return nil, err
		}

		if attempt < maxRetries {
			backoff := c.retryBackoff()
			if backoff > 0 {
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(backoff):
				}
			}
		}
	}
	return nil, err
}

// WarmupCache concurrently pre-fetches all given leagues in the background.
func (c *Client) WarmupCache(ctx context.Context, leagues []model.League) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, lg := range leagues {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			_, _ = c.FetchMatches(reqCtx, key)
		}(lg.Key)
	}
	wg.Wait()
}

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
	case "europa":
		return "uefa.europa"
	case "conference":
		return "uefa.europa.conf"
	case "libertadores":
		return "conmebol.libertadores"
	case "sudamericana":
		return "conmebol.sudamericana"
	case "eliminatorias":
		return "fifa.worldq.conmebol"
	case "copadelrey":
		return "esp.copa_del_rey"
	case "facup":
		return "eng.fa"
	case "argentina":
		return "arg.1"
	case "colombia":
		return "col.1"
	case "brasil":
		return "bra.1"
	case "ligamx":
		return "mex.1"
	case "mls":
		return "usa.1"
	default:
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

func (c *Client) readDiskCache(leagueKey string) (*cachedData, bool) {
	cachePath := c.cacheFilePath(leagueKey)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var cached cachedData
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}

	return &cached, true
}

func (c *Client) getCached(leagueKey string) ([]model.Match, bool) {
	cached, ok := c.readDiskCache(leagueKey)
	if !ok {
		return nil, false
	}

	effectiveTTL := c.cacheTTL
	for _, m := range cached.Matches {
		if strings.EqualFold(m.Status.State, "in") {
			if liveCacheTTL < effectiveTTL {
				effectiveTTL = liveCacheTTL
			}
			break
		}
	}

	if c.now().Sub(cached.CachedAt) > effectiveTTL {
		return nil, false
	}

	return cached.Matches, true
}

func (c *Client) fallbackMatches(leagueKey string, origErr error) ([]model.Match, error) {
	if cached, ok := c.readDiskCache(leagueKey); ok && len(cached.Matches) > 0 {
		matches := make([]model.Match, len(cached.Matches))
		for i, m := range cached.Matches {
			m.IsOffline = true
			if c.loc != nil {
				m.CachedAt = cached.CachedAt.In(c.loc)
			} else {
				m.CachedAt = cached.CachedAt
			}
			matches[i] = m
		}
		return matches, nil
	}
	return nil, origErr
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

func (c *Client) summaryCachePath(eventID string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("summary-%s.json", eventID))
}

func (c *Client) readDiskSummary(eventID string) (*cachedSummary, bool) {
	cachePath := c.summaryCachePath(eventID)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var cached cachedSummary
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}

	return &cached, true
}

func (c *Client) getCachedSummary(eventID string) (*cachedSummary, bool) {
	cached, ok := c.readDiskSummary(eventID)
	if !ok {
		return nil, false
	}

	if c.now().Sub(cached.CachedAt) > c.cacheTTL {
		return nil, false
	}

	return cached, true
}

func (c *Client) fallbackSummary(eventID string, origErr error) (*model.MatchDetails, error) {
	if cached, ok := c.readDiskSummary(eventID); ok {
		cachedAt := cached.CachedAt
		if c.loc != nil {
			cachedAt = cachedAt.In(c.loc)
		}
		return &model.MatchDetails{
			Stats:      cached.Stats,
			Lineups:    cached.Lineups,
			Referee:    cached.Referee,
			Attendance: cached.Attendance,
			HomeCoach:  cached.HomeCoach,
			AwayCoach:  cached.AwayCoach,
			IsOffline:  true,
			CachedAt:   cachedAt,
		}, nil
	}
	return nil, origErr
}

func (c *Client) setCachedSummary(eventID string, stats *model.MatchStats, lineups *model.MatchLineups, referee string, attendance int, homeCoach, awayCoach string) {
	if stats == nil && lineups == nil && referee == "" && attendance == 0 && homeCoach == "" && awayCoach == "" {
		return
	}
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	cachePath := c.summaryCachePath(eventID)
	cached := cachedSummary{
		CachedAt:   c.now(),
		Stats:      stats,
		Lineups:    lineups,
		Referee:    referee,
		Attendance: attendance,
		HomeCoach:  homeCoach,
		AwayCoach:  awayCoach,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = os.WriteFile(cachePath, data, 0o644)
}

// FetchMatches queries ESPN scoreboard API, extracts official TV broadcasts, and caches results.
func (c *Client) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	if !model.IsForceRefresh(ctx) {
		if matches, found := c.getCached(leagueKey); found {
			return matches, nil
		}
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/scoreboard", c.baseURL, leagueCode)
	if strings.Contains(c.baseURL, "espn.com") {
		endpoint += "?region=co&lang=es"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return c.fallbackMatches(leagueKey, fmt.Errorf("network request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackMatches(leagueKey, fmt.Errorf("upstream espn returned status code %d", resp.StatusCode))
	}

	var raw rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.fallbackMatches(leagueKey, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, err))
	}

	now := c.now().In(c.loc)
	todayStr := now.Format("2006-01-02")

	parseEvent := func(ev rawEvent, matchTime time.Time, localMatchTime time.Time) (model.Match, bool) {
		comp := ev.Competitions[0]
		if len(comp.Competitors) == 0 {
			return model.Match{}, false
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

		var events []model.MatchEvent
		for _, d := range comp.Details {
			if !d.ScoringPlay && !d.OwnGoal && !d.RedCard && !d.YellowCard {
				continue
			}

			pName := "Jugador"
			if len(d.AthletesInvolved) > 0 {
				if d.AthletesInvolved[0].DisplayName != "" {
					pName = d.AthletesInvolved[0].DisplayName
				} else if d.AthletesInvolved[0].ShortName != "" {
					pName = d.AthletesInvolved[0].ShortName
				}
			}

			var clockVal string
			var clockSec float64
			if d.Clock != nil {
				clockVal = d.Clock.DisplayValue
				clockSec = d.Clock.Value
			}

			var teamID string
			if d.Team != nil {
				teamID = d.Team.ID
			}

			var desc string
			if d.Type != nil {
				desc = d.Type.Text
			}

			evType := "goal"
			if d.RedCard {
				evType = "card_red"
			} else if d.YellowCard {
				evType = "card_yellow"
			} else if d.OwnGoal {
				evType = "owngoal"
			} else if d.PenaltyKick {
				evType = "penalty"
			}

			events = append(events, model.MatchEvent{
				Type:        evType,
				Description: desc,
				Clock:       clockVal,
				ClockValue:  clockSec,
				TeamID:      teamID,
				PlayerName:  pName,
				IsScoring:   d.ScoringPlay,
				IsPenalty:   d.PenaltyKick,
				IsOwnGoal:   d.OwnGoal,
				IsRedCard:   d.RedCard,
				IsYellow:    d.YellowCard,
			})
		}

		ref := ""
		if len(comp.Officials) > 0 {
			ref = comp.Officials[0].DisplayName
		}

		// Extract official TV/stream broadcasters from ESPN payload
		var broadcasts []string
		seenB := make(map[string]bool)
		addBroadcast := func(bName string) {
			bName = strings.TrimSpace(bName)
			if bName == "" {
				return
			}
			key := strings.ToLower(bName)
			if !seenB[key] {
				seenB[key] = true
				broadcasts = append(broadcasts, bName)
			}
		}
		for _, b := range comp.Broadcasts {
			for _, n := range b.Names {
				addBroadcast(n)
			}
		}
		for _, gb := range comp.GeoBroadcasts {
			if gb.Media != nil {
				addBroadcast(gb.Media.ShortName)
			}
		}

		match := model.Match{
			ID:         ev.ID,
			Title:      fmt.Sprintf("%s vs %s", homeComp.DisplayName, awayComp.DisplayName),
			HomeTeam:   homeComp,
			AwayTeam:   awayComp,
			StartTime:  localMatchTime,
			TimeStr:    localMatchTime.Format("15:04"),
			Venue:      venueName,
			Status:     status,
			LeagueKey:  leagueKey,
			Events:     events,
			Referee:    ref,
			Attendance: comp.Attendance,
			Broadcasts: broadcasts,
		}
		return match, true
	}

	matches := make([]model.Match, 0, len(raw.Events))
	allParsedMatches := make([]model.Match, 0, len(raw.Events))

	seenEventIDs := make(map[string]bool)

	for _, ev := range raw.Events {
		if ev.Date == "" || len(ev.Competitions) == 0 || seenEventIDs[ev.ID] {
			continue
		}

		matchTime, err := parseESPNTime(ev.Date)
		if err != nil {
			continue
		}

		localMatchTime := matchTime.In(c.loc)
		m, ok := parseEvent(ev, matchTime, localMatchTime)
		if !ok {
			continue
		}
		seenEventIDs[ev.ID] = true
		allParsedMatches = append(allParsedMatches, m)

		if localMatchTime.Format("2006-01-02") == todayStr {
			matches = append(matches, m)
		}
	}

	// Check if allParsedMatches contains any future or live matches:
	hasActiveInDefault := false
	for _, m := range allParsedMatches {
		if m.Status.State == "in" || m.Status.State == "pre" {
			hasActiveInDefault = true
			break
		}
	}

	// Probe recent past days (1-4 days) to complete the past matchday (e.g. Barcelona on Tuesday when default only has Wednesday)
	pastEvents := c.fetchRecentPastScoreboardEvents(ctx, leagueCode, now)
	for _, ev := range pastEvents {
		if ev.Date == "" || len(ev.Competitions) == 0 || seenEventIDs[ev.ID] {
			continue
		}
		matchTime, err := parseESPNTime(ev.Date)
		if err != nil {
			continue
		}
		localMatchTime := matchTime.In(c.loc)
		m, ok := parseEvent(ev, matchTime, localMatchTime)
		if ok {
			seenEventIDs[ev.ID] = true
			allParsedMatches = append(allParsedMatches, m)
		}
	}

	// If the default scoreboard only returned past finished matches, probe for the next upcoming matchday!
	if len(matches) == 0 && !hasActiveInDefault && len(allParsedMatches) > 0 {
		upcomingEvents := c.fetchUpcomingScoreboardEvents(ctx, leagueCode, now)
		for _, ev := range upcomingEvents {
			if ev.Date == "" || len(ev.Competitions) == 0 || seenEventIDs[ev.ID] {
				continue
			}
			matchTime, err := parseESPNTime(ev.Date)
			if err != nil {
				continue
			}
			localMatchTime := matchTime.In(c.loc)
			m, ok := parseEvent(ev, matchTime, localMatchTime)
			if ok {
				seenEventIDs[ev.ID] = true
				allParsedMatches = append(allParsedMatches, m)
			}
		}
	}

	if len(matches) == 0 && len(allParsedMatches) > 0 {
		matches = allParsedMatches
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].StartTime.Before(matches[j].StartTime)
	})

	// Assign collision-free SlotIndex for simultaneous matches so they don't all map to ESPN 1
	for i := range matches {
		slot := 0
		for j := 0; j < i; j++ {
			diff := matches[i].StartTime.Sub(matches[j].StartTime)
			if diff < 0 {
				diff = -diff
			}
			if diff <= 90*time.Minute {
				slot++
			}
		}
		matches[i].SlotIndex = slot
	}

	c.setCache(leagueKey, matches)
	return matches, nil
}

// fetchRecentPastScoreboardEvents probes dates 1-4 days into the past to capture the full recent matchday round.
func (c *Client) fetchRecentPastScoreboardEvents(ctx context.Context, leagueCode string, now time.Time) []rawEvent {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	type probeRes struct {
		dayOffset int
		events    []rawEvent
	}

	resChan := make(chan probeRes, 4)
	var wg sync.WaitGroup

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	for d := 1; d <= 4; d++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			select {
			case <-probeCtx.Done():
				return
			default:
			}

			dateStr := now.AddDate(0, 0, -offset).Format("20060102")
			endpoint := fmt.Sprintf("%s/%s/scoreboard?dates=%s", c.baseURL, leagueCode, dateStr)
			if strings.Contains(c.baseURL, "espn.com") {
				endpoint += "&region=co&lang=es"
			}

			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", userAgent)

			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return
			}

			var raw rawResponse
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				return
			}

			if len(raw.Events) > 0 {
				resChan <- probeRes{dayOffset: offset, events: raw.Events}
			}
		}(d)
	}

	wg.Wait()
	close(resChan)

	var pastEvents []rawEvent
	for r := range resChan {
		pastEvents = append(pastEvents, r.events...)
	}
	return pastEvents
}

// fetchUpcomingScoreboardEvents probes dates 1-14 days into the future to find the next active matchday.
func (c *Client) fetchUpcomingScoreboardEvents(ctx context.Context, leagueCode string, now time.Time) []rawEvent {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	type probeRes struct {
		dayOffset int
		events    []rawEvent
	}

	resChan := make(chan probeRes, 14)
	var wg sync.WaitGroup

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	for d := 1; d <= 14; d++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			select {
			case <-probeCtx.Done():
				return
			default:
			}

			dateStr := now.AddDate(0, 0, offset).Format("20060102")
			endpoint := fmt.Sprintf("%s/%s/scoreboard?dates=%s", c.baseURL, leagueCode, dateStr)
			if strings.Contains(c.baseURL, "espn.com") {
				endpoint += "&region=co&lang=es"
			}

			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", userAgent)

			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return
			}

			var raw rawResponse
			if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
				return
			}

			if len(raw.Events) > 0 {
				resChan <- probeRes{dayOffset: offset, events: raw.Events}
			}
		}(d)
	}

	wg.Wait()
	close(resChan)

	var allRes []probeRes
	for r := range resChan {
		allRes = append(allRes, r)
	}

	sort.Slice(allRes, func(i, j int) bool {
		return allRes[i].dayOffset < allRes[j].dayOffset
	})

	if len(allRes) == 0 {
		return nil
	}

	earliestOffset := allRes[0].dayOffset
	var upcomingEvents []rawEvent
	for _, r := range allRes {
		if r.dayOffset <= earliestOffset+2 {
			upcomingEvents = append(upcomingEvents, r.events...)
		}
	}
	return upcomingEvents
}

func parseESPNTime(s string) (time.Time, error) {
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

func (c *Client) FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*model.MatchDetails, error) {
	if eventID == "" {
		return nil, nil
	}

	if !model.IsForceRefresh(ctx) {
		if cached, found := c.getCachedSummary(eventID); found {
			cachedAt := cached.CachedAt
			if c.loc != nil {
				cachedAt = cachedAt.In(c.loc)
			}
			return &model.MatchDetails{
				Stats:      cached.Stats,
				Lineups:    cached.Lineups,
				Referee:    cached.Referee,
				Attendance: cached.Attendance,
				IsOffline:  false,
				CachedAt:   cachedAt,
			}, nil
		}
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/summary?event=%s", c.baseURL, leagueCode, eventID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return c.fallbackSummary(eventID, fmt.Errorf("network request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackSummary(eventID, fmt.Errorf("upstream espn returned status code %d", resp.StatusCode))
	}

	var raw rawSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.fallbackSummary(eventID, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, err))
	}

	var stats *model.MatchStats
	var lineups *model.MatchLineups

	if raw.Boxscore != nil && len(raw.Boxscore.Teams) > 0 {
		var homeStats, awayStats []rawStatisticItem
		if len(raw.Boxscore.Teams) >= 2 {
			t0 := raw.Boxscore.Teams[0]
			t1 := raw.Boxscore.Teams[1]
			if strings.EqualFold(t0.HomeAway, "away") || strings.EqualFold(t1.HomeAway, "home") {
				homeStats = t1.Statistics
				awayStats = t0.Statistics
			} else {
				homeStats = t0.Statistics
				awayStats = t1.Statistics
			}
		} else if len(raw.Boxscore.Teams) == 1 {
			homeStats = raw.Boxscore.Teams[0].Statistics
		}

		if len(homeStats) > 0 || len(awayStats) > 0 {
			extractStat := func(sList []rawStatisticItem, names ...string) string {
				for _, name := range names {
					for _, s := range sList {
						if strings.EqualFold(s.Name, name) {
							return s.DisplayValue
						}
					}
				}
				return ""
			}

			formatPct := func(s string) string {
				s = strings.TrimSpace(s)
				if s == "" {
					return ""
				}
				if strings.HasSuffix(s, "%") {
					return s
				}
				var f float64
				if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
					return fmt.Sprintf("%.0f%%", f)
				}
				return s + "%"
			}

			formatPossession := func(s string) string {
				val := formatPct(s)
				if val == "" {
					return "-"
				}
				return val
			}

			defaultVal := func(s, def string) string {
				s = strings.TrimSpace(s)
				if s == "" {
					return def
				}
				return s
			}

			stats = &model.MatchStats{
				HomePossession:  formatPossession(extractStat(homeStats, "possessionPct")),
				AwayPossession:  formatPossession(extractStat(awayStats, "possessionPct")),
				HomeShots:       defaultVal(extractStat(homeStats, "totalShots"), "0"),
				AwayShots:       defaultVal(extractStat(awayStats, "totalShots"), "0"),
				HomeShotsOnGoal: defaultVal(extractStat(homeStats, "shotsOnTarget"), "0"),
				AwayShotsOnGoal: defaultVal(extractStat(awayStats, "shotsOnTarget"), "0"),
				HomeCorners:     defaultVal(extractStat(homeStats, "wonCorners"), "0"),
				AwayCorners:     defaultVal(extractStat(awayStats, "wonCorners"), "0"),
				HomeFouls:       defaultVal(extractStat(homeStats, "foulsCommitted"), "0"),
				AwayFouls:       defaultVal(extractStat(awayStats, "foulsCommitted"), "0"),
				HomePasses:      defaultVal(extractStat(homeStats, "totalPasses", "passes"), "0"),
				AwayPasses:      defaultVal(extractStat(awayStats, "totalPasses", "passes"), "0"),
				HomePassPct:     formatPct(extractStat(homeStats, "passPct", "passingPct")),
				AwayPassPct:     formatPct(extractStat(awayStats, "passPct", "passingPct")),
				HomeOffsides:    defaultVal(extractStat(homeStats, "offsides", "offside"), "0"),
				AwayOffsides:    defaultVal(extractStat(awayStats, "offsides", "offside"), "0"),
				HomeSaves:       defaultVal(extractStat(homeStats, "saves", "goalieSaves"), "0"),
				AwaySaves:       defaultVal(extractStat(awayStats, "saves", "goalieSaves"), "0"),
			}
		}
	}

	if len(raw.Rosters) > 0 {
		var homeRoster, awayRoster *rawRoster
		var homeTeamID, awayTeamID string

		if raw.Boxscore != nil {
			for _, t := range raw.Boxscore.Teams {
				if t.Team != nil {
					if strings.EqualFold(t.HomeAway, "home") {
						homeTeamID = t.Team.ID
					} else if strings.EqualFold(t.HomeAway, "away") {
						awayTeamID = t.Team.ID
					}
				}
			}
		}

		if homeTeamID != "" && awayTeamID != "" {
			for i := range raw.Rosters {
				r := &raw.Rosters[i]
				if r.Team != nil {
					if r.Team.ID == homeTeamID {
						homeRoster = r
					} else if r.Team.ID == awayTeamID {
						awayRoster = r
					}
				}
			}
		}

		if homeRoster == nil && len(raw.Rosters) > 0 {
			homeRoster = &raw.Rosters[0]
		}
		if awayRoster == nil && len(raw.Rosters) > 1 {
			awayRoster = &raw.Rosters[1]
		}

		parseLineup := func(r *rawRoster) model.TeamLineup {
			if r == nil {
				return model.TeamLineup{}
			}
			var starters, bench []model.Player
			for _, item := range r.Roster {
				p := model.Player{
					Jersey:    item.Jersey,
					IsStarter: item.Starter,
				}
				if item.Athlete != nil {
					if item.Athlete.DisplayName != "" {
						p.DisplayName = item.Athlete.DisplayName
					} else if item.Athlete.ShortName != "" {
						p.DisplayName = item.Athlete.ShortName
					}
				}
				if item.Position != nil {
					p.Position = item.Position.Abbreviation
				}
				if item.Starter {
					starters = append(starters, p)
				} else {
					bench = append(bench, p)
				}
			}

			if len(starters) == 0 && len(r.Roster) > 0 {
				limit := 11
				if len(r.Roster) < limit {
					limit = len(r.Roster)
				}
				for i := 0; i < limit; i++ {
					p := model.Player{
						Jersey:    r.Roster[i].Jersey,
						IsStarter: true,
					}
					if r.Roster[i].Athlete != nil {
						p.DisplayName = r.Roster[i].Athlete.DisplayName
					}
					if r.Roster[i].Position != nil {
						p.Position = r.Roster[i].Position.Abbreviation
					}
					starters = append(starters, p)
				}
				for i := limit; i < len(r.Roster); i++ {
					p := model.Player{
						Jersey:    r.Roster[i].Jersey,
						IsStarter: false,
					}
					if r.Roster[i].Athlete != nil {
						p.DisplayName = r.Roster[i].Athlete.DisplayName
					}
					if r.Roster[i].Position != nil {
						p.Position = r.Roster[i].Position.Abbreviation
					}
					bench = append(bench, p)
				}
			}

			coachName := ""
			if len(r.Coaches) > 0 {
				if r.Coaches[0].DisplayName != "" {
					coachName = r.Coaches[0].DisplayName
				} else if r.Coaches[0].Athlete != nil && r.Coaches[0].Athlete.DisplayName != "" {
					coachName = r.Coaches[0].Athlete.DisplayName
				} else if r.Coaches[0].FirstName != "" || r.Coaches[0].LastName != "" {
					coachName = strings.TrimSpace(r.Coaches[0].FirstName + " " + r.Coaches[0].LastName)
				}
			}
			if coachName == "" {
				for _, item := range r.Roster {
					isCoach := strings.EqualFold(item.Role, "coach") || strings.EqualFold(item.Role, "manager")
					if !isCoach && item.Position != nil {
						posStr := strings.ToLower(item.Position.Abbreviation + " " + item.Position.Name + " " + item.Position.DisplayName)
						if strings.Contains(posStr, "coach") || strings.Contains(posStr, "manager") || strings.Contains(posStr, "dt") {
							isCoach = true
						}
					}
					if isCoach && item.Athlete != nil {
						coachName = item.Athlete.DisplayName
						if coachName == "" {
							coachName = item.Athlete.ShortName
						}
						break
					}
				}
			}

			return model.TeamLineup{
				Formation: r.Formation,
				Starters:  starters,
				Bench:     bench,
				Coach:     coachName,
			}
		}

		homeLineup := parseLineup(homeRoster)
		awayLineup := parseLineup(awayRoster)

		lineups = &model.MatchLineups{
			Home:      homeLineup,
			Away:      awayLineup,
			HomeCoach: homeLineup.Coach,
			AwayCoach: awayLineup.Coach,
		}
	}

	homeCoach := ""
	awayCoach := ""
	if lineups != nil {
		homeCoach = lineups.HomeCoach
		awayCoach = lineups.AwayCoach
	}

	referee := ""
	attendance := 0
	if raw.GameInfo != nil {
		attendance = raw.GameInfo.Attendance
		if len(raw.GameInfo.Officials) > 0 {
			referee = raw.GameInfo.Officials[0].DisplayName
		}
		if homeCoach == "" && len(raw.GameInfo.Coaches) > 0 {
			homeCoach = raw.GameInfo.Coaches[0].DisplayName
		}
		if awayCoach == "" && len(raw.GameInfo.Coaches) > 1 {
			awayCoach = raw.GameInfo.Coaches[1].DisplayName
		}
	}

	c.setCachedSummary(eventID, stats, lineups, referee, attendance, homeCoach, awayCoach)
	now := c.now()
	if c.loc != nil {
		now = now.In(c.loc)
	}
	return &model.MatchDetails{
		Stats:      stats,
		Lineups:    lineups,
		Referee:    referee,
		Attendance: attendance,
		HomeCoach:  homeCoach,
		AwayCoach:  awayCoach,
		IsOffline:  false,
		CachedAt:   now,
	}, nil
}

func (c *Client) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	details, err := c.FetchMatchDetails(ctx, leagueKey, eventID)
	if err != nil || details == nil {
		return nil, err
	}
	return details.Stats, nil
}

// Standings JSON parsing structures
type rawStandingsResponse struct {
	Name     string               `json:"name"`
	Children []rawStandingsChild  `json:"children"`
	Standings *rawStandingsWrapper `json:"standings"`
}

type rawStandingsChild struct {
	Name      string               `json:"name"`
	Standings *rawStandingsWrapper `json:"standings"`
}

type rawStandingsWrapper struct {
	Entries []rawStandingsEntry `json:"entries"`
}

type rawStandingsEntry struct {
	Team  *rawTeam              `json:"team"`
	Stats []rawStatisticItem    `json:"stats"`
}

func (c *Client) standingsCachePath(leagueKey string) string {
	cleanKey := strings.ReplaceAll(leagueKey, "/", "_")
	return filepath.Join(c.cacheDir, fmt.Sprintf("standings_%s.json", cleanKey))
}

func (c *Client) readDiskStandings(leagueKey string) (*cachedStandings, bool) {
	data, err := os.ReadFile(c.standingsCachePath(leagueKey))
	if err != nil {
		return nil, false
	}
	var cached cachedStandings
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}
	return &cached, true
}

func (c *Client) setCachedStandings(leagueKey string, standings *model.LeagueStandings) {
	if standings == nil {
		return
	}
	_ = os.MkdirAll(c.cacheDir, 0o755)
	cached := cachedStandings{
		CachedAt:  c.now(),
		Standings: standings,
	}
	data, err := json.Marshal(cached)
	if err == nil {
		_ = os.WriteFile(c.standingsCachePath(leagueKey), data, 0o644)
	}
}

// FetchStandings retrieves the league standings table from ESPN v2 API.
func (c *Client) FetchStandings(ctx context.Context, leagueKey string) (*model.LeagueStandings, error) {
	if !model.IsForceRefresh(ctx) {
		if cached, ok := c.readDiskStandings(leagueKey); ok {
			if c.now().Sub(cached.CachedAt) <= c.cacheTTL {
				return cached.Standings, nil
			}
		}
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/standings", c.baseURL, leagueCode)
	if strings.Contains(c.baseURL, "apis/site/v2/sports/soccer") {
		// Replace site/v2 with v2 for official ESPN API structure
		endpoint = strings.Replace(endpoint, "apis/site/v2/sports/soccer", "apis/v2/sports/soccer", 1)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return c.fallbackStandings(leagueKey, fmt.Errorf("failed to create standings request: %w", err))
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return c.fallbackStandings(leagueKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackStandings(leagueKey, fmt.Errorf("upstream espn standings returned status code %d", resp.StatusCode))
	}

	var raw rawStandingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.fallbackStandings(leagueKey, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, err))
	}

	parseEntries := func(rawEntries []rawStandingsEntry, groupName string) []model.StandingsEntry {
		var list []model.StandingsEntry
		for _, re := range rawEntries {
			teamName := "Equipo"
			if re.Team != nil {
				if re.Team.DisplayName != "" {
					teamName = re.Team.DisplayName
				} else if re.Team.Name != "" {
					teamName = re.Team.Name
				}
			}

			getStat := func(names ...string) string {
				for _, name := range names {
					for _, s := range re.Stats {
						if strings.EqualFold(s.Name, name) {
							return s.DisplayValue
						}
					}
				}
				return "-"
			}

			rankVal := 0
			for _, s := range re.Stats {
				if strings.EqualFold(s.Name, "rank") {
					rankVal = int(s.Value)
					break
				}
			}

			list = append(list, model.StandingsEntry{
				Rank:        rankVal,
				TeamName:    teamName,
				GroupName:   groupName,
				GamesPlayed: getStat("gamesPlayed"),
				Wins:        getStat("wins"),
				Ties:        getStat("ties"),
				Losses:      getStat("losses"),
				PointDiff:   getStat("pointDifferential"),
				Points:      getStat("points"),
			})
		}

		// Sort by rank if present, or by points descending
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Rank > 0 && list[j].Rank > 0 {
				return list[i].Rank < list[j].Rank
			}
			p1, _ := strconv.Atoi(list[i].Points)
			p2, _ := strconv.Atoi(list[j].Points)
			if p1 != p2 {
				return p1 > p2
			}
			gd1, _ := strconv.Atoi(strings.TrimPrefix(list[i].PointDiff, "+"))
			gd2, _ := strconv.Atoi(strings.TrimPrefix(list[j].PointDiff, "+"))
			return gd1 > gd2
		})

		// Assign sequential rank 1..N if rank was not provided by API
		for i := range list {
			if list[i].Rank <= 0 {
				list[i].Rank = i + 1
			}
		}

		return list
	}

	var groups []model.StandingsGroup
	var allEntries []model.StandingsEntry

	if len(raw.Children) > 0 {
		for _, child := range raw.Children {
			if child.Standings != nil && len(child.Standings.Entries) > 0 {
				gEntries := parseEntries(child.Standings.Entries, child.Name)
				groups = append(groups, model.StandingsGroup{
					Name:    child.Name,
					Entries: gEntries,
				})
				allEntries = append(allEntries, gEntries...)
			}
		}
	} else if raw.Standings != nil {
		allEntries = parseEntries(raw.Standings.Entries, "")
	}

	now := c.now()
	if c.loc != nil {
		now = now.In(c.loc)
	}

	res := &model.LeagueStandings{
		LeagueKey: leagueKey,
		Name:      raw.Name,
		Entries:   allEntries,
		Groups:    groups,
		IsOffline: false,
		CachedAt:  now,
	}

	c.setCachedStandings(leagueKey, res)
	return res, nil
}

func (c *Client) fallbackStandings(leagueKey string, origErr error) (*model.LeagueStandings, error) {
	if cached, ok := c.readDiskStandings(leagueKey); ok && cached.Standings != nil {
		st := *cached.Standings
		st.IsOffline = true
		if c.loc != nil {
			st.CachedAt = cached.CachedAt.In(c.loc)
		} else {
			st.CachedAt = cached.CachedAt
		}
		return &st, nil
	}
	return nil, origErr
}

// Statistics JSON parsing structures
type rawStatisticsResponse struct {
	Stats []rawStatsCategory `json:"stats"`
}

type rawStatsCategory struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	Leaders     []rawStatsLeader  `json:"leaders"`
}

type rawStatsAthlete struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"displayName"`
	ShortName   string             `json:"shortName"`
	Team        *rawTeam           `json:"team"`
	Statistics  []rawStatisticItem `json:"statistics"`
}

type rawStatsLeader struct {
	DisplayValue string             `json:"displayValue"`
	Value        float64            `json:"value"`
	Athlete      *rawStatsAthlete   `json:"athlete"`
	Team         *rawTeam           `json:"team"`
	Statistics   []rawStatisticItem `json:"statistics"`
}

func (c *Client) statisticsCachePath(leagueKey string) string {
	cleanKey := strings.ReplaceAll(leagueKey, "/", "_")
	return filepath.Join(c.cacheDir, fmt.Sprintf("statistics_%s.json", cleanKey))
}

func (c *Client) readDiskStatistics(leagueKey string) (*cachedStatistics, bool) {
	data, err := os.ReadFile(c.statisticsCachePath(leagueKey))
	if err != nil {
		return nil, false
	}
	var cached cachedStatistics
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}
	return &cached, true
}

func (c *Client) setCachedStatistics(leagueKey string, statistics *model.LeagueStatistics) {
	if statistics == nil {
		return
	}
	_ = os.MkdirAll(c.cacheDir, 0o755)
	cached := cachedStatistics{
		CachedAt:   c.now(),
		Statistics: statistics,
	}
	data, err := json.Marshal(cached)
	if err == nil {
		_ = os.WriteFile(c.statisticsCachePath(leagueKey), data, 0o644)
	}
}

// FetchStatistics retrieves top scorers and assists from ESPN site statistics API.
func (c *Client) FetchStatistics(ctx context.Context, leagueKey string) (*model.LeagueStatistics, error) {
	if !model.IsForceRefresh(ctx) {
		if cached, ok := c.readDiskStatistics(leagueKey); ok {
			if c.now().Sub(cached.CachedAt) <= c.cacheTTL {
				return cached.Statistics, nil
			}
		}
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/statistics", c.baseURL, leagueCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return c.fallbackStatistics(leagueKey, fmt.Errorf("failed to create statistics request: %w", err))
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return c.fallbackStatistics(leagueKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackStatistics(leagueKey, fmt.Errorf("upstream espn statistics returned status code %d", resp.StatusCode))
	}

	var raw rawStatisticsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.fallbackStatistics(leagueKey, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, err))
	}

	parseLeaders := func(leaders []rawStatsLeader, isScorer bool) []model.ScorerEntry {
		var res []model.ScorerEntry
		for i, l := range leaders {
			pName := "Jugador"
			tName := ""
			var statsList []rawStatisticItem

			if l.Athlete != nil {
				if l.Athlete.DisplayName != "" {
					pName = l.Athlete.DisplayName
				} else if l.Athlete.ShortName != "" {
					pName = l.Athlete.ShortName
				}
				if l.Athlete.Team != nil && l.Athlete.Team.DisplayName != "" {
					tName = l.Athlete.Team.DisplayName
				}
				statsList = l.Athlete.Statistics
			}
			if tName == "" && l.Team != nil && l.Team.DisplayName != "" {
				tName = l.Team.DisplayName
			}
			if len(statsList) == 0 {
				statsList = l.Statistics
			}

			valStr := ""
			for _, st := range statsList {
				if isScorer && strings.EqualFold(st.Name, "totalGoals") {
					valStr = st.DisplayValue
					break
				}
				if !isScorer && strings.EqualFold(st.Name, "goalAssists") {
					valStr = st.DisplayValue
					break
				}
			}

			if valStr == "" && l.DisplayValue != "" {
				// Parse "Matches: X, Goals: Y" or "Matches: X, Assists: Y"
				dv := l.DisplayValue
				if isScorer && strings.Contains(dv, "Goals: ") {
					parts := strings.Split(dv, "Goals: ")
					if len(parts) > 1 {
						valStr = strings.TrimSpace(parts[1])
					}
				} else if !isScorer && strings.Contains(dv, "Assists: ") {
					parts := strings.Split(dv, "Assists: ")
					if len(parts) > 1 {
						valStr = strings.TrimSpace(parts[1])
					}
				}
			}

			if valStr == "" {
				if l.Value > 0 {
					valStr = fmt.Sprintf("%.0f", l.Value)
				} else if l.DisplayValue != "" {
					valStr = l.DisplayValue
				} else {
					valStr = "0"
				}
			}

			res = append(res, model.ScorerEntry{
				Rank:         i + 1,
				PlayerName:   pName,
				TeamName:     tName,
				Value:        valStr,
				DisplayValue: l.DisplayValue,
			})
		}
		return res
	}

	var scorers, assists []model.ScorerEntry
	for _, cat := range raw.Stats {
		catName := strings.ToLower(cat.Name)
		if strings.Contains(catName, "goal") || strings.Contains(catName, "scorer") {
			scorers = parseLeaders(cat.Leaders, true)
		} else if strings.Contains(catName, "assist") {
			assists = parseLeaders(cat.Leaders, false)
		}
	}

	now := c.now()
	if c.loc != nil {
		now = now.In(c.loc)
	}

	res := &model.LeagueStatistics{
		LeagueKey: leagueKey,
		Scorers:   scorers,
		Assists:   assists,
		IsOffline: false,
		CachedAt:  now,
	}

	c.setCachedStatistics(leagueKey, res)
	return res, nil
}

func (c *Client) fallbackStatistics(leagueKey string, origErr error) (*model.LeagueStatistics, error) {
	if cached, ok := c.readDiskStatistics(leagueKey); ok && cached.Statistics != nil {
		st := *cached.Statistics
		st.IsOffline = true
		if c.loc != nil {
			st.CachedAt = cached.CachedAt.In(c.loc)
		} else {
			st.CachedAt = cached.CachedAt
		}
		return &st, nil
	}
	return nil, origErr
}
