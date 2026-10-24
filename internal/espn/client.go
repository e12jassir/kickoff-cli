package espn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/fsutil"
	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	defaultBaseURL      = "https://site.api.espn.com/apis/site/v2/sports/soccer"
	defaultCacheTTL     = 30 * time.Second
	liveCacheTTL        = 5 * time.Second
	defaultPastCacheTTL = 6 * time.Hour
	defaultTimeout      = 10 * time.Second
	userAgent           = "kickoff-cli/1.0 (+https://github.com/e12jassir/kickoff-cli)"

	// maxRetryAfter caps how long a Retry-After hint may delay the next attempt.
	// Servers occasionally advertise minutes or hours, which would freeze the
	// interactive UI far past the point where a retry is still useful.
	maxRetryAfter = 10 * time.Second

	// maxConcurrentRequests caps how many HTTP requests this process keeps in
	// flight at once. It is enforced once, at the HTTP transport, so every
	// caller path (scoreboard reads, day probes, match details, standings,
	// statistics and internal/bar's league fan-out) shares the same budget
	// instead of each fan-out adding its own. The historical sweep fans out
	// over 14 days, and without a cap a single startup could burst ~29
	// simultaneous requests to ESPN.
	maxConcurrentRequests = 4
	// probeDays is the size of the past and upcoming day windows.
	probeDays = 14

	// probeDeadlineFloor is the minimum time a cold-cache sweep gets: enough
	// for one batch of probes on a slow link, and the fixed value every sweep
	// used before the deadline became derived from the work.
	probeDeadlineFloor = 8 * time.Second
	// probeBatchCost is the allowance per budget batch when deriving a sweep
	// deadline. A sweep that needs n batches of maxConcurrentRequests probes
	// gets n * probeBatchCost, clamped between the floor and the ceiling.
	probeBatchCost = 2 * time.Second
	// maxProbeDeadline caps both the derived and the overridden sweep deadline
	// so a wide window can never hold a startup for minutes.
	maxProbeDeadline = 30 * time.Second

	// cacheSchemaVersion is the on-disk schema revision shared by every
	// persisted ESPN cache entry. Bumping it invalidates older entries by
	// design: a read whose stored version differs is treated as a cache miss
	// and the entry is refetched, with no migration or backfill.
	cacheSchemaVersion = 1
)

type rawResponse struct {
	Events []rawEvent `json:"events"`
}

// scoreboardProbe is the result of probing the scoreboard for a single day.
type scoreboardProbe struct {
	dayOffset int
	events    []rawEvent
}

type rawEvent struct {
	ID           string           `json:"id"`
	Date         string           `json:"date"`
	Competitions []rawCompetition `json:"competitions"`
	Status       *rawStatus       `json:"status"`
}

type rawCompetition struct {
	ID            string            `json:"id"`
	Status        *rawStatus        `json:"status"`
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
	Coaches   []rawCoach      `json:"coaches"`
	Coach     *rawCoach       `json:"coach"`
}

type rawCoach struct {
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	FullName    string    `json:"fullName"`
	DisplayName string    `json:"displayName"`
	Coach       *rawCoach `json:"coach"`
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
	Name         string  `json:"name"`
	DisplayValue string  `json:"displayValue"`
	Value        float64 `json:"value"`
}

type cachedSummary struct {
	SchemaVersion int                 `json:"schema_version"`
	CachedAt      time.Time           `json:"cached_at"`
	Stats         *model.MatchStats   `json:"stats"`
	Lineups       *model.MatchLineups `json:"lineups"`
	Referee       string              `json:"referee"`
	Attendance    int                 `json:"attendance"`
}

type cachedStandings struct {
	SchemaVersion int                    `json:"schema_version"`
	CachedAt      time.Time              `json:"cached_at"`
	Standings     *model.LeagueStandings `json:"standings"`
}

type cachedStatistics struct {
	SchemaVersion int                     `json:"schema_version"`
	CachedAt      time.Time               `json:"cached_at"`
	Statistics    *model.LeagueStatistics `json:"statistics"`
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
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Name         string `json:"name"`
	Logo         string `json:"logo"`
	Abbreviation string `json:"abbreviation"`
}

type rawStatus struct {
	Type *rawStatusType `json:"type"`
}

type rawStatusType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
	State       string `json:"state"`
	Completed   bool   `json:"completed"`
}

