# Plan: Replace yt-dlp with yt-transcript Library

## Background

Healthweb currently shells out to `yt-dlp` for all YouTube data fetching.
The goal is to eliminate that external dependency and make the binary fully
self-contained by extending the local `github.com/paulstuart/yt-transcript`
library (located at `../yt-transcript`, wired via a `replace` directive in
`go.mod`).

The `yt-transcript` library already handles transcript fetching entirely in
pure Go, including the HTTP client, cookie jar, and YouTube page parsing.
This plan builds on those foundations.

---

## What yt-dlp Is Currently Used For

Five methods in `internal/youtube/scraper.go` shell out to yt-dlp:

| Method | yt-dlp flags | Purpose |
|--------|-------------|---------|
| `GetChannelInfo` | `--dump-json --playlist-items 1` | Channel ID + title |
| `GetChannelVideos` | `--flat-playlist --dump-json --playlist-end 500` | Video list with metadata |
| `GetVideoInfo` | `--dump-json --skip-download` | Single video metadata |
| `GetTranscript` | Multi-step subtitle download | **Already replaced** by yt-transcript |
| `DownloadVideo` | `--format bestvideo+bestaudio` | Video file — **out of scope** |

---

## What the Library Already Has (Build-On Foundations)

- `fetchVideoPage(ctx, videoID)` — fetches `watch?v=` page HTML
- Full HTTP client with browser `User-Agent`, cookie jar, and cookie injection (`WithCookies`)
- `extractCaptionTracksFromPage` — demonstrates the pattern: find a JSON key in the raw HTML, slice the object out, unmarshal
- The `ytInitialPlayerResponse` blob already in the HTML contains `videoDetails` (title, channelId, duration, viewCount, etc.) — this is fetched today but only the captions subtree is used

---

## Part 1: Additions to the yt-transcript Library

### 1a. New Types

Add to `types.go` (or a new `channel.go`):

```go
// VideoInfo holds metadata for a single YouTube video.
type VideoInfo struct {
    ID           string
    Title        string
    Description  string
    ChannelID    string
    ChannelTitle string
    PublishedAt  time.Time     // zero if unavailable from listing
    Duration     time.Duration
    ViewCount    int64
    ThumbnailURL string
}

// ChannelInfo holds metadata for a YouTube channel.
type ChannelInfo struct {
    ID          string
    Title       string
    Description string
    URL         string
}
```

---

### 1b. `GetVideoInfo(videoID string) (*VideoInfo, error)` on `Client`

**No new HTTP code required.** Reuses `c.fetcher.fetchVideoPage(ctx, vid)`.

Parsing:

- Slice `ytInitialPlayerResponse` → `"videoDetails":` … `,"playerConfig"` (same
  pattern as captions extraction)
- Extract: `videoId`, `title`, `shortDescription`, `channelId`, `author`,
  `lengthSeconds`, `viewCount`
- Publish date lives in `ytInitialData` as `"publishDate":"2025-10-09"` — slice
  that separately
- Return `*VideoInfo`

---

### 1c. `GetChannelInfo(channelURL string) (*ChannelInfo, error)` on `Client`

New internal helper needed:

```
fetchChannelPage(ctx context.Context, channelURL string) (string, error)
```

Same implementation as `fetchVideoPage` but hits the channel URL directly
(e.g. `https://www.youtube.com/@Physionic`).

Parsing from `ytInitialData`:

```
"metadata":{"channelMetadataRenderer":{"title":…,"description":…,"externalId":…}}
```

Return `*ChannelInfo`.

---

### 1d. `GetChannelVideos(channelURL string, maxResults int, since time.Time) ([]*VideoInfo, error)` on `Client`

This is the largest addition. Two-phase approach:

#### Phase A — Initial page fetch

- Fetch `channelURL + "/videos"` via `fetchChannelPage`
- Parse `ytInitialData` → locate `richGridRenderer.contents` array
- Each `richItemRenderer.content.videoRenderer` yields:

  | Field in JSON | Mapped to |
  |--------------|-----------|
  | `videoId` | `VideoInfo.ID` |
  | `title.runs[0].text` | `VideoInfo.Title` |
  | `lengthText.simpleText` (`"1:23:45"`) | `VideoInfo.Duration` |
  | `viewCountText.simpleText` (`"1.2M views"`) | `VideoInfo.ViewCount` |
  | `publishedTimeText.simpleText` (`"3 weeks ago"`) | `VideoInfo.PublishedAt` (approximate) |
  | `thumbnail.thumbnails[0].url` | `VideoInfo.ThumbnailURL` |

- Last element of `contents` is a `continuationItemRenderer` carrying a
  `continuationCommand.token` — save this for Phase B

#### Phase B — Pagination via InnerTube browse API

