# yt-transcript

A Go library and CLI tool for fetching and processing YouTube video transcripts.

This is effectively a wrapper around a github.com/horiagug/youtube-transcript-api-go,
with the sole focus on returning text that is immediately useful, as youtube's transcript
data is chunked in a way that make it challenging to work with as a full text document.

## Features

- Fetch YouTube video transcripts by video ID or URL
- Support for multiple languages
- Process transcripts into readable "smooshed" text with time-indexed offsets
- Clean and idiomatic Go API
- CLI tool for quick transcript retrieval

## Installation

### As a Library

```bash
go get github.com/paulstuart/yt-transcript
```

### As a CLI Tool

```bash
go install github.com/paulstuart/yt-transcript/cmd/yttranscript@latest
```

## Usage

### Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/paulstuart/yt-transcript"
)

func main() {
    // Create a new client
    client := yttranscript.NewClient()

    // Fetch transcript (supports video ID or full URL)
    result, err := client.FetchTranscript("dQw4w9WgXcQ", &yttranscript.TranscriptConfig{
        Lang: "en",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Smooshed.Text)
}
```

### CLI Usage

Fetch a transcript in plain text format:

```bash
yttranscript -video dQw4w9WgXcQ
```

Fetch with a different language:

```bash
yttranscript -video dQw4w9WgXcQ -lang es
```

Get smooshed text output:

```bash
yttranscript -video dQw4w9WgXcQ -smoosh
```

Output as JSON:

```bash
yttranscript -video dQw4w9WgXcQ -format json
```

Smooshed output as JSON:

```bash
yttranscript -video dQw4w9WgXcQ -smoosh -format json
```

## API Reference

### Client

#### `NewClient() *Client`

Creates a new YouTube transcript client.

#### `FetchTranscript(videoID string, config *TranscriptConfig) (*TranscriptResult, error)`

Fetches the transcript for a YouTube video. The `videoID` parameter accepts:

- Plain video IDs: `dQw4w9WgXcQ`
- Full URLs: `https://www.youtube.com/watch?v=dQw4w9WgXcQ`
- Short URLs: `https://youtu.be/dQw4w9WgXcQ`

### ProcessTranscript

#### `ProcessTranscript(ctx context.Context, channelID, videoID string, transcript *Transcript) (*Smooshed, error)`

Processes a raw transcript into a "smooshed" text format with an index for time-based navigation. The smooshed format:

- Combines transcript chunks into readable paragraphs
- Maintains a timestamp index for seeking to specific times
- Provides offset information for each text segment

### Types

#### `TranscriptConfig`

- `Lang string` - Language code (e.g., "en", "es", "fr")

#### `TranscriptResult`

- `VideoID string` - YouTube video ID
- `VideoTitle string` - Video title
- `Transcripts []TranscriptResponse` - Transcript segments

#### `TranscriptResponse`

- `Text string` - Transcript text
- `Duration float64` - Segment duration in seconds
- `Offset float64` - Start time in seconds
- `Lang string` - Language code

#### `Smooshed`

- `VideoId string` - YouTube video ID
- `Channel string` - Channel ID
- `Text string` - Combined readable text
- `Index []IndexEntry` - Time-based index

## Testing

Run all tests:

```bash
go test ./...
```

Run tests with integration tests (requires internet):

```bash
go test -v ./...
```

Run only unit tests (skip integration):

```bash
go test -v -short ./...
```

## License

See LICENSE file for details.
