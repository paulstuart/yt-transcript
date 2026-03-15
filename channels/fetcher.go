package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	browseURL     = "https://www.youtube.com/youtubei/v1/browse"
	webClientName = "WEB"
	webClientVer  = "2.20240726.00.00"
)

var (
	apiKeyRe    = regexp.MustCompile(`"INNERTUBE_API_KEY":\s*"([a-zA-Z0-9_-]+)"`)
	channelIDRe = regexp.MustCompile(`"channelId"\s*:\s*"(UC[^"]+)"`)

	defaultHTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}
)

// Fetcher retrieves video listings for YouTube channels.
type Fetcher struct {
	httpClient *http.Client
}

// NewFetcher creates a Fetcher with the default HTTP client.
func NewFetcher() *Fetcher {
	return &Fetcher{httpClient: defaultHTTPClient}
}

// NewFetcherWithClient creates a Fetcher using a custom HTTP client.
func NewFetcherWithClient(c *http.Client) *Fetcher {
	return &Fetcher{httpClient: c}
}

// FetchChannel fetches the Channel metadata and all video listings for the
// given channel reference. channelRef may be:
//   - a full YouTube URL (https://www.youtube.com/@handle or /channel/UCxxx)
//   - a channel ID starting with "UC"
//   - a handle starting with "@"
//
// Note: YouTube's channel browse response only provides relative publish times
// ("2 years ago"). PublishedAt will be zero; PublishedText has the raw string.
func (f *Fetcher) FetchChannel(ctx context.Context, channelRef string) (Channel, []Video, error) {
	pageURL := normalizeChannelURL(channelRef)

	pageHTML, err := f.fetchPage(ctx, pageURL)
	if err != nil {
		return Channel{}, nil, fmt.Errorf("fetch channel page: %w", err)
	}

	apiKey, _ := extractAPIKey(pageHTML) // optional; browse works without it

	initialData, err := extractYTInitialData(pageHTML)
	if err != nil {
		return Channel{}, nil, fmt.Errorf("extract ytInitialData: %w", err)
	}

	ch, videos, token, err := parseChannelPage(initialData)
	if err != nil {
		return Channel{}, nil, fmt.Errorf("parse channel page: %w", err)
	}

	// Fill in channel ID from page HTML if not found in initialData
	if ch.ID == "" {
		ch.ID = extractChannelIDFromHTML(pageHTML)
	}

	// Paginate through all continuation pages
	for token != "" {
		more, nextToken, err := f.fetchContinuation(ctx, apiKey, token)
		if err != nil {
			return ch, videos, fmt.Errorf("fetch continuation: %w", err)
		}
		videos = append(videos, more...)
		token = nextToken
	}

	// Backfill channel ID on all videos
	for i := range videos {
		videos[i].ChannelID = ch.ID
		videos[i].URL = "https://www.youtube.com/watch?v=" + videos[i].ID
	}

	return ch, videos, nil
}

// fetchPage performs a GET request and returns the response body as a string.
func (f *Fetcher) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// fetchContinuation calls the Innertube browse API with a continuation token.
func (f *Fetcher) fetchContinuation(ctx context.Context, apiKey, token string) ([]Video, string, error) {
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    webClientName,
				"clientVersion": webClientVer,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"continuation": token,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}

	u := browseURL
	if apiKey != "" {
		u += "?key=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("browse continuation HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	var data browseResponse
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, "", fmt.Errorf("parse browse continuation: %w", err)
	}

	for _, action := range data.OnResponseReceivedActions {
		items := action.AppendContinuationItemsAction.ContinuationItems
		videos, next := parseRichItems(items)
		return videos, next, nil
	}
	return nil, "", nil
}

// normalizeChannelURL converts any channel reference to a /videos page URL.
func normalizeChannelURL(ref string) string {
	ref = strings.TrimSpace(ref)

	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		u, err := url.Parse(ref)
		if err == nil {
			u.Path = strings.TrimSuffix(u.Path, "/") + "/videos"
			u.RawQuery = ""
			return u.String()
		}
	}

	// Channel ID (UCxxxxxxxxxxxxxxxxxxxxxxxx — 24 chars starting with UC)
	if strings.HasPrefix(ref, "UC") {
		return "https://www.youtube.com/channel/" + ref + "/videos"
	}

	// Handle (@name)
	if strings.HasPrefix(ref, "@") {
		return "https://www.youtube.com/" + ref + "/videos"
	}

	// Fall back: treat as channel ID
	return "https://www.youtube.com/channel/" + ref + "/videos"
}

