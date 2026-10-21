//go:build windows

package bar

import (
	"os/exec"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type windowsNotifier struct{}

func newOSNotifier() Notifier {
	return &windowsNotifier{}
}

func (n *windowsNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	bin, err := exec.LookPath("powershell")
	if err != nil {
		bin, err = exec.LookPath("powershell.exe")
		if err != nil {
			return nil
		}
	}

	title, body := FormatGoalNotification(match, homeScore, awayScore, event)
	script := windowsNotifyScript(title, body)

	cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-Command", script)
	_ = cmd.Run()
	return nil
}
