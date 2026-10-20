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

	scoreLine := fmt.Sprintf("%s %s - %s %s", home, homeScore, awayScore, away)

	var title string
	var assistLine string

	if event != nil {
		clock := event.Clock
		if clock == "" {
			clock = "LIVE"
		}

		playerName := event.PlayerName
		if playerName == "" {
			playerName = "Jugador"
		}

		if event.IsPenalty {
			title = fmt.Sprintf("¡GOL de %s! (pen. %s)", playerName, clock)
			assistLine = "Asistencia: Penal"
		} else if event.IsOwnGoal {
			title = fmt.Sprintf("¡GOL en propia puerta de %s! (%s)", playerName, clock)
			assistLine = "Asistencia: Autogol"
		} else {
			title = fmt.Sprintf("¡GOL de %s! (%s)", playerName, clock)
			if event.AssistName != "" {
				assistLine = fmt.Sprintf("Asistencia: %s", event.AssistName)
			} else {
				assistLine = "Asistencia: Sin asistencia"
			}
		}
	} else {
		title = fmt.Sprintf("¡GOL! %s", scoreLine)
	}

	body := scoreLine
	if assistLine != "" {
		body = fmt.Sprintf("%s\n%s", scoreLine, assistLine)
	}

	return title, body
}

// NewNotifier instantiates the platform-appropriate desktop notifier.
func NewNotifier() Notifier {
	return newOSNotifier()
}
