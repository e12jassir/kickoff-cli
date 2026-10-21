//go:build linux

package bar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestLinuxNotifier_InvokesNotifySend(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + logPath + "\n"
	if err := os.WriteFile(filepath.Join(dir, "notify-send"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake notify-send: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	match := model.Match{
		ID:       "m1",
		HomeTeam: model.Competitor{DisplayName: "Barcelona", Score: "2", IsHome: true},
		AwayTeam: model.Competitor{DisplayName: "Real Madrid", Score: "1"},
	}

	if err := NewNotifier().NotifyGoal(match, "2", "1", nil); err != nil {
		t.Fatalf("NotifyGoal returned an error: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("fake notify-send was not invoked: %v", err)
	}
	args := string(data)
	for _, want := range []string{"-a\nkickoff", "Barcelona 2 - 1 Real Madrid"} {
		if !strings.Contains(args, want) {
			t.Fatalf("expected notify-send arguments to contain %q, got:\n%s", want, args)
		}
	}
}

func TestLinuxNotifier_MissingBinaryIsNotAnError(t *testing.T) {
	// An empty PATH means notify-send cannot be found; the notifier must stay
	// silent instead of failing the refresh loop.
	t.Setenv("PATH", t.TempDir())

	if err := NewNotifier().NotifyGoal(model.Match{ID: "m1"}, "0", "0", nil); err != nil {
		t.Fatalf("expected no error when notify-send is missing, got %v", err)
	}
}
