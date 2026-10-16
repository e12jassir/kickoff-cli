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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRepo         = "e12jassir/kickoff-cli"
	defaultTimeout      = 5 * time.Second
	maxReleaseReadSize  = 100 * 1024 * 1024 // 100MB bounded read for binary archives
	maxChecksumReadSize = 10 * 1024 * 1024  // 10MB bounded read for checksums / metadata
)

// ReleaseInfo holds GitHub release metadata.
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Assets      []Asset   `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func splitVersion(v string) (string, string) {
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		return v[:idx], v[idx:]
	}
	return v, ""
}

// CompareVersions returns:
//
//	 1 if v1 > v2
//	-1 if v1 < v2
//	 0 if v1 == v2
func CompareVersions(v1, v2 string) int {
	clean1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	clean2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	if clean1 == clean2 {
		return 0
	}
	if clean2 == "dev" {
		return -1
	}
	if clean1 == "dev" {
		return 1
	}

	base1, meta1 := splitVersion(clean1)
	base2, meta2 := splitVersion(clean2)

	parts1 := strings.Split(base1, ".")
	parts2 := strings.Split(base2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	// When base versions are equal: a version with git metadata (e.g. "-7-gc4a2363")
	// represents commits ahead of the base tag release.
	if meta1 != "" && meta2 == "" {
		return 1
	}
	if meta1 == "" && meta2 != "" {
		return -1
	}

	return 0
}

// CheckForUpdate queries GitHub for the latest release and checks if a newer version is available.
func CheckForUpdate(ctx context.Context, repo, currentVersion string) (*ReleaseInfo, bool, error) {
	var apiURL string
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		apiURL = repo
	} else {
		if repo == "" {
			repo = DefaultRepo
		}
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "kickoff-cli-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxChecksumReadSize)).Decode(&rel); err != nil {
		return nil, false, err
	}

	isNewer := CompareVersions(rel.TagName, currentVersion) > 0
	return &rel, isNewer, nil
}

// FindAsset locates the binary archive Asset corresponding to the current OS and architecture.
// It supports both versioned (kickoff_{version}_{os}_{arch}.tar.gz) and unversioned/releaser
// (kickoff_{os}_{arch}.tar.gz) formats, standardizing on 'darwin' while retaining backward
// compatibility for 'macos'.
func FindAsset(rel *ReleaseInfo, goos, goarch string) (*Asset, error) {
	normOS := strings.ToLower(goos)
	normArch := strings.ToLower(goarch)

	hasOS := func(name string) (bool, bool) {
		tokens := strings.FieldsFunc(name, func(r rune) bool {
			return r == '_' || r == '-' || r == '.'
		})
		matchedDarwin := false
		matchedMacos := false
		for _, t := range tokens {
			if normOS == "darwin" || normOS == "macos" {
				if t == "darwin" {
					matchedDarwin = true
				}
				if t == "macos" {
					matchedMacos = true
				}
			} else if t == normOS {
				return true, false
			}
		}
		if matchedDarwin {
			return true, true
		}
		if matchedMacos {
			return true, false
		}
		return false, false
	}

	hasArch := func(name string) bool {
		tokens := strings.FieldsFunc(name, func(r rune) bool {
			return r == '_' || r == '-' || r == '.'
		})
		for _, t := range tokens {
			if t == normArch {
				return true
			}
		}
		return false
	}

	var fallbackAsset *Asset
	for i := range rel.Assets {
		a := &rel.Assets[i]
		name := strings.ToLower(a.Name)
		isZip := strings.HasSuffix(name, ".zip")
		isTarGz := strings.HasSuffix(name, ".tar.gz")

		if !isZip && !isTarGz {
			continue
		}

		matchedOS, isDarwinToken := hasOS(name)
		if !matchedOS || !hasArch(name) {
			continue
		}

		if normOS == "windows" {
			if isZip {
				return a, nil
			}
			if isTarGz && fallbackAsset == nil {
				fallbackAsset = a
			}
		} else {
			if isTarGz {
				if (normOS == "darwin" || normOS == "macos") && isDarwinToken {
					return a, nil
				}
				if fallbackAsset == nil {
					fallbackAsset = a
				}
			}
		}
	}

	if fallbackAsset != nil {
		return fallbackAsset, nil
	}

	return nil, fmt.Errorf("no release asset found for %s/%s", normOS, normArch)
}

// FindAssetURL locates the binary archive URL corresponding to the current OS and architecture.
func FindAssetURL(rel *ReleaseInfo, goos, goarch string) (string, error) {
	asset, err := FindAsset(rel, goos, goarch)
	if err != nil {
		return "", err
	}
	return asset.BrowserDownloadURL, nil
}

// FindChecksumURL locates the download URL for checksums.txt.
func FindChecksumURL(rel *ReleaseInfo, assetURL string) (string, error) {
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, "checksums.txt") {
			return a.BrowserDownloadURL, nil
		}
	}
	if idx := strings.LastIndex(assetURL, "/"); idx != -1 {
		return assetURL[:idx+1] + "checksums.txt", nil
	}
	return "", errors.New("checksums.txt not found in release assets")
}

// ParseChecksums parses a standard sha256sum file into a map of filename -> expected sha256 hex hash.
func ParseChecksums(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			hash := strings.ToLower(fields[0])
			rest := strings.TrimSpace(line[len(fields[0]):])
			rest = strings.TrimPrefix(rest, "*")
			rest = strings.TrimSpace(rest)
			rest = strings.TrimPrefix(rest, "./")
			base := filepath.Base(rest)
			result[strings.ToLower(base)] = hash
		}
	}
	return result
}

// VerifyChecksum verifies that the SHA-256 hash of assetData matches the entry in checksums.txt.
// It fails closed if the asset is missing from checksums.txt or if the hash mismatches.
func VerifyChecksum(assetData []byte, assetName string, checksumsContent string) error {
	checksums := ParseChecksums(checksumsContent)
	baseName := strings.ToLower(filepath.Base(assetName))

	expectedHash, ok := checksums[baseName]
	if !ok {
		return fmt.Errorf("no checksum entry found for %q in checksums.txt", assetName)
	}

	h := sha256.Sum256(assetData)
	actualHash := hex.EncodeToString(h[:])

	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch for %q: expected %s, got %s", assetName, expectedHash, actualHash)
	}

	return nil
}

// ExtractBinaryFromZip extracts the executable binary named 'kickoff' (or 'kickoff.exe') from a zip archive.
func ExtractBinaryFromZip(r io.ReaderAt, size int64) ([]byte, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to read zip archive: %w", err)
	}

	for _, f := range zr.File {
		baseName := filepath.Base(f.Name)
		if baseName == "kickoff" || baseName == "kickoff.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open file inside zip: %w", err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}

	return nil, errors.New("binary 'kickoff.exe' not found inside zip archive")
}

// ExtractBinaryFromTarGz extracts the executable binary named 'kickoff' (or 'kickoff.exe') from a gzip tar stream.
func ExtractBinaryFromTarGz(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar reading error: %w", err)
		}

		baseName := filepath.Base(header.Name)
		if baseName == "kickoff" || baseName == "kickoff.exe" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("failed to read binary bytes: %w", err)
			}
			return data, nil
		}
	}

	return nil, errors.New("binary 'kickoff' not found inside release archive")
}

// SelfUpdate performs an in-place atomic update of the currently executing kickoff binary.
func SelfUpdate(ctx context.Context, repo, currentVersion string) error {
	cleanVer := strings.TrimPrefix(currentVersion, "v")
	fmt.Printf("⚽ Checking for kickoff updates (current version: v%s)...\n", cleanVer)

	rel, newer, err := CheckForUpdate(ctx, repo, currentVersion)
	if err != nil {
		return fmt.Errorf("error querying GitHub: %w", err)
	}

	if !newer {
		fmt.Printf("✓ kickoff is already at the latest version (%s). All up to date!\n", currentVersion)
		return nil
	}

	fmt.Printf("✨ New version available: %s! Downloading update...\n", rel.TagName)

	asset, err := FindAsset(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("no precompiled package found for your system (%s/%s): %w", runtime.GOOS, runtime.GOARCH, err)
	}
	assetURL := asset.BrowserDownloadURL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kickoff-cli-updater")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error downloading package: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download server responded with HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseReadSize))
	if err != nil {
		return fmt.Errorf("error reading downloaded package: %w", err)
	}

	// Verify SHA-256 checksum against checksums.txt (fail-closed)
	checksumURL, err := FindChecksumURL(rel, assetURL)
	if err != nil {
		return fmt.Errorf("failed to locate checksums.txt: %w", err)
	}

	chkReq, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for checksums: %w", err)
	}
	chkReq.Header.Set("User-Agent", "kickoff-cli-updater")

	chkResp, err := client.Do(chkReq)
	if err != nil {
		return fmt.Errorf("error downloading checksums.txt: %w", err)
	}
	defer chkResp.Body.Close()

	if chkResp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums download server responded with HTTP %d", chkResp.StatusCode)
	}

	chkBytes, err := io.ReadAll(io.LimitReader(chkResp.Body, maxChecksumReadSize))
	if err != nil {
		return fmt.Errorf("error reading checksums.txt: %w", err)
	}

	if err := VerifyChecksum(bodyBytes, asset.Name, string(chkBytes)); err != nil {
		return fmt.Errorf("security verification failed: %w", err)
	}

	var binData []byte
	if strings.HasSuffix(strings.ToLower(assetURL), ".zip") {
		binData, err = ExtractBinaryFromZip(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	} else {
		binData, err = ExtractBinaryFromTarGz(bytes.NewReader(bodyBytes))
	}
	if err != nil {
		return fmt.Errorf("error extracting binary: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("could not resolve symlink of executable: %w", err)
	}

	// Atomic replacement using temp file in the same directory
	dir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(dir, "kickoff-update-*")
	if err != nil {
		return fmt.Errorf("could not create temporary file in %s (permissions?): %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(binData); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("error writing new binary: %w", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("error granting execute permissions: %w", err)
	}
	_ = tmpFile.Close()

	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)
	if runtime.GOOS == "windows" {
		if err := os.Rename(exePath, oldPath); err != nil {
			return fmt.Errorf("could not move running binary: %w", err)
		}
	}
	if err := os.Rename(tmpName, exePath); err != nil {
		if runtime.GOOS == "windows" {
			_ = os.Rename(oldPath, exePath) // Rollback
		}
		return fmt.Errorf("error replacing binary at %s: %w", exePath, err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(oldPath)
	}

	// Clean outdated cache files so newly installed version fetches fresh data
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	if cacheBase != "" {
		cacheDir := filepath.Join(cacheBase, "kickoff")
		_ = os.RemoveAll(cacheDir)
	}

	fmt.Printf("🎉 kickoff successfully updated to version %s!\n", rel.TagName)
	fmt.Printf("Binary location: %s\n", exePath)
	return nil
}

// RunSetup diagnoses system dependencies, configurations, and environment prerequisites.
func RunSetup() error {
	fmt.Println("⚽ kickoff setup — Checking environment and dependencies")
	fmt.Println(strings.Repeat("─", 55))

	var warnings []string

	// 1. MPV binary
	mpvBin := "mpv"
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if runtime.GOOS == "windows" {
			for _, cand := range []string{"mpv.exe", "mpvnet.exe", "mpv"} {
				candidate := filepath.Join(dir, cand)
				if _, err := os.Stat(candidate); err == nil {
					mpvBin = candidate
					break
				}
			}
		} else {
			candidate := filepath.Join(dir, "mpv")
			if _, err := os.Stat(candidate); err == nil {
				mpvBin = candidate
			}
		}
	}

	mpvPath, err := exec.LookPath(mpvBin)
	if err != nil && runtime.GOOS == "windows" {
		for _, cand := range []string{"mpv.exe", "mpvnet.exe", "mpv"} {
			if p, e := exec.LookPath(cand); e == nil {
				mpvPath = p
				err = nil
				break
			}
		}
	}

	if err != nil {
		fmt.Println("❌ mpv: NOT FOUND")
		if runtime.GOOS == "windows" {
			warnings = append(warnings, "mpv is required to play streams. Install it with 'winget install mpv.net' in PowerShell or place mpv.exe next to kickoff.exe")
		} else {
			warnings = append(warnings, "mpv is required to play streams. Install it with 'sudo pacman -S mpv' (Arch), 'sudo apt install mpv' (Ubuntu/Debian) or 'brew install mpv' (macOS)")
		}
	} else {
		out, _ := exec.Command(mpvPath, "--version").Output()
		firstLine := strings.Split(string(out), "\n")[0]
		if filepath.Dir(mpvPath) == filepath.Dir(os.Args[0]) {
			fmt.Printf("✓ mpv (local portable): %s (%s)\n", mpvPath, firstLine)
		} else {
			fmt.Printf("✓ mpv: %s (%s)\n", mpvPath, firstLine)
		}
	}

	// 2. FFprobe (optional but recommended)
	if ffPath, err := exec.LookPath("ffprobe"); err == nil {
		fmt.Printf("✓ ffprobe: %s\n", ffPath)
	} else {
		fmt.Println("ℹ️  ffprobe: Not installed (optional)")
	}

	// 3. Emoji fonts on Linux
	if runtime.GOOS == "linux" {
		if fcPath, err := exec.LookPath("fc-list"); err == nil {
			out, _ := exec.Command(fcPath, ":", "family").Output()
			outStr := strings.ToLower(string(out))
			if strings.Contains(outStr, "emoji") || strings.Contains(outStr, "noto color") {
				fmt.Println("✓ Emoji fonts: Available on system")
			} else {
				fmt.Println("⚠️  Emoji fonts: Not detected")
				warnings = append(warnings, "Install emoji fonts (e.g. 'sudo pacman -S noto-fonts-emoji' or 'sudo apt install fonts-noto-color-emoji') to view flags and balls in the TUI, or run with --no-emoji")
			}
		}
	}

	// 4. Config Directory & channels.json
	configBase, err := os.UserConfigDir()
	if err != nil || configBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configBase = filepath.Join(home, ".config")
		}
	}
	if configBase != "" {
		cfgDir := filepath.Join(configBase, "kickoff")
		_ = os.MkdirAll(cfgDir, 0o755)
		channelsCfg := filepath.Join(cfgDir, "channels.json")
		if _, err := os.Stat(channelsCfg); err == nil {
			fmt.Printf("✓ Channels configuration: %s\n", channelsCfg)
		} else {
			fmt.Printf("ℹ️  Channels configuration: Built-in default (you can create %s for custom channels)\n", channelsCfg)
		}
	}

	// 5. Cache Directory
	cacheBase, err := os.UserCacheDir()
	if err != nil || cacheBase == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	if cacheBase != "" {
		cacheDir := filepath.Join(cacheBase, "kickoff")
		_ = os.MkdirAll(cacheDir, 0o755)
		fmt.Printf("✓ Cache directory: %s\n", cacheDir)
	}

	fmt.Println(strings.Repeat("─", 55))
	if len(warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, w := range warnings {
			fmt.Printf("  • %s\n", w)
		}
		return errors.New("incomplete dependencies")
	}

	fmt.Println("🚀 All set! kickoff is configured and ready.")
	return nil
}
