package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/catalog"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

const (
	defaultPollTimeout = 25 * time.Second
	defaultDialTimeout = 200 * time.Millisecond
)

type mpvResponse struct {
	Data      any    `json:"data"`
	Event     string `json:"event"`
	Error     string `json:"error"`
	RequestID int    `json:"request_id"`
}

// Controller supervises the MPV player lifecycle and IPC communication.
type Controller struct {
	mu          sync.Mutex
	mpvBinary   string
	pollTimeout time.Duration
	currentCmd  *exec.Cmd
	currentDir  string
	currentSock string
	doneCh      chan struct{}
}

// Option configures the player controller.
type Option func(*Controller)

func WithMPVBinary(bin string) Option {
	return func(c *Controller) {
		c.mpvBinary = bin
	}
}

func WithPollTimeout(d time.Duration) Option {
	return func(c *Controller) {
		c.pollTimeout = d
	}
}

// ResolveMPVBinary detects a local mpv binary in the same directory as kickoff,
// falling back to "mpv" in system PATH.
func ResolveMPVBinary(custom string) string {
	if custom != "" {
		return custom
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if runtime.GOOS == "windows" {
			for _, candidate := range []string{"mpv.exe", "mpvnet.exe", "mpv"} {
				target := filepath.Join(dir, candidate)
				if _, err := os.Stat(target); err == nil {
					return target
				}
			}
		} else {
			candidate := filepath.Join(dir, "mpv")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	if p, err := checkMPVInPath(); err == nil && p != "" {
		return p
	}
	return "mpv"
}

func NewController(opts ...Option) *Controller {
	c := &Controller{
		mpvBinary:   ResolveMPVBinary(""),
		pollTimeout: defaultPollTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CreateSocketDir creates an isolated temporary directory with 0700 permissions.
func CreateSocketDir() (string, string, error) {
	dir, err := os.MkdirTemp("", "kickoff-mpv-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir for socket: %w", err)
	}

	if err := enforceSocketDirPermissions(dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", "", fmt.Errorf("failed to enforce permissions on socket dir: %w", err)
	}

	sockPath := filepath.Join(dir, "mpv.sock")
	return dir, sockPath, nil
}

// BuildArgs constructs the full MPV command arguments for a playback request.
func BuildArgs(req model.PlayRequest, sockPath string) []string {
	bitrate := req.BitrateMode
	if bitrate == "" {
		bitrate = req.Channel.BitrateMode
	}
	if bitrate == "" {
		bitrate = "min"
	}

	cleanURL, err := catalog.SanitizeURL(req.Channel.URL)
	if err != nil {
		cleanURL = req.Channel.URL
	}

	title := fmt.Sprintf("kickoff: %s (%s)", req.Match.Title, req.Channel.Name)

	args := []string{
		"--fs",
		"--force-window=immediate",
		"--hwdec=auto-safe",
		"--no-terminal",
		"--no-input-terminal",
		"--really-quiet",
		"--no-ytdl",
		"--user-agent=Mozilla/5.0",
		fmt.Sprintf("--title=%s", title),
		"--cache=yes",
		"--demuxer-max-bytes=40M",
		"--demuxer-readahead-secs=5",
		fmt.Sprintf("--hls-bitrate=%s", bitrate),
		"--network-timeout=20",
	}
	if sockPath != "" {
		args = append(args, fmt.Sprintf("--input-ipc-server=%s", sockPath))
	}
	args = append(args, cleanURL)
	return args
}

// Stop terminates any currently supervised MPV instance and cleans up socket resources.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.stopLocked()
}

func (c *Controller) stopLocked() error {
	var errs []string

	if c.currentCmd != nil && c.currentCmd.Process != nil {
		killProcess(c.currentCmd)
		_ = c.currentCmd.Wait()
		c.currentCmd = nil
	}

	if c.currentDir != "" {
		if err := os.RemoveAll(c.currentDir); err != nil {
			errs = append(errs, err.Error())
		}
		c.currentDir = ""
		c.currentSock = ""
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// PlayStream launches an MPV instance detached from stdin and awaits IPC video confirmation.
func (c *Controller) PlayStream(ctx context.Context, req model.PlayRequest) error {
	c.mu.Lock()

	bin := ResolveMPVBinary(c.mpvBinary)
	if _, err := exec.LookPath(bin); err != nil {
		if p, errPath := checkMPVInPath(); errPath == nil {
			bin = p
		} else {
			c.mu.Unlock()
			if runtime.GOOS == "windows" {
				return fmt.Errorf("mpv is not installed on your system. Install it with 'winget install mpv.net' in PowerShell or place mpv.exe next to kickoff.exe")
			}
			return fmt.Errorf("mpv is not installed on your system. Install it with 'sudo pacman -S mpv' (Arch), 'sudo apt install mpv' (Ubuntu/Debian) or 'brew install mpv' (macOS) — https://mpv.io")
		}
	}

	_ = c.stopLocked()

	dir, sockPath, err := CreateSocketDir()
	if err != nil {
		c.mu.Unlock()
		return err
	}

	c.currentDir = dir
	c.currentSock = sockPath

	args := BuildArgs(req, sockPath)
	cmd := exec.Command(bin, args...)

	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		_ = os.RemoveAll(dir)
		c.mu.Unlock()
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	cmd.Stdin = devNull
	cmd.Stdout = nil
	cmd.Stderr = nil
	setProcessAttrs(cmd)

	if err := cmd.Start(); err != nil {
		_ = devNull.Close()
		_ = os.RemoveAll(dir)
		c.mu.Unlock()
		return fmt.Errorf("failed to start mpv: %w", err)
	}
	_ = devNull.Close()

	doneCh := make(chan struct{})
	c.doneCh = doneCh
	c.currentCmd = cmd
	c.mu.Unlock()

	return c.pollIPCConfirmation(ctx, cmd, sockPath, dir, doneCh)
}

// Wait blocks until the current MPV playback process exits.
func (c *Controller) Wait() error {
	c.mu.Lock()
	ch := c.doneCh
	c.mu.Unlock()

	if ch != nil {
		<-ch
	}
	return nil
}

func (c *Controller) pollIPCConfirmation(ctx context.Context, cmd *exec.Cmd, sockPath, dir string, doneCh chan struct{}) error {
	exitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		select {
		case exitCh <- err:
		default:
		}
		close(doneCh)
	}()

	return c.platformWait(ctx, cmd, sockPath, dir, exitCh)
}

func (c *Controller) cleanupFailure(cmd *exec.Cmd, dir string) {
	killProcess(cmd)
	_ = os.RemoveAll(dir)
}

func queryVoConfigured(conn net.Conn) (bool, error) {
	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))

	req := `{"command": ["get_property", "vo-configured"]}` + "\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return false, err
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return false, err
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		var resp mpvResponse
		if err := json.Unmarshal([]byte(lineStr), &resp); err != nil {
			continue
		}

		if b, ok := resp.Data.(bool); ok && b {
			return true, nil
		}
		if resp.Event == "video-reconfig" || resp.Event == "playback-restart" {
			return true, nil
		}

		if resp.Data != nil {
			return false, nil
		}
	}
}
