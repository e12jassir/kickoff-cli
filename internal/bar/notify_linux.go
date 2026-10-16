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
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return nil
	}

	title, body := FormatGoalNotification(match, homeScore, awayScore, event)
	cmd := exec.Command(bin, "-a", "kickoff", title, body)
	_ = cmd.Run()
	return nil
}
