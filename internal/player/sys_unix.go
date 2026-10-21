//go:build !windows

package player

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

// checkMPVInPath searches for mpv in PATH on Unix.
func checkMPVInPath() (string, error) {
	return exec.LookPath("mpv")
}

func setProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		pid := cmd.Process.Pid
		if pid > 0 {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
		_ = cmd.Process.Kill()
	}
}

func enforceSocketDirPermissions(dir string) error {
	return os.Chmod(dir, 0o700)
}

// platformIPCPath returns the Unix domain socket path for a playback session
// rooted in dir. Unix keeps the historical filesystem socket semantics.
func platformIPCPath(dir string) string {
	return unixIPCPath(dir)
}

func (c *Controller) platformWait(ctx context.Context, cmd *exec.Cmd, sockPath, dir string, exitCh <-chan error) error {
	start := time.Now()
	timeout := c.pollTimeout

	var conn net.Conn
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.cleanupFailure(cmd, dir)
			return ctx.Err()
		case err := <-exitCh:
			_ = os.RemoveAll(dir)
			return fmt.Errorf("%w: %v", model.ErrPlayerExited, err)
		default:
		}

		if time.Since(start) > timeout {
			c.cleanupFailure(cmd, dir)
			return model.ErrPlayerTimeout
		}

		if conn == nil {
			if _, err := os.Stat(sockPath); err == nil {
				dConn, err := net.DialTimeout("unix", sockPath, defaultDialTimeout)
				if err == nil {
					conn = dConn
				}
			}
		}

		if conn != nil {
			confirmed, err := queryVoConfigured(conn)
			if err == nil && confirmed {
				_ = os.Remove(sockPath)
				return nil
			}
			if err != nil {
				_ = conn.Close()
				conn = nil
			}
		}

		time.Sleep(50 * time.Millisecond)
	}
}
