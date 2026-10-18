package extractor

import (
	"context"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// ExtractedStream represents a parsed stream option for a match.
type ExtractedStream struct {
	ChannelName string            `json:"channelName"` // e.g. "DSports", "TNT Sports", "ESPN Premium"
	PlayerURL   string            `json:"playerUrl"`   // Web page or embed player URL
	StreamURL   string            `json:"streamUrl"`   // Direct HLS (.m3u8) manifest URL once resolved
	Headers     map[string]string `json:"headers"`     // Required HTTP headers (e.g. Referer, User-Agent)
	Resolution  string            `json:"resolution"`  // e.g. "1080p", "720p"
	Alive       bool              `json:"alive"`       // Validated via HLS probe
}

// ExtractedEvent represents a live or upcoming match parsed from a web portal.
type ExtractedEvent struct {
	HomeTeam     string            `json:"homeTeam"`
	AwayTeam     string            `json:"awayTeam"`
	RawTitle     string            `json:"rawTitle"`
	StartTime    time.Time         `json:"startTime"`
	TimeStr      string            `json:"timeStr"`
	Tournament   string            `json:"tournament"`
	Streams      []ExtractedStream `json:"streams"`
	SourceDomain string            `json:"sourceDomain"`
}

// Extractor defines the contract for fetching live match agendas and resolving stream manifests.
type Extractor interface {
	// FetchAgenda retrieves all currently listed matches from the dynamic portal.
	FetchAgenda(ctx context.Context) ([]ExtractedEvent, error)

	// ResolveStream inspects a player/embed URL, extracts the HLS manifest URL, and discovers required headers.
	ResolveStream(ctx context.Context, stream *ExtractedStream) error

	// ProbeStream performs an ultra-fast Range GET check (0-512 bytes) looking for #EXTM3U.
	ProbeStream(ctx context.Context, stream *ExtractedStream) bool

	// MatchEvent pairs an official ESPN match with the best matching portal event, returning the event and MatchScore.
	MatchEvent(match model.Match, events []ExtractedEvent) (*ExtractedEvent, int)

	// ResolveMatchStreams coordinates agenda fetching, event matching, manifest resolution, and probe verification.
	ResolveMatchStreams(ctx context.Context, match model.Match) ([]ExtractedStream, error)
}
