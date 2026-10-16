package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/tvguide"
)

// Public free-to-air (FTA) channels with legal webcasts.
const (
	StreamRealMadrid  = "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8"
	StreamTeledeporte = "https://rtvelivestream.akamaized.net/rtvesec/tdp/tdp_main.m3u8"
)

// Optional US stream endpoints (configurable via environment variables).
var (
	StreamCBSSportsNetwork = os.Getenv("KICKOFF_STREAM_CBSSN")
	StreamCBSGolazo        = os.Getenv("KICKOFF_STREAM_GOLAZO")
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
	if strings.TrimSpace(rawURL) == "" {
		return "", model.ErrInvalidURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: %v", model.ErrInvalidURL, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: unsupported scheme %s", model.ErrInvalidURL, parsed.Scheme)
	}

	if parsed.User != nil {
		return "", fmt.Errorf("%w: embedded credentials not allowed", model.ErrInvalidURL)
	}

	return parsed.String(), nil
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
		tvGuide:         tvguide.NewService(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.loadExternalConfig()
	// Capture the env-configured US stream globals before the probe goroutine starts,
	// so the goroutine never reads package-level mutable state concurrently with
	// runtime overrides (e.g. tests that set StreamCBSSportsNetwork/StreamCBSGolazo).
	envStreams := make([]string, 0, 2)
	if StreamCBSSportsNetwork != "" {
		envStreams = append(envStreams, StreamCBSSportsNetwork)
	}
	if StreamCBSGolazo != "" {
		envStreams = append(envStreams, StreamCBSGolazo)
	}
	go c.probeAllStreamsBackground(envStreams)
	return c
}

// SetTVGuide sets a custom or updated TV guide service.
func (c *Catalog) SetTVGuide(svc *tvguide.Service) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tvGuide = svc
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
				n, _ := io.ReadAtLeast(resp.Body, buf, 7)
				bodyStr := string(buf[:n])
				if strings.Contains(bodyStr, "#EXTM3U") || resp.StatusCode == http.StatusOK {
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
		return
	}

	var ext ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
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
	{[]string{"espn 2", "espn2", "espn deportes"}, "espn2", "ESPN 2 HD", true},
	{[]string{"espn extra"}, "espn_extra", "ESPN Extra", true},
	{[]string{"espn"}, "espn1", "ESPN 1 HD", true},
	{[]string{"fox sports 3", "fs3"}, "fox3", "Fox Sports 3", true},
	{[]string{"fox sports 2", "fs2"}, "fox2", "Fox Sports 2", true},
	{[]string{"fox sports 1", "fs1", "fox deportes", "fox sports", "fox"}, "fox1", "Fox Sports 1 HD", false},
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
		if containsAny(bl, "caracol", "gol caracol") {
			c.appendSingleEndpoint(&result, "caracol", "Caracol TV HD", len(result) == 0)
			c.appendSingleEndpoint(&result, "caracol_hd2", "Caracol HD2", false)
			continue
		}
		if containsAny(bl, "rcn", "canal rcn", "deportes rcn") {
			c.appendSingleEndpoint(&result, "rcn", "Canal RCN HD", len(result) == 0)
			c.appendSingleEndpoint(&result, "rcn_hd2", "RCN HD2", false)
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
			c.appendSingleFromURL(&result, StreamCBSSportsNetwork, "CBS Sports Network", len(result) == 0)
			continue
		}
		if containsAny(bl, "paramount", "golazo", "cbs sports golazo") {
			c.appendSingleFromURL(&result, StreamCBSGolazo, "CBS Sports Golazo Network", len(result) == 0)
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

func (c *Catalog) espnBySlot(slot int) (name, url string, ok bool) {
	slots := []struct {
		name    string
		streamK string
	}{
		{"ESPN 1 HD", "espn1"},
		{"ESPN 2 HD", "espn2"},
		{"ESPN 3 HD", "espn3"},
		{"ESPN 4 HD", "espn4"},
		{"ESPN 5 HD", "espn5"},
		{"ESPN 6 HD", "espn6"},
		{"ESPN 7 HD", "espn7"},
	}
	if slot < 0 || slot >= len(slots) {
		return "", "", false
	}
	u, found := c.endpointOrM3U(slots[slot].streamK, slots[slot].name)
	if !found {
		return "", "", false
	}
	return slots[slot].name, u, true
}

// m3uSnapshot returns the current M3U channel slice under the read lock.
func (c *Catalog) m3uSnapshot() []M3UChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m3uChannels
}

