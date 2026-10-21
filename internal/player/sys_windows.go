//go:build windows

package player

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// checkMPVInPath searches for mpv.exe, mpv, or mpvnet.exe in PATH.
func checkMPVInPath() (string, error) {
	for _, bin := range []string{"mpv.exe", "mpv", "mpvnet.exe"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

func setProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		_ = cmd.Process.Kill()
	}
}

func enforceSocketDirPermissions(dir string) error {
	// Permissions are managed by Windows ACLs on user temp directory.
	return nil
}

// platformIPCPath returns the Windows named pipe address for a playback session
// rooted in dir. mpv opens the pipe itself when it receives the address through
// --input-ipc-server, so there is no filesystem socket to create or remove.
func platformIPCPath(dir string) string {
	return windowsIPCPath(dir)
}

// pipeAddr is the net.Addr reported by a named pipe connection.
type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

// pipeConn adapts an *os.File opened on a Windows named pipe to net.Conn so the
// shared JSON IPC query works unchanged on Windows.
//
// Reads are blocking: os.OpenFile opens the pipe without FILE_FLAG_OVERLAPPED,
// so the Go runtime poller cannot enforce deadlines and SetDeadline returns
// ErrNoDeadline (queryVoConfigured ignores that error). Reads are bounded in
// practice because mpv answers every command and because the pipe breaks as
// soon as the mpv process exits.
type pipeConn struct {
	*os.File
	addr pipeAddr
}

func (c *pipeConn) LocalAddr() net.Addr  { return c.addr }
func (c *pipeConn) RemoteAddr() net.Addr { return c.addr }

// dialNamedPipe opens an existing mpv named pipe. It fails fast while mpv has
// not created the pipe yet, which is what the polling loop retries on.
func dialNamedPipe(pipePath string) (net.Conn, error) {
	f, err := os.OpenFile(pipePath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &pipeConn{File: f, addr: pipeAddr(pipePath)}, nil
}

// dialIPC opens the mpv IPC endpoint for a poll iteration. It is a package
// variable so the Windows poll loop can be exercised with a stub connection,
// which is the only way this platform-specific contract gets executable
// coverage on a Windows CI runner.
var dialIPC = dialNamedPipe

// platformWait performs the same mpv IPC handshake as Unix, over a Windows
// named pipe. It polls until mpv confirms that video output is configured, the
// configured poll timeout expires, or the process exits.
//
// It degrades to launching mpv without IPC only when the pipe could never be
// opened, which is the case the no-IPC fallback exists for. If the pipe did
// open but vo-configured was never confirmed within the timeout, the running
// process is a healthy stream the user is already watching, so it is left
// alone instead of being killed and relaunched.
func (c *Controller) platformWait(ctx context.Context, cmd *exec.Cmd, pipePath, dir string, exitCh <-chan error) error {
	start := time.Now()
	timeout := c.pollTimeout
	dialed := false

	var conn net.Conn
	closeConn := func() {
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
	}
	defer closeConn()

	for {
		select {
		case <-ctx.Done():
			closeConn()
			c.cleanupFailure(cmd, dir)
			return ctx.Err()
		case <-exitCh:
			closeConn()
			_ = os.RemoveAll(dir)
			return c.launchFallbackNoIPC(ctx, cmd, dir)
		default:
		}

		if time.Since(start) > timeout {
			closeConn()
			if !dialed {
				c.cleanupFailure(cmd, dir)
				return c.launchFallbackNoIPC(ctx, cmd, dir)
			}
			return nil
		}

		if conn == nil {
			if dConn, err := dialIPC(pipePath); err == nil {
				conn = dConn
				dialed = true
			}
		}

		if conn != nil {
			confirmed, err := queryVoConfiguredBounded(conn, ipcQueryTimeout)
			switch {
			case err == nil && confirmed:
				closeConn()
				return nil
			case errors.Is(err, errIPCQueryTimeout):
				// The pipe opened but mpv stopped answering. Status is
				// best-effort from here: keep the running stream and stop
				// polling, instead of leaving blocked helpers behind or
				// killing a stream the user is watching.
				closeConn()
				return nil
			case err != nil:
				closeConn()
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func (c *Controller) launchFallbackNoIPC(ctx context.Context, origCmd *exec.Cmd, dir string) error {
	var fallbackArgs []string
	for _, arg := range origCmd.Args[1:] {
		if !strings.HasPrefix(arg, "--input-ipc-server") {
			fallbackArgs = append(fallbackArgs, arg)
		}
	}

	bin := origCmd.Path
	if bin == "" && len(origCmd.Args) > 0 {
		bin = origCmd.Args[0]
	}

	cmd := exec.Command(bin, fallbackArgs...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err == nil {
		cmd.Stdin = devNull
		defer devNull.Close()
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	setProcessAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv in fallback mode without IPC socket: %w", err)
	}

	c.mu.Lock()
	c.currentCmd = cmd
	doneCh := make(chan struct{})
	c.doneCh = doneCh
	c.mu.Unlock()

	exitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		select {
		case exitCh <- err:
		default:
		}
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		c.cleanupFailure(cmd, dir)
		return ctx.Err()
	case err := <-exitCh:
		_ = os.RemoveAll(dir)
		return fmt.Errorf("%w: %v", model.ErrPlayerExited, err)
	case <-time.After(1500 * time.Millisecond):
		return nil
	}
}
