//go:build !windows

package player

import (
	"os"
	"os/exec"
	"syscall"
)

func setProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
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
