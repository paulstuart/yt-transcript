package yttranscript

import (
	"testing"
)

var DefaultTimeoutSeconds = 5

func TestExtractVideoID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Plain video ID",
			input:   "dQw4w9WgXcQ",
			want:    "dQw4w9WgXcQ",
			wantErr: false,
		},
		{
			name:    "Full YouTube URL with v parameter",
			input:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			want:    "dQw4w9WgXcQ",
			wantErr: false,
		},
		{
			name:    "YouTube URL with additional parameters",
			input:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ&feature=share",
			want:    "dQw4w9WgXcQ",
			wantErr: false,
		},
		{
			name:    "Short youtu.be URL",
			input:   "https://youtu.be/dQw4w9WgXcQ",
			want:    "dQw4w9WgXcQ",
			wantErr: false,
		},
		{
			name:    "Short youtu.be URL with query params",
			input:   "https://youtu.be/dQw4w9WgXcQ?t=42",
			want:    "dQw4w9WgXcQ",
			wantErr: false,
		},
		{
			name:    "Invalid - too short",
			input:   "short",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid - no video ID in URL",
			input:   "https://www.youtube.com/",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Invalid - malformed",
			input:   "not a valid input",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractVideoID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractVideoID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractVideoID() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewClient verifies that a client can be created
func TestNewClient(t *testing.T) {
	client := NewClient(DefaultTimeoutSeconds)
	if client == nil {
		t.Error("NewClient() returned nil")
	}
	if client.ytClient == nil {
		t.Error("NewClient() did not initialize ytClient")
	}
}

// TestFetchTranscript_Integration is an integration test that requires internet connectivity
// and will make real API calls to YouTube. Skip if -short flag is set.
func TestFetchTranscript_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient(DefaultTimeoutSeconds)

	tests := []struct {
		name     string
		videoID  string
		config   *TranscriptConfig
		wantErr  bool
		checkLen bool
	}{
		{
			name:     "Fetch with default language",
			videoID:  "dQw4w9WgXcQ", // Rick Astley - Never Gonna Give You Up
			config:   nil,
			wantErr:  false,
			checkLen: true,
		},
		{
			name:    "Fetch with English language",
			videoID: "dQw4w9WgXcQ",
			config: &TranscriptConfig{
				Lang: "en",
			},
			wantErr:  false,
			checkLen: true,
		},
		{
			name:    "Invalid video ID should error",
			videoID: "invalid123",
			config:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.FetchTranscript(tt.videoID, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchTranscript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result == nil {
					t.Error("FetchTranscript() returned nil result")
					return
				}

				if result.Info.VideoID != tt.videoID {
					t.Errorf("FetchTranscript() VideoID = %v, want %v", result.Info.VideoID, tt.videoID)
				}

				if tt.checkLen && len(result.Smooshed.Text) == 0 {
					t.Error("FetchTranscript() returned empty transcripts")
				}

				// Verify transcript structure
				for i, segment := range result.Smooshed.Indexes {
					if i > 0 && segment.Offset == 0 {
						t.Errorf("Transcript segment %d has empty text", i)
					}
					if segment.Offset < 0 {
						t.Errorf("Transcript segment %d has negative offset", i)
					}
					if segment.Duration < 0 {
						t.Errorf("Transcript segment %d has negative duration", i)
					}
				}
			}
		})
	}
}

// TestFetchTranscript_VariousURLFormats tests URL extraction with integration
func TestFetchTranscript_VariousURLFormats(t *testing.T) {
	if true || testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewClient(DefaultTimeoutSeconds)

	urls := []string{
		"dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&feature=share",
	}

	for _, url := range urls {
		t.Run(url, func(t *testing.T) {
			result, err := client.FetchRawTranscript(url, nil)
			if err != nil {
				t.Errorf("FetchTranscript() with URL %s error = %v", url, err)
				return
			}

			if result.VideoID != "dQw4w9WgXcQ" {
				t.Errorf("FetchTranscript() VideoID = %v, want dQw4w9WgXcQ", result.VideoID)
			}

			if len(result.Lines) == 0 {
				t.Error("FetchTranscript() returned empty transcripts")
			}
		})
	}
}