// extractAPIKey extracts the Innertube API key from page HTML.
func extractAPIKey(html string) (string, error) {
	m := apiKeyRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return "", errors.New("INNERTUBE_API_KEY not found")
	}
	return m[1], nil
}

// extractChannelIDFromHTML looks for a channelId in page HTML.
func extractChannelIDFromHTML(html string) string {
	m := channelIDRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// extractYTInitialData extracts the ytInitialData JSON object from page HTML.
func extractYTInitialData(pageHTML string) (map[string]json.RawMessage, error) {
	// The regex above is too greedy for nested braces; use index-based extraction.
	const marker1 = "var ytInitialData = "
	const marker2 = `window["ytInitialData"] = `

	start := strings.Index(pageHTML, marker1)
	if start == -1 {
		start = strings.Index(pageHTML, marker2)
		if start == -1 {
			return nil, errors.New("ytInitialData not found in page")
		}
		start += len(marker2)
	} else {
		start += len(marker1)
	}

	// Find the matching closing brace
	jsonStr, err := extractJSON(pageHTML[start:])
	if err != nil {
		return nil, fmt.Errorf("extract ytInitialData JSON: %w", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse ytInitialData: %w", err)
	}
	return result, nil
}

// extractJSON extracts a complete JSON object from a string starting with '{'.
func extractJSON(s string) (string, error) {
	if len(s) == 0 || s[0] != '{' {
		return "", errors.New("expected JSON object")
	}
	depth := 0
	inStr := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inStr {
			escaped = true
			continue
		}
		if ch == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], nil
			}
		}
	}
	return "", errors.New("unterminated JSON object")
}

// --- JSON types for ytInitialData and browse API response ---

type browseResponse struct {
	OnResponseReceivedActions []struct {
		AppendContinuationItemsAction struct {
			ContinuationItems []richItem `json:"continuationItems"`
		} `json:"appendContinuationItemsAction"`
	} `json:"onResponseReceivedActions"`
}

type richItem struct {
	RichItemRenderer         *richItemRenderer         `json:"richItemRenderer"`
	ContinuationItemRenderer *continuationItemRenderer `json:"continuationItemRenderer"`
}

type richItemRenderer struct {
	Content struct {
		VideoRenderer *videoRenderer `json:"videoRenderer"`
	} `json:"content"`
}

type continuationItemRenderer struct {
	ContinuationEndpoint struct {
		ContinuationCommand struct {
			Token string `json:"token"`
		} `json:"continuationCommand"`
	} `json:"continuationEndpoint"`
}

type videoRenderer struct {
	VideoID string `json:"videoId"`
	Title   struct {
		Runs []struct {
			Text string `json:"text"`
		} `json:"runs"`
		AccessibilityData struct {
			AccessibilityData struct {
				Label string `json:"label"`
			} `json:"accessibilityData"`
		} `json:"accessibility"`
	} `json:"title"`
	PublishedTimeText struct {
		SimpleText string `json:"simpleText"`
	} `json:"publishedTimeText"`
	ViewCountText struct {
		SimpleText string `json:"simpleText"`
		Runs       []struct {
			Text string `json:"text"`
		} `json:"runs"`
	} `json:"viewCountText"`
	LengthText struct {
		SimpleText string `json:"simpleText"`
	} `json:"lengthText"`
}

func (v *videoRenderer) title() string {
	if len(v.Title.Runs) > 0 {
		return v.Title.Runs[0].Text
	}
	return v.Title.AccessibilityData.AccessibilityData.Label
}

func (v *videoRenderer) viewCount() string {
	if v.ViewCountText.SimpleText != "" {
		return v.ViewCountText.SimpleText
	}
	var parts []string
	for _, r := range v.ViewCountText.Runs {
		parts = append(parts, r.Text)
	}
	return strings.Join(parts, "")
}

