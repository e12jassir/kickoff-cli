// Package fsutil provides small filesystem helpers shared across services.
package fsutil

import (
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to a temporary file in the same directory as
// targetPath and then renames it into place. Readers therefore observe either
// the previous contents or the complete new contents, never a partial write,
// even if the process dies mid-write. Parent directories are created as needed.
func AtomicWriteFile(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, perm)

	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
