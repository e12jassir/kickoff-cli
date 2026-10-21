package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
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

func TestParseChecksums(t *testing.T) {
	content := `# Release checksums
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  kickoff_linux_amd64.tar.gz
a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90 *kickoff_windows_amd64.zip
f0e1d2c3b4a5968778695a4b3c2d1e0ff0e1d2c3b4a5968778695a4b3c2d1e0f  ./dist/kickoff_darwin_arm64.tar.gz

`
	res := ParseChecksums(content)

	if res["kickoff_linux_amd64.tar.gz"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("failed parsing kickoff_linux_amd64.tar.gz checksum")
	}
	if res["kickoff_windows_amd64.zip"] != "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90" {
		t.Errorf("failed parsing binary asterisk syntax for windows zip")
	}
	if res["kickoff_darwin_arm64.tar.gz"] != "f0e1d2c3b4a5968778695a4b3c2d1e0ff0e1d2c3b4a5968778695a4b3c2d1e0f" {
		t.Errorf("failed parsing relative path syntax for darwin archive")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello kickoff world")
	h := sha256.Sum256(data)
	correctHash := hex.EncodeToString(h[:])

	checksumsContent := correctHash + "  kickoff_linux_amd64.tar.gz\n"

	// 1. Success case
	if err := VerifyChecksum(data, "kickoff_linux_amd64.tar.gz", checksumsContent); err != nil {
		t.Fatalf("expected valid checksum verification, got: %v", err)
	}

	// 2. Mismatched checksum case (fail closed)
	badData := []byte("tampered binary content")
	if err := VerifyChecksum(badData, "kickoff_linux_amd64.tar.gz", checksumsContent); err == nil {
		t.Fatalf("expected fail closed on checksum mismatch, got nil")
	} else if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected 'checksum mismatch' error message, got: %v", err)
	}

	// 3. Missing asset in checksums.txt case (fail closed)
	if err := VerifyChecksum(data, "other_package.tar.gz", checksumsContent); err == nil {
		t.Fatalf("expected fail closed on missing asset in checksums.txt, got nil")
	} else if !strings.Contains(err.Error(), "no checksum entry found") {
		t.Fatalf("expected 'no checksum entry found' error message, got: %v", err)
	}
}

func TestFindChecksumURL(t *testing.T) {
	rel := &ReleaseInfo{
		TagName: "v0.1.1",
		Assets: []Asset{
			{Name: "kickoff_0.1.1_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64.tar.gz"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	u, err := FindChecksumURL(rel, "https://example.com/linux_amd64.tar.gz")
	if err != nil || u != "https://example.com/checksums.txt" {
		t.Fatalf("expected checksums.txt URL, got: %s (err: %v)", u, err)
	}

	// Test fallback when checksums.txt is not in rel.Assets
	relNoChk := &ReleaseInfo{
		TagName: "v0.1.1",
		Assets: []Asset{
			{Name: "kickoff_0.1.1_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/releases/v0.1.1/linux_amd64.tar.gz"},
		},
	}
	uFallback, err := FindChecksumURL(relNoChk, "https://example.com/releases/v0.1.1/linux_amd64.tar.gz")
	if err != nil || uFallback != "https://example.com/releases/v0.1.1/checksums.txt" {
		t.Fatalf("expected derived fallback checksums.txt URL, got: %s (err: %v)", uFallback, err)
	}
}

func TestSelfUpdate_ChecksumMismatchFailsClosed(t *testing.T) {
	// The updater resolves the asset for the running platform, so the fake
	// release has to advertise one for whatever OS this test binary was
	// compiled for instead of hardcoding linux/amd64.
	assetName := fmt.Sprintf("kickoff_99.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			rel := ReleaseInfo{
				TagName: "v99.9.9",
				Assets: []Asset{
					{
						Name:               assetName,
						BrowserDownloadURL: "http://" + r.Host + "/asset.tar.gz",
					},
					{
						Name:               "checksums.txt",
						BrowserDownloadURL: "http://" + r.Host + "/checksums.txt",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case "/asset.tar.gz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fake-corrupted-archive-content"))
		case "/checksums.txt":
			w.WriteHeader(http.StatusOK)
			// Return a mismatched hash
			_, _ = w.Write([]byte("1111111111111111111111111111111111111111111111111111111111111111  " + assetName + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	err := SelfUpdate(context.Background(), ts.URL+"/releases/latest", "v0.1.0")
	if err == nil {
		t.Fatalf("expected SelfUpdate to fail closed on checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "security verification failed") {
		t.Fatalf("expected security verification failed error, got: %v", err)
	}
}
