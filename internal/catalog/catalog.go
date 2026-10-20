package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/extractor"
	"github.com/e12jassir/kickoff-cli/internal/locale"
	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/parrilla"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

// Public free-to-air (FTA) channels with legal webcasts.
const (
	StreamRealMadrid    = "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8"
	StreamTeledeporte   = "https://rtvelivestream.akamaized.net/rtvesec/tdp/tdp_main.m3u8"
	maxPlaylistReadSize = 50 * 1024 * 1024 // 50MB bounded read for remote playlists

	// parrillaFetchTimeout bounds a single daily parrilla fetch so a slow or
	// unreachable Noticias Caracol endpoint cannot stall match resolution
	// indefinitely. The snapshot is cached per local date on success AND failure.
	parrillaFetchTimeout = 10 * time.Second

	// parrillaMatchWindowMinutes mirrors the tvguide assumption that a fixture
	// stays on air for two hours from kickoff. It is used only to prefer the
	// correct parrilla entry for a fixture, never to promote a candidate.
	parrillaMatchWindowMinutes = 120

	// parrillaFailureTTL bounds how long a failed daily parrilla fetch is cached.
	// A success stays valid for its source day (the cache key), but a failure
	// must expire quickly so an outage does not leave the grid empty until
	// midnight.
	parrillaFailureTTL = 10 * time.Minute
)

// Environment variables carrying the optional US stream endpoints.
const (
	envStreamCBSSN  = "KICKOFF_STREAM_CBSSN"
	envStreamGolazo = "KICKOFF_STREAM_GOLAZO"
)

type ExternalConfig struct {
	PlaylistURL    string                     `json:"playlist_url"`
	PlaylistPath   string                     `json:"playlist_path"`
	Streams        map[string]string          `json:"streams"`
	LeagueChannels map[string][]model.Channel `json:"league_channels"`
	CustomRules    []struct {
		League   string          `json:"league"`
		Pattern  string          `json:"pattern"`
		Channels []model.Channel `json:"channels"`
	} `json:"custom_rules"`
}

// SanitizeURL validates URL safety, enforcing http/https schemes, non-empty host,
// and no embedded credentials (userinfo).
func SanitizeURL(rawURL string) (string, error) {
	return model.SanitizeURL(rawURL)
}

// Option configures Catalog resolver options.
type Option func(*Catalog)

// WithTVGuide sets a custom tvguide.Service.
func WithTVGuide(svc *tvguide.Service) Option {
	return func(c *Catalog) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.tvGuide = svc
	}
}

// WithParrilla sets a custom Gol Caracol parrilla fetcher. The default is
// parrilla.NewGolCaracol(); tests inject a fake to stay offline and deterministic.
func WithParrilla(fetcher parrilla.Fetcher) Option {
	return func(c *Catalog) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.parrillaFetcher = fetcher
	}
}

// WithConfigFile sets a custom path for the channels configuration file.
func WithConfigFile(path string) Option {
	return func(c *Catalog) {
		c.configPath = path
	}
}

// WithStreams sets in-memory stream endpoints (ideal for testing or dynamic injection).
func WithStreams(streams map[string]string) Option {
	return func(c *Catalog) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.userStreams == nil {
			c.userStreams = make(map[string]string)
		}
		for k, v := range streams {
			c.userStreams[strings.ToLower(strings.TrimSpace(k))] = v
		}
	}
}

// WithM3UChannels sets in-memory M3U channels (ideal for testing or dynamic injection).
func WithM3UChannels(channels []M3UChannel) Option {
	return func(c *Catalog) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.m3uChannels = append(c.m3uChannels, channels...)
	}
}

// WithExtractor sets a custom dynamic match extractor.
func WithExtractor(ext extractor.Extractor) Option {
	return func(c *Catalog) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.extractor = ext
	}
}

// WithUSStreams sets the optional US stream endpoints. It overrides the
// KICKOFF_STREAM_* environment variables read by New.
func WithUSStreams(cbssn, golazo string) Option {
	return func(c *Catalog) {
		c.streamCBSSN = cbssn
		c.streamGolazo = golazo
	}
}

// WithNow overrides the clock used for cache expiry decisions (tests only).
func WithNow(nowFunc func() time.Time) Option {
	return func(c *Catalog) {
		c.now = nowFunc
	}
}

// Catalog implements model.Resolver with dynamic broadcast matching, TV guide crawling,
// and user-configured stream mapping.
type Catalog struct {
	mu              sync.RWMutex
	streamAlive     map[string]bool
	customRules     []customMatchRule
	leagueOverrides map[string][]model.Channel
	userStreams     map[string]string
	m3uChannels     []M3UChannel
	configPath      string
	tvGuide         *tvguide.Service
	extractor       extractor.Extractor
	now             func() time.Time

	// Optional US stream endpoints, resolved from KICKOFF_STREAM_* by New and
	// overridable through WithUSStreams. Reading them in the constructor keeps
	// configuration out of package initialization.
	streamCBSSN  string
	streamGolazo string

	// parrilla is the Gol Caracol daily schedule fetcher: the authoritative
	// per-match source of which channel airs each fixture in Colombia (T1).
	// parrillaMu serializes cache misses so a given local date is fetched at most
	// once even under concurrent match resolution, and parrillaCache holds the
	// parsed snapshot per date, including failed attempts (see parrillaCacheEntry).
	parrillaFetcher parrilla.Fetcher
	parrillaCache   map[string]parrillaCacheEntry
	parrillaMu      sync.Mutex

	webCache   map[string]webCacheEntry
	webCacheMu sync.RWMutex
}

// parrillaCacheEntry is one cached daily snapshot. ok distinguishes a real
// schedule from a failed fetch, and fetchedAt bounds how long a failure is
// trusted so a transient outage can heal without restarting the process.
type parrillaCacheEntry struct {
	snap      parrilla.Snapshot
	ok        bool
	fetchedAt time.Time
}

type webCacheEntry struct {
	channels []model.Channel
	cachedAt time.Time
}

