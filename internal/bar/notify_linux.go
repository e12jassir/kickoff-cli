//go:build linux

package bar

import (
	"os/exec"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type linuxNotifier struct{}

func newOSNotifier() Notifier {
	return &linuxNotifier{}
}

func (n *linuxNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	title, body := FormatGoalNotification(match, homeScore, awayScore, event)
	name, args := linuxNotifyCommand(title, body)

	bin, err := exec.LookPath(name)
	if err != nil {
		return nil
	}

	cmd := exec.Command(bin, args...)
	_ = cmd.Run()
	return nil
}