// parseChannelPage extracts channel info, initial videos, and a continuation token
// from the parsed ytInitialData map.
func parseChannelPage(data map[string]json.RawMessage) (Channel, []Video, string, error) {
	var ch Channel

	// metadata.channelMetadataRenderer is the most reliable source.
	// externalId is the UCxxx channel ID; vanityUrl is the @handle.
	if raw, ok := data["metadata"]; ok {
		var meta struct {
			ChannelMetadataRenderer struct {
				ExternalID string `json:"externalId"`
				Title      string `json:"title"`
				VanityURL  string `json:"vanityUrl"`
			} `json:"channelMetadataRenderer"`
		}
		if json.Unmarshal(raw, &meta) == nil {
			ch.ID = meta.ChannelMetadataRenderer.ExternalID
			ch.Name = meta.ChannelMetadataRenderer.Title
			ch.Handle = meta.ChannelMetadataRenderer.VanityURL
		}
	}

	// Fall back to header renderers for any fields still missing.
	if raw, ok := data["header"]; ok {
		var header struct {
			// Classic layout
			C4TabbedHeaderRenderer struct {
				ChannelID       string `json:"channelId"`
				Title           string `json:"title"`
				ChannelHandleText struct {
					Runs []struct{ Text string `json:"text"` } `json:"runs"`
					SimpleText string `json:"simpleText"`
				} `json:"channelHandleText"`
			} `json:"c4TabbedHeaderRenderer"`
			// Newer layout
			PageHeaderRenderer struct {
				PageTitle string `json:"pageTitle"`
			} `json:"pageHeaderRenderer"`
		}
		if json.Unmarshal(raw, &header) == nil {
			c4 := header.C4TabbedHeaderRenderer
			if ch.ID == "" {
				ch.ID = c4.ChannelID
			}
			if ch.Name == "" {
				ch.Name = c4.Title
				if ch.Name == "" {
					ch.Name = header.PageHeaderRenderer.PageTitle
				}
			}
			if ch.Handle == "" {
				if c4.ChannelHandleText.SimpleText != "" {
					ch.Handle = c4.ChannelHandleText.SimpleText
				} else if len(c4.ChannelHandleText.Runs) > 0 {
					ch.Handle = c4.ChannelHandleText.Runs[0].Text
				}
			}
		}
	}

	// Drill into contents → twoColumnBrowseResultsRenderer → tabs → selected tab
	var contents struct {
		TwoColumnBrowseResultsRenderer struct {
			Tabs []struct {
				TabRenderer struct {
					Title    string `json:"title"`
					Selected bool   `json:"selected"`
					Content  struct {
						RichGridRenderer struct {
							Contents []richItem `json:"contents"`
						} `json:"richGridRenderer"`
						SectionListRenderer struct {
							Contents []struct {
								ItemSectionRenderer struct {
									Contents []struct {
										GridRenderer struct {
											Items []struct {
												GridVideoRenderer *videoRenderer `json:"gridVideoRenderer"`
											} `json:"items"`
										} `json:"gridRenderer"`
									} `json:"contents"`
								} `json:"itemSectionRenderer"`
							} `json:"contents"`
						} `json:"sectionListRenderer"`
					} `json:"content"`
				} `json:"tabRenderer"`
			} `json:"tabs"`
		} `json:"twoColumnBrowseResultsRenderer"`
	}
	if raw, ok := data["contents"]; ok {
		if err := json.Unmarshal(raw, &contents); err != nil {
			return ch, nil, "", fmt.Errorf("parse contents: %w", err)
		}
	}

	var videos []Video
	var token string

	for _, tab := range contents.TwoColumnBrowseResultsRenderer.Tabs {
		tr := tab.TabRenderer
		if !tr.Selected {
			continue
		}
		// richGridRenderer path (most common)
		items := tr.Content.RichGridRenderer.Contents
		if len(items) > 0 {
			videos, token = parseRichItems(items)
			break
		}
		// sectionListRenderer / gridRenderer path (some channels)
		for _, section := range tr.Content.SectionListRenderer.Contents {
			for _, inner := range section.ItemSectionRenderer.Contents {
				for _, gi := range inner.GridRenderer.Items {
					if gi.GridVideoRenderer != nil {
						v := gridVideoRendererToVideo(gi.GridVideoRenderer)
						if v.ID != "" {
							videos = append(videos, v)
						}
					}
				}
			}
		}
		break
	}

	return ch, videos, token, nil
}

// parseRichItems converts a slice of richItems into Videos and returns the
// next continuation token (if any).
func parseRichItems(items []richItem) ([]Video, string) {
	var videos []Video
	var token string
	for _, item := range items {
		if item.ContinuationItemRenderer != nil {
			token = item.ContinuationItemRenderer.ContinuationEndpoint.ContinuationCommand.Token
		}
		if item.RichItemRenderer != nil {
			vr := item.RichItemRenderer.Content.VideoRenderer
			if vr != nil {
				v := richVideoRendererToVideo(vr)
				if v.ID != "" {
					videos = append(videos, v)
				}
			}
		}
	}
	return videos, token
}

func richVideoRendererToVideo(vr *videoRenderer) Video {
	return Video{
		ID:            vr.VideoID,
		Title:         vr.title(),
		PublishedText: vr.PublishedTimeText.SimpleText,
		Duration:      vr.LengthText.SimpleText,
		ViewCount:     vr.viewCount(),
	}
}

func gridVideoRendererToVideo(vr *videoRenderer) Video {
	return richVideoRendererToVideo(vr)
}