type customMatchRule struct {
	league   string
	pattern  *regexp.Regexp
	channels []model.Channel
}

// New creates a new Catalog resolver and starts background health checks on stream signals.
//
// Options are applied exactly once, before the external config file is read, because
// loadExternalConfig depends on the path set by WithConfigFile. Precedence rule:
// programmatic options always win over channels.json, so loadExternalConfig only
// fills in stream keys that are not already present in memory.
func New(opts ...Option) *Catalog {
	c := &Catalog{
		streamAlive:     make(map[string]bool),
		leagueOverrides: make(map[string][]model.Channel),
		userStreams:     make(map[string]string),
		tvGuide:         tvguide.NewService(tvguide.WithAutoRefresh(true)),
		parrillaFetcher: parrilla.NewGolCaracol(),
		parrillaCache:   make(map[string]parrillaCacheEntry),
		extractor:       extractor.NewService(),
		webCache:        make(map[string]webCacheEntry),
		now:             time.Now,
		streamCBSSN:     os.Getenv(envStreamCBSSN),
		streamGolazo:    os.Getenv(envStreamGolazo),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.loadExternalConfig()
	// Capture the configured US stream endpoints before the probe goroutine
	// starts, so the goroutine never reads the catalog's fields after New returns.
	envStreams := make([]string, 0, 2)
	if c.streamCBSSN != "" {
		envStreams = append(envStreams, c.streamCBSSN)
	}
	if c.streamGolazo != "" {
		envStreams = append(envStreams, c.streamGolazo)
	}
	go c.probeAllStreamsBackground(envStreams)
	if c.extractor != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = c.extractor.FetchAgenda(ctx)
		}()
	}
	return c
}

// SetTVGuide sets a custom or updated TV guide service.
func (c *Catalog) SetTVGuide(svc *tvguide.Service) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tvGuide = svc
}

// SetExtractor sets a custom or updated extractor service.
func (c *Catalog) SetExtractor(ext extractor.Extractor) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extractor = ext
}

func NewResolver() model.Resolver {
	return New()
}

// StreamEndpoint resolves a user-configured stream URL by canonical key.
func (c *Catalog) StreamEndpoint(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cleanKey := strings.ToLower(strings.TrimSpace(key))
	u, ok := c.userStreams[cleanKey]
	if !ok || strings.TrimSpace(u) == "" {
		return "", false
	}
	return u, true
}

func (c *Catalog) probeAllStreamsBackground(envStreams []string) {
	var urls []string

	c.mu.RLock()
	for _, u := range c.userStreams {
		if u != "" {
			urls = append(urls, u)
		}
	}
	c.mu.RUnlock()

	urls = append(urls, StreamRealMadrid, StreamTeledeporte)
	urls = append(urls, envStreams...)

	client := &http.Client{Timeout: 2500 * time.Millisecond}
	var wg sync.WaitGroup

	for _, u := range urls {
		cleanU, err := SanitizeURL(u)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")
			req.Header.Set("Range", "bytes=0-512")

			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				buf := make([]byte, 256)
				n, _ := io.ReadAtLeast(io.LimitReader(resp.Body, 1024), buf, 7)
				bodyStr := string(buf[:n])
				contentType := strings.ToLower(resp.Header.Get("Content-Type"))
				isHTML := strings.Contains(contentType, "text/html")
				isStreamContent := strings.Contains(contentType, "application/vnd.apple.mpegurl") ||
					strings.Contains(contentType, "application/x-mpegurl") ||
					strings.Contains(contentType, "audio/mpegurl") ||
					strings.Contains(contentType, "video/mp2t")

				if !isHTML && (strings.Contains(bodyStr, "#EXTM3U") || isStreamContent) {
					c.mu.Lock()
					c.streamAlive[target] = true
					c.mu.Unlock()
				}
			}
		}(cleanU)
	}
	wg.Wait()
}

// normalizeLeagueKey converts human-friendly league keys or ESPN league codes into the canonical league key.
func normalizeLeagueKey(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	for _, l := range model.DefaultLeagues {
		if strings.EqualFold(l.Key, clean) || strings.EqualFold(l.Code, clean) {
			return l.Key
		}
	}
	return clean
}

