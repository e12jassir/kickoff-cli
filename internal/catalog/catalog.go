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

// Expanded pool of verified sports stream URLs.
const (
	StreamESPN1       = "http://example.com/espn.m3u8"
	StreamESPN2       = "http://example.com/espn2.m3u8"
	StreamESPN3       = "http://example.com/espn3.m3u8"
	StreamESPN4       = "http://example.com/espn4.m3u8"
	StreamESPN5       = "http://example.com/espn5.m3u8"
	StreamESPN6       = "http://example.com/espn6.m3u8"
	StreamESPN7       = "http://example.com/espn7.m3u8"
	StreamESPNExtra   = "http://example.com/espnextra.m3u8"
	StreamFoxSports1  = "http://example.com/fox1.m3u8"
	StreamFoxSports2  = "http://example.com/fox2.m3u8"
	StreamFoxSports3  = "http://example.com/fox3.m3u8"
	StreamRealMadrid  = "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8"
	StreamTyCSports   = "http://example.com/tyc.m3u8"
	StreamTeledeporte = "https://rtvelivestream.akamaized.net/rtvesec/tdp/tdp_main.m3u8"
	StreamCaracolTV   = "http://example.com/caracol.m3u8"
	StreamCaracolHD2  = "http://example.com/caracolhd2.m3u8"
	StreamCanalRCN    = "http://example.com/rcn.m3u8"
	StreamRCNHD2      = "http://example.com/rcnhd2.m3u8"
	StreamWinSports   = "http://example.com/win.m3u8"
	StreamWinPlus     = "http://example.com/winplus.m3u8"
)

// Optional US stream endpoints (configurable via environment variables).
var (
	StreamCBSSportsNetwork = os.Getenv("KICKOFF_STREAM_CBSSN")
	StreamCBSGolazo        = os.Getenv("KICKOFF_STREAM_GOLAZO")
)

type ExternalConfig struct {
	LeagueChannels map[string][]model.Channel `json:"league_channels"`
	CustomRules    []struct {
		League   string          `json:"league"`
		Pattern  string          `json:"pattern"`
		Channels []model.Channel `json:"channels"`
	} `json:"custom_rules"`
}

