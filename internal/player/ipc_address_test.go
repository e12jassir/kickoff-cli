package player

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// TestUnixIPCPath locks the historical Unix socket address: the address must
// stay exactly filepath.Join(dir, "mpv.sock") so existing Unix behaviour and
// socket cleanup are untouched.
func TestUnixIPCPath(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "absolute temp dir", dir: "/tmp/kickoff-mpv-123"},
		{name: "relative dir", dir: "kickoff-mpv-rel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(tt.dir, "mpv.sock")
			if got := unixIPCPath(tt.dir); got != want {
				t.Fatalf("unixIPCPath(%q) = %q, want %q", tt.dir, got, want)
			}
		})
	}
}

// TestWindowsIPCPath covers S5: mpv on Windows must be pointed at a named pipe
// address (--input-ipc-server=\\.\pipe\...), never at a filesystem path.
func TestWindowsIPCPath(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{
			name: "windows temp dir",
			dir:  `C:\Users\esteban\AppData\Local\Temp\kickoff-mpv-4242`,
			want: `\\.\pipe\kickoff-mpv-4242`,
		},
		{
			name: "trailing separator",
			dir:  `C:\Users\esteban\AppData\Local\Temp\kickoff-mpv-7\`,
			want: `\\.\pipe\kickoff-mpv-7`,
		},
		{
			name: "unix style dir produces the same pipe name",
			dir:  "/tmp/kickoff-mpv-9",
			want: `\\.\pipe\kickoff-mpv-9`,
		},
		{
			name: "characters windows rejects are replaced",
			dir:  `C:\Temp\kick off$mpv:1`,
			want: `\\.\pipe\kick-off-mpv-1`,
		},
		{
			name: "empty dir falls back to a stable pipe name",
			dir:  "",
			want: `\\.\pipe\kickoff-mpv`,
		},
		{
			name: "root only falls back to a stable pipe name",
			dir:  "/",
			want: `\\.\pipe\kickoff-mpv`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowsIPCPath(tt.dir); got != tt.want {
				t.Fatalf("windowsIPCPath(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

// TestWindowsIPCPath_ProducesLegalPipeName asserts the generated address is
// usable by mpv: correct prefix and no separator or reserved characters in the
// pipe name component.
func TestWindowsIPCPath_ProducesLegalPipeName(t *testing.T) {
	addresses := []string{
		windowsIPCPath(`C:\Users\esteban\AppData\Local\Temp\kickoff-mpv-1`),
		windowsIPCPath(`/tmp/kickoff-mpv-2`),
		windowsIPCPath(`weird dir\with spaces and $ymbols`),
		windowsIPCPath(""),
	}

	for _, address := range addresses {
		if !strings.HasPrefix(address, `\\.\pipe\`) {
			t.Fatalf("address %q does not start with the Windows pipe prefix", address)
		}
		name := strings.TrimPrefix(address, `\\.\pipe\`)
		if name == "" {
			t.Fatalf("address %q has an empty pipe name", address)
		}
		for _, forbidden := range []string{`\`, `/`, ":", " ", "$"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("pipe name %q contains forbidden character %q", name, forbidden)
			}
		}
	}
}

// TestWindowsIPCPath_UniquePerSession proves two playback sessions never share
// a pipe, so concurrent handshakes cannot cross-talk.
func TestWindowsIPCPath_UniquePerSession(t *testing.T) {
	a := windowsIPCPath(`C:\Temp\kickoff-mpv-100`)
	b := windowsIPCPath(`C:\Temp\kickoff-mpv-101`)

	if a == b {
		t.Fatalf("distinct session dirs produced the same pipe address %q", a)
	}
}

// TestBuildArgs_WindowsPipeAddress proves the Windows address reaches mpv
// through the normal argument builder and keeps the "--" URL delimiter.
func TestBuildArgs_WindowsPipeAddress(t *testing.T) {
	req := model.PlayRequest{
		Channel: model.Channel{Name: "Sports HD", URL: "https://example.invalid/live.m3u8"},
		Match:   model.Match{Title: "Barcelona vs Galatasaray"},
	}

	pipe := windowsIPCPath(`C:\Temp\kickoff-mpv-55`)
	args := BuildArgs(req, pipe)

	want := "--input-ipc-server=" + `\\.\pipe\kickoff-mpv-55`
	argStr := strings.Join(args, " ")
	if !strings.Contains(argStr, want) {
		t.Fatalf("BuildArgs missing %q in: %s", want, argStr)
	}
	if len(args) < 2 || args[len(args)-2] != "--" {
		t.Fatalf("BuildArgs lost the '--' delimiter before the URL: %v", args)
	}
}

// TestCreateSocketDir_UsesPlatformIPCPath proves CreateSocketDir routes address
// construction through the platform signature instead of hardcoding a socket.
func TestCreateSocketDir_UsesPlatformIPCPath(t *testing.T) {
	dir, address, err := CreateSocketDir()
	if err != nil {
		t.Fatalf("CreateSocketDir failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if want := platformIPCPath(dir); address != want {
		t.Fatalf("CreateSocketDir address = %q, want platformIPCPath %q", address, want)
	}
}

// TestUnixIPCPath_MatchesHistoricalJoin locks the exact expression the player
// used before IPC addresses became platform-aware, including the edge shapes
// filepath.Join normalizes. The test above only compares platformIPCPath with
// itself, so this one is what actually protects Unix behaviour.
func TestUnixIPCPath_MatchesHistoricalJoin(t *testing.T) {
	for _, dir := range []string{
		"/tmp/kickoff-mpv-1",
		"/tmp/kickoff-mpv-1/",
		"/tmp/a b/mpv dir",
		".",
		"/",
		"",
		"/tmp/x/../y",
	} {
		if got, want := unixIPCPath(dir), filepath.Join(dir, unixSocketFileName); got != want {
			t.Fatalf("unixIPCPath(%q) = %q, want %q", dir, got, want)
		}
	}
}
