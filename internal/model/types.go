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
	Key      string `json:"key"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Emoji    string `json:"emoji"`
	Category string `json:"category"`
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

// MatchEvent represents an in-game event (goal, card, penalty, own goal).
type MatchEvent struct {
	Type        string `json:"type"`        // "goal", "penalty", "owngoal", "card_yellow", "card_red"
	Description string `json:"description"` // e.g. "Goal - Header", "Penalty - Scored"
	Clock       string `json:"clock"`       // e.g. "38'"
	TeamID      string `json:"teamId"`
	PlayerName  string `json:"playerName"`  // e.g. "Robert Lewandowski"
	IsScoring   bool   `json:"isScoring"`
	IsPenalty   bool   `json:"isPenalty"`
	IsOwnGoal   bool   `json:"isOwnGoal"`
	IsRedCard   bool   `json:"isRedCard"`
	IsYellow    bool   `json:"isYellow"`
}

// Player represents an athlete on the match sheet.
type Player struct {
	Jersey      string `json:"jersey"`
	DisplayName string `json:"displayName"`
	Position    string `json:"position"`
	IsStarter   bool   `json:"isStarter"`
}

// TeamLineup represents the starting XI and bench for a team.
type TeamLineup struct {
	Formation string   `json:"formation"`
	Starters  []Player `json:"starters"`
	Bench     []Player `json:"bench"`
}

// MatchLineups holds tactical lineups for both home and away teams.
type MatchLineups struct {
	Home TeamLineup `json:"home"`
	Away TeamLineup `json:"away"`
}

// MatchStats represents team statistics for a fixture.
type MatchStats struct {
	HomePossession  string `json:"homePossession"`  // "58%"
	AwayPossession  string `json:"awayPossession"`  // "42%"
	HomeShots       string `json:"homeShots"`       // "14"
	AwayShots       string `json:"awayShots"`       // "8"
	HomeShotsOnGoal string `json:"homeShotsOnGoal"` // "6"
	AwayShotsOnGoal string `json:"awayShotsOnGoal"` // "2"
	HomeCorners     string `json:"homeCorners"`     // "7"
	AwayCorners     string `json:"awayCorners"`     // "3"
	HomeFouls       string `json:"homeFouls"`       // "11"
	AwayFouls       string `json:"awayFouls"`       // "13"
	HomePasses      string `json:"homePasses"`      // "450"
	AwayPasses      string `json:"awayPasses"`      // "380"
	HomePassPct     string `json:"homePassPct"`     // "88%"
	AwayPassPct     string `json:"awayPassPct"`     // "82%"
	HomeOffsides    string `json:"homeOffsides"`    // "2"
	AwayOffsides    string `json:"awayOffsides"`    // "1"
	HomeSaves       string `json:"homeSaves"`       // "4"
	AwaySaves       string `json:"awaySaves"`       // "2"
}

// Match represents a soccer fixture.
type Match struct {
	ID        string        `json:"id"`
	Title     string        `json:"title"`
	HomeTeam  Competitor    `json:"homeTeam"`
	AwayTeam  Competitor    `json:"awayTeam"`
	StartTime time.Time     `json:"startTime"`
	TimeStr   string        `json:"timeStr"`
	Venue     string        `json:"venue"`
	Status    MatchStatus   `json:"status"`
	LeagueKey string        `json:"leagueKey"`
	Events    []MatchEvent  `json:"events"`
	Stats     *MatchStats   `json:"stats"`
	Lineups   *MatchLineups `json:"lineups"`
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

// DefaultLeagues lists all 10 supported leagues grouped by category.
var DefaultLeagues = []League{
	{Key: "champions", Name: "UEFA Champions League", Code: "uefa.champions", Emoji: "🏆", Category: "Torneos Internacionales"},
	{Key: "libertadores", Name: "Copa Libertadores", Code: "conmebol.libertadores", Emoji: "🌎", Category: "Torneos Internacionales"},
	{Key: "premier", Name: "Premier League", Code: "eng.1", Emoji: "🏴󠁧󠁢󠁥󠁮󠁧󠁿", Category: "Ligas Europeas"},
	{Key: "laliga", Name: "La Liga", Code: "esp.1", Emoji: "🇪🇸", Category: "Ligas Europeas"},
	{Key: "seriea", Name: "Serie A", Code: "ita.1", Emoji: "🇮🇹", Category: "Ligas Europeas"},
	{Key: "bundesliga", Name: "Bundesliga", Code: "ger.1", Emoji: "🇩🇪", Category: "Ligas Europeas"},
	{Key: "ligue1", Name: "Ligue 1", Code: "fra.1", Emoji: "🇫🇷", Category: "Ligas Europeas"},
	{Key: "colombia", Name: "Liga BetPlay", Code: "col.1", Emoji: "🇨🇴", Category: "Ligas Sudamericanas"},
	{Key: "argentina", Name: "Liga Profesional", Code: "arg.1", Emoji: "🇦🇷", Category: "Ligas Sudamericanas"},
	{Key: "brasil", Name: "Brasileirão", Code: "bra.1", Emoji: "🇧🇷", Category: "Ligas Sudamericanas"},
}

// Client defines the contract for fetching fixtures and match details from ESPN.
type Client interface {
	FetchMatches(ctx context.Context, leagueKey string) ([]Match, error)
	FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*MatchStats, error)
	FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*MatchStats, *MatchLineups, error)
}

// Resolver defines the contract for resolving match channels.
type Resolver interface {
	GetChannels(match Match) []Channel
}

// Controller defines the contract for MPV process supervision.
type Controller interface {
	PlayStream(ctx context.Context, req PlayRequest) error
	Stop() error
	Wait() error
}
