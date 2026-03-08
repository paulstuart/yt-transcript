package yttranscript

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Client provides methods to fetch YouTube transcripts.
type Client struct {
	timeout time.Duration
}

// NewClient creates a new YouTube transcript client.
// timeoutSeconds is applied per-request.
func NewClient(timeoutSeconds int) *Client {
	return &Client{
		timeout: time.Duration(timeoutSeconds) * time.Second,
	}
}

// FetchRawTranscript fetches the raw transcript for a YouTube video.
// videoID may be a full URL or an 11-character video ID.
// config may be nil to use defaults (English, first available).
func (c *Client) FetchRawTranscript(videoID string, config *TranscriptConfig) (*TranscriptRaw, error) {
	vid, err := extractVideoID(videoID)
	if err != nil {
		return nil, fmt.Errorf("extract video ID: %w", err)
	}

	lang := "en"
	if config != nil && config.Lang != "" {
		lang = config.Lang
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	pageHTML, err := fetchVideoPage(ctx, vid)
	if err != nil {
		return nil, fmt.Errorf("fetch video page for %s: %w", vid, err)
	}

	apiKey, err := extractInnertubeKey(pageHTML)
	if err != nil {
		return nil, fmt.Errorf("extract innertube key for %s: %w", vid, err)
	}

	visitorData := extractVisitorData(pageHTML)

	tracks, err := fetchCaptionTracks(ctx, vid, apiKey, visitorData)
	if err != nil {
		return nil, fmt.Errorf("fetch caption tracks for %s: %w", vid, err)
	}

	track, err := selectTrack(tracks, lang)
	if err != nil {
		return nil, fmt.Errorf("select track for %s lang=%s: %w", vid, lang, err)
	}

	xmlStr, err := fetchTranscriptXML(ctx, vid, track.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript XML for %s: %w", vid, err)
	}

	lines, err := parseTranscriptXML(xmlStr)
	if err != nil {
		return nil, fmt.Errorf("parse transcript for %s: %w", vid, err)
	}

	return &TranscriptRaw{
		VideoID:        vid,
		Language:       track.Name,
		LanguageCode:   track.LanguageCode,
		IsGenerated:    track.IsGenerated,
		IsTranslatable: track.IsTranslatable,
		Lines:          lines,
	}, nil
}

// FetchTranscript fetches and smooshes the transcript for a YouTube video.
func (c *Client) FetchTranscript(videoID string, config *TranscriptConfig) (*TranscriptResult, error) {
	raw, err := c.FetchRawTranscript(videoID, config)
	if err != nil {
		return nil, fmt.Errorf("fetch raw transcript: %w", err)
	}
	smsh, err := ProcessTranscript(raw)
	if err != nil {
		return nil, fmt.Errorf("process transcript for %s: %w", videoID, err)
	}
	return &TranscriptResult{
		Info:     GetInfo(raw),
		Smooshed: smsh,
	}, nil
}

// selectTrack picks the best caption track for the given language code.
// Preference: exact non-generated match → exact generated match → prefix match → first available.
func selectTrack(tracks []captionTrack, lang string) (captionTrack, error) {
	if len(tracks) == 0 {
		return captionTrack{}, fmt.Errorf("no caption tracks available")
	}

	for _, t := range tracks {
		if t.LanguageCode == lang && !t.IsGenerated {
			return t, nil
		}
	}
	for _, t := range tracks {
		if t.LanguageCode == lang {
			return t, nil
		}
	}
	for _, t := range tracks {
		if strings.HasPrefix(t.LanguageCode, lang) {
			return t, nil
		}
	}
	return tracks[0], nil
}

// extractVideoID extracts the 11-character video ID from a URL or returns the
// ID directly if it is already in that form.
func extractVideoID(input string) (string, error) {
	if len(input) == 11 && !strings.ContainsAny(input, "/?") {
		return input, nil
	}
	if idx := strings.Index(input, "v="); idx != -1 {
		id := input[idx+2:]
		if len(id) >= 11 {
			id = id[:11]
		}
		if i := strings.IndexAny(id, "&?#"); i != -1 {
			id = id[:i]
		}
		if len(id) == 11 {
			return id, nil
		}
	}
	if i := strings.Index(input, "youtu.be/"); i != -1 {
		id := input[i+9:]
		if j := strings.IndexAny(id, "?&#/"); j != -1 {
			id = id[:j]
		}
		if len(id) == 11 {
			return id, nil
		}
	}
	return "", fmt.Errorf("unable to extract video ID from: %s", input)
}
