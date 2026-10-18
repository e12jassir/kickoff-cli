package player

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestCreateSocketDir(t *testing.T) {
	dir, sockPath, err := CreateSocketDir()
	if err != nil {
		t.Fatalf("CreateSocketDir failed: %v", err)
	}
	defer os.RemoveAll(dir)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat created dir: %v", err)
	}

	// Threat matrix verification: must be 0700
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected socket dir permissions 0700, got %o", perm)
	}

	expectedSock := filepath.Join(dir, "mpv.sock")
	if sockPath != expectedSock {
		t.Fatalf("expected sockPath %s, got %s", expectedSock, sockPath)
	}
}

func TestBuildArgs(t *testing.T) {
	req := model.PlayRequest{
		Channel: model.Channel{
			Name:        "Sports HD (720p)",
			URL:         "https://example.invalid/live/feed.m3u8",
			BitrateMode: "min",
		},
		Match: model.Match{
			Title: "Barcelona vs Galatasaray",
		},
	}

	sock := "/tmp/mock.sock"
	args := BuildArgs(req, sock)

	expectedSubstrings := []string{
		"--fs",
		"--force-window=immediate",
		"--hwdec=auto-safe",
		"--no-terminal",
		"--no-input-terminal",
		"--really-quiet",
		"--no-ytdl",
		"--user-agent=Mozilla/5.0",
		"--demuxer-max-bytes=40M",
		"--demuxer-readahead-secs=5",
		"--hls-bitrate=min",
		"--network-timeout=20",
		"--input-ipc-server=/tmp/mock.sock",
		"--title=kickoff: Barcelona vs Galatasaray (Sports HD (720p))",
		// Sanitized URL check (valid RFC domain and scheme preserved)
		"https://example.invalid/live/feed.m3u8",
	}

	argStr := strings.Join(args, " ")
	for _, sub := range expectedSubstrings {
		if !strings.Contains(argStr, sub) {
			t.Errorf("BuildArgs missing expected substring %q in: %s", sub, argStr)
		}
	}

	// Threat matrix verification: '--' must directly precede cleanURL
	if len(args) < 2 {
		t.Fatalf("BuildArgs returned too few arguments: %v", args)
	}
	if args[len(args)-2] != "--" {
		t.Errorf("BuildArgs expected '--' delimiter before URL, got %q", args[len(args)-2])
	}
	if args[len(args)-1] != "https://example.invalid/live/feed.m3u8" {
		t.Errorf("BuildArgs expected URL as final argument, got %q", args[len(args)-1])
	}
}

func TestBuildArgs_ArgumentInjectionProtection(t *testing.T) {
	// A malicious URL that attempts to inject mpv flags
	req := model.PlayRequest{
		Channel: model.Channel{
			Name: "Evil Channel",
			URL:  "--script=/tmp/malicious.lua",
		},
		Match: model.Match{Title: "Match"},
	}

	args := BuildArgs(req, "/tmp/test.sock")
	if len(args) < 2 {
		t.Fatalf("BuildArgs returned too few arguments: %v", args)
	}
	if args[len(args)-2] != "--" {
		t.Fatalf("expected '--' before final argument, got %q", args[len(args)-2])
	}
	if args[len(args)-1] != "--script=/tmp/malicious.lua" {
		t.Fatalf("expected cleanURL as final argument, got %q", args[len(args)-1])
	}
}

func TestBuildArgs_WithHeaders(t *testing.T) {
	req := model.PlayRequest{
		Channel: model.Channel{
			Name: "DSports HD",
			URL:  "https://edge.invalid/live.m3u8",
			Headers: map[string]string{
				"Referer":    "https://portal.example.com/embed",
				"User-Agent": "CustomUA/2.0",
			},
		},
		Match: model.Match{
			Title: "Real Madrid vs Barcelona",
		},
	}

	args := BuildArgs(req, "/tmp/test.sock")
	argStr := strings.Join(args, " ")

	expectedReferer := "--http-header-fields=Referer: https://portal.example.com/embed"
	if !strings.Contains(argStr, expectedReferer) {
		t.Errorf("expected argument %q not found in args: %s", expectedReferer, argStr)
	}

	expectedUA := "--user-agent=CustomUA/2.0"
	if !strings.Contains(argStr, expectedUA) {
		t.Errorf("expected argument %q not found in args: %s", expectedUA, argStr)
	}

	if strings.Contains(argStr, "--user-agent=Mozilla/5.0") {
		t.Errorf("generic user-agent should have been replaced, but found in args: %s", argStr)
	}

	// Threat matrix verification: '--' must directly precede cleanURL
	if len(args) < 2 {
		t.Fatalf("BuildArgs returned too few arguments: %v", args)
	}
	if args[len(args)-2] != "--" {
		t.Errorf("BuildArgs expected '--' delimiter before URL, got %q", args[len(args)-2])
	}
	if args[len(args)-1] != "https://edge.invalid/live.m3u8" {
		t.Errorf("BuildArgs expected URL as final argument, got %q", args[len(args)-1])
	}
}

