package espn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type DemoClient struct {
	mu     sync.Mutex
	minute int
}

func NewDemoClient() *DemoClient {
	return &DemoClient{minute: 66}
}

func (d *DemoClient) FetchMatches(ctx context.Context, leagueKey string) ([]model.Match, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if model.IsForceRefresh(ctx) && d.minute < 90 {
		d.minute++
	}

	now := time.Now()
	homeScore := "2"
	events := []model.MatchEvent{
		{Type: "goal", PlayerName: "R. Lewandowski", Clock: "24'", ClockValue: 24 * 60, TeamID: "home-1", IsScoring: true},
		{Type: "card_yellow", PlayerName: "Gavi", Clock: "41'", ClockValue: 41 * 60, TeamID: "home-1", IsYellow: true},
		{Type: "penalty", PlayerName: "K. Mbappé", Clock: "54'", ClockValue: 54 * 60, TeamID: "away-1", IsScoring: true, IsPenalty: true},
		{Type: "card_red", PlayerName: "A. Rüdiger", Clock: "63'", ClockValue: 63 * 60, TeamID: "away-1", IsRedCard: true},
	}

	if d.minute >= 69 {
		homeScore = "3"
		events = append(events, model.MatchEvent{
			Type:       "goal",
			PlayerName: "Lamine Yamal",
			Clock:      "69'",
			ClockValue: 69 * 60,
			TeamID:     "home-1",
			IsScoring:  true,
		})
	}

	return []model.Match{
		{
			ID:        "demo-live-1",
			Title:     "FC Barcelona vs Real Madrid",
			HomeTeam:  model.Competitor{ID: "home-1", DisplayName: "FC Barcelona", Score: homeScore, IsHome: true},
			AwayTeam:  model.Competitor{ID: "away-1", DisplayName: "Real Madrid", Score: "1", IsHome: false},
			StartTime: now.Add(-1 * time.Hour),
			TimeStr:   now.Add(-1 * time.Hour).Format("15:04"),
			Venue:     "Spotify Camp Nou",
			Status: model.MatchStatus{
				Description: "Segundo Tiempo",
				Detail:      fmt.Sprintf("%d'", d.minute),
				State:       "in",
			},
			LeagueKey:  leagueKey,
			Events:     events,
			Referee:    "Szymon Marciniak",
			Attendance: 89420,
			Broadcasts: []string{"ESPN", "ESPN 2"},
			SlotIndex:  0,
		},
		{
			ID:        "demo-live-2",
			Title:     "Bayern Munich vs Arsenal",
			HomeTeam:  model.Competitor{ID: "home-2", DisplayName: "Bayern Munich", Score: "1", IsHome: true},
			AwayTeam:  model.Competitor{ID: "away-2", DisplayName: "Arsenal", Score: "1", IsHome: false},
			StartTime: now.Add(-45 * time.Minute),
			TimeStr:   now.Add(-45 * time.Minute).Format("15:04"),
			Venue:     "Allianz Arena",
			Status: model.MatchStatus{
				Description: "Primer Tiempo",
				Detail:      fmt.Sprintf("%d'", d.minute-25),
				State:       "in",
			},
			LeagueKey:  leagueKey,
			Referee:    "Clément Turpin",
			Attendance: 75000,
			Broadcasts: []string{"ESPN 2", "Fox Sports 2"},
			SlotIndex:  1,
		},
		{
			ID:        "demo-pre-1",
			Title:     "Liverpool vs Inter Milan",
			HomeTeam:  model.Competitor{ID: "home-3", DisplayName: "Liverpool", IsHome: true},
			AwayTeam:  model.Competitor{ID: "away-3", DisplayName: "Inter Milan", IsHome: false},
			StartTime: now.Add(2 * time.Hour),
			TimeStr:   now.Add(2 * time.Hour).Format("15:04"),
			Venue:     "Anfield",
			Status: model.MatchStatus{
				Description: "Programado",
				State:       "pre",
			},
			LeagueKey:  leagueKey,
			Broadcasts: []string{"ESPN 3", "Fox Sports 3"},
			SlotIndex:  2,
		},
	}, nil
}

