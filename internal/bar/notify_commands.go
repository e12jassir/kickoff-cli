package bar

import (
	"fmt"
	"strings"
)

// windowsToastAppID is a registered AppUserModelID. Toast notifications only
// render on Windows when the notifier is bound to an AppID that exists in the
// registry; the bare application name used before was unregistered and the
// toast was silently dropped.
const windowsToastAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// windowsToastTextEscaper prepares free text for a Windows PowerShell
// double-quoted string literal: it doubles backticks, escapes `"` and `$`, and
// turns real line breaks into the backtick-n escape. A single pass guarantees
// the inserted backticks are never escaped again.
var windowsToastTextEscaper = strings.NewReplacer(
	"`", "``",
	`"`, "`\"",
	"$", "`$",
	"\r\n", "`n",
	"\n", "`n",
)

// escapeWindowsToastText makes text safe to embed inside a PowerShell
// double-quoted string.
func escapeWindowsToastText(text string) string {
	return windowsToastTextEscaper.Replace(text)
}

// linuxNotifyCommand builds the notify-send invocation used on Linux.
func linuxNotifyCommand(title, body string) (binary string, args []string) {
	return "notify-send", []string{"-a", "kickoff", title, body}
}

// darwinNotifyScript builds the AppleScript expression used on macOS. The title
// and every body line are Go-quoted so embedded quotes cannot terminate the
// literal early, and body lines are joined with the AppleScript `linefeed`
// constant because AppleScript string literals do not interpret backslash
// escapes such as \n. Without this a two-line toast body would render as one
// line (or fail to compile) instead of matching the Linux and Windows output.
func darwinNotifyScript(title, body string) string {
	return fmt.Sprintf(`display notification %s with title %q`, darwinBodyExpression(body), title)
}

// darwinBodyExpression turns a plain multi-line body into an AppleScript
// expression that produces real line breaks.
func darwinBodyExpression(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) == 1 {
		return fmt.Sprintf("%q", body)
	}

	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, fmt.Sprintf("%q", line))
	}
	return strings.Join(parts, " & linefeed & ")
}

// windowsNotifyScript builds the full PowerShell program that shows a
// two-line WinRT toast on Windows.
func windowsNotifyScript(title, body string) string {
	return fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($template.CreateTextNode("%s")) > $null
$textNodes.Item(1).AppendChild($template.CreateTextNode("%s")) > $null
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("%s")
$notification = [Windows.UI.Notifications.ToastNotification]::new($template)
$notifier.Show($notification)
`, escapeWindowsToastText(title), escapeWindowsToastText(body), windowsToastAppID)
}
