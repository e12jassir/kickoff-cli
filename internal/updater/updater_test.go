package updater

import (
	"archive/tar"
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
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	u, err := FindAssetURL(rel, "linux", "amd64")
	if err != nil || u != "https://example.com/linux_amd64.tar.gz" {
		t.Fatalf("expected linux/amd64 asset, got: %s (err: %v)", u, err)
	}

	uDarwin, err := FindAssetURL(rel, "darwin", "arm64")
	if err != nil || uDarwin != "https://example.com/darwin_arm64.tar.gz" {
		t.Fatalf("expected darwin/arm64 asset, got: %s (err: %v)", uDarwin, err)
	}

	_, err = FindAssetURL(rel, "freebsd", "riscv64")
	if err == nil {
		t.Fatalf("expected error for unsupported platform, got nil")
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
