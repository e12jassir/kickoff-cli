package catalog

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/e12jassir/kickoff/internal/model"
)

// Known stream URLs.
const (
	StreamESPN1      = "http://example.com/espn.m3u8"
	StreamESPN2      = "http://example.com/espn2.m3u8"
	StreamESPN3      = "http://example.com/espn3.m3u8"
	StreamESPN4      = "http://example.com/espn4.m3u8"
	StreamESPN5      = "http://example.com/espn5.m3u8"
	StreamESPN6      = "http://example.com/espn6.m3u8"
	StreamESPN7      = "http://example.com/espn7.m3u8"
	StreamFoxSports1 = "http://example.com/fox1.m3u8"
	StreamRealMadrid = "https://rmtv.akamaized.net/hls/live/2043153/rmtv-es-web/master.m3u8"
	StreamTyCSports  = "http://example.com/tyc.m3u8"
)

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

	// Rewriting https://stream.example.com to http://stream.example.com
	if strings.Contains(parsed.Host, "boldmss.com") && parsed.Scheme == "https" {
		parsed.Scheme = "http"
	}

	return parsed.String(), nil
}

// Catalog implements model.Resolver.
type Catalog struct {
	championsRules []matchRule
}

type matchRule struct {
	pattern  *regexp.Regexp
	channels []model.Channel
}

// New creates a new Catalog resolver with verified channel definitions.
func New() *Catalog {
	c := &Catalog{}
	c.initChampionsRules()
	return c
}

// NewResolver is an alias for New returning model.Resolver.
func NewResolver() model.Resolver {
	return New()
}

func espnProfile(name, url string) []model.Channel {
	cleanURL, _ := SanitizeURL(url)
	return []model.Channel{
		{
			DisplayName: fmt.Sprintf("  · %s (720p Fluido - Sin Cortes)   [Recomendado]", name),
			Name:        fmt.Sprintf("%s (720p)", name),
			URL:         cleanURL,
			Resolution:  "720p",
			BitrateMode: "min",
			Recommended: true,
		},
		{
			DisplayName: fmt.Sprintf("  · %s (1080p Full HD - Original)", name),
			Name:        fmt.Sprintf("%s (1080p)", name),
			URL:         cleanURL,
			Resolution:  "1080p",
			BitrateMode: "max",
			Recommended: false,
		},
	}
}

func (c *Catalog) initChampionsRules() {
	c.championsRules = []matchRule{
		{
			pattern:  regexp.MustCompile(`(?i)(Barcelona|Galatasaray)`),
			channels: espnProfile("ESPN 5 HD", StreamESPN5),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Bayern|Viking)`),
			channels: espnProfile("ESPN 1 HD", StreamESPN1),
		},
		{
			pattern:  regexp.MustCompile(`(?i)(Manchester|Atlético|Atletico)`),
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

var realMadridRegex = regexp.MustCompile(`(?i)(Real Madrid|Madrid)`)

// GetChannels returns match-exclusive streaming signals for a fixture.
func (c *Catalog) GetChannels(match model.Match) []model.Channel {
	// If the match already finished, no live signals should be shown
	if match.Status.Completed || strings.ToLower(match.Status.State) == "post" {
		return nil
	}

	title := match.Title
	if title == "" {
		title = fmt.Sprintf("%s vs %s", match.HomeTeam.DisplayName, match.AwayTeam.DisplayName)
	}

	leagueKey := strings.ToLower(match.LeagueKey)

	switch leagueKey {
	case "champions":
		for _, rule := range c.championsRules {
			if rule.pattern.MatchString(title) {
				return rule.channels
			}
		}
		// Default Champions signal
		return []model.Channel{
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: true,
			},
			{
				DisplayName: "  · Fox Sports 1 HD (1080p Directo)",
				Name:        "Fox Sports 1",
				URL:         StreamFoxSports1,
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: false,
			},
		}

	case "laliga":
		if realMadridRegex.MatchString(title) {
			return []model.Channel{
				{
					DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
					Name:        "ESPN 1 HD (720p)",
					URL:         StreamESPN1,
					Resolution:  "720p",
					BitrateMode: "min",
					Recommended: true,
				},
				{
					DisplayName: "  · Real Madrid TV (1080p Señal Oficial Club)",
					Name:        "Real Madrid TV",
					URL:         StreamRealMadrid,
					Resolution:  "1080p",
					BitrateMode: "max",
					Recommended: false,
				},
			}
		}
		return espnProfile("ESPN 1 HD", StreamESPN1)

	case "premier":
		return espnProfile("ESPN 1 HD", StreamESPN1)

	case "seriea", "bundesliga", "ligue1":
		return espnProfile("ESPN 2 HD", StreamESPN2)

	case "libertadores":
		return []model.Channel{
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: true,
			},
			{
				DisplayName: "  · Fox Sports 1 HD (1080p Directo)",
				Name:        "Fox Sports 1",
				URL:         StreamFoxSports1,
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: false,
			},
		}

	case "argentina":
		return []model.Channel{
			{
				DisplayName: "  · TyC Sports HD (1080p Señal Oficial Fútbol Argentino)",
				Name:        "TyC Sports",
				URL:         StreamTyCSports,
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: true,
			},
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: false,
			},
		}

	case "colombia":
		return []model.Channel{
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: true,
			},
			{
				DisplayName: "  · ESPN 2 HD (720p Fluido - Sin Cortes)",
				Name:        "ESPN 2 HD (720p)",
				URL:         StreamESPN2,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: false,
			},
		}

	case "brasil":
		return []model.Channel{
			{
				DisplayName: "  · Fox Sports 1 HD (1080p Directo)",
				Name:        "Fox Sports 1",
				URL:         StreamFoxSports1,
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: true,
			},
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido)",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: false,
			},
		}

	default:
		// Unknown league fallback
		return []model.Channel{
			{
				DisplayName: "  · ESPN 1 HD (720p Fluido - Sin Cortes)   [Recomendado]",
				Name:        "ESPN 1 HD (720p)",
				URL:         StreamESPN1,
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: true,
			},
			{
				DisplayName: "  · Fox Sports 1 HD (1080p Directo)",
				Name:        "Fox Sports 1",
				URL:         StreamFoxSports1,
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: false,
			},
		}
	}
}
