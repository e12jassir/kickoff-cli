package bar

import (
	"strconv"
	"strings"
	"testing"
)

func TestLinuxNotifyCommand(t *testing.T) {
	const (
		title = "¡GOL de Luis Díaz! (45+2)"
		body  = "Barcelona 2 - 1 Real Madrid\nAsistencia: Lamine Yamal"
	)

	binary, args := linuxNotifyCommand(title, body)

	if binary != "notify-send" {
		t.Fatalf("expected binary %q, got %q", "notify-send", binary)
	}

	want := []string{"-a", "kickoff", title, body}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d: %q", len(want), len(args), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestDarwinNotifyScriptQuotesTitleAndBody(t *testing.T) {
	title := `¡GOL de "Lucho" Díaz!`
	body := "Barcelona 2 - 1 Real Madrid\nAsistencia: Lamine Yamal"

	script := darwinNotifyScript(title, body)

	if !strings.HasPrefix(script, "display notification ") {
		t.Fatalf("expected a display notification expression, got: %s", script)
	}
	if !strings.Contains(script, "with title "+strconv.Quote(title)) {
		t.Fatalf("expected the title as one quoted literal %s, got: %s", strconv.Quote(title), script)
	}
	if want := `"Barcelona 2 - 1 Real Madrid" & linefeed & "Asistencia: Lamine Yamal"`; !strings.Contains(script, want) {
		t.Fatalf("expected the two body lines joined by linefeed %s, got: %s", want, script)
	}
	if strings.Contains(script, `\n`) {
		t.Fatalf("expected no backslash-n in the AppleScript body, got: %s", script)
	}
	// A quote inside the text must never be emitted raw, or it would close the
	// AppleScript literal early.
	if strings.Contains(script, `"Lucho"`) {
		t.Fatalf("title quotes were not escaped: %s", script)
	}
}

func TestDarwinNotifyScriptSingleLineBody(t *testing.T) {
	title := "¡GOL de Luis Díaz!"
	body := "Barcelona 2 - 1 Real Madrid"

	script := darwinNotifyScript(title, body)

	if want := "display notification " + strconv.Quote(body) + " with title " + strconv.Quote(title); script != want {
		t.Fatalf("single-line script = %s, want %s", script, want)
	}
}

func TestWindowsNotifyScriptUsesRegisteredAppID(t *testing.T) {
	const registeredAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

	title := "¡GOL de Luis Díaz! (45+2)"
	body := "Barcelona 2 - 1 Real Madrid\nAsistencia: Lamine Yamal"

	script := windowsNotifyScript(title, body)

	if want := `CreateToastNotifier("` + registeredAppID + `")`; !strings.Contains(script, want) {
		t.Fatalf("expected a toast notifier bound to the registered AppUserModelID %q, got: %s", want, script)
	}
	if strings.Contains(script, `CreateToastNotifier("Kickoff")`) {
		t.Fatalf("toast notifier still uses the unregistered AppUserModelID: %s", script)
	}
	if want := `CreateTextNode("` + title + `")`; !strings.Contains(script, want) {
		t.Fatalf("expected the title text node %q, got: %s", want, script)
	}
}

func TestWindowsNotifyScriptRendersTwoLineBody(t *testing.T) {
	title := "¡GOL de Luis Díaz! (45+2)"
	body := "Barcelona 2 - 1 Real Madrid\nAsistencia: Lamine Yamal"

	script := windowsNotifyScript(title, body)

	// PowerShell renders a newline inside a double-quoted string with a
	// backtick-n escape; a literal \n would show up in the toast.
	if want := `CreateTextNode("Barcelona 2 - 1 Real Madrid` + "`n" + `Asistencia: Lamine Yamal")`; !strings.Contains(script, want) {
		t.Fatalf("expected the body line break as a PowerShell backtick-n, got: %s", script)
	}
	if strings.Contains(script, `\n`) {
		t.Fatalf("expected no literal backslash-n in the script, got: %s", script)
	}
}

func TestWindowsNotifyScriptEscapesPowerShellSpecialCharacters(t *testing.T) {
	title := `Gol de "Ñ" $HOME`
	body := "Costo `total` $x\n$env:PATH"

	script := windowsNotifyScript(title, body)

	// Inside a Windows PowerShell double-quoted string, " and $ are escaped
	// with a backtick and a backtick itself is doubled.
	escapedTitle := "Gol de `\"Ñ`\" `$HOME"
	if want := "CreateTextNode(\"" + escapedTitle + "\")"; !strings.Contains(script, want) {
		t.Fatalf("expected the title node %q, got: %s", want, script)
	}
	escapedBody := "Costo ``total`` `$x`n`$env:PATH"
	if want := "CreateTextNode(\"" + escapedBody + "\")"; !strings.Contains(script, want) {
		t.Fatalf("expected the body node %q, got: %s", want, script)
	}
	// The raw forms must be gone, otherwise the text could close the literal or
	// expand a PowerShell variable.
	if strings.Contains(script, `"Ñ"`) {
		t.Fatalf("title quotes were not escaped: %s", script)
	}
	assertBacktickEscaped(t, script, "$HOME")
	assertBacktickEscaped(t, script, "$x")
}

// assertBacktickEscaped fails unless the single occurrence of fragment is
// preceded by a PowerShell backtick escape.
func assertBacktickEscaped(t *testing.T, script, fragment string) {
	t.Helper()
	idx := strings.Index(script, fragment)
	if idx < 0 {
		t.Fatalf("expected %q in the script, got: %s", fragment, script)
	}
	if idx == 0 || script[idx-1] != '`' {
		t.Fatalf("expected %q to be backtick-escaped, got: %s", fragment, script)
	}
}
