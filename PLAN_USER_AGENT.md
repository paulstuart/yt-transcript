# Plan: Replace horiagug dependency with direct fetcher

## Problem

The `github.com/horiagug/youtube-transcript-api-go` dependency does not set a
`User-Agent` header on its HTTP GET requests. YouTube identifies `Go-http-client/2.0`
as a bot and rate-limits it (HTTP 429) quickly, even on the first request of a
batch session.

The library provides a `WithCustomFetcher` option, but the fetcher interface type
(`HTMLFetcherType`) lives in its `internal/repository` package. Go's visibility
rules prevent any code outside the `horiagug` module from importing or implementing
this interface — so we cannot inject a custom fetcher.

## Solution

Remove the `horiagug` dependency entirely and implement the fetching directly in
this package. The public API stays identical; only the internal implementation
changes.

## How YouTube transcript fetching works

Three HTTP requests per video, in order:

1. **GET** `https://www.youtube.com/watch?v={videoID}`
   - Fetches the full page HTML
   - Parse out `INNERTUBE_API_KEY` via regex: `"INNERTUBE_API_KEY":\s*"([a-zA-Z0-9_-]+)"`
   - Also used to detect consent requirement (look for `consent.youtube.com`)

2. **POST** `https://www.youtube.com/youtubei/v1/player?key={apiKey}`
   - Content-Type: `application/json`
   - Body:
     ```json
     {
       "context": {
         "client": {
           "clientName": "WEB",
           "clientVersion": "2.20240101.00.00"
         }
       },
       "videoId": "{videoID}"
     }
     ```
   - Response JSON contains `captions.playerCaptionsTracklistRenderer.captionTracks`
   - Each track has `languageCode`, `baseUrl`, `name.simpleText`, `kind` (optional, "asr" = auto-generated), `isTranslatable`

3. **GET** `{captionTrack.baseUrl}` (strip `&fmt=srv3` if present)
   - Returns XML like:
     ```xml
     <transcript>
       <text start="0.5" dur="2.3">Hello world</text>
       ...
     </transcript>
     ```
   - Parse `start` and `dur` attributes as float64, text content as the line text
   - HTML-decode the text content (YouTube encodes `&amp;`, `&#39;`, etc.)

All GET requests must include:
```
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36
Accept-Language: en-US,en;q=0.9
```

## Current type aliases to replace

In `client.go` today:
```go
type TranscriptRaw  = models.Transcript      // from horiagug models
type TranscriptLine = models.TranscriptLine   // from horiagug models
```

The concrete structs from the horiagug models package are:
```go
// models.Transcript
type TranscriptRaw struct {
    VideoID        string
    VideoTitle     string
    Language       string
    LanguageCode   string
    IsGenerated    bool
    IsTranslatable bool
    Lines          []TranscriptLine
}

// models.TranscriptLine
type TranscriptLine struct {
    Text     string  `json:"text"`
    Start    float64 `json:"start"`
    Duration float64 `json:"duration"`
}
```

## Files to change

### 1. New file: `fetcher.go`

Implement all HTTP fetching and XML parsing. No public symbols — everything
unexported. Package: `yttranscript`.

```go
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
    "regexp"
    "strings"
    "time"
)

const (
    userAgent       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    videoBaseURL    = "https://www.youtube.com/watch?v=%s"
    innertubeURL    = "https://www.youtube.com/youtubei/v1/player"
)

var (
    apiKeyRegex    = regexp.MustCompile(`"INNERTUBE_API_KEY":\s*"([a-zA-Z0-9_-]+)"`)
    httpClient     = &http.Client{
        Timeout: 30 * time.Second,
        Transport: &http.Transport{
            MaxIdleConns:        10,
            MaxIdleConnsPerHost: 5,
            IdleConnTimeout:     90 * time.Second,
        },
    }
)

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

// fetchCaptionTracks calls the Innertube player API and returns available caption tracks.
func fetchCaptionTracks(ctx context.Context, videoID, apiKey string) ([]captionTrack, error) {
    payload := map[string]any{
        "context": map[string]any{
            "client": map[string]any{
                "clientName":    "WEB",
                "clientVersion": "2.20240101.00.00",
            },
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
    req.Header.Set("User-Agent", userAgent)

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

    // Parse the nested JSON
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
func fetchTranscriptXML(ctx context.Context, trackURL string) (string, error) {
    // Strip srv3 format param — plain XML is easier to parse.
    trackURL = strings.Replace(trackURL, "&fmt=srv3", "", 1)

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, trackURL, nil)
    if err != nil {
        return "", fmt.Errorf("create request: %w", err)
    }
    req.Header.Set("User-Agent", userAgent)
    req.Header.Set("Accept-Language", "en-US,en;q=0.9")

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
```

### 2. Modify `types.go`

Replace the type aliases (which depend on horiagug) with concrete struct definitions.