func (c *Catalog) loadExternalConfig() {
	cfgPath := c.configPath
	if cfgPath == "" {
		cfgPath = os.Getenv("KICKOFF_CHANNELS_PATH")
	}
	if cfgPath == "" {
		// Check next to executable (portable installation)
		if exe, exeErr := os.Executable(); exeErr == nil {
			exeDirCfg := filepath.Join(filepath.Dir(exe), "channels.json")
			if _, err := os.Stat(exeDirCfg); err == nil {
				cfgPath = exeDirCfg
			}
		}
	}
	if cfgPath == "" {
		// Check current working directory
		if _, err := os.Stat("channels.json"); err == nil {
			cfgPath = "channels.json"
		}
	}
	if cfgPath == "" {
		configBase, err := os.UserConfigDir()
		if err != nil || configBase == "" {
			if home, err := os.UserHomeDir(); err == nil {
				configBase = filepath.Join(home, ".config")
			}
		}
		if configBase != "" {
			cfgPath = filepath.Join(configBase, "kickoff", "channels.json")
		}
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		c.mu.Lock()
		c.loadAdjacentPlaylists()
		c.mu.Unlock()
		return
	}

	var ext ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
		// A malformed config silently degrades every channel lookup, which looks
		// like "nothing configured". Warn once, before the TUI takes the terminal.
		fmt.Fprintf(os.Stderr, "kickoff: invalid %s: %v\n", cfgPath, err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range ext.Streams {
		cleanK := strings.ToLower(strings.TrimSpace(k))
		// Precedence rule: programmatic streams (WithStreams) win over channels.json.
		if _, exists := c.userStreams[cleanK]; exists {
			continue
		}
		if cleanU, err := SanitizeURL(v); err == nil {
			c.userStreams[cleanK] = cleanU
		}
	}

	for lg, chs := range ext.LeagueChannels {
		sanitized := make([]model.Channel, 0, len(chs))
		for _, ch := range chs {
			if u, err := SanitizeURL(ch.URL); err == nil {
				ch.URL = u
				sanitized = append(sanitized, ch)
			}
		}
		if len(sanitized) > 0 {
			c.leagueOverrides[normalizeLeagueKey(lg)] = sanitized
		}
	}

	for _, r := range ext.CustomRules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			continue
		}
		sanitized := make([]model.Channel, 0, len(r.Channels))
		for _, ch := range r.Channels {
			if u, err := SanitizeURL(ch.URL); err == nil {
				ch.URL = u
				sanitized = append(sanitized, ch)
			}
		}
		if len(sanitized) > 0 {
			c.customRules = append(c.customRules, customMatchRule{
				league:   normalizeLeagueKey(r.League),
				pattern:  re,
				channels: sanitized,
			})
		}
	}

	if ext.PlaylistPath != "" {
		p := ext.PlaylistPath
		if strings.HasPrefix(p, "~/") || p == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if m3uList, err := ParseM3UFile(p); err == nil && len(m3uList) > 0 {
			c.m3uChannels = append(c.m3uChannels, m3uList...)
		}
	} else if ext.PlaylistURL != "" {
		if cleanURL, err := SanitizeURL(ext.PlaylistURL); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, cleanURL, nil)
			if err == nil {
				req.Header.Set("User-Agent", "kickoff-cli")
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				if err == nil {
					defer resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						limitedReader := io.LimitReader(resp.Body, maxPlaylistReadSize)
						if m3uList, err := ParseM3U(limitedReader); err == nil && len(m3uList) > 0 {
							c.m3uChannels = append(c.m3uChannels, m3uList...)
						}
					}
				}
			}
		}
	}
	c.loadAdjacentPlaylists()
}

func (c *Catalog) loadAdjacentPlaylists() {
	if len(c.m3uChannels) > 0 {
		return
	}
	var candidateM3Us []string
	if exe, exeErr := os.Executable(); exeErr == nil {
		exeDir := filepath.Dir(exe)
		candidateM3Us = append(candidateM3Us,
			filepath.Join(exeDir, "signals.m3u"),
			filepath.Join(exeDir, "playlist.m3u"),
		)
	}
	candidateM3Us = append(candidateM3Us, "signals.m3u", "playlist.m3u")
	for _, mPath := range candidateM3Us {
		if m3uList, err := ParseM3UFile(mPath); err == nil && len(m3uList) > 0 {
			c.m3uChannels = append(c.m3uChannels, m3uList...)
			break
		}
	}
}

func m3uToChannel(m3u M3UChannel, recommended bool) model.Channel {
	res := "1080p"
	nameLower := strings.ToLower(m3u.Name)
	if strings.Contains(nameLower, "720p") {
		res = "720p"
	} else if strings.Contains(nameLower, "4k") || strings.Contains(nameLower, "uhd") {
		res = "4K"
	}
	return model.Channel{
		DisplayName: m3u.Name,
		Name:        m3u.Name,
		URL:         m3u.URL,
		Resolution:  res,
		BitrateMode: "max",
		Recommended: recommended,
	}
}

func makeDualProfile(label, rawURL string, recommended bool) []model.Channel {
	cleanURL, err := SanitizeURL(rawURL)
	if err != nil || cleanURL == "" {
		return nil
	}
	return []model.Channel{
		{
			DisplayName: label,
			Name:        label,
			URL:         cleanURL,
			Resolution:  "720p",
			BitrateMode: "min",
			Recommended: recommended,
		},
		{
			DisplayName: label,
			Name:        label,
			URL:         cleanURL,
			Resolution:  "1080p",
			BitrateMode: "max",
			Recommended: false,
		},
	}
}

func makeSingleChannel(label, rawURL, res, bitrate string, recommended bool) (model.Channel, bool) {
	cleanURL, err := SanitizeURL(rawURL)
	if err != nil || cleanURL == "" {
		return model.Channel{}, false
	}
	return model.Channel{
		DisplayName: label,
		Name:        label,
		URL:         cleanURL,
		Resolution:  res,
		BitrateMode: bitrate,
		Recommended: recommended,
	}, true
}

// officialEndpointRule maps a set of lower-cased broadcast substrings to a
// configured stream endpoint. Rules are order-sensitive: the first rule whose
// needle matches wins, so specific channels must precede generic ones
// (e.g. "ESPN 7" before "espn").
type officialEndpointRule struct {
	needles     []string
	endpointKey string
	label       string
	// dual emits a 720p/min plus a 1080p/max profile; otherwise a single 1080p/max channel.
	dual bool
}

var officialEndpointRules = []officialEndpointRule{
	{[]string{"espn 7", "espn7"}, "espn7", "ESPN 7 HD", true},
	{[]string{"espn 6", "espn6"}, "espn6", "ESPN 6 HD", true},
	{[]string{"espn 5", "espn5"}, "espn5", "ESPN 5 HD", true},
	{[]string{"espn 4", "espn4"}, "espn4", "ESPN 4 HD", true},
	{[]string{"espn 3", "espn3"}, "espn3", "ESPN 3 HD", true},
	{[]string{"espn 2", "espn2"}, "espn2", "ESPN 2 HD", true},
	{[]string{"espn deportes"}, "espn_deportes", "ESPN Deportes", true},
	{[]string{"espn+"}, "espn_plus", "ESPN+", false},
	{[]string{"espn extra"}, "espn4", "ESPN 4 HD", true},
	{[]string{"espn"}, "espn1", "ESPN 1 HD", true},
	// DSports family (Argentina/Colombia football). The specific channel needles
	// must precede the generic "dsports" needle for first-match-wins ordering.
	{[]string{"dsports 4", "dsports4"}, "dsports4", "DSports 4 HD", false},
	{[]string{"dsports 2", "dsports2"}, "dsports2", "DSports 2 HD", false},
	{[]string{"dsports+", "dsports plus"}, "dsports_plus", "DSports+", false},
	{[]string{"dsports"}, "dsports", "DSports HD", false},
	{[]string{"claro sports", "claro_sports"}, "claro_sports", "Claro Sports", false},
	{[]string{"tyc"}, "tyc", "TyC Sports HD", false},
}

