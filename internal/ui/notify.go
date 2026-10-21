package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/e12jassir/kickoff-cli/internal/bar"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// detectGoalNotifications compares the freshly loaded scores against the last
// observed ones and returns a command that raises a desktop notification for
// every goal scored in a match that is currently live.
//
// The comparison runs inside the update loop so it stays deterministic; only the
// notifier call is deferred to a command, because spawning notify-send must never
// block the Bubble Tea event loop. The first observation of a fixture only primes
// the baseline, so opening Kickoff never replays goals scored earlier, and
// completed fixtures are ignored even when their final score arrives late.
func (m *Model) detectGoalNotifications() tea.Cmd {
	if m.goalTracker == nil || m.notifier == nil {
		return nil
	}

	observed := make([]model.Match, 0, len(m.matches)+len(m.pastMatches))
	observed = append(observed, m.matches...)
	observed = append(observed, m.pastMatches...)

	var live []bar.ScoreChange
	for _, change := range m.goalTracker.Update(observed) {
		if change.Match.Status.State != "in" {
			continue
		}
		live = append(live, change)
	}
	if len(live) == 0 {
		return nil
	}

	notifier := m.notifier
	return func() tea.Msg {
		for _, change := range live {
			_ = notifier.NotifyGoal(change.Match, change.NewHome, change.NewAway, change.GoalEvent)
		}
		return nil
	}
}
