package yttranscript

import (
	"fmt"
	"strings"

	"github.com/horiagug/youtube-transcript-api-go/pkg/yt_transcript"
	models "github.com/horiagug/youtube-transcript-api-go/pkg/yt_transcript_models"
)

type TranscriptRaw = models.Transcript
type TranscriptLine = models.TranscriptLine

// Client provides methods to fetch YouTube transcripts
type Client struct {
	ytClient *yt_transcript.YtTranscriptClient
}

// NewClient creates a new YouTube transcript client
func NewClient(timeoutSeconds int) *Client {
	// Create the underlying YouTube transcript API client
	// Using default options (JSON formatter without timestamps for raw data)
	ytClient := yt_transcript.NewClient(yt_transcript.WithTimeout(timeoutSeconds))

	return &Client{
		ytClient: ytClient,
	}
}

// FetchRawTranscript fetches the transcript for a given YouTube video
// videoID can be either a full YouTube URL or just the video ID
// config can be nil to use default settings (first available language)
func (c *Client) FetchRawTranscript(videoID string, config *TranscriptConfig) (*TranscriptRaw, error) {
	// Extract the actual video ID if a URL was provided
	vid, err := extractVideoID(videoID)
	if err != nil {
		return nil, fmt.Errorf("extract video ID: %w", err)
	}

	// Set language preference if specified
	languages := []string{"en"} // default to English
	if config != nil && config.Lang != "" {
		languages = []string{config.Lang}
	}

	// Fetch the transcript using the library
	// The library returns a slice of Transcript objects (one per language)
	transcripts, err := c.ytClient.GetTranscripts(vid, languages)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript for video %s: %w", vid, err)
	}

	if len(transcripts) == 0 {
		return nil, fmt.Errorf("no transcripts available for video %s", vid)
	}

	// Use the first transcript (the one in the requested language)
	transcript := transcripts[0]
	return &transcript, nil
}

// FetchTranscript fetches the transcript for a given YouTube video
// videoID can be either a full YouTube URL or just the video ID
// config can be nil to use default settings (first available language)
func (c *Client) FetchTranscript(videoID string, config *TranscriptConfig) (*TranscriptResult, error) {
	// Extract the actual video ID if a URL was provided

	trans, err := c.FetchRawTranscript(videoID, config)
	if err != nil {
		return nil, fmt.Errorf("fetch raw transcript: %w", err)
	}
	smsh, err := ProcessTranscript(trans)
	if err != nil {
		return nil, fmt.Errorf("process transcript for video %s: %w", videoID, err)
	}

	result := &TranscriptResult{
		Info:     GetInfo(trans),
		Smooshed: smsh,
	}
	return result, nil
}

// extractVideoID extracts the video ID from a YouTube URL or returns the ID if already provided
// Supports formats like:
//   - Full URL: https://www.youtube.com/watch?v=VIDEO_ID
//   - Short URL: https://youtu.be/VIDEO_ID
//   - Just the ID: VIDEO_ID (11 characters)
func extractVideoID(input string) (string, error) {
	// If it's already an 11-character ID, return it
	if len(input) == 11 && !strings.Contains(input, "/") && !strings.Contains(input, "?") {
		return input, nil
	}

	// Try to extract from URL
	// Handle youtube.com URLs with v= parameter
	if idx := strings.Index(input, "v="); idx != -1 {
		start := idx + 2
		end := start + 11
		if end <= len(input) {
			// Extract the 11-character ID
			videoID := input[start:end]
			// Remove any trailing query parameters or fragments
			if idx := strings.IndexAny(videoID, "&?#"); idx != -1 {
				videoID = videoID[:idx]
			}
			return videoID, nil
		}
	}

	// Handle youtu.be short URLs
	if strings.Contains(input, "youtu.be/") {
		parts := strings.Split(input, "youtu.be/")
		if len(parts) >= 2 {
			videoID := parts[1]
			// Remove any query parameters or fragments
			if idx := strings.IndexAny(videoID, "?&#/"); idx != -1 {
				videoID = videoID[:idx]
			}
			if len(videoID) == 11 {
				return videoID, nil
			}
		}
	}

	return "", fmt.Errorf("unable to extract video ID from: %s", input)
}
