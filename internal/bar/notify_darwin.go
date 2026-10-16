//go:build darwin

package bar

import (
	"fmt"
	"os/exec"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type darwinNotifier struct{}

func newOSNotifier() Notifier {
	return &darwinNotifier{}
}

func (n *darwinNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	bin, err := exec.LookPath("osascript")
	if err != nil {
		return nil
	}

	title, body := FormatGoalNotification(match, homeScore, awayScore, event)
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	cmd := exec.Command(bin, "-e", script)
	_ = cmd.Run()
	return nil
}