type cachedData struct {
	SchemaVersion int           `json:"schema_version"`
	CachedAt      time.Time     `json:"cached_at"`
	Matches       []model.Match `json:"matches"`
}

type cachedPastEvents struct {
	SchemaVersion int        `json:"schema_version"`
	CachedAt      time.Time  `json:"cached_at"`
	Events        []rawEvent `json:"events"`
}

// Client implements model.Client for fetching ESPN soccer fixtures.
type Client struct {
	baseURL      string
	httpClient   *http.Client
	cacheDir     string
	cacheTTL     time.Duration
	pastCacheTTL time.Duration
	loc          *time.Location
	now          func() time.Time
	retryBackoff func() time.Duration

	// probeBudget, when positive, replaces the deadline derived from the size
	// of a cold-cache sweep. It is still clamped to maxProbeDeadline.
	probeBudget time.Duration
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

func WithPastCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.pastCacheTTL = ttl
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

// WithProbeBudget overrides the deadline a cold-cache scoreboard sweep gets.
// An explicit value always wins over the deadline derived from the number of
// day probes, and is still capped at maxProbeDeadline. Non-positive values are
// ignored so callers can pass a zero value to keep the derived deadline.
func WithProbeBudget(d time.Duration) Option {
	return func(c *Client) {
		c.probeBudget = d
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

	// Default display timezone comes from the shared locale resolver
	// (KICKOFF_TZ, else America/Bogota). WithTimezone still overrides it.
	loc := locale.Location()

	c := &Client{
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cacheDir:     defaultCache,
		cacheTTL:     defaultCacheTTL,
		pastCacheTTL: defaultPastCacheTTL,
		loc:          loc,
		now:          time.Now,
		retryBackoff: func() time.Duration {
			return time.Duration(200+rand.IntN(301)) * time.Millisecond
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// Install the shared HTTP budget last so it also covers a caller-supplied
	// http.Client: no caller path may bypass the cap.
	c.httpClient = withRequestBudget(c.httpClient)

	return c
}

// requestBudget is the process-wide slot pool every ESPN client shares. It
// lives at package scope on purpose: the cap describes the whole process
// talking to ESPN, so two clients cannot each spend the full budget and double
// the request burst.
var requestBudget = make(chan struct{}, maxConcurrentRequests)

// budgetTransport bounds how many requests are in flight at the same time,
// regardless of which Client method or fan-out issued them. It holds one slot
// from the moment a request starts until its response body is closed, because
// the body, not the RoundTrip call, is the unit of work: every caller decodes
// the response after RoundTrip returns.
type budgetTransport struct {
	base  http.RoundTripper
	slots chan struct{}
}

// newRequestBudgetTransport wraps base in the shared budget. It refuses to wrap
// an existing budgetTransport: stacking two budgets would let the outer
// acquisition block on the inner one while the outer slot is still held, which
// no other goroutine can release.
func newRequestBudgetTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if _, alreadyBounded := base.(*budgetTransport); alreadyBounded {
		return base
	}
	return &budgetTransport{base: base, slots: requestBudget}
}

// withRequestBudget returns a copy of client whose transport enforces the shared
// budget. The caller's client is copied rather than mutated so a client shared
// with another subsystem keeps its own behavior.
func withRequestBudget(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	if _, alreadyBounded := client.Transport.(*budgetTransport); alreadyBounded {
		return client
	}

	bounded := *client
	bounded.Transport = newRequestBudgetTransport(client.Transport)
	return &bounded
}

func (t *budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case t.slots <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		<-t.slots
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		<-t.slots
		return resp, nil
	}

	resp.Body = &budgetBody{ReadCloser: resp.Body, slots: t.slots}
	return resp, nil
}

// budgetBody releases its budget slot when closed. Release is idempotent
// because net/http and the retry loop may close a body more than once.
type budgetBody struct {
	io.ReadCloser
	slots     chan struct{}
	closeOnce sync.Once
}

func (b *budgetBody) Close() error {
	err := b.ReadCloser.Close()
	b.closeOnce.Do(func() { <-b.slots })
	return err
}

// parseRetryAfter parses the standard HTTP Retry-After header.
// It supports either an integer number of seconds (e.g. "120")
// or an HTTP-date (RFC 1123 / RFC 850 / ANSI C as parsed by http.ParseTime).
func parseRetryAfter(headerVal string, now time.Time) (time.Duration, bool) {
	headerVal = strings.TrimSpace(headerVal)
	if headerVal == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(headerVal); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(headerVal); err == nil {
		d := t.Sub(now)
		if d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

func (c *Client) calculateBackoff(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if d, ok := parseRetryAfter(ra, c.now()); ok {
				// A server can ask for minutes or hours. Honor the hint but never
				// let it stall the caller for longer than maxRetryAfter.
				if d > maxRetryAfter {
					return maxRetryAfter
				}
				return d
			}
		}
	}

	if c.retryBackoff != nil {
		base := c.retryBackoff()
		if base <= 0 {
			return 0
		}
		return base * time.Duration(1<<attempt)
	}

	base := time.Duration(200+rand.IntN(301)) * time.Millisecond
	return base * time.Duration(1<<attempt)
}

// doWithRetry executes HTTP request with transient retry and backoff + jitter.
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	const maxRetries = 2
	var resp *http.Response
	var err error

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		clonedReq := req.Clone(req.Context())
		resp, err = client.Do(clonedReq)

		if req.Context().Err() != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, req.Context().Err()
		}

		isTransient := err != nil || (resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500))
		if !isTransient {
			return resp, nil
		}

		if attempt < maxRetries {
			backoff := c.calculateBackoff(attempt, resp)
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if backoff > 0 {
				select {
				case <-req.Context().Done():
					return nil, req.Context().Err()
				case <-time.After(backoff):
				}
			}
		}
	}
	return resp, err
}

