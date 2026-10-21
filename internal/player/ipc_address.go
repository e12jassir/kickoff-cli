package player

import (
	"path/filepath"
	"strings"
)

const (
	// unixSocketFileName is the historical socket file name inside the
	// per-playback temporary directory.
	unixSocketFileName = "mpv.sock"

	// windowsPipePrefix is the Windows named pipe namespace prefix required by
	// mpv's --input-ipc-server and by the Go standard library pipe opener.
	windowsPipePrefix = `\\.\pipe\`

	// defaultPipeName is used when a playback directory has no usable base
	// name, so the address is always a legal, non-empty pipe name.
	defaultPipeName = "kickoff-mpv"
)

// unixIPCPath returns the Unix domain socket address for a playback session
// rooted in dir. It must stay identical to the historical
// filepath.Join(dir, "mpv.sock") so socket creation, dialing and cleanup keep
// working exactly as before.
func unixIPCPath(dir string) string {
	return filepath.Join(dir, unixSocketFileName)
}

// windowsIPCPath returns the Windows named pipe address mpv must listen on for
// a playback session rooted in dir. The pipe name is derived from the session
// directory base so concurrent sessions never collide.
func windowsIPCPath(dir string) string {
	return windowsPipePrefix + windowsPipeName(dir)
}

func windowsPipeName(dir string) string {
	base := strings.TrimRight(dir, `/\`)
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}

	if name := sanitizePipeName(base); name != "" {
		return name
	}
	return defaultPipeName
}

// sanitizePipeName replaces every character Windows rejects in a pipe name.
// Only ASCII letters, digits, '-' and '_' survive.
func sanitizePipeName(base string) string {
	var b strings.Builder
	b.Grow(len(base))

	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}

	return strings.Trim(b.String(), "-")
}