// containsAny reports whether s contains any of the given needles.
func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// appendSingleEndpoint appends a single 1080p/max channel when the endpoint key resolves.
func (c *Catalog) appendSingleEndpoint(result *[]model.Channel, key, label string, recommended bool) {
	if u, ok := c.StreamEndpoint(key); ok {
		if ch, valid := makeSingleChannel(label, u, "1080p", "max", recommended); valid {
			*result = append(*result, ch)
		}
	}
}

// appendDualEndpoint appends a 720p/min plus 1080p/max profile when the endpoint key resolves.
func (c *Catalog) appendDualEndpoint(result *[]model.Channel, key, label string) {
	if u, ok := c.StreamEndpoint(key); ok {
		*result = append(*result, makeDualProfile(label, u, len(*result) == 0)...)
	}
}

// appendSingleFromURL appends a single 1080p/max channel built from a raw URL.
func (c *Catalog) appendSingleFromURL(result *[]model.Channel, rawURL, label string, recommended bool) {
	if ch, valid := makeSingleChannel(label, rawURL, "1080p", "max", recommended); valid {
		*result = append(*result, ch)
	}
}

// applyOfficialEndpointRule applies the first matching rule from officialEndpointRules
// and reports whether the broadcast was handled. A matched rule always consumes the
// broadcast, even when its endpoint is not configured, preserving first-match-wins order.
func (c *Catalog) applyOfficialEndpointRule(bl string, result *[]model.Channel) bool {
	for _, rule := range officialEndpointRules {
		if !containsAny(bl, rule.needles...) {
			continue
		}
		if rule.dual {
			c.appendDualEndpoint(result, rule.endpointKey, rule.label)
		} else {
			c.appendSingleEndpoint(result, rule.endpointKey, rule.label, len(*result) == 0)
		}
		return true
	}
	return false
}

func (c *Catalog) resolveFromOfficialBroadcasts(broadcasts []string) []model.Channel {
	var result []model.Channel
	for _, b := range broadcasts {
		bl := strings.ToLower(strings.TrimSpace(b))

		// Compound national broadcasters emit a primary plus a secondary channel.
		if containsAny(bl, "gol caracol") {
			c.appendSingleEndpoint(&result, "gol_caracol", "Gol Caracol", len(result) == 0)
			c.appendSingleEndpoint(&result, "caracol", "Caracol TV HD", len(result) == 0)
			c.appendSingleEndpoint(&result, "caracol_hd2", "Caracol HD2", false)
			continue
		}
		if containsAny(bl, "caracol") {
			c.appendSingleEndpoint(&result, "caracol", "Caracol TV HD", len(result) == 0)
			c.appendSingleEndpoint(&result, "caracol_hd2", "Caracol HD2", false)
			continue
		}
		if containsAny(bl, "rcn", "canal rcn", "deportes rcn") {
			c.appendSingleEndpoint(&result, "rcn", "Canal RCN HD", len(result) == 0)
			c.appendSingleEndpoint(&result, "rcn_hd2", "RCN HD2", false)
			continue
		}
		if containsAny(bl, "win sports mas", "win sports más", "win mas", "win más") {
			c.appendSingleEndpoint(&result, "win_sports_mas", "Win Sports Más", len(result) == 0)
			c.appendSingleEndpoint(&result, "win_plus", "Win+ Fútbol", len(result) == 0)
			c.appendSingleEndpoint(&result, "win", "Win Sports HD", false)
			continue
		}
		if containsAny(bl, "win", "win sports", "win+") {
			c.appendSingleEndpoint(&result, "win_plus", "Win+ Fútbol", len(result) == 0)
			c.appendSingleEndpoint(&result, "win", "Win Sports HD", false)
			continue
		}

		// Simple endpoint rules, evaluated in declaration order (first match wins).
		if c.applyOfficialEndpointRule(bl, &result) {
			continue
		}

		// Free-to-air fallbacks and env-configured US streams.
		if containsAny(bl, "real madrid", "rmtv") {
			u, ok := c.StreamEndpoint("realmadrid")
			if !ok {
				u = StreamRealMadrid
			}
			c.appendSingleFromURL(&result, u, "Real Madrid TV", len(result) == 0)
			continue
		}
		if containsAny(bl, "teledeporte", "rtve") {
			u, ok := c.StreamEndpoint("teledeporte")
			if !ok {
				u = StreamTeledeporte
			}
			c.appendSingleFromURL(&result, u, "Teledeporte HD", len(result) == 0)
			continue
		}
		if containsAny(bl, "cbssn", "cbs sports network", "cbs sports") {
			c.appendSingleFromURL(&result, c.streamCBSSN, "CBS Sports Network", len(result) == 0)
			continue
		}
		if containsAny(bl, "paramount", "golazo", "cbs sports golazo") {
			c.appendSingleFromURL(&result, c.streamGolazo, "CBS Sports Golazo Network", len(result) == 0)
			continue
		}
	}
	return result
}

func hasUSBroadcast(broadcasts []string) bool {
	for _, b := range broadcasts {
		bl := strings.ToLower(strings.TrimSpace(b))
		if strings.Contains(bl, "cbssn") ||
			strings.Contains(bl, "cbs sports") ||
			strings.Contains(bl, "paramount") ||
			strings.Contains(bl, "golazo") ||
			strings.Contains(bl, "peacock") ||
			strings.Contains(bl, "usa net") ||
			strings.Contains(bl, "nbc") ||
			strings.Contains(bl, "universo") ||
			strings.Contains(bl, "telemundo") ||
			strings.Contains(bl, "espn+") ||
			strings.Contains(bl, "vix") ||
			strings.Contains(bl, "fubo") ||
			strings.Contains(bl, "tudn") {
			return true
		}
	}
	return false
}