// LeagueCode maps a user-facing league key to the ESPN league code.
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
	if cached.SchemaVersion != cacheSchemaVersion {
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
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      c.now(),
		Matches:       matches,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = fsutil.AtomicWriteFile(cachePath, data, 0o644)
}

func (c *Client) pastCacheFilePath(leagueKey string) string {
	cleanKey := strings.ReplaceAll(leagueKey, "/", "_")
	return filepath.Join(c.cacheDir, fmt.Sprintf("past_matches_%s.json", cleanKey))
}

func (c *Client) readDiskPastEvents(leagueKey string) (*cachedPastEvents, bool) {
	cachePath := c.pastCacheFilePath(leagueKey)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	var cached cachedPastEvents
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}
	if cached.SchemaVersion != cacheSchemaVersion {
		return nil, false
	}

	return &cached, true
}

func (c *Client) getCachedPastEvents(leagueKey string) ([]rawEvent, bool) {
	cached, ok := c.readDiskPastEvents(leagueKey)
	if !ok {
		return nil, false
	}

	ttl := c.pastCacheTTL
	if ttl <= 0 {
		ttl = defaultPastCacheTTL
	}

	// If empty events were cached (e.g. from a network timeout or transient error),
	// allow retrying after 30 seconds rather than locking out for 6 hours.
	if len(cached.Events) == 0 && c.now().Sub(cached.CachedAt) > 30*time.Second {
		return nil, false
	}

	if c.now().Sub(cached.CachedAt) > ttl {
		return nil, false
	}

	return cached.Events, true
}

