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

	"github.com/e12jassir/kickoff/internal/model"
	"github.com/e12jassir/kickoff/internal/tvguide"
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
	for _, opt := range opts {
		opt(c)
	}
	go c.probeAllStreamsBackground()
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

func (c *Catalog) probeAllStreamsBackground() {
	var urls []string

	c.mu.RLock()
	for _, u := range c.userStreams {
		if u != "" {
			urls = append(urls, u)
		}
	}
	c.mu.RUnlock()

	urls = append(urls, StreamRealMadrid, StreamTeledeporte)
	if StreamCBSSportsNetwork != "" {
		urls = append(urls, StreamCBSSportsNetwork)
	}
	if StreamCBSGolazo != "" {
		urls = append(urls, StreamCBSGolazo)
	}

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
		configBase := os.Getenv("XDG_CONFIG_HOME")
		if configBase == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return
			}
			configBase = filepath.Join(home, ".config")
		}
		cfgPath = filepath.Join(configBase, "kickoff", "channels.json")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}

	var ext ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
		return
	}

	for k, v := range ext.Streams {
		cleanK := strings.ToLower(strings.TrimSpace(k))
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

func espnProfile(name, rawURL string) []model.Channel {
	cleanURL, err := SanitizeURL(rawURL)
	if err != nil || cleanURL == "" {
		return nil
	}
	return []model.Channel{
		{
			DisplayName: name,
			Name:        name,
			URL:         cleanURL,
			Resolution:  "720p",
			BitrateMode: "min",
			Recommended: true,
		},
		{
			DisplayName: name,
			Name:        name,
			URL:         cleanURL,
			Resolution:  "1080p",
			BitrateMode: "max",
			Recommended: false,
		},
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


func (c *Catalog) resolveFromOfficialBroadcasts(broadcasts []string) []model.Channel {
	var result []model.Channel
	for _, b := range broadcasts {
		bl := strings.ToLower(strings.TrimSpace(b))
		switch {
		case strings.Contains(bl, "caracol") || strings.Contains(bl, "gol caracol"):
			if u, ok := c.StreamEndpoint("caracol"); ok {
				if ch, valid := makeSingleChannel("Caracol TV HD", u, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
			if u, ok := c.StreamEndpoint("caracol_hd2"); ok {
				if ch, valid := makeSingleChannel("Caracol HD2", u, "1080p", "max", false); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "rcn") || strings.Contains(bl, "canal rcn") || strings.Contains(bl, "deportes rcn"):
			if u, ok := c.StreamEndpoint("rcn"); ok {
				if ch, valid := makeSingleChannel("Canal RCN HD", u, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
			if u, ok := c.StreamEndpoint("rcn_hd2"); ok {
				if ch, valid := makeSingleChannel("RCN HD2", u, "1080p", "max", false); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "win") || strings.Contains(bl, "win sports") || strings.Contains(bl, "win+"):
			if u, ok := c.StreamEndpoint("win_plus"); ok {
				if ch, valid := makeSingleChannel("Win+ Fútbol", u, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
			if u, ok := c.StreamEndpoint("win"); ok {
				if ch, valid := makeSingleChannel("Win Sports HD", u, "1080p", "max", false); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "espn 7") || strings.Contains(bl, "espn7"):
			if u, ok := c.StreamEndpoint("espn7"); ok {
				result = append(result, makeDualProfile("ESPN 7 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn 6") || strings.Contains(bl, "espn6"):
			if u, ok := c.StreamEndpoint("espn6"); ok {
				result = append(result, makeDualProfile("ESPN 6 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn 5") || strings.Contains(bl, "espn5"):
			if u, ok := c.StreamEndpoint("espn5"); ok {
				result = append(result, makeDualProfile("ESPN 5 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn 4") || strings.Contains(bl, "espn4"):
			if u, ok := c.StreamEndpoint("espn4"); ok {
				result = append(result, makeDualProfile("ESPN 4 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn 3") || strings.Contains(bl, "espn3"):
			if u, ok := c.StreamEndpoint("espn3"); ok {
				result = append(result, makeDualProfile("ESPN 3 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn 2") || strings.Contains(bl, "espn2") || strings.Contains(bl, "espn deportes"):
			if u, ok := c.StreamEndpoint("espn2"); ok {
				result = append(result, makeDualProfile("ESPN 2 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn extra"):
			if u, ok := c.StreamEndpoint("espn_extra"); ok {
				result = append(result, makeDualProfile("ESPN Extra", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "espn"):
			if u, ok := c.StreamEndpoint("espn1"); ok {
				result = append(result, makeDualProfile("ESPN 1 HD", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "fox sports 3") || strings.Contains(bl, "fs3"):
			if u, ok := c.StreamEndpoint("fox3"); ok {
				result = append(result, makeDualProfile("Fox Sports 3", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "fox sports 2") || strings.Contains(bl, "fs2"):
			if u, ok := c.StreamEndpoint("fox2"); ok {
				result = append(result, makeDualProfile("Fox Sports 2", u, len(result) == 0)...)
			}
		case strings.Contains(bl, "fox sports 1") || strings.Contains(bl, "fs1") || strings.Contains(bl, "fox deportes") || strings.Contains(bl, "fox sports") || strings.Contains(bl, "fox"):
			if u, ok := c.StreamEndpoint("fox1"); ok {
				if ch, valid := makeSingleChannel("Fox Sports 1 HD", u, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "tyc"):
			if u, ok := c.StreamEndpoint("tyc"); ok {
				if ch, valid := makeSingleChannel("TyC Sports HD", u, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "real madrid") || strings.Contains(bl, "rmtv"):
			u, ok := c.StreamEndpoint("realmadrid")
			if !ok {
				u = StreamRealMadrid
			}
			if ch, valid := makeSingleChannel("Real Madrid TV", u, "1080p", "max", len(result) == 0); valid {
				result = append(result, ch)
			}
		case strings.Contains(bl, "teledeporte") || strings.Contains(bl, "rtve"):
			u, ok := c.StreamEndpoint("teledeporte")
			if !ok {
				u = StreamTeledeporte
			}
			if ch, valid := makeSingleChannel("Teledeporte HD", u, "1080p", "max", len(result) == 0); valid {
				result = append(result, ch)
			}
		case strings.Contains(bl, "cbssn") || strings.Contains(bl, "cbs sports network") || strings.Contains(bl, "cbs sports"):
			if StreamCBSSportsNetwork != "" {
				if ch, valid := makeSingleChannel("CBS Sports Network", StreamCBSSportsNetwork, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
		case strings.Contains(bl, "paramount") || strings.Contains(bl, "golazo") || strings.Contains(bl, "cbs sports golazo"):
			if StreamCBSGolazo != "" {
				if ch, valid := makeSingleChannel("CBS Sports Golazo Network", StreamCBSGolazo, "1080p", "max", len(result) == 0); valid {
					result = append(result, ch)
				}
			}
		}
	}
	return result
}

func hasUSBroadcast(broadcasts []string) bool {
	for _, b := range broadcasts {
		bl := strings.ToLower(strings.TrimSpace(b))
		if strings.Contains(bl, "cbssn") || strings.Contains(bl, "cbs sports") || strings.Contains(bl, "paramount") || strings.Contains(bl, "golazo") {
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
	u, found := c.StreamEndpoint(slots[slot].streamK)
	if !found || u == "" {
		c.mu.RLock()
		m3uList := c.m3uChannels
		c.mu.RUnlock()
		if len(m3uList) > 0 {
			if m3u, ok := MatchM3UChannel(slots[slot].name, m3uList); ok {
				return slots[slot].name, m3u.URL, true
			}
		}
		return "", "", false
	}
	return slots[slot].name, u, true
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
		c.mu.RLock()
		m3uList := c.m3uChannels
		c.mu.RUnlock()
		if len(m3uList) > 0 {
			if m3u, ok := MatchM3UChannel(target.Name, m3uList); ok {
				streamURL = m3u.URL
			}
		}
	}

	cleanURL, _ := SanitizeURL(streamURL)
	if cleanURL == "" {
		return nil
	}

	switch {
	case strings.HasPrefix(target.Name, "ESPN"):
		return espnProfile(target.Name, cleanURL)
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

	// 1. Check user custom rules & overrides from ~/.config/kickoff/channels.json
	for _, rule := range c.customRules {
		if (rule.league == "" || rule.league == leagueKey) && rule.pattern.MatchString(title) {
			return rule.channels
		}
	}
	if override, ok := c.leagueOverrides[leagueKey]; ok && len(override) > 0 {
		return override
	}

	// 2. Official broadcasters reported by ESPN metadata for this specific match (verified truth).
	if len(match.Broadcasts) > 0 {
		candidates := c.resolveFromOfficialBroadcasts(match.Broadcasts)
		c.mu.RLock()
		m3uList := c.m3uChannels
		c.mu.RUnlock()
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
		// If an event specifically has US metadata (cbssn/paramount/golazo) without a configured US stream,
		// fall through gracefully to the verified ESPN channel if user has configured it.
		if !hasUSBroadcast(match.Broadcasts) {
			return nil
		}
	}

	// 3. TV Guide Crawler resolution
	if c.tvGuide != nil {
		if target, found := c.tvGuide.ResolveMatch(match); found {
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
		winPlusURL, hasWinPlus := c.StreamEndpoint("win_plus")
		if !hasWinPlus {
			c.mu.RLock()
			m3uList := c.m3uChannels
			c.mu.RUnlock()
			if m3u, ok := MatchM3UChannel("Win+ Fútbol", m3uList); ok {
				winPlusURL = m3u.URL
				hasWinPlus = true
			}
		}
		if hasWinPlus {
			if ch, valid := makeSingleChannel("Win+ Fútbol", winPlusURL, "1080p", "max", len(candidates) == 0); valid {
				candidates = append(candidates, ch)
			}
		}

		winURL, hasWin := c.StreamEndpoint("win")
		if !hasWin {
			c.mu.RLock()
			m3uList := c.m3uChannels
			c.mu.RUnlock()
			if m3u, ok := MatchM3UChannel("Win Sports", m3uList); ok {
				winURL = m3u.URL
				hasWin = true
			}
		}
		if hasWin {
			if ch, valid := makeSingleChannel("Win Sports HD", winURL, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}

		caracolURL, hasCaracol := c.StreamEndpoint("caracol")
		if !hasCaracol {
			c.mu.RLock()
			m3uList := c.m3uChannels
			c.mu.RUnlock()
			if m3u, ok := MatchM3UChannel("Caracol TV", m3uList); ok {
				caracolURL = m3u.URL
				hasCaracol = true
			}
		}
		if hasCaracol {
			if ch, valid := makeSingleChannel("Caracol TV HD", caracolURL, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}

		rcnURL, hasRCN := c.StreamEndpoint("rcn")
		if !hasRCN {
			c.mu.RLock()
			m3uList := c.m3uChannels
			c.mu.RUnlock()
			if m3u, ok := MatchM3UChannel("Canal RCN", m3uList); ok {
				rcnURL = m3u.URL
				hasRCN = true
			}
		}
		if hasRCN {
			if ch, valid := makeSingleChannel("Canal RCN HD", rcnURL, "1080p", "max", false); valid {
				candidates = append(candidates, ch)
			}
		}

	case "argentina":
		tycURL, hasTyC := c.StreamEndpoint("tyc")
		if !hasTyC {
			c.mu.RLock()
			m3uList := c.m3uChannels
			c.mu.RUnlock()
			if m3u, ok := MatchM3UChannel("TyC Sports", m3uList); ok {
				tycURL = m3u.URL
				hasTyC = true
			}
		}
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
