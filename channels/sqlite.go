package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	yttranscript "github.com/paulstuart/yt-transcript"
	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

const schema = `
CREATE TABLE IF NOT EXISTS channels (
    id         TEXT PRIMARY KEY,
    handle     TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS videos (
    id             TEXT PRIMARY KEY,
    channel_id     TEXT NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    url            TEXT NOT NULL DEFAULT '',
    published_at   DATETIME,
    published_text TEXT NOT NULL DEFAULT '',
    duration       TEXT NOT NULL DEFAULT '',
    view_count     TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (channel_id) REFERENCES channels(id)
);

CREATE INDEX IF NOT EXISTS idx_videos_channel ON videos(channel_id, published_at DESC);

CREATE TABLE IF NOT EXISTS transcripts (
    video_id     TEXT PRIMARY KEY,
    lang         TEXT NOT NULL DEFAULT '',
    is_generated INTEGER NOT NULL DEFAULT 0,
    text         TEXT NOT NULL DEFAULT '',
    lines        TEXT NOT NULL DEFAULT '[]',
    fetched_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (video_id) REFERENCES videos(id)
);
`

// SQLiteRepo is a SQLite-backed Repository.
type SQLiteRepo struct {
	db *sql.DB
}

// NewSQLiteRepo opens (or creates) a SQLite database at dsn and applies the
// schema. Use ":memory:" for an ephemeral in-process database.
func NewSQLiteRepo(dsn string) (*SQLiteRepo, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &SQLiteRepo{db: db}, nil
}

// Close closes the underlying database connection.
func (r *SQLiteRepo) Close() error {
	return r.db.Close()
}

// UpsertChannel inserts or updates a channel record.
func (r *SQLiteRepo) UpsertChannel(ctx context.Context, ch Channel) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO channels (id, handle, name, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			handle     = excluded.handle,
			name       = excluded.name,
			updated_at = CURRENT_TIMESTAMP
	`, ch.ID, ch.Handle, ch.Name)
	return err
}

// GetChannel returns a channel by its UCxxx ID.
func (r *SQLiteRepo) GetChannel(ctx context.Context, channelID string) (Channel, error) {
	var ch Channel
	err := r.db.QueryRowContext(ctx,
		`SELECT id, handle, name FROM channels WHERE id = ?`, channelID,
	).Scan(&ch.ID, &ch.Handle, &ch.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return ch, err
}

// FindChannelByHandle returns a channel matching handle or name (case-insensitive).
func (r *SQLiteRepo) FindChannelByHandle(ctx context.Context, handle string) (Channel, error) {
	// Normalise: strip leading @ for comparison but search both forms.
	normalized := strings.TrimPrefix(handle, "@")
	var ch Channel
	err := r.db.QueryRowContext(ctx, `
		SELECT id, handle, name FROM channels
		WHERE LOWER(TRIM(handle, '@')) = LOWER(?)
		   OR LOWER(name)              = LOWER(?)
		LIMIT 1
	`, normalized, normalized).Scan(&ch.ID, &ch.Handle, &ch.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return ch, err
}

// UpsertVideos inserts or updates video records.
func (r *SQLiteRepo) UpsertVideos(ctx context.Context, videos []Video) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO videos (id, channel_id, title, url, published_at, published_text, duration, view_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title          = excluded.title,
			url            = excluded.url,
			published_at   = excluded.published_at,
			published_text = excluded.published_text,
			duration       = excluded.duration,
			view_count     = excluded.view_count
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, v := range videos {
		var publishedAt any
		if !v.PublishedAt.IsZero() {
			publishedAt = v.PublishedAt.UTC()
		}
		if _, err := stmt.ExecContext(ctx, v.ID, v.ChannelID, v.Title, v.URL,
			publishedAt, v.PublishedText, v.Duration, v.ViewCount); err != nil {
			return fmt.Errorf("upsert video %s: %w", v.ID, err)
		}
	}
	return tx.Commit()
}

// ListVideos returns all videos for a channel, ordered by published_at desc
// (nulls last), then by rowid desc as a stable secondary sort.
func (r *SQLiteRepo) ListVideos(ctx context.Context, channelID string) ([]Video, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, channel_id, title, url,
		       COALESCE(published_at, ''), published_text, duration, view_count
		FROM videos
		WHERE channel_id = ?
		ORDER BY COALESCE(published_at, '0000-01-01') DESC, rowid DESC
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var publishedAt string
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.Title, &v.URL,
			&publishedAt, &v.PublishedText, &v.Duration, &v.ViewCount); err != nil {
			return nil, err
		}
		if publishedAt != "" && publishedAt != "0000-01-01" {
			_ = v.PublishedAt.UnmarshalText([]byte(publishedAt))
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

// GetVideo returns a single video by ID.
func (r *SQLiteRepo) GetVideo(ctx context.Context, videoID string) (Video, error) {
	var v Video
	var publishedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, channel_id, title, url,
		       COALESCE(published_at, ''), published_text, duration, view_count
		FROM videos WHERE id = ?
	`, videoID).Scan(&v.ID, &v.ChannelID, &v.Title, &v.URL,
		&publishedAt, &v.PublishedText, &v.Duration, &v.ViewCount)
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrNotFound
	}
	if err != nil {
		return Video{}, err
	}
	if publishedAt != "" {
		_ = v.PublishedAt.UnmarshalText([]byte(publishedAt))
	}
	return v, nil
}