// m3uSnapshot returns the current M3U channel slice under the read lock.
func (c *Catalog) m3uSnapshot() []M3UChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m3uChannels
}

func (c *Catalog) buildChannelsForTarget(target tvguide.ChannelTarget) []model.Channel {
	streamURL := target.StreamURL
	if streamURL == "" {
		switch target.Name {
		case "ESPN 1 HD":
			streamURL, _ = c.StreamEndpoint("espn1")
		case "ESPN 2 HD":
			streamURL, _ = c.StreamEndpoint("espn2")
		case "ESPN 3 HD":
			streamURL, _ = c.StreamEndpoint("espn3")
		case "ESPN 4 HD":
			streamURL, _ = c.StreamEndpoint("espn4")
		case "ESPN 5 HD":
			streamURL, _ = c.StreamEndpoint("espn5")
		case "ESPN 6 HD":
			streamURL, _ = c.StreamEndpoint("espn6")
		case "ESPN 7 HD":
			streamURL, _ = c.StreamEndpoint("espn7")
		case "Win Sports HD":
			streamURL, _ = c.StreamEndpoint("win")
		case "Win+ Fútbol":
			streamURL, _ = c.StreamEndpoint("win_plus")
		case "Caracol TV HD":
			streamURL, _ = c.StreamEndpoint("caracol")
		case "Canal RCN HD":
			streamURL, _ = c.StreamEndpoint("rcn")
		}
	}

	if streamURL == "" {
		if m3u, ok := MatchM3UChannel(target.Name, c.m3uSnapshot()); ok {
			streamURL = m3u.URL
		}
	}

	cleanURL, _ := SanitizeURL(streamURL)
	if cleanURL == "" {
		return nil
	}

	switch {
	case strings.HasPrefix(target.Name, "ESPN"):
		return makeDualProfile(target.Name, cleanURL, true)
	case target.Name == "Win+ Fútbol":
		var list []model.Channel
		if ch, ok := makeSingleChannel("Win+ Fútbol", cleanURL, "1080p", "max", true); ok {
			list = append(list, ch)
		}
		if u, found := c.StreamEndpoint("win"); found {
			if ch, ok := makeSingleChannel("Win Sports HD", u, "1080p", "max", false); ok {
				list = append(list, ch)
			}
		}
		return list
	case target.Name == "Win Sports HD":
		var list []model.Channel
		if ch, ok := makeSingleChannel("Win Sports HD", cleanURL, "1080p", "max", true); ok {
			list = append(list, ch)
		}
		if u, found := c.StreamEndpoint("win_plus"); found {
			if ch, ok := makeSingleChannel("Win+ Fútbol", u, "1080p", "max", false); ok {
				list = append(list, ch)
			}
		}
		return list
	case target.Name == "Caracol TV HD":
		var list []model.Channel
		if ch, ok := makeSingleChannel("Caracol TV HD", cleanURL, "1080p", "max", true); ok {
			list = append(list, ch)
		}
		if u, found := c.StreamEndpoint("caracol_hd2"); found {
			if ch, ok := makeSingleChannel("Caracol HD2", u, "1080p", "max", false); ok {
				list = append(list, ch)
			}
		}
		return list
	case target.Name == "Canal RCN HD":
		var list []model.Channel
		if ch, ok := makeSingleChannel("Canal RCN HD", cleanURL, "1080p", "max", true); ok {
			list = append(list, ch)
		}
		if u, found := c.StreamEndpoint("rcn_hd2"); found {
			if ch, ok := makeSingleChannel("RCN HD2", u, "1080p", "max", false); ok {
				list = append(list, ch)
			}
		}
		return list
	default:
		if ch, ok := makeSingleChannel(target.Name, cleanURL, "1080p", "max", true); ok {
			return []model.Channel{ch}
		}
		return nil
	}
}

// GetChannels resolves match-specific broadcast channels based on user configuration.
func (c *Catalog) GetChannels(match model.Match) []model.Channel {
	if match.Status.Completed || strings.ToLower(match.Status.State) == "post" {
		return nil
	}

	title := match.Title
	if title == "" {
		title = fmt.Sprintf("%s vs %s", match.HomeTeam.DisplayName, match.AwayTeam.DisplayName)
	}
	leagueKey := normalizeLeagueKey(match.LeagueKey)

	// Snapshot shared, mutable resolver state under the read lock. Reading into
	// locals keeps the lock held only briefly and avoids re-entrant RLock while
	// helper functions (which take their own RLock) are called below.
	c.mu.RLock()
	customRules := c.customRules
	leagueOverrides := c.leagueOverrides
	tvGuide := c.tvGuide
	ext := c.extractor
	c.mu.RUnlock()

	// 1. Check user custom rules & overrides from ~/.config/kickoff/channels.json
	for _, rule := range customRules {
		if (rule.league == "" || rule.league == leagueKey) && rule.pattern.MatchString(title) {
			return rule.channels
		}
	}
	if override, ok := leagueOverrides[leagueKey]; ok && len(override) > 0 {
		return override
	}

	var candidates []model.Channel

	// 2. Official broadcasters reported by ESPN metadata for this specific match (verified truth).
	if len(match.Broadcasts) > 0 {
		candidates = append(candidates, c.resolveFromOfficialBroadcasts(match.Broadcasts)...)
		m3uList := c.m3uSnapshot()
		if len(m3uList) > 0 {
			for _, b := range match.Broadcasts {
				if m3u, found := MatchM3UChannel(b, m3uList); found {
					candidates = append(candidates, m3uToChannel(m3u, len(candidates) == 0))
				}
			}
		}
	}

	// 3. Gol Caracol daily parrilla (T1): the authoritative per-match Colombian
	// broadcast listing. It sits below ESPN's own metadata (real per-match
	// evidence) but above the generic TV guide crawl, and it never invents a
	// channel: entries still marked "por definir" and channels without a
	// configured stream URL yield nothing.
	if len(candidates) == 0 {
		if parrillaChs := c.resolveFromParrilla(match); len(parrillaChs) > 0 {
			candidates = append(candidates, parrillaChs...)
		}
	}

	// 4. TV Guide Crawler resolution. A non-US broadcaster that ESPN already
	// named and that we cannot play rules out the generic TV guide crawl.
	if len(candidates) == 0 && (len(match.Broadcasts) == 0 || hasUSBroadcast(match.Broadcasts)) {
		if tvGuide != nil {
			if target, found := tvGuide.ResolveMatch(match); found {
				candidates = append(candidates, c.buildChannelsForTarget(target)...)
			}
		}
	}

	// 5. Dynamic Web Extractor (Enrichment & Multi-Source Options):
	// Check web cache first. If not cached, attempt a fast 50ms check
	// for in-memory mocks in tests, leaving network fetches for async enrichment.
	if ext != nil {
		cacheKey := match.ID
		if cacheKey == "" {
			cacheKey = fmt.Sprintf("%s:%s", match.LeagueKey, title)
		}

		c.webCacheMu.RLock()
		entry, cached := c.webCache[cacheKey]
		c.webCacheMu.RUnlock()

		if cached && time.Since(entry.cachedAt) < 30*time.Minute {
			candidates = append(candidates, entry.channels...)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if webChs := c.ResolveWebChannels(ctx, match); len(webChs) > 0 {
				candidates = append(candidates, webChs...)
			}
		}
	}

	if len(candidates) > 0 {
		return c.filterAndDeduplicate(candidates)
	}

	// No verified evidence for this match. Never guess a channel: an unverified
	// channel is strictly worse than none, because it sends the user to the wrong
	// feed. Return an empty result.
	return nil
}

