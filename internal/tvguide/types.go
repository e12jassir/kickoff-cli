package tvguide

import (
	"context"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
)

// Verified stream endpoints matching catalog constants.
const (
	StreamESPN1     = "http://example.com/espn.m3u8"
	StreamESPN2     = "http://example.com/espn2.m3u8"
	StreamESPN3     = "http://example.com/espn3.m3u8"
	StreamESPN4     = "http://example.com/espn4.m3u8"
	StreamESPN5     = "http://example.com/espn5.m3u8"
	StreamESPN6     = "http://example.com/espn6.m3u8"
	StreamESPN7     = "http://example.com/espn7.m3u8"
	StreamESPNExtra = "http://example.com/espnextra.m3u8"
	StreamCaracolTV = "http://example.com/caracol.m3u8"
	StreamCanalRCN  = "http://example.com/rcn.m3u8"
	StreamWinSports = "http://example.com/win.m3u8"
	StreamWinPlus   = "http://example.com/winplus.m3u8"
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
	{Name: "ESPN 1 HD", Slug: "espn_colombia", StreamURL: StreamESPN1},
	{Name: "ESPN 2 HD", Slug: "espn_2_colombia", StreamURL: StreamESPN2},
	{Name: "ESPN 3 HD", Slug: "espn_3_colombia", StreamURL: StreamESPN3},
	{Name: "ESPN 4 HD", Slug: "espn_4_colombia", StreamURL: StreamESPN4},
	{Name: "ESPN 5 HD", Slug: "espn_5_colombia", StreamURL: StreamESPN5},
	{Name: "ESPN 6 HD", Slug: "espn_6_colombia", StreamURL: StreamESPN6},
	{Name: "ESPN 7 HD", Slug: "espn_7_colombia", StreamURL: StreamESPN7},
	{Name: "Win Sports HD", Slug: "win_sports", StreamURL: StreamWinSports},
	{Name: "Win+ Fútbol", Slug: "win_sports_plus", StreamURL: StreamWinPlus},
	{Name: "Caracol TV HD", Slug: "caracol_television", StreamURL: StreamCaracolTV},
	{Name: "Caracol TV HD", Slug: "caracol_colombia", StreamURL: StreamCaracolTV},
	{Name: "Canal RCN HD", Slug: "rcn_television", StreamURL: StreamCanalRCN},
	{Name: "Canal RCN HD", Slug: "rcn_colombia", StreamURL: StreamCanalRCN},
}
