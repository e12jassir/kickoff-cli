package bar

import (
	"fmt"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// Notifier defines the interface for desktop notifications.
type Notifier interface {
	NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error
}

// FormatGoalNotification creates a formatted title and body text for goal notifications.
func FormatGoalNotification(match model.Match, homeScore, awayScore string, event *model.MatchEvent) (string, string) {
	home := match.HomeTeam.DisplayName
	if home == "" {
		home = match.HomeTeam.Name
	}
	if home == "" {
		home = "Local"
	}

	away := match.AwayTeam.DisplayName
	if away == "" {
		away = match.AwayTeam.Name
	}
	if away == "" {
		away = "Visitante"
	}

	title := fmt.Sprintf("⚽ ¡GOL! %s %s - %s %s", home, homeScore, awayScore, away)

	var body string
	if event != nil && event.PlayerName != "" {
		clock := event.Clock
		if clock == "" {
			clock = "LIVE"
		}
		detail := ""
		if event.IsPenalty {
			detail = " (pen.)"
		} else if event.IsOwnGoal {
			detail = " (a.g.)"
		}
		body = fmt.Sprintf("%s%s (%s)", event.PlayerName, detail, clock)
	} else {
		body = fmt.Sprintf("%s vs %s", home, away)
	}

	return title, body
}

// NewNotifier instantiates the platform-appropriate desktop notifier.
func NewNotifier() Notifier {
	return newOSNotifier()
}
