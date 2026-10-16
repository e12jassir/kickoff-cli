package model

import (
	"context"
	"encoding/json"
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

type contextKey string

const forceRefreshKey contextKey = "force_refresh"

// WithForceRefresh returns a context that instructs the client to bypass local TTL cache.
func WithForceRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceRefreshKey, true)
}

// IsForceRefresh checks if the context requests a cache bypass.
func IsForceRefresh(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(forceRefreshKey).(bool)
	return ok && v
}

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
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Clock       string  `json:"clock"`
	ClockValue  float64 `json:"clockValue"`
	TeamID      string  `json:"teamId"`
	PlayerName  string  `json:"playerName"`
	IsScoring   bool    `json:"isScoring"`
	IsPenalty   bool    `json:"isPenalty"`
	IsOwnGoal   bool    `json:"isOwnGoal"`
	IsRedCard   bool    `json:"isRedCard"`
	IsYellow    bool    `json:"isYellow"`
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
	Coach     string   `json:"coach,omitempty"`
}

// MatchLineups holds tactical lineups for both home and away teams.
type MatchLineups struct {
	Home      TeamLineup `json:"home"`
	Away      TeamLineup `json:"away"`
	HomeCoach string     `json:"homeCoach,omitempty"`
	AwayCoach string     `json:"awayCoach,omitempty"`
}

// StandingsGroup represents a named group or conference in tournament standings.
type StandingsGroup struct {
	Name    string           `json:"name"`
	Entries []StandingsEntry `json:"entries"`
}

// StandingsEntry represents a team row in league standings.
type StandingsEntry struct {
	Rank        int    `json:"rank"`
	TeamName    string `json:"teamName"`
	GroupName   string `json:"groupName,omitempty"`
	GamesPlayed string `json:"gamesPlayed"`
	Wins        string `json:"wins"`
	Ties        string `json:"ties"`
	Losses      string `json:"losses"`
	PointDiff   string `json:"pointDiff"`
	Points      string `json:"points"`
}

// LeagueStandings represents a competition standings table.
type LeagueStandings struct {
	LeagueKey string           `json:"leagueKey"`
	Name      string           `json:"name"`
	Entries   []StandingsEntry `json:"entries"`
	Groups    []StandingsGroup `json:"groups,omitempty"`
	IsOffline bool             `json:"isOffline,omitempty"`
	CachedAt  time.Time        `json:"cachedAt,omitempty"`
}

// ScorerEntry represents an athlete in top scorers or assists list.
type ScorerEntry struct {
	Rank         int    `json:"rank"`
	PlayerName   string `json:"playerName"`
	TeamName     string `json:"teamName"`
	Value        string `json:"value"`
	DisplayValue string `json:"displayValue"`
}

// LeagueStatistics represents top scorers and top assists in a competition.
type LeagueStatistics struct {
	LeagueKey string        `json:"leagueKey"`
	Scorers   []ScorerEntry `json:"scorers"`
	Assists   []ScorerEntry `json:"assists"`
	IsOffline bool          `json:"isOffline,omitempty"`
	CachedAt  time.Time     `json:"cachedAt,omitempty"`
}

// MatchStats represents team statistics for a fixture.
type MatchStats struct {
	HomePossession  string `json:"homePossession"`
	AwayPossession  string `json:"awayPossession"`
	HomeShots       string `json:"homeShots"`
	AwayShots       string `json:"awayShots"`
	HomeShotsOnGoal string `json:"homeShotsOnGoal"`
	AwayShotsOnGoal string `json:"awayShotsOnGoal"`
	HomeCorners     string `json:"homeCorners"`
	AwayCorners     string `json:"awayCorners"`
	HomeFouls       string `json:"homeFouls"`
	AwayFouls       string `json:"awayFouls"`
	HomePasses      string `json:"homePasses"`
	AwayPasses      string `json:"awayPasses"`
	HomePassPct     string `json:"homePassPct"`
	AwayPassPct     string `json:"awayPassPct"`
	HomeOffsides    string `json:"homeOffsides"`
	AwayOffsides    string `json:"awayOffsides"`
	HomeSaves       string `json:"homeSaves"`
	AwaySaves       string `json:"awaySaves"`
}

// Match represents a soccer fixture.
type Match struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	HomeTeam   Competitor    `json:"homeTeam"`
	AwayTeam   Competitor    `json:"awayTeam"`
	StartTime  time.Time     `json:"startTime"`
	TimeStr    string        `json:"timeStr"`
	Venue      string        `json:"venue"`
	Status     MatchStatus   `json:"status"`
	LeagueKey  string        `json:"leagueKey"`
	Events     []MatchEvent  `json:"events"`
	Stats      *MatchStats   `json:"stats"`
	Lineups    *MatchLineups `json:"lineups"`
	Referee    string        `json:"referee"`
	Attendance int           `json:"attendance"`
	Broadcasts []string      `json:"broadcasts,omitempty"`
	SlotIndex  int           `json:"slotIndex,omitempty"`
	IsOffline  bool          `json:"isOffline,omitempty"`
	CachedAt   time.Time     `json:"cachedAt,omitempty"`
}

// HomeTeamCoach returns the coach of the home team if available.
func (m Match) HomeTeamCoach() string {
	if m.Lineups != nil {
		if m.Lineups.HomeCoach != "" {
			return m.Lineups.HomeCoach
		}
		return m.Lineups.Home.Coach
	}
	return ""
}