// SanitizeURL enforces HTTP for boldmss CDN to prevent FFmpeg TLS decoding failures,
// and validates URL safety.
func SanitizeURL(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", model.ErrInvalidURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: %v", model.ErrInvalidURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: unsupported scheme %s", model.ErrInvalidURL, parsed.Scheme)
	}

	if strings.Contains(parsed.Host, "boldmss.com") && parsed.Scheme == "https" {
		parsed.Scheme = "http"
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

// Catalog implements model.Resolver with dynamic broadcast matching, TV guide crawling, and stream health probing.
type Catalog struct {
	mu              sync.RWMutex
	streamAlive     map[string]bool
	championsRules  []matchRule
	customRules     []customMatchRule
	leagueOverrides map[string][]model.Channel
	tvGuide         *tvguide.Service
}

type matchRule struct {
	pattern  *regexp.Regexp
	channels []model.Channel
}

type customMatchRule struct {
	league   string
	pattern  *regexp.Regexp
	channels []model.Channel
}

// New creates a new Catalog resolver and starts background health checks on all stream signals.
func New(opts ...Option) *Catalog {
	c := &Catalog{
		streamAlive:     make(map[string]bool),
		leagueOverrides: make(map[string][]model.Channel),
		tvGuide:         tvguide.NewService(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.initChampionsRules()
	c.loadExternalConfig()
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

// probeAllStreamsBackground checks in parallel which M3U8 endpoints are online.
func (c *Catalog) probeAllStreamsBackground() {
	urls := []string{
		StreamESPN1, StreamESPN2, StreamESPN3, StreamESPN4, StreamESPN5, StreamESPN6, StreamESPN7,
		StreamESPNExtra, StreamFoxSports1, StreamFoxSports2, StreamFoxSports3,
		StreamRealMadrid, StreamTyCSports, StreamTeledeporte,
		StreamCaracolTV, StreamCaracolHD2, StreamCanalRCN, StreamRCNHD2, StreamWinSports, StreamWinPlus,
	}
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

func (c *Catalog) loadExternalConfig() {
	configBase := os.Getenv("XDG_CONFIG_HOME")
	if configBase == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		configBase = filepath.Join(home, ".config")
	}
	cfgPath := filepath.Join(configBase, "kickoff", "channels.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}

	var ext ExternalConfig
	if err := json.Unmarshal(data, &ext); err != nil {
		return
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
			c.leagueOverrides[strings.ToLower(lg)] = sanitized
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
				league:   strings.ToLower(r.League),
				pattern:  re,
				channels: sanitized,
			})
		}
	}
}

func espnProfile(name, rawURL string) []model.Channel {
	cleanURL, _ := SanitizeURL(rawURL)
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
	cleanURL, _ := SanitizeURL(rawURL)
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

func makeSingleChannel(label, rawURL, res, bitrate string, recommended bool) model.Channel {
	cleanURL, _ := SanitizeURL(rawURL)
	return model.Channel{
		DisplayName: label,
		Name:        label,
		URL:         cleanURL,
		Resolution:  res,
		BitrateMode: bitrate,
		Recommended: recommended,
	}
}

func (c *Catalog) initChampionsRules() {
	c.championsRules = []matchRule{
		{
			pattern:  regexp.MustCompile(`(?i)(Liverpool|LASK)`),
			channels: espnProfile("ESPN 1 HD", StreamESPN1),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Feyenoord|Como)`),
			channels: espnProfile("ESPN 2 HD", StreamESPN2),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Real Madrid|Roma)`),
			channels: espnProfile("ESPN 1 HD", StreamESPN1),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Manchester City|Paris Saint-Germain|PSG)`),
			channels: espnProfile("ESPN 2 HD", StreamESPN2),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Aston Villa|Fenerbahce|Fenerbahçe)`),
			channels: espnProfile("ESPN 3 HD", StreamESPN3),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Bodo|Dortmund|Glimt)`),
			channels: espnProfile("ESPN 5 HD", StreamESPN5),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Betis|Porto)`),
			channels: espnProfile("ESPN 7 HD", StreamESPN7),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Barcelona|Galatasaray)`),
			channels: espnProfile("ESPN 5 HD", StreamESPN5),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Bayern|Viking)`),
			channels: espnProfile("ESPN 1 HD", StreamESPN1),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Atlético|Atletico)`),
			channels: espnProfile("ESPN 2 HD", StreamESPN2),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Inter|Internazionale|Brugge)`),
			channels: espnProfile("ESPN 3 HD", StreamESPN3),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Arsenal|Lille)`),
			channels: espnProfile("ESPN 4 HD", StreamESPN4),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Villarreal|Napoli)`),
			channels: espnProfile("ESPN 6 HD", StreamESPN6),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Leipzig|PSV)`),
			channels: espnProfile("ESPN 7 HD", StreamESPN7),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Lens|Sporting)`),
			channels: espnProfile("ESPN 1 HD", StreamESPN1),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Slavia|Sabah)`),
			channels: espnProfile("ESPN 2 HD", StreamESPN2),
		},
	}
}

// resolveFromOfficialBroadcasts maps ESPN's official broadcast metadata for the match to exact stream signals.
func (c *Catalog) resolveFromOfficialBroadcasts(broadcasts []string) []model.Channel {
	var result []model.Channel
	for _, b := range broadcasts {
		bl := strings.ToLower(strings.TrimSpace(b))
		switch {
		case strings.Contains(bl, "caracol") || strings.Contains(bl, "gol caracol"):
			result = append(result, makeSingleChannel("Caracol TV HD", StreamCaracolTV, "1080p", "max", len(result) == 0))
			result = append(result, makeSingleChannel("Caracol HD2", StreamCaracolHD2, "1080p", "max", false))
		case strings.Contains(bl, "rcn") || strings.Contains(bl, "canal rcn") || strings.Contains(bl, "deportes rcn"):
			result = append(result, makeSingleChannel("Canal RCN HD", StreamCanalRCN, "1080p", "max", len(result) == 0))
			result = append(result, makeSingleChannel("RCN HD2", StreamRCNHD2, "1080p", "max", false))
		case strings.Contains(bl, "win") || strings.Contains(bl, "win sports") || strings.Contains(bl, "win+"):
			result = append(result, makeSingleChannel("Win+ Fútbol", StreamWinPlus, "1080p", "max", len(result) == 0))
			result = append(result, makeSingleChannel("Win Sports HD", StreamWinSports, "1080p", "max", false))
		case strings.Contains(bl, "espn 7") || strings.Contains(bl, "espn7"):
			result = append(result, makeDualProfile("ESPN 7 HD", StreamESPN7, len(result) == 0)...)
		case strings.Contains(bl, "espn 6") || strings.Contains(bl, "espn6"):
			result = append(result, makeDualProfile("ESPN 6 HD", StreamESPN6, len(result) == 0)...)
		case strings.Contains(bl, "espn 5") || strings.Contains(bl, "espn5"):
			result = append(result, makeDualProfile("ESPN 5 HD", StreamESPN5, len(result) == 0)...)
		case strings.Contains(bl, "espn 4") || strings.Contains(bl, "espn4"):
			result = append(result, makeDualProfile("ESPN 4 HD", StreamESPN4, len(result) == 0)...)
		case strings.Contains(bl, "espn 3") || strings.Contains(bl, "espn3"):
			result = append(result, makeDualProfile("ESPN 3 HD", StreamESPN3, len(result) == 0)...)
		case strings.Contains(bl, "espn 2") || strings.Contains(bl, "espn2") || strings.Contains(bl, "espn deportes"):
			result = append(result, makeDualProfile("ESPN 2 HD", StreamESPN2, len(result) == 0)...)
		case strings.Contains(bl, "espn extra"):
			result = append(result, makeDualProfile("ESPN Extra", StreamESPNExtra, len(result) == 0)...)
		case strings.Contains(bl, "espn"):
			result = append(result, makeDualProfile("ESPN 1 HD", StreamESPN1, len(result) == 0)...)
		case strings.Contains(bl, "fox sports 3") || strings.Contains(bl, "fs3"):
			result = append(result, makeDualProfile("Fox Sports 3", StreamFoxSports3, len(result) == 0)...)
		case strings.Contains(bl, "fox sports 2") || strings.Contains(bl, "fs2"):
			result = append(result, makeDualProfile("Fox Sports 2", StreamFoxSports2, len(result) == 0)...)
		case strings.Contains(bl, "fox sports 1") || strings.Contains(bl, "fs1") || strings.Contains(bl, "fox deportes") || strings.Contains(bl, "fox sports") || strings.Contains(bl, "fox"):
			result = append(result, makeSingleChannel("Fox Sports 1 HD", StreamFoxSports1, "1080p", "max", len(result) == 0))
		case strings.Contains(bl, "tyc"):
			result = append(result, makeSingleChannel("TyC Sports HD", StreamTyCSports, "1080p", "max", len(result) == 0))
		case strings.Contains(bl, "real madrid") || strings.Contains(bl, "rmtv"):
			result = append(result, makeSingleChannel("Real Madrid TV", StreamRealMadrid, "1080p", "max", len(result) == 0))
		case strings.Contains(bl, "teledeporte") || strings.Contains(bl, "rtve"):
			result = append(result, makeSingleChannel("Teledeporte HD", StreamTeledeporte, "1080p", "max", len(result) == 0))
		case strings.Contains(bl, "cbssn") || strings.Contains(bl, "cbs sports network") || strings.Contains(bl, "cbs sports"):
			if StreamCBSSportsNetwork != "" {
				result = append(result, makeSingleChannel("CBS Sports Network", StreamCBSSportsNetwork, "1080p", "max", len(result) == 0))
			}
		case strings.Contains(bl, "paramount") || strings.Contains(bl, "golazo") || strings.Contains(bl, "cbs sports golazo"):
			if StreamCBSGolazo != "" {
				result = append(result, makeSingleChannel("CBS Sports Golazo Network", StreamCBSGolazo, "1080p", "max", len(result) == 0))
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

// espnBySlot picks a distinct ESPN channel (1..7) based on the simultaneous match slot index.
// If slot exceeds the available ESPN channels (or is negative), it returns ok = false.
func espnBySlot(slot int) (name, url string, ok bool) {
	slots := []struct {
		name string
		url  string
	}{
		{"ESPN 1 HD", StreamESPN1},
		{"ESPN 2 HD", StreamESPN2},
		{"ESPN 3 HD", StreamESPN3},
		{"ESPN 4 HD", StreamESPN4},
		{"ESPN 5 HD", StreamESPN5},
		{"ESPN 6 HD", StreamESPN6},
		{"ESPN 7 HD", StreamESPN7},
	}
	if slot < 0 || slot >= len(slots) {
		return "", "", false
	}
	return slots[slot].name, slots[slot].url, true
}

func (c *Catalog) buildChannelsForTarget(target tvguide.ChannelTarget) []model.Channel {
	cleanURL, _ := SanitizeURL(target.StreamURL)
	switch {
	case strings.HasPrefix(target.Name, "ESPN"):
		return espnProfile(target.Name, cleanURL)
	case strings.HasPrefix(target.Name, "Fox"):
		return makeDualProfile(target.Name, cleanURL, true)
	case target.Name == "Win+ Fútbol":
		return []model.Channel{
			makeSingleChannel("Win+ Fútbol", StreamWinPlus, "1080p", "max", true),
			makeSingleChannel("Win Sports HD", StreamWinSports, "1080p", "max", false),
		}
	case target.Name == "Win Sports HD":
		return []model.Channel{
			makeSingleChannel("Win Sports HD", StreamWinSports, "1080p", "max", true),
			makeSingleChannel("Win+ Fútbol", StreamWinPlus, "1080p", "max", false),
		}
	case target.Name == "Caracol TV HD":
		return []model.Channel{
			makeSingleChannel("Caracol TV HD", StreamCaracolTV, "1080p", "max", true),
			makeSingleChannel("Caracol HD2", StreamCaracolHD2, "1080p", "max", false),
		}
	case target.Name == "Canal RCN HD":
		return []model.Channel{
			makeSingleChannel("Canal RCN HD", StreamCanalRCN, "1080p", "max", true),
			makeSingleChannel("RCN HD2", StreamRCNHD2, "1080p", "max", false),
		}
	default:
		return []model.Channel{
			makeSingleChannel(target.Name, cleanURL, "1080p", "max", true),
		}
	}
}

// GetChannels resolves match-specific broadcast channels.
// Official broadcasts reported in match.Broadcasts are prioritized and verified without guesswork.
// If no official broadcasts are reported, the autonomous TV Guide crawler is consulted.
// If neither matched, simultaneous matches are routed to distinct collision-free channels.
func (c *Catalog) GetChannels(match model.Match) []model.Channel {
	if match.Status.Completed || strings.ToLower(match.Status.State) == "post" {
		return nil
	}

	title := match.Title
	if title == "" {
		title = fmt.Sprintf("%s vs %s", match.HomeTeam.DisplayName, match.AwayTeam.DisplayName)
	}
	leagueKey := strings.ToLower(match.LeagueKey)

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
	// If official broadcasts are present, DO NOT append fallback guesswork channels.
	if len(match.Broadcasts) > 0 {
		candidates := c.resolveFromOfficialBroadcasts(match.Broadcasts)
		if len(candidates) > 0 {
			return c.filterAndDeduplicate(candidates)
		}
		// If an event specifically has US metadata (cbssn/paramount/golazo) without a configured US stream,
		// fall through gracefully to the verified ESPN Latin America channel rather than returning no channels.
		if !hasUSBroadcast(match.Broadcasts) {
			return nil
		}
	}

	// 3. Autonomous Daily TV Guide Crawler resolution
	if c.tvGuide != nil {
		if target, found := c.tvGuide.ResolveMatch(match); found {
			candidates := c.buildChannelsForTarget(target)
			if len(candidates) > 0 {
				return c.filterAndDeduplicate(candidates)
			}
		}
	}

	// 4. Fallback resolution when NO official broadcasts nor TV guide matched.
	// Each simultaneous match slot must map to a DISTINCT, collision-free channel
	// without blindly appending Fox Sports 1 or duplicate channels across matches.
	var candidates []model.Channel

	switch leagueKey {
	case "colombia":
		candidates = append(candidates, makeSingleChannel("Win+ Fútbol", StreamWinPlus, "1080p", "max", len(candidates) == 0))
		candidates = append(candidates, makeSingleChannel("Win Sports HD", StreamWinSports, "1080p", "max", false))
		candidates = append(candidates, makeSingleChannel("Caracol TV HD", StreamCaracolTV, "1080p", "max", false))
		candidates = append(candidates, makeSingleChannel("Canal RCN HD", StreamCanalRCN, "1080p", "max", false))

	case "argentina":
		if match.SlotIndex == 0 {
			candidates = append(candidates, makeSingleChannel("TyC Sports HD", StreamTyCSports, "1080p", "max", true))
			slotName, slotURL, ok := espnBySlot(0)
			if ok {
				candidates = append(candidates, makeSingleChannel(slotName, slotURL, "720p", "min", false))
			}
		} else {
			slotName, slotURL, ok := espnBySlot(match.SlotIndex)
			if ok {
				candidates = append(candidates, makeDualProfile(slotName, slotURL, true)...)
			}
		}

	case "champions":
		matchedRule := false
		for _, rule := range c.championsRules {
			if rule.pattern.MatchString(title) {
				candidates = append(candidates, rule.channels...)
				matchedRule = true
				break
			}
		}
		if !matchedRule {
			slotName, slotURL, ok := espnBySlot(match.SlotIndex)
			if ok {
				candidates = append(candidates, makeDualProfile(slotName, slotURL, true)...)
			}
		}

	default:
		// Premier League, La Liga, Serie A, Bundesliga, Ligue 1, Brasileirao, Libertadores, etc.
		slotName, slotURL, ok := espnBySlot(match.SlotIndex)
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

	// Deduplicate by (URL + Resolution) while preserving order and prioritizing probed alive streams
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

	// Ensure exactly the top signal is highlighted as Recommended
	for i := range final {
		final[i].Recommended = (i == 0)
	}

	return final
}
