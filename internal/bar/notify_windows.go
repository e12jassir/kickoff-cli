//go:build windows

package bar

import (
	"fmt"
	"os/exec"
	"strings"

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
	safeTitle := strings.ReplaceAll(title, `"`, `\"`)
	safeBody := strings.ReplaceAll(body, `"`, `\"`)

	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($template.CreateTextNode("%s")) > $null
$textNodes.Item(1).AppendChild($template.CreateTextNode("%s")) > $null
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("Kickoff")
$notification = [Windows.UI.Notifications.ToastNotification]::new($template)
$notifier.Show($notification)
`, safeTitle, safeBody)

	cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-Command", script)
	_ = cmd.Run()
	return nil
}