// AwayTeamCoach returns the coach of the away team if available.
func (m Match) AwayTeamCoach() string {
	if m.Lineups != nil {
		if m.Lineups.AwayCoach != "" {
			return m.Lineups.AwayCoach
		}
		return m.Lineups.Away.Coach
	}
	return ""
}

// Channel represents an HLS stream signal for a match.
type Channel struct {
	DisplayName string `json:"displayName"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Resolution  string `json:"resolution"`  // "720p", "1080p"
	BitrateMode string `json:"bitrateMode"` // "min", "max"
	Recommended bool   `json:"recommended"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Channel to support
// both camelCase ("bitrateMode") and snake_case ("bitrate_mode") fields.
func (c *Channel) UnmarshalJSON(data []byte) error {
	type channelAlias Channel
	var aux struct {
		channelAlias
		BitrateModeSnake string `json:"bitrate_mode"`
		DisplayNameSnake string `json:"display_name"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*c = Channel(aux.channelAlias)
	if c.BitrateMode == "" && aux.BitrateModeSnake != "" {
		c.BitrateMode = aux.BitrateModeSnake
	}
	if c.DisplayName == "" && aux.DisplayNameSnake != "" {
		c.DisplayName = aux.DisplayNameSnake
	}
	return nil
}

// PlayRequest contains parameters required to launch playback.
type PlayRequest struct {
	Channel     Channel `json:"channel"`
	Match       Match   `json:"match"`
	BitrateMode string  `json:"bitrateMode"`
}

// DefaultLeagues lists all 18 supported leagues grouped by category.
var DefaultLeagues = []League{
	{Key: "champions", Name: "UEFA Champions League", Code: "uefa.champions", Emoji: "🏆", Category: "Torneos Internacionales"},
	{Key: "europa", Name: "UEFA Europa League", Code: "uefa.europa", Emoji: "🟠", Category: "Torneos Internacionales"},
	{Key: "conference", Name: "UEFA Conference League", Code: "uefa.europa.conf", Emoji: "🟢", Category: "Torneos Internacionales"},
	{Key: "libertadores", Name: "Copa Libertadores", Code: "conmebol.libertadores", Emoji: "🌎", Category: "Torneos Internacionales"},
	{Key: "sudamericana", Name: "Copa Sudamericana", Code: "conmebol.sudamericana", Emoji: "🟡", Category: "Torneos Internacionales"},
	{Key: "eliminatorias", Name: "Eliminatorias CONMEBOL", Code: "fifa.worldq.conmebol", Emoji: "🌎", Category: "Selecciones"},
	{Key: "premier", Name: "Premier League", Code: "eng.1", Emoji: "🇬🇧", Category: "Ligas Europeas"},
	{Key: "laliga", Name: "La Liga", Code: "esp.1", Emoji: "🇪🇸", Category: "Ligas Europeas"},
	{Key: "seriea", Name: "Serie A", Code: "ita.1", Emoji: "🇮🇹", Category: "Ligas Europeas"},
	{Key: "bundesliga", Name: "Bundesliga", Code: "ger.1", Emoji: "🇩🇪", Category: "Ligas Europeas"},
	{Key: "ligue1", Name: "Ligue 1", Code: "fra.1", Emoji: "🇫🇷", Category: "Ligas Europeas"},
	{Key: "copadelrey", Name: "Copa del Rey", Code: "esp.copa_del_rey", Emoji: "👑", Category: "Copas Nacionales"},
	{Key: "facup", Name: "English FA Cup", Code: "eng.fa", Emoji: "🦁", Category: "Copas Nacionales"},
	{Key: "colombia", Name: "Liga BetPlay", Code: "col.1", Emoji: "🇨🇴", Category: "Ligas Sudamericanas"},
	{Key: "argentina", Name: "Liga Profesional", Code: "arg.1", Emoji: "🇦🇷", Category: "Ligas Sudamericanas"},
	{Key: "brasil", Name: "Brasileirão", Code: "bra.1", Emoji: "🇧🇷", Category: "Ligas Sudamericanas"},
	{Key: "ligamx", Name: "Liga MX", Code: "mex.1", Emoji: "🇲🇽", Category: "Ligas de América"},
	{Key: "mls", Name: "Major League Soccer", Code: "usa.1", Emoji: "🇺🇸", Category: "Ligas de América"},
}

// MatchDetails aggregates on-demand statistics, lineups, and match metadata.
type MatchDetails struct {
	Stats      *MatchStats   `json:"stats"`
	Lineups    *MatchLineups `json:"lineups"`
	Referee    string        `json:"referee"`
	Attendance int           `json:"attendance"`
	HomeCoach  string        `json:"homeCoach,omitempty"`
	AwayCoach  string        `json:"awayCoach,omitempty"`
	IsOffline  bool          `json:"isOffline,omitempty"`
	CachedAt   time.Time     `json:"cachedAt,omitempty"`
}

// Client defines the contract for fetching fixtures, match details, standings, and statistics from ESPN.
type Client interface {
	FetchMatches(ctx context.Context, leagueKey string) ([]Match, error)
	FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*MatchStats, error)
	FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*MatchDetails, error)
	FetchStandings(ctx context.Context, leagueKey string) (*LeagueStandings, error)
	FetchStatistics(ctx context.Context, leagueKey string) (*LeagueStatistics, error)
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
