package channels

import (
	"context"
	"time"
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

// Repository defines storage operations for channel videos.
type Repository interface {
	// UpsertChannel inserts or updates a channel record.
	UpsertChannel(ctx context.Context, ch Channel) error

	// UpsertVideos inserts or updates video records.
	UpsertVideos(ctx context.Context, videos []Video) error

	// ListVideos returns all videos for a channel, ordered by published_at desc.
	ListVideos(ctx context.Context, channelID string) ([]Video, error)

	// GetVideo returns a single video by ID, or ErrNotFound if absent.
	GetVideo(ctx context.Context, videoID string) (Video, error)

	// Close releases any held resources.
	Close() error
}