// ResolveWebChannels queries the dynamic web extractor for additional live streams,
// updates the in-memory webCache, and returns the newly resolved channels.
func (c *Catalog) ResolveWebChannels(ctx context.Context, match model.Match) []model.Channel {
	c.mu.RLock()
	ext := c.extractor
	c.mu.RUnlock()
	if ext == nil {
		return nil
	}

	title := match.Title
	if title == "" {
		title = fmt.Sprintf("%s vs %s", match.HomeTeam.DisplayName, match.AwayTeam.DisplayName)
	}

	cacheKey := match.ID
	if cacheKey == "" {
		cacheKey = fmt.Sprintf("%s:%s", normalizeLeagueKey(match.LeagueKey), title)
	}

	c.webCacheMu.RLock()
	entry, cached := c.webCache[cacheKey]
	c.webCacheMu.RUnlock()

	if cached && time.Since(entry.cachedAt) < 30*time.Minute && len(entry.channels) > 0 {
		return entry.channels
	}

	streams, err := ext.ResolveMatchStreams(ctx, match)
	if err != nil || len(streams) == 0 {
		if cached && len(entry.channels) > 0 {
			return entry.channels
		}
		return nil
	}

	var webChs []model.Channel
	for _, s := range streams {
		if s.StreamURL == "" {
			continue
		}
		cleanURL, err := SanitizeURL(s.StreamURL)
		if err != nil {
			continue
		}
		name := formatWebChannelName(s.ChannelName)
		res := "1080p"
		if s.Resolution != "" {
			res = s.Resolution
		}
		webChs = append(webChs, model.Channel{
			DisplayName: name,
			Name:        name,
			URL:         cleanURL,
			Resolution:  res,
			BitrateMode: "max",
			Headers:     s.Headers,
			Recommended: false,
		})
	}

	if len(webChs) > 0 {
		c.webCacheMu.Lock()
		c.webCache[cacheKey] = webCacheEntry{
			channels: webChs,
			cachedAt: time.Now(),
		}
		c.webCacheMu.Unlock()
	}

	return webChs
}

// PeekCachedChannels returns currently cached channels for a match in memory
// without performing any network I/O, safely callable from the event loop in 0ms.
func (c *Catalog) PeekCachedChannels(match model.Match) []model.Channel {
	title := match.Title
	if title == "" {
		title = fmt.Sprintf("%s vs %s", match.HomeTeam.DisplayName, match.AwayTeam.DisplayName)
	}
	cacheKey := match.ID
	if cacheKey == "" {
		cacheKey = fmt.Sprintf("%s:%s", normalizeLeagueKey(match.LeagueKey), title)
	}

	c.webCacheMu.RLock()
	entry, cached := c.webCache[cacheKey]
	c.webCacheMu.RUnlock()

	if cached && time.Since(entry.cachedAt) < 30*time.Minute && len(entry.channels) > 0 {
		return entry.channels
	}
	return nil
}

// PrewarmMatches pre-resolves web streams for matches in the background with bounded
// concurrency so that user interactions resolve in 0ms.
func (c *Catalog) PrewarmMatches(ctx context.Context, matches []model.Match) {
	c.mu.RLock()
	ext := c.extractor
	c.mu.RUnlock()
	if ext == nil || len(matches) == 0 {
		return
	}

	// Warm the agenda once up-front so worker goroutines don't race on network fetching
	_, _ = ext.FetchAgenda(ctx)

	// Prioritize live matches ("in") and active fixtures first
	prioritized := make([]model.Match, 0, len(matches))
	for _, m := range matches {
		if m.Status.Completed || strings.ToLower(m.Status.State) == "post" {
			continue
		}
		if strings.ToLower(m.Status.State) == "in" {
			prioritized = append([]model.Match{m}, prioritized...)
		} else {
			prioritized = append(prioritized, m)
		}
	}

	sem := make(chan struct{}, 4)
	for _, m := range prioritized {
		title := m.Title
		if title == "" {
			title = fmt.Sprintf("%s vs %s", m.HomeTeam.DisplayName, m.AwayTeam.DisplayName)
		}
		cacheKey := m.ID
		if cacheKey == "" {
			cacheKey = fmt.Sprintf("%s:%s", normalizeLeagueKey(m.LeagueKey), title)
		}

		c.webCacheMu.RLock()
		entry, cached := c.webCache[cacheKey]
		c.webCacheMu.RUnlock()
		if cached && time.Since(entry.cachedAt) < 30*time.Minute && len(entry.channels) > 0 {
			continue
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		go func(targetMatch model.Match) {
			defer func() { <-sem }()
			pCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			_ = c.ResolveWebChannels(pCtx, targetMatch)
		}(m)
	}
}

func formatWebChannelName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = "Stream"
	}
	if strings.HasPrefix(name, "[Web]") {
		return name
	}
	upper := strings.ToUpper(name)
	if !strings.Contains(upper, "HD") && !strings.Contains(upper, "4K") && !strings.Contains(upper, "720P") && !strings.Contains(upper, "1080P") {
		return fmt.Sprintf("[Web] %s HD", name)
	}
	return fmt.Sprintf("[Web] %s", name)
}

