package player

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff/internal/model"
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
			Name:        "ESPN 5 HD (720p)",
			URL:         "https://example.com/espn5.m3u8",
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
		"--title=kickoff: Barcelona vs Galatasaray (ESPN 5 HD (720p))",
		// Sanitized URL check (must be http, not https)
		"http://example.com/espn5.m3u8",
	}

	argStr := strings.Join(args, " ")
	for _, sub := range expectedSubstrings {
		if !strings.Contains(argStr, sub) {
			t.Errorf("BuildArgs missing expected substring %q in: %s", sub, argStr)
		}
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
		Channel: model.Channel{Name: "Test Channel", URL: "http://example.com/test.m3u8"},
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
		Channel: model.Channel{Name: "Test Channel", URL: "http://example.com/test.m3u8"},
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
