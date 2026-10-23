package model_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestChannel_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected model.Channel
		wantErr  bool
	}{
		{
			name: "camelCase bitrateMode and standard fields",
			input: `{
				"displayName": "ESPN 1 HD",
				"name": "ESPN 1",
				"url": "https://stream.example.com/live.m3u8",
				"resolution": "1080p",
				"bitrateMode": "max",
				"recommended": true
			}`,
			expected: model.Channel{
				DisplayName: "ESPN 1 HD",
				Name:        "ESPN 1",
				URL:         "https://stream.example.com/live.m3u8",
				Resolution:  "1080p",
				BitrateMode: "max",
				Recommended: true,
			},
			wantErr: false,
		},
		{
			name: "snake_case bitrate_mode",
			input: `{
				"displayName": "TyC Sports HD",
				"name": "TyC Sports",
				"url": "https://stream.example.com/tyc.m3u8",
				"resolution": "720p",
				"bitrate_mode": "max",
				"recommended": false
			}`,
			expected: model.Channel{
				DisplayName: "TyC Sports HD",
				Name:        "TyC Sports",
				URL:         "https://stream.example.com/tyc.m3u8",
				Resolution:  "720p",
				BitrateMode: "max",
				Recommended: false,
			},
			wantErr: false,
		},
		{
			name: "snake_case bitrate_mode with min bitrate and display_name",
			input: `{
				"display_name": "Win Sports+ HD",
				"name": "Win+",
				"url": "https://stream.example.com/win.m3u8",
				"resolution": "720p",
				"bitrate_mode": "min",
				"recommended": true
			}`,
			expected: model.Channel{
				DisplayName: "Win Sports+ HD",
				Name:        "Win+",
				URL:         "https://stream.example.com/win.m3u8",
				Resolution:  "720p",
				BitrateMode: "min",
				Recommended: true,
			},
			wantErr: false,
		},
		{
			name: "both camelCase and snake_case present (camelCase precedence)",
			input: `{
				"name": "Dual Test",
				"bitrateMode": "max",
				"bitrate_mode": "min"
			}`,
			expected: model.Channel{
				Name:        "Dual Test",
				BitrateMode: "max",
			},
			wantErr: false,
		},
		{
			name: "empty bitrate mode fields",
			input: `{
				"name": "No Bitrate",
				"url": "https://example.com/test.m3u8"
			}`,
			expected: model.Channel{
				Name:        "No Bitrate",
				URL:         "https://example.com/test.m3u8",
				BitrateMode: "",
			},
			wantErr: false,
		},
		{
			name: "headers map preserved",
			input: `{
				"name": "Stream with Headers",
				"url": "https://example.com/test.m3u8",
				"headers": {
					"Referer": "https://example.com",
					"User-Agent": "Custom-UA"
				}
			}`,
			expected: model.Channel{
				Name: "Stream with Headers",
				URL:  "https://example.com/test.m3u8",
				Headers: map[string]string{
					"Referer":    "https://example.com",
					"User-Agent": "Custom-UA",
				},
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON payload",
			input:   `{invalid_json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ch model.Channel
			err := json.Unmarshal([]byte(tt.input), &ch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(ch, tt.expected) {
				t.Errorf("UnmarshalJSON() = %+v, want %+v", ch, tt.expected)
			}
		})
	}
}

func TestChannelSlice_UnmarshalJSON(t *testing.T) {
	input := `[
		{
			"name": "Channel 1",
			"bitrate_mode": "max"
		},
		{
			"name": "Channel 2",
			"bitrateMode": "min"
		}
	]`

	var channels []model.Channel
	if err := json.Unmarshal([]byte(input), &channels); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].BitrateMode != "max" {
		t.Errorf("channel 0 expected BitrateMode 'max', got %q", channels[0].BitrateMode)
	}
	if channels[1].BitrateMode != "min" {
		t.Errorf("channel 1 expected BitrateMode 'min', got %q", channels[1].BitrateMode)
	}
}

func TestForceRefreshContext(t *testing.T) {
	// IsForceRefresh documents that a nil Context is tolerated, so that case has
	// to stay covered. A typed nil keeps the assertion while avoiding an untyped
	// nil literal in the call.
	var nilCtx context.Context
	if model.IsForceRefresh(nilCtx) {
		t.Error("expected IsForceRefresh(nil) to be false")
	}

	ctx := context.Background()
	if model.IsForceRefresh(ctx) {
		t.Error("expected IsForceRefresh(background) to be false")
	}

	forcedCtx := model.WithForceRefresh(ctx)
	if !model.IsForceRefresh(forcedCtx) {
		t.Error("expected IsForceRefresh(forcedCtx) to be true")
	}
}
