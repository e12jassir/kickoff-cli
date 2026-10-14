//go:build windows

package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
)

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
		return fmt.Errorf("%w: %v", model.ErrPlayerExited, err)
	case <-time.After(1500 * time.Millisecond):
		return nil
	}
}