// ListVideosMissingTranscripts returns up to limit videos for a channel that
// have no transcript row yet. Pass limit <= 0 for all.
func (r *SQLiteRepo) ListVideosMissingTranscripts(ctx context.Context, channelID string, limit int) ([]Video, error) {
	q := `
		SELECT v.id, v.channel_id, v.title, v.url,
		       COALESCE(v.published_at, ''), v.published_text, v.duration, v.view_count
		FROM videos v
		LEFT JOIN transcripts t ON t.video_id = v.id
		WHERE v.channel_id = ? AND t.video_id IS NULL
		ORDER BY COALESCE(v.published_at, '0000-01-01') DESC, v.rowid DESC
	`
	args := []any{channelID}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var publishedAt string
		if err := rows.Scan(&v.ID, &v.ChannelID, &v.Title, &v.URL,
			&publishedAt, &v.PublishedText, &v.Duration, &v.ViewCount); err != nil {
			return nil, err
		}
		if publishedAt != "" && publishedAt != "0000-01-01" {
			_ = v.PublishedAt.UnmarshalText([]byte(publishedAt))
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

// UpsertTranscript inserts or updates a transcript record.
func (r *SQLiteRepo) UpsertTranscript(ctx context.Context, t Transcript) error {
	linesJSON, err := json.Marshal(t.Lines)
	if err != nil {
		return fmt.Errorf("marshal transcript lines: %w", err)
	}
	fetchedAt := t.FetchedAt
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO transcripts (video_id, lang, is_generated, text, lines, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(video_id) DO UPDATE SET
			lang         = excluded.lang,
			is_generated = excluded.is_generated,
			text         = excluded.text,
			lines        = excluded.lines,
			fetched_at   = excluded.fetched_at
	`, t.VideoID, t.Lang, t.IsGenerated, t.Text, string(linesJSON), fetchedAt)
	return err
}

// GetTranscript returns the transcript for a video.
func (r *SQLiteRepo) GetTranscript(ctx context.Context, videoID string) (Transcript, error) {
	var t Transcript
	var linesJSON string
	var fetchedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT video_id, lang, is_generated, text, lines, fetched_at
		FROM transcripts WHERE video_id = ?
	`, videoID).Scan(&t.VideoID, &t.Lang, &t.IsGenerated, &t.Text, &linesJSON, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Transcript{}, ErrNotFound
	}
	if err != nil {
		return Transcript{}, err
	}
	if err := json.Unmarshal([]byte(linesJSON), &t.Lines); err != nil {
		return Transcript{}, fmt.Errorf("unmarshal transcript lines: %w", err)
	}
	if fetchedAt != "" {
		_ = t.FetchedAt.UnmarshalText([]byte(fetchedAt))
	}
	return t, nil
}

// TranscriptFromRaw builds a Transcript from a yttranscript.TranscriptRaw and
// its smooshed text. The smooshed text is stored in Text for full-text use;
// the original timed lines are preserved in Lines.
func TranscriptFromRaw(raw *yttranscript.TranscriptRaw, smooshed string) Transcript {
	return Transcript{
		VideoID:     raw.VideoID,
		Lang:        raw.LanguageCode,
		IsGenerated: raw.IsGenerated,
		Text:        smooshed,
		Lines:       raw.Lines,
		FetchedAt:   time.Now().UTC(),
	}
}