**Remove** from `client.go`:
```go
type TranscriptRaw  = models.Transcript
type TranscriptLine = models.TranscriptLine
```

**Add** to `types.go` (before the `TranscriptInfo` definition):
```go
// TranscriptRaw is the raw transcript data for a video.
type TranscriptRaw struct {
    VideoID        string
    VideoTitle     string
    Language       string
    LanguageCode   string
    IsGenerated    bool
    IsTranslatable bool
    Lines          []TranscriptLine
}

// TranscriptLine is a single timed segment of a transcript.
type TranscriptLine struct {
    Text     string  `json:"text"`
    Start    float64 `json:"start"`
    Duration float64 `json:"duration"`
}
```

Also update `GetInfo` in `types.go` — it currently accesses `transcript.Lines` which still works
since we're defining `Lines []TranscriptLine` on the struct.

### 3. Rewrite `client.go`

The new `client.go` is much simpler — no horiagug imports at all.

```go
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

    tracks, err := fetchCaptionTracks(ctx, vid, apiKey)
    if err != nil {
        return nil, fmt.Errorf("fetch caption tracks for %s: %w", vid, err)
    }

    // Find the best matching track for the requested language.
    track, err := selectTrack(tracks, lang)
    if err != nil {
        return nil, fmt.Errorf("select track for %s lang=%s: %w", vid, lang, err)
    }

    xmlStr, err := fetchTranscriptXML(ctx, track.BaseURL)
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
// Preference: exact language code match → auto-generated (asr) → first available.
func selectTrack(tracks []captionTrack, lang string) (captionTrack, error) {
    if len(tracks) == 0 {
        return captionTrack{}, fmt.Errorf("no caption tracks available")
    }

    // Exact match, prefer non-generated.
    for _, t := range tracks {
        if t.LanguageCode == lang && !t.IsGenerated {
            return t, nil
        }
    }
    // Exact match, auto-generated.
    for _, t := range tracks {
        if t.LanguageCode == lang {
            return t, nil
        }
    }
    // Prefix match (e.g. "en" matches "en-US").
    for _, t := range tracks {
        if strings.HasPrefix(t.LanguageCode, lang) {
            return t, nil
        }
    }
    // Fall back to first available.
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
```

### 4. Modify `go.mod`

Run `go mod tidy` after the code changes to drop `horiagug` and its transitive
dependencies (`golang.org/x/net`, `golang.org/x/sync`, `golang.org/x/sys`,
`golang.org/x/telemetry`, `golang.org/x/tools`, `golang.org/x/mod`).

The `tool` directive for `deadcode` (`golang.org/x/tools/cmd/deadcode`) should
be kept if it was intentional. If `go mod tidy` removes it as unused, re-add:
```
tool golang.org/x/tools/cmd/deadcode
```

## Steps to execute

1. Create `fetcher.go` with the content in section 1 above.
2. Add `TranscriptRaw` and `TranscriptLine` struct definitions to `types.go`.
3. Remove the two type alias lines from `client.go` (`TranscriptRaw = models.Transcript`
   and `TranscriptLine = models.TranscriptLine`) and the horiagug imports.
4. Replace `client.go` content with the new implementation from section 3.
5. Run `go build ./...` — fix any compile errors before proceeding.
6. Run `go mod tidy` to remove the horiagug dependency.
7. Run `go build ./...` again to confirm clean build.
8. Run `go test -run TestExtractVideoID ./...` — all unit tests should pass.
9. Run `go test -run TestFetchTranscript_Integration ./...` to verify live fetching works.
   (This makes real network calls; skip with `-short` if on a rate-limited IP.)

## Acceptance criteria

- `go build ./...` succeeds with no errors
- `go test -run TestExtractVideoID` passes
- `go test -run TestFetchTranscript_Integration` passes (with live network, non-rate-limited IP)
- The `horiagug` module no longer appears in `go.mod` or `go.sum`
- A single `FetchTranscript("dQw4w9WgXcQ", nil)` call returns non-empty `Smooshed.Text`
- No `fmt.Printf` / stdout output from the library during normal operation
  (the horiagug library printed retry messages to stdout; this implementation must not)

## Notes

- The `VideoTitle` field on `TranscriptRaw` is populated by horiagug via an
  HTML page title scrape. In the new implementation it will be empty string —
  `GetInfo` passes it through to `TranscriptInfo.VideoTitle`. If the title is
  needed, extract it from the video page HTML using a `<title>` regex or
  `golang.org/x/net/html` parser (optional enhancement, not required for the fix).
- The `processor_test.go` file tests `ProcessTranscript` which has no horiagug
  dependency — it should continue to pass without changes.
- Do not add retry logic inside the fetcher. Callers (e.g. the batch handler in
  healthweb) are responsible for backoff. The fetcher should fail fast and return
  meaningful errors including the HTTP status code.
