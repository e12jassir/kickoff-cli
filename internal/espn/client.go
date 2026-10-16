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
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	defaultBaseURL  = "https://site.api.espn.com/apis/site/v2/sports/soccer"
	defaultCacheTTL = 300 * time.Second
	liveCacheTTL    = 15 * time.Second
	defaultTimeout  = 10 * time.Second
	userAgent       = "Mozilla/5.0"
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
}

type rawOfficial struct {
	DisplayName string `json:"displayName"`
}

type rawRoster struct {
	Team      *rawTeam        `json:"team"`
	Formation string          `json:"formation"`
	Roster    []rawRosterItem `json:"roster"`
}

type rawRosterItem struct {
	Starter  bool        `json:"starter"`
	Jersey   string      `json:"jersey"`
	Position *rawPos     `json:"position"`
	Athlete  *rawAthlete `json:"athlete"`
}

type rawPos struct {
	Abbreviation string `json:"abbreviation"`
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
	Name         string `json:"name"`
	DisplayValue string `json:"displayValue"`
}

type cachedSummary struct {
	CachedAt   time.Time           `json:"cached_at"`
	Stats      *model.MatchStats   `json:"stats"`
	Lineups    *model.MatchLineups `json:"lineups"`
	Referee    string              `json:"referee"`
	Attendance int                 `json:"attendance"`
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
	case "libertadores":
		return "conmebol.libertadores"
	case "argentina":
		return "arg.1"
	case "colombia":
		return "col.1"
	case "brasil":
		return "bra.1"
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

func (c *Client) getCachedSummary(eventID string) (*model.MatchStats, *model.MatchLineups, string, int, bool) {
	cachePath := c.summaryCachePath(eventID)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, nil, "", 0, false
	}

	var cached cachedSummary
	if err := json.Unmarshal(data, &cached); err != nil {
		_ = os.Remove(cachePath)
		return nil, nil, "", 0, false
	}

	if c.now().Sub(cached.CachedAt) > c.cacheTTL {
		return nil, nil, "", 0, false
	}

	return cached.Stats, cached.Lineups, cached.Referee, cached.Attendance, true
}

func (c *Client) setCachedSummary(eventID string, stats *model.MatchStats, lineups *model.MatchLineups, referee string, attendance int) {
	if stats == nil && lineups == nil && referee == "" && attendance == 0 {
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

		matches = append(matches, match)
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
		if stats, lineups, ref, att, found := c.getCachedSummary(eventID); found {
			return &model.MatchDetails{
				Stats:      stats,
				Lineups:    lineups,
				Referee:    ref,
				Attendance: att,
			}, nil
		}
	}

	leagueCode := LeagueCode(leagueKey)
	endpoint := fmt.Sprintf("%s/%s/summary?event=%s", c.baseURL, leagueCode, eventID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var raw rawSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, nil
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

			return model.TeamLineup{
				Formation: r.Formation,
				Starters:  starters,
				Bench:     bench,
			}
		}

		lineups = &model.MatchLineups{
			Home: parseLineup(homeRoster),
			Away: parseLineup(awayRoster),
		}
	}
	referee := ""
	attendance := 0
	if raw.GameInfo != nil {
		attendance = raw.GameInfo.Attendance
		if len(raw.GameInfo.Officials) > 0 {
			referee = raw.GameInfo.Officials[0].DisplayName
		}
	}

	c.setCachedSummary(eventID, stats, lineups, referee, attendance)
	return &model.MatchDetails{
		Stats:      stats,
		Lineups:    lineups,
		Referee:    referee,
		Attendance: attendance,
	}, nil
}

func (c *Client) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	details, err := c.FetchMatchDetails(ctx, leagueKey, eventID)
	if err != nil || details == nil {
		return nil, err
	}
	return details.Stats, nil
}
