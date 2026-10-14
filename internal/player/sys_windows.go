//go:build windows

package player

import (
	"os/exec"
	"syscall"
)

func setProcessAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func enforceSocketDirPermissions(dir string) error {
	// Permissions are managed by Windows ACLs on user temp directory.
	return nil
}