POST to:
```
https://www.youtube.com/youtubei/v1/browse?key=AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8
```

Body:
```json
{
  "continuation": "<token>",
  "context": {
    "client": { "clientName": "WEB", "clientVersion": "2.20240101" }
  }
}
```

Response has the same `richGridRenderer` structure. Parse identically to Phase A.
Loop until:
- `since` early-exit fires (video date is before `since`)
- `maxResults` reached
- No continuation token in response

#### Date parsing — `parseRelativeDate(text string, now time.Time) time.Time`

Convert YouTube's relative strings to approximate `time.Time`:

| Input | Output |
|-------|--------|
| `"3 days ago"` | `now.Add(-3 * 24 * time.Hour)` |
| `"2 weeks ago"` | `now.Add(-14 * 24 * time.Hour)` |
| `"3 months ago"` | `now.AddDate(0, -3, 0)` |
| `"1 year ago"` | `now.AddDate(-1, 0, 0)` |
| anything else | `time.Time{}` (zero) |

> **Accuracy note**: This matches what yt-dlp's `--flat-playlist` mode
> provides. Exact per-video dates require individual video page fetches
> (a future improvement). The early-exit `since` guard already handles
> zero dates safely (skips the comparison when `publishedAt.IsZero()`).

#### Early-exit condition (matches existing scraper behaviour)

```go
if !since.IsZero() && !v.PublishedAt.IsZero() && v.PublishedAt.Before(since) {
    break
}
```

---

## Part 2: Changes to healthweb

### 2a. New `LibraryClient` — `internal/youtube/library.go`

Implements the existing `youtube.Client` interface by delegating to
`yttranscript.Client`:

```go
type LibraryClient struct {
    c *yttranscript.Client
}

func (l *LibraryClient) GetChannelInfo(ctx, url) (*ChannelInfo, error)   // → c.GetChannelInfo
func (l *LibraryClient) GetChannelVideos(ctx, url, max, since) ([]*VideoInfo, error) // → c.GetChannelVideos
func (l *LibraryClient) GetVideoInfo(ctx, videoURL) (*VideoInfo, error)  // → c.GetVideoInfo
func (l *LibraryClient) GetTranscript(ctx, videoID) (*Transcript, error) // → c.FetchRawTranscript (already works)
func (l *LibraryClient) DownloadVideo(ctx, url, path) error              // returns fmt.Errorf("not supported")
func (l *LibraryClient) Close() error                                    // no-op
```

Type mapping (yttranscript types → healthweb youtube types) lives in
`library.go` and is straightforward field copies.

### 2b. Update `internal/youtube/client.go` — `NewClient`

Add a `"library"` case:

```go
case "library", "":
    return NewLibraryClient(cfg)
```

`NewLibraryClient` constructs a `yttranscript.Client` with the configured
timeout and optional cookies from config (already supported by `WithCookies`).

### 2c. Update `config.yaml`

```yaml
youtube:
  downloader: library   # was "yt-dlp"
```

### 2d. `internal/youtube/api.go` — no changes

The stub already satisfies the interface. `LibraryClient` is a drop-in.

---

## Files to Create / Modify

| File | Action | Notes |
|------|--------|-------|
| `../yt-transcript/channel.go` | **Create** | `VideoInfo`, `ChannelInfo` types; `GetVideoInfo`, `GetChannelInfo`, `GetChannelVideos` on `Client` |
| `../yt-transcript/fetcher.go` | **Modify** | Add `fetchChannelPage`, InnerTube browse POST helper |
| `internal/youtube/library.go` | **Create** | `LibraryClient` wrapping `yttranscript.Client` |
| `internal/youtube/client.go` | **Modify** | Add `"library"` case to `NewClient` |
| `config.yaml` | **Modify** | `downloader: library` |
| `internal/youtube/scraper.go` | Keep (unused) | Delete after `library` is confirmed stable |

---

## Risks and Notes

- **InnerTube API key**: `AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8` is
  semi-public and stable but occasionally rotates. Confirm by inspecting a
  live YouTube network request before implementing.

- **Date approximation**: Relative-date parsing is accurate to roughly ±1 day
  for recent videos. This is sufficient for the early-exit `since` comparison
  and is the same accuracy yt-dlp provides in flat-playlist mode. Per-video
  exact dates are a future improvement.

- **Rate limiting**: No new mechanism needed. The existing `WithCookies` option
  on `yttranscript.Client` handles this identically to today.

- **Video download**: `DownloadVideo` intentionally returns an error. If needed
  later, streaming URLs can be extracted from `ytInitialPlayerResponse.streamingData`
  (separate effort, requires format selection and likely ffmpeg for merging).

- **`cmd/yttest`**: The test binary will need its import updated to use
  `LibraryClient` instead of `ScraperClient` after the switch.
