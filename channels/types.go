package channels

import (
	"context"
	"time"

	yttranscript "github.com/paulstuart/yt-transcript"
)

// Video holds the metadata for a single YouTube video.
type Video struct {
	ID            string
	ChannelID     string
	Title         string
	URL           string
	PublishedAt   time.Time // zero if not available from channel listing
	PublishedText string    // raw relative text from YT, e.g. "2 years ago"
	Duration      string    // e.g. "10:23"
	ViewCount     string    // raw text, e.g. "1,234,567 views"
}

// Channel holds the metadata for a YouTube channel.
type Channel struct {
	ID     string
	Handle string
	Name   string
}

// Transcript holds the full transcript for a video.
type Transcript struct {
	VideoID     string
	Lang        string
	IsGenerated bool
	Text        string                    // plain joined text (smooshed)
	Lines       []yttranscript.TranscriptLine // timestamped segments
	FetchedAt   time.Time
}

// Repository defines storage operations for channel videos and transcripts.
type Repository interface {
	// UpsertChannel inserts or updates a channel record.
	UpsertChannel(ctx context.Context, ch Channel) error

	// UpsertVideos inserts or updates video records.
	UpsertVideos(ctx context.Context, videos []Video) error

	// ListVideos returns all videos for a channel, ordered by published_at desc.
	ListVideos(ctx context.Context, channelID string) ([]Video, error)

	// GetVideo returns a single video by ID, or ErrNotFound if absent.
	GetVideo(ctx context.Context, videoID string) (Video, error)

	// GetChannel returns a channel by its UCxxx ID, or ErrNotFound if absent.
	GetChannel(ctx context.Context, channelID string) (Channel, error)

	// FindChannelByHandle returns a channel matching handle or name (case-insensitive),
	// or ErrNotFound if absent.
	FindChannelByHandle(ctx context.Context, handle string) (Channel, error)

	// UpsertTranscript inserts or updates a transcript record.
	UpsertTranscript(ctx context.Context, t Transcript) error

	// GetTranscript returns the transcript for a video, or ErrNotFound if absent.
	GetTranscript(ctx context.Context, videoID string) (Transcript, error)

	// ListVideosMissingTranscripts returns up to limit videos for channelID
	// that have no transcript row yet.
	ListVideosMissingTranscripts(ctx context.Context, channelID string, limit int) ([]Video, error)

	// Close releases any held resources.
	Close() error
}
