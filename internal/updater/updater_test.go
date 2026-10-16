package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v0.1.1", "v0.1.0", 1},
		{"0.1.1", "0.1.0", 1},
		{"v0.1.0", "v0.1.1", -1},
		{"v0.1.0", "v0.1.0", 0},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.2.0", "v0.1.9", 1},
		{"v0.1.0", "v1.0.0", -1},
		{"v0.1.2", "v0.1.2-7-gc4a2363", -1},
		{"v0.1.2-7-gc4a2363", "v0.1.2", 1},
		{"v0.1.3", "v0.1.2-7-gc4a2363", 1},
		{"v0.1.2", "dev", -1},
		{"dev", "v0.1.2", 1},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestFindAssetURL(t *testing.T) {
	rel := &ReleaseInfo{
		TagName: "v0.1.1",
		Assets: []Asset{
			{Name: "kickoff_0.1.1_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64.tar.gz"},
			{Name: "kickoff_0.1.1_linux_arm64.tar.gz", BrowserDownloadURL: "https://example.com/linux_arm64.tar.gz"},
			{Name: "kickoff_0.1.1_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64.tar.gz"},
			{Name: "kickoff_0.1.1_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_amd64.tar.gz"},
			{Name: "kickoff_0.1.1_windows_amd64.zip", BrowserDownloadURL: "https://example.com/windows_amd64.zip"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	u, err := FindAssetURL(rel, "linux", "amd64")
	if err != nil || u != "https://example.com/linux_amd64.tar.gz" {
		t.Fatalf("expected linux/amd64 asset, got: %s (err: %v)", u, err)
	}

	uDarwinArm, err := FindAssetURL(rel, "darwin", "arm64")
	if err != nil || uDarwinArm != "https://example.com/darwin_arm64.tar.gz" {
		t.Fatalf("expected darwin_arm64 asset for darwin/arm64, got: %s (err: %v)", uDarwinArm, err)
	}

	uDarwinAmd, err := FindAssetURL(rel, "darwin", "amd64")
	if err != nil || uDarwinAmd != "https://example.com/darwin_amd64.tar.gz" {
		t.Fatalf("expected darwin_amd64 asset for darwin/amd64, got: %s (err: %v)", uDarwinAmd, err)
	}

	uWin, err := FindAssetURL(rel, "windows", "amd64")
	if err != nil || uWin != "https://example.com/windows_amd64.zip" {
		t.Fatalf("expected windows_amd64 zip asset for windows/amd64, got: %s (err: %v)", uWin, err)
	}

	_, err = FindAssetURL(rel, "freebsd", "riscv64")
	if err == nil {
		t.Fatalf("expected error for unsupported platform, got nil")
	}

	// Test unversioned/releaser asset format
	unversionedRel := &ReleaseInfo{
		TagName: "v0.1.2",
		Assets: []Asset{
			{Name: "kickoff_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/unversioned_linux_amd64.tar.gz"},
			{Name: "kickoff_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/unversioned_darwin_arm64.tar.gz"},
			{Name: "kickoff_windows_amd64.zip", BrowserDownloadURL: "https://example.com/unversioned_windows_amd64.zip"},
		},
	}

	uUnvLinux, err := FindAssetURL(unversionedRel, "linux", "amd64")
	if err != nil || uUnvLinux != "https://example.com/unversioned_linux_amd64.tar.gz" {
		t.Fatalf("expected unversioned linux asset, got: %s (err: %v)", uUnvLinux, err)
	}

	uUnvDarwin, err := FindAssetURL(unversionedRel, "darwin", "arm64")
	if err != nil || uUnvDarwin != "https://example.com/unversioned_darwin_arm64.tar.gz" {
		t.Fatalf("expected unversioned darwin asset, got: %s (err: %v)", uUnvDarwin, err)
	}

	uUnvWin, err := FindAssetURL(unversionedRel, "windows", "amd64")
	if err != nil || uUnvWin != "https://example.com/unversioned_windows_amd64.zip" {
		t.Fatalf("expected unversioned windows asset, got: %s (err: %v)", uUnvWin, err)
	}

	// Test backward compatibility for legacy macos naming
	legacyRel := &ReleaseInfo{
		TagName: "v0.1.0",
		Assets: []Asset{
			{Name: "kickoff_0.1.0_macos_arm64.tar.gz", BrowserDownloadURL: "https://example.com/legacy_macos_arm64.tar.gz"},
		},
	}
	uLegacy, err := FindAssetURL(legacyRel, "darwin", "arm64")
	if err != nil || uLegacy != "https://example.com/legacy_macos_arm64.tar.gz" {
		t.Fatalf("expected legacy macos asset fallback, got: %s (err: %v)", uLegacy, err)
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := []byte("#!/bin/sh\necho kickoff binary")
	hdr := &tar.Header{
		Name: "kickoff",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()

	extracted, err := ExtractBinaryFromTarGz(&buf)
	if err != nil {
		t.Fatalf("failed to extract: %v", err)
	}
	if string(extracted) != string(content) {
		t.Errorf("extracted content mismatch: got %q, expected %q", string(extracted), string(content))
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	content := []byte("MZ Windows binary mock content")
	fw, err := zw.Create("kickoff.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	raw := buf.Bytes()
	extracted, err := ExtractBinaryFromZip(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("failed to extract zip: %v", err)
	}
	if string(extracted) != string(content) {
		t.Errorf("extracted content mismatch: got %q, expected %q", string(extracted), string(content))
	}
}
