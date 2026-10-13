package model

import (
	"context"
	"errors"
	"time"
)

// Standard domain errors.
var (
	ErrNoMatchesFound   = errors.New("no matches found")
	ErrCorruptedPayload = errors.New("corrupted or invalid payload")
	ErrNetworkTimeout   = errors.New("network timeout")
	ErrCacheExpired     = errors.New("cache expired")
	ErrPlaybackFailed   = errors.New("playback failed")
	ErrPlayerTimeout    = errors.New("player initialization timed out")
	ErrPlayerExited     = errors.New("player exited prematurely")
	ErrInvalidURL       = errors.New("invalid stream url")
)

// League represents a football league or competition.
type League struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// Competitor represents a participating team in a match.
type Competitor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Logo        string `json:"logo"`
	IsHome      bool   `json:"isHome"`
	Score       string `json:"score"`
}

// MatchStatus represents the state of a match.
type MatchStatus struct {
	Description string `json:"description"`
	Detail      string `json:"detail"`
	State       string `json:"state"` // "pre", "in", "post"
	Completed   bool   `json:"completed"`
}

// Match represents a soccer fixture.
type Match struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	HomeTeam  Competitor  `json:"homeTeam"`
	AwayTeam  Competitor  `json:"awayTeam"`
	StartTime time.Time   `json:"startTime"`
	TimeStr   string      `json:"timeStr"`
	Venue     string      `json:"venue"`
	Status    MatchStatus `json:"status"`
	LeagueKey string      `json:"leagueKey"`
}

// Channel represents an HLS stream signal for a match.
type Channel struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Resolution  string `json:"resolution"` // "720p", "1080p"
	BitrateMode string `json:"bitrateMode"` // "min", "max"
	Recommended bool   `json:"recommended"`
}

// PlayRequest contains parameters required to launch playback.
type PlayRequest struct {
	Channel     Channel `json:"channel"`
	Match       Match   `json:"match"`
	BitrateMode string  `json:"bitrateMode"`
}

// DefaultLeagues lists all 10 supported leagues.
var DefaultLeagues = []League{
	{Key: "laliga", Name: "La Liga (España)", Code: "esp.1"},
	{Key: "premier", Name: "Premier League (Inglaterra)", Code: "eng.1"},
	{Key: "seriea", Name: "Serie A (Italia)", Code: "ita.1"},
	{Key: "bundesliga", Name: "Bundesliga (Alemania)", Code: "ger.1"},
	{Key: "ligue1", Name: "Ligue 1 (Francia)", Code: "fra.1"},
	{Key: "champions", Name: "UEFA Champions League", Code: "uefa.champions"},
	{Key: "libertadores", Name: "Copa Libertadores", Code: "conmebol.libertadores"},
	{Key: "argentina", Name: "Liga Profesional Argentina", Code: "arg.1"},
	{Key: "colombia", Name: "Liga BetPlay (Colombia)", Code: "col.1"},
	{Key: "brasil", Name: "Brasileirão Série A", Code: "bra.1"},
}

// Client defines the contract for fetching fixtures from ESPN.
type Client interface {
	FetchMatches(ctx context.Context, leagueKey string) ([]Match, error)
}

// Resolver defines the contract for resolving match channels.
type Resolver interface {
	GetChannels(match Match) []Channel
}

// Controller defines the contract for MPV process supervision.
type Controller interface {
	PlayStream(ctx context.Context, req PlayRequest) error
	Stop() error
}
