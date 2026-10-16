//go:build windows

package player

import (
	"context"
	"fmt"
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

func (c *Controller) platformWait(ctx context.Context, cmd *exec.Cmd, sockPath, dir string, exitCh <-chan error) error {
	select {
	case <-ctx.Done():
		c.cleanupFailure(cmd, dir)
		return ctx.Err()
	case err := <-exitCh:
		_ = os.RemoveAll(dir)
		if sockPath != "" {
			// If IPC named pipe fails or is unsupported, degrade gracefully to launching mpv without IPC socket.
			return c.launchFallbackNoIPC(ctx, cmd, dir)
		}
		return fmt.Errorf("%w: %v", model.ErrPlayerExited, err)
	case <-time.After(1500 * time.Millisecond):
		return nil
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
