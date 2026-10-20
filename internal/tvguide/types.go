package tvguide

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
	"github.com/e12jassir/kickoff-cli/internal/parrilla"
)

// reChannelFormatSuffix matches a trailing broadcast-format label. HD, FHD, 4K
// and friends describe the same signal at a different quality, so they must not
// split one channel into two identities.
var reChannelFormatSuffix = regexp.MustCompile(`(?i)\s+(?:fhd|uhd|hdr|hd|sd|4k|8k)$`)

// channelIdentity returns the canonical identity of a published channel name.
//
// Two sources can publish the same signal under different spellings or quality
// labels ("Caracol TV" / "Caracol TV HD") and under different slugs
// (caracol_television / caracol_colombia). Identity is therefore derived from the
// channel itself, never from the slug: format labels are stripped, then the name
// is resolved to the canonical stream key owned by parrilla when it is known, and
// otherwise reduced with Normalize. An empty result means the name carries no
// identity and callers must not treat it as a duplicate.
func channelIdentity(name string) string {
	base := strings.TrimSpace(name)
	for {
		stripped := reChannelFormatSuffix.ReplaceAllString(base, "")
		if stripped == base {
			break
		}
		base = stripped
	}

	if key, _, known := parrilla.CanonicalChannel(base); known {
		return key
	}
	return Normalize(base)
}

// guideChannelIdentity returns the identity of a crawled guide, falling back to
// its slug when the display name is empty. The slug prefix keeps the fallback
// from colliding with a normalized display name.
func guideChannelIdentity(guide ChannelGuide) string {
	if id := channelIdentity(guide.Name); id != "" {
		return id
	}
	if slug := strings.TrimSpace(guide.Slug); slug != "" {
		return "slug:" + strings.ToLower(slug)
	}
	return ""
}

// ChannelTarget maps a standard broadcast channel name to its Gatotv URL slug and stream URL.
type ChannelTarget struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	StreamURL string `json:"stream_url"`
}

// ProgramEntry holds program title, scheduled broadcast times, and live status.
type ProgramEntry struct {
	Title     string `json:"title"`
	StartTime string `json:"start_time,omitempty"` // e.g. "13:40"
	EndTime   string `json:"end_time,omitempty"`   // e.g. "16:00"
	IsLiveNow bool   `json:"is_live_now,omitempty"`
}

// ChannelGuide holds the crawled programs for a specific channel.
type ChannelGuide struct {
	Name      string         `json:"name"`
	Slug      string         `json:"slug"`
	StreamURL string         `json:"stream_url"`
	Programs  []string       `json:"programs"`
	Entries   []ProgramEntry `json:"entries,omitempty"`
}

// GuideCache represents the serialized on-disk cache for a day's programming.
type GuideCache struct {
	Date     string         `json:"date"`
	CachedAt time.Time      `json:"cached_at"`
	Channels []ChannelGuide `json:"channels"`
}

// Crawler defines the contract for crawling TV guide channels.
type Crawler interface {
	Crawl(ctx context.Context, targets []ChannelTarget) ([]ChannelGuide, error)
	FetchChannel(ctx context.Context, slug string) ([]string, error)
}

// MatchResolver defines the contract for resolving match channels via TV guide.
type MatchResolver interface {
	ResolveMatch(match model.Match) (ChannelTarget, bool)
}

// DefaultTargets defines standard target channels in Colombia.
//
// The list holds one entry per signal, deduplicated by channelIdentity. GatoTV
// publishes some channels under more than one slug (caracol_television and
// caracol_colombia, rcn_television and rcn_colombia); crawling both registers the
// same signal twice, which makes every fixture on it look like an ambiguous tie.
// For each signal the FIRST declared slug is kept as the preferred crawl target:
// declaration order is the only ranking the project maintains, so keeping the
// first one is deterministic and stable across runs. The alias slug is dropped;
// when a preferred slug stops working, replace it in place instead of appending
// the alias.
//
// TestDefaultTargets_NoDuplicateIdentities enforces the invariant.
var DefaultTargets = []ChannelTarget{
	{Name: "ESPN 1 HD", Slug: "espn_colombia"},
	{Name: "ESPN 2 HD", Slug: "espn_2_colombia"},
	{Name: "ESPN 3 HD", Slug: "espn_3_colombia"},
	{Name: "ESPN 4 HD", Slug: "espn_4_colombia"},
	{Name: "ESPN 5 HD", Slug: "espn_5_colombia"},
	{Name: "ESPN 6 HD", Slug: "espn_6_colombia"},
	{Name: "ESPN 7 HD", Slug: "espn_7_colombia"},
	{Name: "Win Sports HD", Slug: "win_sports"},
	{Name: "Win+ Fútbol", Slug: "win_sports_plus"},
	{Name: "Caracol TV HD", Slug: "caracol_television"},
	{Name: "Canal RCN HD", Slug: "rcn_television"},
}
