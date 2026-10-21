//go:build windows

package player

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Compile-time proof that the named-pipe adapter satisfies net.Conn, so the
// shared vo-configured query can run over it unchanged.
var _ net.Conn = &pipeConn{}

// TestPlatformIPCPathWindows locks the Windows endpoint shape: mpv must receive
// a named-pipe address, never a filesystem path.
func TestPlatformIPCPathWindows(t *testing.T) {
	for _, dir := range []string{
		`C:\Users\esteban\AppData\Local\Temp\kickoff-mpv-42`,
		`C:\Temp\kickoff-mpv-42\`,
	} {
		if got, want := platformIPCPath(dir), `\\.\pipe\kickoff-mpv-42`; got != want {
			t.Fatalf("platformIPCPath(%q) = %q, want %q", dir, got, want)
		}
	}
}

// TestPipeConnAddresses exercises the adapter's address methods and Close path
// on a real file handle, which is the closest a Windows CI runner can get to a
// live mpv pipe without spawning mpv.
func TestPipeConnAddresses(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "pipe-probe-*")
	if err != nil {
		t.Fatalf("failed to create probe file: %v", err)
	}

	conn := &pipeConn{File: f, addr: pipeAddr(`\\.\pipe\kickoff-mpv-probe`)}

	if conn.LocalAddr().Network() != "pipe" {
		t.Fatalf("LocalAddr network = %q, want %q", conn.LocalAddr().Network(), "pipe")
	}
	if conn.RemoteAddr().String() != `\\.\pipe\kickoff-mpv-probe` {
		t.Fatalf("RemoteAddr = %q, want the pipe address", conn.RemoteAddr().String())
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// TestPlatformWaitKeepsStreamAliveWhenConfirmationTimesOut locks the Windows
// timeout contract: once mpv opened the pipe, a missing vo-configured answer
// must not kill or relaunch the process. Reverting to cleanupFailure plus
// launchFallbackNoIPC fails here, because the fallback cannot start from an
// empty command and would return an error instead of nil.
func TestPlatformWaitKeepsStreamAliveWhenConfirmationTimesOut(t *testing.T) {
	conn := newScriptedConn() // opens, then never answers

	original := dialIPC
	dialIPC = func(string) (net.Conn, error) { return conn, nil }
	t.Cleanup(func() {
		dialIPC = original
		_ = conn.Close()
	})

	controller := NewController(WithPollTimeout(120 * time.Millisecond))

	start := time.Now()
	err := controller.platformWait(context.Background(), &exec.Cmd{}, `\\.\pipe\probe`, t.TempDir(), make(chan error))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("platformWait must leave a running stream alone, got error %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("platformWait took %s; a wedged pipe must not be polled until pollTimeout", elapsed)
	}
}
