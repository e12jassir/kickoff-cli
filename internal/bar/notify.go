package bar

import (
	"fmt"

	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// Notifier defines the interface for desktop notifications.
type Notifier interface {
	NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error
}

// FormatGoalNotification creates a formatted title and body text for goal notifications.
func FormatGoalNotification(match model.Match, homeScore, awayScore string, event *model.MatchEvent, optCat ...*i18n.Catalog) (string, string) {
	cat := i18n.Default()
	if len(optCat) > 0 && optCat[0] != nil {
		cat = optCat[0]
	}
	home := match.HomeTeam.DisplayName
	if home == "" {
		home = match.HomeTeam.Name
	}
	if home == "" {
		home = cat.FallbackHome
	}

	away := match.AwayTeam.DisplayName
	if away == "" {
		away = match.AwayTeam.Name
	}
	if away == "" {
		away = cat.FallbackAway
	}

	scoreLine := fmt.Sprintf("%s %s - %s %s", home, homeScore, awayScore, away)

	var title string
	var assistLine string

	if event != nil {
		clock := event.Clock
		if clock == "" {
			clock = cat.FallbackClock
		}

		playerName := event.PlayerName
		if playerName == "" {
			playerName = cat.FallbackPlayer
		}

		if event.IsPenalty {
			title = cat.GoalPenaltyTitle(playerName, clock)
			assistLine = cat.AssistPenalty
		} else if event.IsOwnGoal {
			title = cat.GoalOwnGoalTitle(playerName, clock)
			assistLine = cat.AssistOwnGoal
		} else {
			title = cat.GoalTitle(playerName, clock)
			if event.AssistName != "" {
				assistLine = cat.AssistWith(event.AssistName)
			} else {
				assistLine = cat.AssistNone
			}
		}
	} else {
		title = cat.GoalScoreTitle(scoreLine)
	}

	body := scoreLine
	if assistLine != "" {
		body = cat.GoalBody(scoreLine, assistLine)
	}

	return title, body
}

// NewNotifier instantiates the platform-appropriate desktop notifier.
func NewNotifier() Notifier {
	return newOSNotifier()
}