func (c *Client) setCachedPastEvents(leagueKey string, events []rawEvent) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	cachePath := c.pastCacheFilePath(leagueKey)
	cached := cachedPastEvents{
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      c.now(),
		Events:        events,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = fsutil.AtomicWriteFile(cachePath, data, 0o644)
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
	if cached.SchemaVersion != cacheSchemaVersion {
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
			IsOffline:  true,
			CachedAt:   cachedAt,
		}, nil
	}
	return nil, origErr
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
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      c.now(),
		Stats:         stats,
		Lineups:       lineups,
		Referee:       referee,
		Attendance:    attendance,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = fsutil.AtomicWriteFile(cachePath, data, 0o644)
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
		// ESPN scoreboard query params (verified live on 2026-10-17 against
		// https://site.api.espn.com/apis/site/v2/sports/soccer/<league>/scoreboard):
		//
		//   ?region=co&lang=es -> competitions[].broadcasts and geoBroadcasts are EMPTY
		//   ?lang=es           -> both arrays populate from names / media.shortName,
		//                         and status strings stay in Spanish
		//   ?region=us&lang=es -> same as ?lang=es
		//   ?lang=en           -> both arrays populate, but status strings flip to
		//                         English ("HT", "Full Time") and internal/locale
		//                         has no lexicon to translate them back
		//
		// The erase trigger is the region+lang=es COMBINATION: region=co/ar/mx
		// together with lang=es wipe the broadcasts. Omitting region alone is
		// sufficient to keep them. We therefore drop only region and keep lang=es,
		// because broadcast metadata is the strongest evidence tier in the channel
		// catalog and these status strings reach the UI untranslated.
		endpoint += "?lang=es"
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

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return c.fallbackMatches(leagueKey, fmt.Errorf("upstream espn returned status code %d", resp.StatusCode))
	}

	// The body is closed before the probe fan-out below, never with defer:
	// the transport's HTTP budget is process-wide, so holding this response
	// open while the sweep asks for slots of its own would let concurrent
	// FetchMatches calls pin every slot and stall the sweep until its deadline.
	var raw rawResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&raw)
	_ = resp.Body.Close()
	if decodeErr != nil {
		return c.fallbackMatches(leagueKey, fmt.Errorf("%w: %v", model.ErrCorruptedPayload, decodeErr))
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
			dispName = normalizeTeamDisplayName(dispName)

			abbrev := ""
			if competitor.Team != nil {
				abbrev = strings.TrimSpace(competitor.Team.Abbreviation)
			}

			cModel := model.Competitor{
				ID:           competitor.ID,
				Name:         name,
				DisplayName:  dispName,
				Abbreviation: abbrev,
				Logo:         logo,
				Score:        competitor.Score,
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
			homeComp = model.Competitor{DisplayName: "", IsHome: true}
		}
		if !hasAway {
			awayComp = model.Competitor{DisplayName: "", IsHome: false}
		}

		venueName := ""
		if comp.Venue != nil && comp.Venue.FullName != "" {
			venueName = comp.Venue.FullName
		}

		status := model.MatchStatus{
			State: "pre",
		}
		// ESPN scoreboard responses carry the status inside the competition.
		// Some payloads duplicate it at event level, so keep that as fallback.
		feedStatus := comp.Status
		if feedStatus == nil {
			feedStatus = ev.Status
		}
		if feedStatus != nil && feedStatus.Type != nil {
			if feedStatus.Type.Description != "" {
				status.Description = feedStatus.Type.Description
			}
			status.Code = feedStatus.Type.Name
			status.Detail = feedStatus.Type.Detail
			status.State = feedStatus.Type.State
			status.Completed = feedStatus.Type.Completed
		}

		var events []model.MatchEvent
		for _, d := range comp.Details {
			if !d.ScoringPlay && !d.OwnGoal && !d.RedCard && !d.YellowCard {
				continue
			}

			var pName, aName string
			if len(d.AthletesInvolved) > 0 {
				if d.AthletesInvolved[0].DisplayName != "" {
					pName = d.AthletesInvolved[0].DisplayName
				} else if d.AthletesInvolved[0].ShortName != "" {
					pName = d.AthletesInvolved[0].ShortName
				}
				if len(d.AthletesInvolved) > 1 {
					if d.AthletesInvolved[1].DisplayName != "" {
						aName = d.AthletesInvolved[1].DisplayName
					} else if d.AthletesInvolved[1].ShortName != "" {
						aName = d.AthletesInvolved[1].ShortName
					}
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
				AssistName:  aName,
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

	// Probe recent past days (1-4 days) to complete the past matchday (e.g. Barcelona on Tuesday when default only has Wednesday).
	// Decouple past days query from live 5s auto-sync polling loop: cached separately with a long TTL (6 to 12 hours)
	// and only fetched if the past matches cache is empty or expired, never on every 5s live tick.
	pastEvents, ok := c.getCachedPastEvents(leagueKey)
	if !ok {
		pastEvents = c.fetchRecentPastScoreboardEvents(ctx, leagueCode, now)
		if ctx.Err() == nil {
			c.setCachedPastEvents(leagueKey, pastEvents)
		}
	}
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
	} else if len(matches) > 0 {
		// Include completed past matches so Tab 1 (Resumen) displays previous matchday results
		// even when today has scheduled or live matches in Tab 2 (Partidos).
		seenMatchIDs := make(map[string]bool, len(matches))
		for _, m := range matches {
			seenMatchIDs[m.ID] = true
		}
		for _, m := range allParsedMatches {
			if (m.Status.Completed || strings.ToLower(m.Status.State) == "post") && !seenMatchIDs[m.ID] {
				seenMatchIDs[m.ID] = true
				matches = append(matches, m)
			}
		}
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

// probeScoreboardDays fetches one scoreboard per day offset. The HTTP transport
// budget is the only limiter here: this dispatches one goroutine per offset and
// lets the transport queue them, so probes, main scoreboard reads and the bar
// league fan-out all draw from the same pool. A goroutine still waiting for a
// slot gives up as soon as ctx is cancelled.
func (c *Client) probeScoreboardDays(ctx context.Context, leagueCode string, now time.Time, offsets []int) []scoreboardProbe {
	if len(offsets) == 0 {
		return nil
	}

	// Buffering to len(offsets) guarantees senders never block, so wg.Wait
	// cannot deadlock while results are still unread.
	results := make(chan scoreboardProbe, len(offsets))
	var wg sync.WaitGroup

	for _, offset := range offsets {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(dayOffset int) {
			defer wg.Done()

			if probe, ok := c.fetchScoreboardDay(ctx, leagueCode, now, dayOffset); ok {
				results <- probe
			}
		}(offset)
	}

	wg.Wait()
	close(results)

	probes := make([]scoreboardProbe, 0, len(offsets))
	for probe := range results {
		probes = append(probes, probe)
	}
	return probes
}

// probeTimeout reports the deadline a sweep over days day probes gets. An
// explicit WithProbeBudget value wins and is clamped to maxProbeDeadline;
// otherwise the deadline is derived from how many budget batches the sweep
// implies.
func (c *Client) probeTimeout(days int) time.Duration {
	if c.probeBudget > 0 {
		return min(c.probeBudget, maxProbeDeadline)
	}
	return derivedProbeDeadline(days, maxConcurrentRequests)
}

// derivedProbeDeadline sizes a cold-cache sweep from its work:
// max(probeDeadlineFloor, probeBatchCost * ceil(days/concurrency)), clamped to
// maxProbeDeadline. The floor keeps a normal 14-day sweep on the value it always
// had; the per-batch term lets a wider window or a narrower budget buy the time
// its extra batches need instead of being cut off mid-sweep.
func derivedProbeDeadline(days, concurrency int) time.Duration {
	if concurrency <= 0 {
		concurrency = 1
	}

	batches := 0
	if days > 0 {
		batches = (days + concurrency - 1) / concurrency
	}

	deadline := time.Duration(batches) * probeBatchCost
	if deadline < probeDeadlineFloor {
		return probeDeadlineFloor
	}
	return min(deadline, maxProbeDeadline)
}

// fetchScoreboardDay requests the scoreboard for a single day offset. Requests
// without events are reported as not found so callers only merge real data.
func (c *Client) fetchScoreboardDay(ctx context.Context, leagueCode string, now time.Time, dayOffset int) (scoreboardProbe, bool) {
	dateStr := now.AddDate(0, 0, dayOffset).Format("20060102")
	endpoint := fmt.Sprintf("%s/%s/scoreboard?dates=%s", c.baseURL, leagueCode, dateStr)
	if strings.Contains(c.baseURL, "espn.com") {
		// Same broadcast-preserving params as FetchMatches: see the comment there.
		endpoint += "&lang=es"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return scoreboardProbe{}, false
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return scoreboardProbe{}, false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return scoreboardProbe{}, false
	}

	var raw rawResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return scoreboardProbe{}, false
	}
	if len(raw.Events) == 0 {
		return scoreboardProbe{}, false
	}

	return scoreboardProbe{dayOffset: dayOffset, events: raw.Events}, true
}

// fetchRecentPastScoreboardEvents probes dates 1-14 days into the past to capture the full recent matchday round.
func (c *Client) fetchRecentPastScoreboardEvents(ctx context.Context, leagueCode string, now time.Time) []rawEvent {
	offsets := make([]int, 0, probeDays)
	for offset := 1; offset <= probeDays; offset++ {
		offsets = append(offsets, -offset)
	}

	probeCtx, cancel := context.WithTimeout(ctx, c.probeTimeout(len(offsets)))
	defer cancel()

	allRes := c.probeScoreboardDays(probeCtx, leagueCode, now, offsets)
	if len(allRes) == 0 {
		return nil
	}

	sort.Slice(allRes, func(i, j int) bool {
		return allRes[i].dayOffset < allRes[j].dayOffset
	})

	// Offsets are negative for past probes, so the most recent day is the one
	// closest to zero, i.e. the last element after sorting ascending.
	mostRecentOffset := allRes[len(allRes)-1].dayOffset
	var pastEvents []rawEvent
	for _, r := range allRes {
		if r.dayOffset >= mostRecentOffset-3 {
			pastEvents = append(pastEvents, r.events...)
		}
	}
	return pastEvents
}

// fetchUpcomingScoreboardEvents probes dates 1-14 days into the future to find the next active matchday.
// It starts at offset 1 because the current day is already covered by the default scoreboard request.
func (c *Client) fetchUpcomingScoreboardEvents(ctx context.Context, leagueCode string, now time.Time) []rawEvent {
	offsets := make([]int, 0, probeDays)
	for offset := 1; offset <= probeDays; offset++ {
		offsets = append(offsets, offset)
	}

	probeCtx, cancel := context.WithTimeout(ctx, c.probeTimeout(len(offsets)))
	defer cancel()

	allRes := c.probeScoreboardDays(probeCtx, leagueCode, now, offsets)
	if len(allRes) == 0 {
		return nil
	}

	sort.Slice(allRes, func(i, j int) bool {
		return allRes[i].dayOffset < allRes[j].dayOffset
	})

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
	defer func() { _ = resp.Body.Close() }()

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
					switch r.Team.ID {
					case homeTeamID:
						homeRoster = r
					case awayTeamID:
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
			var coachName string
			if r.Coach != nil {
				coachName = extractCoachName(r.Coach)
			}
			if coachName == "" && len(r.Coaches) > 0 {
				for _, c := range r.Coaches {
					coachName = extractCoachName(&c)
					if coachName != "" {
						break
					}
				}
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
				Coach:     coachName,
				Starters:  starters,
				Bench:     bench,
			}
		}

		homeLineup := parseLineup(homeRoster)
		awayLineup := parseLineup(awayRoster)

		lineups = &model.MatchLineups{
			Home: homeLineup,
			Away: awayLineup,
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
	now := c.now()
	if c.loc != nil {
		now = now.In(c.loc)
	}
	return &model.MatchDetails{
		Stats:      stats,
		Lineups:    lineups,
		Referee:    referee,
		Attendance: attendance,
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
	Name      string               `json:"name"`
	Children  []rawStandingsChild  `json:"children"`
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
	Team  *rawTeam           `json:"team"`
	Stats []rawStatisticItem `json:"stats"`
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
	if cached.SchemaVersion != cacheSchemaVersion {
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
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      c.now(),
		Standings:     standings,
	}
	data, err := json.Marshal(cached)
	if err == nil {
		_ = fsutil.AtomicWriteFile(c.standingsCachePath(leagueKey), data, 0o644)
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
	defer func() { _ = resp.Body.Close() }()

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
			teamName := ""
			if re.Team != nil {
				if re.Team.DisplayName != "" {
					teamName = re.Team.DisplayName
				} else if re.Team.Name != "" {
					teamName = re.Team.Name
				}
			}
			teamName = normalizeTeamDisplayName(teamName)

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
	Name        string           `json:"name"`
	DisplayName string           `json:"displayName"`
	Leaders     []rawStatsLeader `json:"leaders"`
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
	if cached.SchemaVersion != cacheSchemaVersion {
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
		SchemaVersion: cacheSchemaVersion,
		CachedAt:      c.now(),
		Statistics:    statistics,
	}
	data, err := json.Marshal(cached)
	if err == nil {
		_ = fsutil.AtomicWriteFile(c.statisticsCachePath(leagueKey), data, 0o644)
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
	defer func() { _ = resp.Body.Close() }()

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
			var pName string
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
			tName = normalizeTeamDisplayName(tName)
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

func extractCoachName(c *rawCoach) string {
	if c == nil {
		return ""
	}
	if c.DisplayName != "" {
		return c.DisplayName
	}
	if c.FullName != "" {
		return c.FullName
	}
	name := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if name != "" {
		return name
	}
	if c.Coach != nil {
		return extractCoachName(c.Coach)
	}
	return ""
}

// normalizeTeamDisplayName standardizes upstream club names into popular Spanish naming conventions.
func normalizeTeamDisplayName(name string) string {
	trimmed := strings.TrimSpace(name)
	switch {
	case strings.EqualFold(trimmed, "Internazionale"),
		strings.EqualFold(trimmed, "FC Internazionale Milano"),
		strings.EqualFold(trimmed, "Inter Milan"):
		return "Inter de Milán"
	default:
		return name
	}
}
