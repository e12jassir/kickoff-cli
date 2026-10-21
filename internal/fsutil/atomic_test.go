package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "sub", "test.txt")

	data1 := []byte("first content")
	if err := AtomicWriteFile(targetPath, data1, 0o644); err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	read1, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(read1) != string(data1) {
		t.Errorf("expected %q, got %q", string(data1), string(read1))
	}

	// Atomic overwrite
	data2 := []byte("overwritten content")
	if err := AtomicWriteFile(targetPath, data2, 0o644); err != nil {
		t.Fatalf("AtomicWriteFile overwrite failed: %v", err)
	}

	read2, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read overwritten file: %v", err)
	}
	if string(read2) != string(data2) {
		t.Errorf("expected %q, got %q", string(data2), string(read2))
	}

	// No temporary files may survive a successful write.
	entries, err := os.ReadDir(filepath.Join(tempDir, "sub"))
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("unexpected temporary file found: %s", entry.Name())
		}
	}
}

func TestAtomicWriteFile_CreatesParentAndAppliesPerm(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "deep", "nested", "data.json")

	if err := AtomicWriteFile(targetPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat target: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("expected a regular file at %s", targetPath)
	}
	// Windows synthesizes the reported mode from the read-only attribute, so the
	// POSIX bits requested here cannot be observed there. Parent creation and
	// content are still asserted on every platform.
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("expected perm 0600, got %o", got)
		}
	}
}

func TestAtomicWriteFile_ReplacesSymlinkTarget(t *testing.T) {
	tempDir := t.TempDir()
	realPath := filepath.Join(tempDir, "real.json")
	if err := os.WriteFile(realPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("failed to seed file: %v", err)
	}

	// A rename over the target must replace the directory entry, never write
	// through it, so a stale symlink cannot redirect cache writes elsewhere.
	linkPath := filepath.Join(tempDir, "link.json")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := AtomicWriteFile(linkPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("expected the symlink to be replaced by a regular file")
	}
	if data, _ := os.ReadFile(realPath); string(data) != "old" {
		t.Errorf("expected the symlink target to be untouched, got %q", data)
	}
}
