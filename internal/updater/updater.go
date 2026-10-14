package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
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
	DefaultRepo    = "e12jassir/kickoff-cli"
	defaultTimeout = 5 * time.Second
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

// CompareVersions returns:
//
//	 1 if v1 > v2
//	-1 if v1 < v2
//	 0 if v1 == v2
func CompareVersions(v1, v2 string) int {
	clean1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	clean2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(clean1, ".")
	parts2 := strings.Split(clean2, ".")

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
	return 0
}

// CheckForUpdate queries GitHub for the latest release and checks if a newer version is available.
func CheckForUpdate(ctx context.Context, repo, currentVersion string) (*ReleaseInfo, bool, error) {
	if repo == "" {
		repo = DefaultRepo
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

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
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, false, err
	}

	isNewer := CompareVersions(rel.TagName, currentVersion) > 0
	return &rel, isNewer, nil
}

// FindAssetURL locates the binary tarball URL corresponding to the current OS and architecture.
func FindAssetURL(rel *ReleaseInfo, goos, goarch string) (string, error) {
	normOS := strings.ToLower(goos)
	normArch := strings.ToLower(goarch)

	// In goreleaser tarballs, names typically look like kickoff_0.1.1_linux_amd64.tar.gz
	for _, a := range rel.Assets {
		name := strings.ToLower(a.Name)
		if strings.Contains(name, normOS) && strings.Contains(name, normArch) && strings.HasSuffix(name, ".tar.gz") {
			return a.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no release asset found for %s/%s", normOS, normArch)
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
	fmt.Printf("⚽ Comprobando actualizaciones para kickoff (versión actual: v%s)...\n", currentVersion)

	rel, newer, err := CheckForUpdate(ctx, repo, currentVersion)
	if err != nil {
		return fmt.Errorf("error al consultar GitHub: %w", err)
	}

	if !newer {
		fmt.Printf("✓ kickoff ya está en la versión más reciente (%s). ¡Todo al día!\n", currentVersion)
		return nil
	}

	fmt.Printf("✨ ¡Nueva versión encontrada: %s! Descargando actualización...\n", rel.TagName)

	assetURL, err := FindAssetURL(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("no se encontró paquete precompilado para tu sistema (%s/%s): %w", runtime.GOOS, runtime.GOARCH, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kickoff-cli-updater")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error al descargar paquete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servidor de descargas respondió con código %d", resp.StatusCode)
	}

	binData, err := ExtractBinaryFromTarGz(resp.Body)
	if err != nil {
		return fmt.Errorf("error al descomprimir binario: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("no se pudo determinar la ruta del ejecutable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("no se pudo resolver symlink del ejecutable: %w", err)
	}

	// Atomic replacement using temp file in the same directory
	dir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(dir, "kickoff-update-*")
	if err != nil {
		return fmt.Errorf("no se pudo crear archivo temporal en %s (¿permisos?): %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(binData); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("error al escribir nuevo binario: %w", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("error al otorgar permisos de ejecución: %w", err)
	}
	_ = tmpFile.Close()

	if err := os.Rename(tmpName, exePath); err != nil {
		return fmt.Errorf("error al reemplazar binario en %s: %w", exePath, err)
	}

	// Clean outdated cache files so newly installed version fetches fresh data
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cacheBase == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	if cacheBase != "" {
		cacheDir := filepath.Join(cacheBase, "kickoff")
		_ = os.RemoveAll(cacheDir)
	}

	fmt.Printf("🎉 ¡kickoff actualizado con éxito a la versión %s!\n", rel.TagName)
	fmt.Printf("Ubicación del binario: %s\n", exePath)
	return nil
}

// RunSetup diagnoses system dependencies, configurations, and environment prerequisites.
func RunSetup() error {
	fmt.Println("⚽ kickoff setup — Comprobando entorno y dependencias")
	fmt.Println(strings.Repeat("─", 55))

	var warnings []string

	// 1. MPV binary
	mpvPath, err := exec.LookPath("mpv")
	if err != nil {
		fmt.Println("❌ mpv: NO ENCONTRADO")
		warnings = append(warnings, "mpv es obligatorio para reproducir partidos. Instálalo con 'sudo pacman -S mpv' o 'sudo apt install mpv'")
	} else {
		out, _ := exec.Command("mpv", "--version").Output()
		firstLine := strings.Split(string(out), "\n")[0]
		fmt.Printf("✓ mpv: %s (%s)\n", mpvPath, firstLine)
	}

	// 2. FFprobe (optional but recommended)
	if ffPath, err := exec.LookPath("ffprobe"); err == nil {
		fmt.Printf("✓ ffprobe: %s\n", ffPath)
	} else {
		fmt.Println("ℹ️  ffprobe: No instalado (opcional)")
	}

	// 3. Emoji fonts on Linux
	if runtime.GOOS == "linux" {
		if fcPath, err := exec.LookPath("fc-list"); err == nil {
			out, _ := exec.Command(fcPath, ":", "family").Output()
			outStr := strings.ToLower(string(out))
			if strings.Contains(outStr, "emoji") || strings.Contains(outStr, "noto color") {
				fmt.Println("✓ Fuentes de emojis: Disponibles en el sistema")
			} else {
				fmt.Println("⚠️  Fuentes de emojis: No detectadas")
				warnings = append(warnings, "Instala fuentes de emojis (ej. 'sudo pacman -S noto-fonts-emoji' o 'sudo apt install fonts-noto-color-emoji') para visualizar banderas y balones en la TUI")
			}
		}
	}

	// 4. Config Directory & channels.json
	configBase := os.Getenv("XDG_CONFIG_HOME")
	if configBase == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			configBase = filepath.Join(home, ".config")
		}
	}
	if configBase != "" {
		cfgDir := filepath.Join(configBase, "kickoff")
		_ = os.MkdirAll(cfgDir, 0o755)
		channelsCfg := filepath.Join(cfgDir, "channels.json")
		if _, err := os.Stat(channelsCfg); err == nil {
			fmt.Printf("✓ Configuración de canales: %s\n", channelsCfg)
		} else {
			fmt.Printf("ℹ️  Configuración de canales: Predeterminada integrada (puedes crear %s si quieres canales personalizados)\n", channelsCfg)
		}
	}

	// 5. Cache Directory
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cacheBase == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheBase = filepath.Join(home, ".cache")
		}
	}
	if cacheBase != "" {
		cacheDir := filepath.Join(cacheBase, "kickoff")
		_ = os.MkdirAll(cacheDir, 0o755)
		fmt.Printf("✓ Directorio de caché: %s\n", cacheDir)
	}

	fmt.Println(strings.Repeat("─", 55))
	if len(warnings) > 0 {
		fmt.Println("⚠️  Atención:")
		for _, w := range warnings {
			fmt.Printf("  • %s\n", w)
		}
		return errors.New("dependencias incompletas")
	}

	fmt.Println("🚀 ¡Todo listo! kickoff está configurado al 100%.")
	return nil
}
