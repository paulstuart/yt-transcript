package channels

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