func (d *DemoClient) FetchMatchDetails(ctx context.Context, leagueKey, eventID string) (*model.MatchDetails, error) {
	d.mu.Lock()
	min := d.minute
	d.mu.Unlock()

	shotsHome := "14"
	shotsOnGoalHome := "6"
	if min >= 69 {
		shotsHome = "16"
		shotsOnGoalHome = "8"
	}

	return &model.MatchDetails{
		Referee:    "Szymon Marciniak",
		Attendance: 89420,
		Stats: &model.MatchStats{
			HomePossession:  "61%",
			AwayPossession:  "39%",
			HomeShots:       shotsHome,
			AwayShots:       "9",
			HomeShotsOnGoal: shotsOnGoalHome,
			AwayShotsOnGoal: "3",
			HomePasses:      fmt.Sprintf("%d", min*8),
			AwayPasses:      fmt.Sprintf("%d", min*5),
			HomePassPct:     "90%",
			AwayPassPct:     "83%",
			HomeFouls:       "8",
			AwayFouls:       "13",
			HomeCorners:     "7",
			AwayCorners:     "4",
			HomeOffsides:    "2",
			AwayOffsides:    "4",
			HomeSaves:       "2",
			AwaySaves:       "5",
		},
		Lineups: &model.MatchLineups{
			Home: model.TeamLineup{
				Formation: "4-3-3",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "M. ter Stegen", Position: "GK", IsStarter: true},
					{Jersey: "23", DisplayName: "J. Koundé", Position: "DF", IsStarter: true},
					{Jersey: "4", DisplayName: "R. Araújo", Position: "DF", IsStarter: true},
					{Jersey: "2", DisplayName: "P. Cubarsí", Position: "DF", IsStarter: true},
					{Jersey: "3", DisplayName: "A. Balde", Position: "DF", IsStarter: true},
					{Jersey: "8", DisplayName: "Pedri", Position: "MF", IsStarter: true},
					{Jersey: "21", DisplayName: "F. de Jong", Position: "MF", IsStarter: true},
					{Jersey: "6", DisplayName: "Gavi", Position: "MF", IsStarter: true},
					{Jersey: "19", DisplayName: "Lamine Yamal", Position: "FW", IsStarter: true},
					{Jersey: "9", DisplayName: "R. Lewandowski", Position: "FW", IsStarter: true},
					{Jersey: "11", DisplayName: "Raphinha", Position: "FW", IsStarter: true},
				},
			},
			Away: model.TeamLineup{
				Formation: "4-2-3-1",
				Starters: []model.Player{
					{Jersey: "1", DisplayName: "T. Courtois", Position: "GK", IsStarter: true},
					{Jersey: "2", DisplayName: "D. Carvajal", Position: "DF", IsStarter: true},
					{Jersey: "3", DisplayName: "É. Militão", Position: "DF", IsStarter: true},
					{Jersey: "22", DisplayName: "A. Rüdiger", Position: "DF", IsStarter: true},
					{Jersey: "23", DisplayName: "F. Mendy", Position: "DF", IsStarter: true},
					{Jersey: "14", DisplayName: "A. Tchouaméni", Position: "MF", IsStarter: true},
					{Jersey: "8", DisplayName: "F. Valverde", Position: "MF", IsStarter: true},
					{Jersey: "11", DisplayName: "Rodrygo", Position: "MF", IsStarter: true},
					{Jersey: "5", DisplayName: "J. Bellingham", Position: "MF", IsStarter: true},
					{Jersey: "7", DisplayName: "Vinícius Jr", Position: "FW", IsStarter: true},
					{Jersey: "9", DisplayName: "K. Mbappé", Position: "FW", IsStarter: true},
				},
			},
		},
	}, nil
}

func (d *DemoClient) FetchMatchStats(ctx context.Context, leagueKey, eventID string) (*model.MatchStats, error) {
	details, err := d.FetchMatchDetails(ctx, leagueKey, eventID)
	if err != nil || details == nil {
		return nil, err
	}
	return details.Stats, nil
}

func (d *DemoClient) FetchStandings(ctx context.Context, leagueKey string) (*model.LeagueStandings, error) {
	return &model.LeagueStandings{
		LeagueKey: leagueKey,
		Name:      "LaLiga 2024-25",
		Entries: []model.StandingsEntry{
			{Rank: 1, TeamName: "FC Barcelona", GamesPlayed: "9", Wins: "8", Ties: "0", Losses: "1", PointDiff: "+19", Points: "24"},
			{Rank: 2, TeamName: "Real Madrid", GamesPlayed: "9", Wins: "6", Ties: "3", Losses: "0", PointDiff: "+13", Points: "21"},
			{Rank: 3, TeamName: "Atlético Madrid", GamesPlayed: "9", Wins: "4", Ties: "5", Losses: "0", PointDiff: "+8", Points: "17"},
			{Rank: 4, TeamName: "Villarreal", GamesPlayed: "9", Wins: "5", Ties: "2", Losses: "2", PointDiff: "+1", Points: "17"},
			{Rank: 5, TeamName: "Osasuna", GamesPlayed: "9", Wins: "4", Ties: "3", Losses: "2", PointDiff: "-1", Points: "15"},
		},
		CachedAt: time.Now(),
	}, nil
}

func (d *DemoClient) FetchStatistics(ctx context.Context, leagueKey string) (*model.LeagueStatistics, error) {
	return &model.LeagueStatistics{
		LeagueKey: leagueKey,
		Scorers: []model.ScorerEntry{
			{Rank: 1, PlayerName: "R. Lewandowski", TeamName: "FC Barcelona", Value: "10", DisplayValue: "Partidos: 9, Goles: 10"},
			{Rank: 2, PlayerName: "Ayoze Pérez", TeamName: "Villarreal", Value: "6", DisplayValue: "Partidos: 7, Goles: 6"},
			{Rank: 3, PlayerName: "K. Mbappé", TeamName: "Real Madrid", Value: "5", DisplayValue: "Partidos: 8, Goles: 5"},
			{Rank: 4, PlayerName: "Raphinha", TeamName: "FC Barcelona", Value: "5", DisplayValue: "Partidos: 9, Goles: 5"},
			{Rank: 5, PlayerName: "Vinícius Jr", TeamName: "Real Madrid", Value: "4", DisplayValue: "Partidos: 9, Goles: 4"},
		},
		Assists: []model.ScorerEntry{
			{Rank: 1, PlayerName: "Lamine Yamal", TeamName: "FC Barcelona", Value: "5", DisplayValue: "Partidos: 9, Asistencias: 5"},
			{Rank: 2, PlayerName: "Raphinha", TeamName: "FC Barcelona", Value: "4", DisplayValue: "Partidos: 9, Asistencias: 4"},
			{Rank: 3, PlayerName: "Vinícius Jr", TeamName: "Real Madrid", Value: "4", DisplayValue: "Partidos: 9, Asistencias: 4"},
			{Rank: 4, PlayerName: "Alex Baena", TeamName: "Villarreal", Value: "3", DisplayValue: "Partidos: 8, Asistencias: 3"},
			{Rank: 5, PlayerName: "Antoine Griezmann", TeamName: "Atlético Madrid", Value: "3", DisplayValue: "Partidos: 9, Asistencias: 3"},
		},
		CachedAt: time.Now(),
	}, nil
}