func TestQueryVoConfigured_MockSocket(t *testing.T) {
	tests := []struct {
		name       string
		responses  []string
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "Confirmed via data: true",
			responses:  []string{`{"data": true, "request_id": 0}` + "\n"},
			wantResult: true,
			wantErr:    false,
		},
		{
			name:       "Not yet configured via data: false",
			responses:  []string{`{"data": false, "request_id": 0}` + "\n"},
			wantResult: false,
			wantErr:    false,
		},
		{
			name:       "Confirmed via event video-reconfig",
			responses:  []string{`{"event": "video-reconfig"}` + "\n"},
			wantResult: true,
			wantErr:    false,
		},
		{
			name:       "Confirmed via event playback-restart",
			responses:  []string{`{"event": "playback-restart"}` + "\n"},
			wantResult: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "test-sock-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			sockPath := filepath.Join(tempDir, "test.sock")
			listener, err := net.Listen("unix", sockPath)
			if err != nil {
				t.Fatalf("failed to listen on mock socket: %v", err)
			}
			defer listener.Close()

			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()

				buf := make([]byte, 1024)
				_, _ = conn.Read(buf)

				for _, resp := range tt.responses {
					_, _ = conn.Write([]byte(resp))
				}
			}()

			clientConn, err := net.Dial("unix", sockPath)
			if err != nil {
				t.Fatalf("failed to dial mock socket: %v", err)
			}
			defer clientConn.Close()

			got, err := queryVoConfigured(clientConn)
			if (err != nil) != tt.wantErr {
				t.Fatalf("queryVoConfigured err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantResult {
				t.Fatalf("queryVoConfigured result = %v, wantResult %v", got, tt.wantResult)
			}
		})
	}
}

func TestController_Lifecycle_PrematureExit(t *testing.T) {
	// Create a mock executable that exits immediately with status 1
	mockScript, err := os.CreateTemp("", "mock-mpv-exit-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp mock script: %v", err)
	}
	defer os.Remove(mockScript.Name())

	_, _ = mockScript.WriteString("#!/bin/sh\nexit 1\n")
	_ = mockScript.Close()
	_ = os.Chmod(mockScript.Name(), 0o755)

	ctrl := NewController(
		WithMPVBinary(mockScript.Name()),
		WithPollTimeout(2*time.Second),
	)
	defer ctrl.Stop()

	req := model.PlayRequest{
		Channel: model.Channel{Name: "Test Channel", URL: "http://example.invalid/test.m3u8"},
		Match:   model.Match{Title: "Team A vs Team B"},
	}

	err = ctrl.PlayStream(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on premature process exit, got nil")
	}

	if !strings.Contains(err.Error(), model.ErrPlayerExited.Error()) {
		t.Fatalf("expected ErrPlayerExited, got: %v", err)
	}
}

func TestController_Lifecycle_Timeout(t *testing.T) {
	// Create a mock executable that sleeps indefinitely without creating or responding to socket
	mockScript, err := os.CreateTemp("", "mock-mpv-sleep-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp mock script: %v", err)
	}
	defer os.Remove(mockScript.Name())

	_, _ = mockScript.WriteString("#!/bin/sh\nsleep 30\n")
	_ = mockScript.Close()
	_ = os.Chmod(mockScript.Name(), 0o755)

	ctrl := NewController(
		WithMPVBinary(mockScript.Name()),
		WithPollTimeout(300*time.Millisecond),
	)
	defer ctrl.Stop()

	req := model.PlayRequest{
		Channel: model.Channel{Name: "Test Channel", URL: "http://example.invalid/test.m3u8"},
		Match:   model.Match{Title: "Team A vs Team B"},
	}

	err = ctrl.PlayStream(context.Background(), req)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), model.ErrPlayerTimeout.Error()) {
		t.Fatalf("expected ErrPlayerTimeout, got: %v", err)
	}
}

// TestController_StopConcurrentWithPlayStream_NoDoubleWait is a regression test
// for the double cmd.Wait() data race. It launches a long-lived fake player,
// lets the reaper goroutine enter cmd.Wait(), and then calls Stop()
// concurrently. Before the fix, stopLocked called cmd.Wait() on the same
// *exec.Cmd, racing the reaper; under -race this test fails.
func TestController_StopConcurrentWithPlayStream_NoDoubleWait(t *testing.T) {
	mockScript, err := os.CreateTemp("", "mock-mpv-live-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp mock script: %v", err)
	}
	defer os.Remove(mockScript.Name())

	_, _ = mockScript.WriteString("#!/bin/sh\nsleep 30\n")
	_ = mockScript.Close()
	_ = os.Chmod(mockScript.Name(), 0o755)

	ctrl := NewController(
		WithMPVBinary(mockScript.Name()),
		WithPollTimeout(5*time.Second),
	)
	defer ctrl.Stop()

	req := model.PlayRequest{
		Channel: model.Channel{Name: "Test Channel", URL: "http://example.invalid/test.m3u8"},
		Match:   model.Match{Title: "Team A vs Team B"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	playErr := make(chan error, 1)
	go func() {
		playErr <- ctrl.PlayStream(ctx, req)
	}()

	// Give the reaper goroutine time to start and block in cmd.Wait().
	time.Sleep(200 * time.Millisecond)

	// Stop concurrently with the live reaper: this must not call Wait() again.
	_ = ctrl.Stop()

	cancel()
	select {
	case <-playErr:
	case <-time.After(5 * time.Second):
		t.Fatal("PlayStream did not return after Stop")
	}
}
