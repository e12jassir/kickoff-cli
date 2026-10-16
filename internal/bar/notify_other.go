//go:build !linux && !darwin && !windows

package bar

import "github.com/e12jassir/kickoff-cli/internal/model"

type nullNotifier struct{}

func newOSNotifier() Notifier {
	return &nullNotifier{}
}

func (n *nullNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	return nil
}