// parrillaSignal identifies a canonical stream key together with the label used
// when the signal is emitted as a channel.
type parrillaSignal struct {
	key   string
	label string
}

// parrillaCompanionSignals mirrors the compound national broadcaster expansion
// used for ESPN metadata (tier 2): a fixture published on one of these feeds is
// also available on the companion feeds listed here.
func parrillaCompanionSignals(primary string) []parrillaSignal {
	switch primary {
	case "gol_caracol":
		return []parrillaSignal{
			{key: "caracol", label: "Caracol TV HD"},
			{key: "caracol_hd2", label: "Caracol HD2"},
		}
	case "caracol":
		return []parrillaSignal{{key: "caracol_hd2", label: "Caracol HD2"}}
	case "rcn":
		return []parrillaSignal{{key: "rcn_hd2", label: "RCN HD2"}}
	case "win_sports_mas":
		return []parrillaSignal{
			{key: "win_plus", label: "Win+ Fútbol"},
			{key: "win", label: "Win Sports HD"},
		}
	case "win_plus":
		return []parrillaSignal{{key: "win", label: "Win Sports HD"}}
	case "win":
		return []parrillaSignal{{key: "win_plus", label: "Win+ Fútbol"}}
	}
	return nil
}

// resolveSignalURL resolves a canonical signal from the user's stream map first
// and falls back to the imported M3U playlist, the same order tiers 2 and 4 use.
func (c *Catalog) resolveSignalURL(key, label string) (string, bool) {
	if u, ok := c.StreamEndpoint(key); ok {
		return u, true
	}
	if m3u, ok := MatchM3UChannel(label, c.m3uSnapshot()); ok {
		return m3u.URL, true
	}
	return "", false
}

// resolveFromParrilla resolves a match through the Gol Caracol daily parrilla.
//
// It matches entries with the same BOTH-teams rule the tvguide package uses
// (tvguide.MatchScore), preferring the entry whose published start time falls in
// the match window. Each published channel is canonicalized to a stream key and
// resolved from the user's stream map, falling back to the imported M3U playlist.
// A resolved signal is emitted as a 720p/min plus 1080p/max pair, and compound
// national broadcasters also emit their companion feeds. When no listed channel
// resolves to a URL, or the schedule is unreachable, it returns nothing so lower
// tiers can run.
func (c *Catalog) resolveFromParrilla(match model.Match) []model.Channel {
	snap, ok := c.parrillaSnapshot(parrillaDay(match))
	if !ok {
		return nil
	}

	entry, found := matchParrillaEntry(snap, match)
	// An empty channel list is the honest "por definir" state: emit nothing
	// rather than guess.
	if !found || len(entry.Channels) == 0 {
		return nil
	}

	var candidates []model.Channel
	for _, name := range entry.Channels {
		key, _, known := parrilla.CanonicalChannel(name)
		if !known {
			continue
		}
		label, known := parrilla.CanonicalDisplay(name)
		if !known {
			label = name
		}
		if url, resolved := c.resolveSignalURL(key, label); resolved {
			candidates = append(candidates, makeDualProfile(label, url, len(candidates) == 0)...)
		}
		for _, companion := range parrillaCompanionSignals(key) {
			url, resolved := c.resolveSignalURL(companion.key, companion.label)
			if !resolved {
				continue
			}
			if ch, valid := makeSingleChannel(companion.label, url, "1080p", "max", len(candidates) == 0); valid {
				candidates = append(candidates, ch)
			}
		}
	}
	return c.filterAndDeduplicate(candidates)
}

// parrillaDay returns the Colombian calendar date whose Gol Caracol schedule
// applies to a match: its kickoff day in the source timezone, or today in the
// source timezone when the kickoff is unknown. The times published in the
// parrilla are Colombian local times, so the display timezone must never decide
// which daily schedule is requested.
func parrillaDay(match model.Match) time.Time {
	loc := locale.BogotaLocation()
	if match.StartTime.IsZero() {
		return time.Now().In(loc)
	}
	return match.StartTime.In(loc)
}

// parrillaSnapshot returns the cached parrilla snapshot for a date, fetching it at
// most once per source calendar day. A successful snapshot is cached for its whole
// day; a failed fetch is cached only for parrillaFailureTTL, so a transient outage
// is retried during the same day instead of hiding the grid until midnight.
func (c *Catalog) parrillaSnapshot(date time.Time) (parrilla.Snapshot, bool) {
	key := date.In(locale.BogotaLocation()).Format("2006-01-02")

	c.mu.RLock()
	fetcher := c.parrillaFetcher
	entry, ok := c.parrillaCache[key]
	c.mu.RUnlock()
	if ok && c.parrillaEntryFresh(entry) {
		return entry.snap, true
	}
	if fetcher == nil {
		return parrilla.Snapshot{}, false
	}

	// Serialize cache misses: without this, two concurrent resolutions before the
	// first fetch completes would both hit the network. The double-check settles
	// whichever caller lost the race.
	c.parrillaMu.Lock()
	defer c.parrillaMu.Unlock()

	c.mu.RLock()
	fetcher = c.parrillaFetcher
	entry, ok = c.parrillaCache[key]
	c.mu.RUnlock()
	if ok && c.parrillaEntryFresh(entry) {
		return entry.snap, true
	}
	if fetcher == nil {
		return parrilla.Snapshot{}, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), parrillaFetchTimeout)
	defer cancel()
	snap, err := fetcher.Fetch(ctx, date)
	fetchOK := err == nil
	if !fetchOK {
		// Degrade gracefully: cache the empty result and let lower tiers run.
		snap = parrilla.Snapshot{Date: key}
	}

	c.mu.Lock()
	if c.parrillaCache == nil {
		c.parrillaCache = make(map[string]parrillaCacheEntry)
	}
	c.parrillaCache[key] = parrillaCacheEntry{snap: snap, ok: fetchOK, fetchedAt: c.now()}
	c.mu.Unlock()

	return snap, true
}