// endpointOrM3U resolves a channel URL by configured endpoint key, falling back to a
// best-effort M3U playlist match by channel name. The M3U fallback exists because users
// may supply playlists instead of per-key endpoints; without it those channels would be
// unresolvable even when present in the playlist.
func (c *Catalog) endpointOrM3U(key, m3uName string) (string, bool) {
	if u, ok := c.StreamEndpoint(key); ok {
		return u, true
	}
	if m3u, found := MatchM3UChannel(m3uName, c.m3uSnapshot()); found {
		return m3u.URL, true
	}
	return "", false
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
	case strings.HasPrefix(target.Name, "Fox"):
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

	// 2. Official broadcasters reported by ESPN metadata for this specific match (verified truth).
	if len(match.Broadcasts) > 0 {
		candidates := c.resolveFromOfficialBroadcasts(match.Broadcasts)
		m3uList := c.m3uSnapshot()
		if len(m3uList) > 0 {
			for _, b := range match.Broadcasts {
				if m3u, found := MatchM3UChannel(b, m3uList); found {
					candidates = append(candidates, m3uToChannel(m3u, len(candidates) == 0))
				}
			}
		}
		if len(candidates) > 0 {
			return c.filterAndDeduplicate(candidates)
		}
		// If an event specifically has US metadata (cbssn/paramount/golazo/peacock/etc) without a configured US stream,
		// fall through gracefully to the verified ESPN channel if user has configured it.
		if !hasUSBroadcast(match.Broadcasts) {
			return nil
		}
	}

	// 3. TV Guide Crawler resolution
	if tvGuide != nil {
		if target, found := tvGuide.ResolveMatch(match); found {
			candidates := c.buildChannelsForTarget(target)
			if len(candidates) > 0 {
				return c.filterAndDeduplicate(candidates)
			}
		}
	}

	// 4. Fallback resolution based on user configured channels
	var candidates []model.Channel

	switch leagueKey {
	case "colombia":
		if u, ok := c.endpointOrM3U("win_plus", "Win+ Fútbol"); ok {
			if ch, valid := makeSingleChannel("Win+ Fútbol", u, "1080p", "max", len(candidates) == 0); valid {
				candidates = append(candidates, ch)
			}
		}
		if u, ok := c.endpointOrM3U("win", "Win Sports"); ok {
			if ch, valid := makeSingleChannel("Win Sports HD", u, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}
		if u, ok := c.endpointOrM3U("caracol", "Caracol TV"); ok {
			if ch, valid := makeSingleChannel("Caracol TV HD", u, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}
		if u, ok := c.endpointOrM3U("rcn", "Canal RCN"); ok {
			if ch, valid := makeSingleChannel("Canal RCN HD", u, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}

	case "argentina":
		tycURL, hasTyC := c.endpointOrM3U("tyc", "TyC Sports")
		if match.SlotIndex == 0 {
			if hasTyC {
				if ch, valid := makeSingleChannel("TyC Sports HD", tycURL, "1080p", "max", true); valid {
					candidates = append(candidates, ch)
				}
			}
			slotName, slotURL, ok := c.espnBySlot(0)
			if ok {
				if ch, valid := makeSingleChannel(slotName, slotURL, "720p", "min", false); valid {
					candidates = append(candidates, ch)
				}
			}
		} else {
			slotName, slotURL, ok := c.espnBySlot(match.SlotIndex)
			if ok {
				candidates = append(candidates, makeDualProfile(slotName, slotURL, true)...)
			}
		}

	default:
		// Champions League, Premier League, La Liga, Serie A, Bundesliga, Ligue 1, Brasileirao, Libertadores, etc.
		slotName, slotURL, ok := c.espnBySlot(match.SlotIndex)
		if ok {
			candidates = append(candidates, makeDualProfile(slotName, slotURL, true)...)
		}
	}

	return c.filterAndDeduplicate(candidates)
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
