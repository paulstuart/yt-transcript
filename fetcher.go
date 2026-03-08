package yttranscript

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	userAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	videoBaseURL = "https://www.youtube.com/watch?v=%s"
	innertubeURL = "https://www.youtube.com/youtubei/v1/player"
)

var (
	apiKeyRegex      = regexp.MustCompile(`"INNERTUBE_API_KEY":\s*"([a-zA-Z0-9_-]+)"`)
	visitorDataRegex = regexp.MustCompile(`"visitorData":"([^"]+)"`)
	httpClient       = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

// DebugWriter is the destination for HTTP debug logging when YTT_DEBUG=1.
// It defaults to os.Stderr and may be reassigned in main() before any requests are made.
var DebugWriter io.Writer = os.Stderr

func init() {
	if os.Getenv("YTT_DEBUG") == "1" {
		httpClient.Transport = &logTransport{Transport: httpClient.Transport}
	}
}

// logTransport wraps an http.RoundTripper to log requests and responses.
type logTransport struct {
	Transport http.RoundTripper
}

func (l *logTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	dumpReq, _ := httputil.DumpRequestOut(req, true)
	fmt.Fprintf(DebugWriter, "--- Request ---\n%s\n", dumpReq)

	resp, err := l.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	dumpResp, _ := httputil.DumpResponse(resp, true)
	fmt.Fprintf(DebugWriter, "--- Response ---\n%s\n", dumpResp)
	return resp, nil
}

// captionTrack represents a single caption track from the Innertube response.
type captionTrack struct {
	BaseURL        string
	LanguageCode   string
	Name           string
	IsGenerated    bool
	IsTranslatable bool
}

// fetchVideoPage fetches the YouTube video page and returns the HTML body.
func fetchVideoPage(ctx context.Context, videoID string) (string, error) {
	url := fmt.Sprintf(videoBaseURL, videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch page: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read page body: %w", err)
	}
	return string(body), nil
}

// extractInnertubeKey extracts the INNERTUBE_API_KEY from page HTML.
func extractInnertubeKey(pageHTML string) (string, error) {
	m := apiKeyRegex.FindStringSubmatch(pageHTML)
	if len(m) < 2 {
		return "", fmt.Errorf("INNERTUBE_API_KEY not found in page")
	}
	return m[1], nil
}

// extractVisitorData extracts the visitorData session identifier from page HTML.
// Returns empty string if not found; this value is optional.
func extractVisitorData(pageHTML string) string {
	m := visitorDataRegex.FindStringSubmatch(pageHTML)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// fetchCaptionTracks calls the Innertube player API and returns available caption tracks.
// The ANDROID_VR client is used because it returns caption URLs without the exp=xpe
// experiment flag in their sparams. That flag enables po_token enforcement on the
// timedtext server, causing it to silently return an empty 200 response for
// non-browser requests. ANDROID_VR is exempt from this enforcement.
func fetchCaptionTracks(ctx context.Context, videoID, apiKey, visitorData string) ([]captionTrack, error) {
	client := map[string]any{
		"clientName":        "ANDROID_VR",
		"clientVersion":     "1.57.29",
		"deviceMake":        "Oculus",
		"deviceModel":       "Quest 3",
		"androidSdkVersion": 32,
		"osName":            "Android",
		"osVersion":         "12L",
		"hl":                "en",
		"gl":                "US",
	}
	if visitorData != "" {
		client["visitorData"] = visitorData
	}
	payload := map[string]any{
		"context": map[string]any{
			"client": client,
		},
		"videoId": videoID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := innertubeURL
	if apiKey != "" {
		url += "?key=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "com.google.android.apps.youtube.vr.oculus/1.57.29 (Linux; U; Android 12L; eureka-user Build/SP2A.210812.015) gzip")
	req.Header.Set("X-YouTube-Client-Name", "56")
	req.Header.Set("X-YouTube-Client-Version", "1.57.29")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("innertube request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("innertube request: HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read innertube response: %w", err)
	}

	var data struct {
		Captions struct {
			PlayerCaptionsTracklistRenderer struct {
				CaptionTracks []struct {
					BaseURL      string `json:"baseUrl"`
					LanguageCode string `json:"languageCode"`
					Name         struct {
						SimpleText string `json:"simpleText"`
					} `json:"name"`
					Kind           string `json:"kind"`
					IsTranslatable bool   `json:"isTranslatable"`
				} `json:"captionTracks"`
			} `json:"playerCaptionsTracklistRenderer"`
		} `json:"captions"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, fmt.Errorf("parse innertube response: %w", err)
	}

	raw := data.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks
	if len(raw) == 0 {
		return nil, fmt.Errorf("no caption tracks found for video %s", videoID)
	}

	tracks := make([]captionTrack, len(raw))
	for i, t := range raw {
		tracks[i] = captionTrack{
			BaseURL:        t.BaseURL,
			LanguageCode:   t.LanguageCode,
			Name:           t.Name.SimpleText,
			IsGenerated:    t.Kind == "asr",
			IsTranslatable: t.IsTranslatable,
		}
	}
	return tracks, nil
}

// fetchTranscriptXML fetches the transcript XML for a caption track URL.
func fetchTranscriptXML(ctx context.Context, videoID, trackURL string) (string, error) {
	trackURL = strings.Replace(trackURL, "&fmt=srv3", "", 1)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trackURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", fmt.Sprintf(videoBaseURL, videoID))
	req.Header.Set("Origin", "https://www.youtube.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch transcript XML: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch transcript XML: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read transcript XML: %w", err)
	}
	return string(body), nil
}

// xmlText is used for XML parsing of <text start="..." dur="...">...</text> elements.
type xmlText struct {
	XMLName  xml.Name `xml:"text"`
	Start    float64  `xml:"start,attr"`
	Duration float64  `xml:"dur,attr"`
	Text     string   `xml:",chardata"`
}

// parseTranscriptXML parses YouTube's transcript XML into TranscriptLine entries.
func parseTranscriptXML(xmlStr string) ([]TranscriptLine, error) {
	type transcript struct {
		Texts []xmlText `xml:"text"`
	}

	var t transcript
	if err := xml.Unmarshal([]byte(xmlStr), &t); err != nil {
		return nil, fmt.Errorf("parse transcript XML: %w", err)
	}

	lines := make([]TranscriptLine, 0, len(t.Texts))
	for _, x := range t.Texts {
		text := strings.TrimSpace(html.UnescapeString(x.Text))
		if text == "" {
			continue
		}
		lines = append(lines, TranscriptLine{
			Text:     text,
			Start:    x.Start,
			Duration: x.Duration,
		})
	}
	return lines, nil
}
