package tvguide

import (
	"context"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)


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
	{Name: "Caracol TV HD", Slug: "caracol_colombia"},
	{Name: "Canal RCN HD", Slug: "rcn_television"},
	{Name: "Canal RCN HD", Slug: "rcn_colombia"},
}