// parrillaEntryFresh reports whether a cached entry may still be served.
// Successful entries live for their whole source day (the cache key), while
// failures expire after parrillaFailureTTL to allow an in-day retry.
func (c *Catalog) parrillaEntryFresh(entry parrillaCacheEntry) bool {
	if entry.ok {
		return true
	}
	return c.now().Sub(entry.fetchedAt) < parrillaFailureTTL
}

// matchParrillaEntry finds the parrilla entry for a match. Team matching reuses
// tvguide.MatchScore, which requires BOTH teams (or the full fixture title) to be
// identified; a single team never matches. When several entries match, the one
// whose published start time overlaps the match window wins; if none overlaps and
// the match is unambiguous, that single entry is used.
func matchParrillaEntry(snap parrilla.Snapshot, match model.Match) (parrilla.Entry, bool) {
	matchMins, hasTime := localClockMinutes(match.StartTime)

	matchedCount := 0
	var first, inWindow *parrilla.Entry
	for i := range snap.Entries {
		entry := &snap.Entries[i]
		title := strings.TrimSpace(entry.Raw)
		if title == "" {
			title = fmt.Sprintf("%s vs %s", entry.HomeTeam, entry.AwayTeam)
		}
		if ok, _ := tvguide.MatchScore(title, match); !ok {
			continue
		}
		matchedCount++
		if first == nil {
			first = entry
		}
		if hasTime && entry.StartMins >= 0 && parrillaWindowOverlap(entry.StartMins, matchMins) {
			if inWindow == nil {
				inWindow = entry
			}
		}
	}

	if inWindow != nil {
		return *inWindow, true
	}
	if matchedCount == 1 {
		return *first, true
	}
	return parrilla.Entry{}, false
}

// localClockMinutes converts a source schedule timestamp into minutes since
// midnight in the source timezone. Guide and parrilla times are published as
// Colombian wall-clock values, so the match kickoff must be read in the same
// zone before the two windows are compared.
func localClockMinutes(t time.Time) (int, bool) {
	if t.IsZero() {
		return 0, false
	}
	lt := t.In(locale.BogotaLocation())
	return lt.Hour()*60 + lt.Minute(), true
}

// parrillaWindowOverlap reports whether a parrilla entry starting at programStart
// overlaps the match window starting at matchStart. It mirrors the tvguide
// intervalsOverlap helper: both intervals last parrillaMatchWindowMinutes and
// midnight wrap-around is tolerated.
func parrillaWindowOverlap(programStart, matchStart int) bool {
	const dur = parrillaMatchWindowMinutes
	for _, shiftA := range [...]int{-1440, 0, 1440} {
		a1, a2 := programStart+shiftA, programStart+dur+shiftA
		for _, shiftB := range [...]int{-1440, 0, 1440} {
			b1, b2 := matchStart+shiftB, matchStart+dur+shiftB
			if a1 < b2 && b1 < a2 {
				return true
			}
		}
	}
	return false
}

func (c *Catalog) filterAndDeduplicate(candidates []model.Channel) []model.Channel {
	if len(candidates) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var aliveList, unprobedList []model.Channel

	c.mu.RLock()
	hasProbes := len(c.streamAlive) > 0
	for _, ch := range candidates {
		cleanURL, err := SanitizeURL(ch.URL)
		if err != nil {
			// Playability gate: a channel with no resolvable stream URL is not a
			// playable option and must never be emitted as one.
			continue
		}
		ch.URL = cleanURL

		key := ch.URL + "|" + ch.Resolution
		if seen[key] {
			continue
		}
		seen[key] = true

		if !hasProbes || c.streamAlive[ch.URL] {
			aliveList = append(aliveList, ch)
		} else {
			unprobedList = append(unprobedList, ch)
		}
	}
	c.mu.RUnlock()

	final := append(aliveList, unprobedList...)
	if len(final) == 0 {
		return nil
	}
	if len(final) > 8 {
		final = final[:8]
	}

	for i := range final {
		final[i].Recommended = (i == 0)
	}

	return final
}

// HasPlayableSource reports whether GetChannels yields at least one channel the
// user can actually play, i.e. a channel whose stream URL resolves. The UI uses
// this to decide visibility under the product rule: a match aired only on
// channels we cannot play (for example Disney+ Premium only, with no Disney+
// configured) is neither shown nor mentioned anywhere.
func (c *Catalog) HasPlayableSource(match model.Match) bool {
	for _, ch := range c.GetChannels(match) {
		if _, err := SanitizeURL(ch.URL); err == nil {
			return true
		}
	}
	return false
}

// HasConfiguredStreams reports whether the user has ANY usable stream source
// configured: a non-empty channels.json "streams" entry, a programmatic stream, or
// a loaded M3U playlist.
//
// The UI uses it as a safety valve: when nothing is configured, the visibility
// rule would filter out every fixture and leave the app empty, so the TUI shows an
// actionable configuration hint instead of a silent empty list.
func (c *Catalog) HasConfiguredStreams() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, u := range c.userStreams {
		if strings.TrimSpace(u) != "" {
			return true
		}
	}
	return len(c.m3uChannels) > 0
}
