# Plan: yt-channel — Channel Vacuum

## Purpose

Monitor one or more YouTube channels and ensure every video's transcript is captured into the local transcript store. This is the ingestion layer — it feeds raw transcripts to `yt-digest`.

## Repo

`github.com/paulstuart/yt-channel`

## Inputs and Outputs

**Input:**

- One or more YouTube channel IDs or URLs (configured via file or CLI)
- The local transcript store path (SQLite or directory)

**Output:**

- For each video: video metadata + raw transcript stored locally
- A record of which videos have been seen and processed (deduplication state)

## Core Features

- List all videos for a channel (most recent first, paginated)
- Check which video IDs are already in the store
- Fetch transcripts for any new/unseen videos using `yt-transcript` as a library
- Store results with metadata: video ID, title, channel, publish date, language
- Run on demand or on a cron-style schedule
- Support multiple channels from a config file

## Technical Approach

### Channel Video Discovery

YouTube does not have a public API for listing channel videos without an API key. Two options:

1. **YouTube Data API v3** — requires a Google API key, quota-limited (preferred for reliability)
2. **Page scraping** — parse the channel's `/videos` page HTML (fragile but no key needed)

Start with the Data API v3 approach. Accept the API key via env var (`YOUTUBE_API_KEY`) or config.

### Storage

SQLite database with two tables:

```sql
CREATE TABLE videos (
    video_id    TEXT PRIMARY KEY,
    channel_id  TEXT NOT NULL,
    title       TEXT,
    published   DATETIME,
    fetched     DATETIME,
    lang        TEXT,
    status      TEXT  -- 'pending', 'fetched', 'failed', 'no_transcript'
);

CREATE TABLE transcripts (
    video_id    TEXT PRIMARY KEY REFERENCES videos(video_id),
    raw_json    TEXT,  -- JSON-encoded transcript lines
    fetched_at  DATETIME
);
```

### Processing Loop

1. For each configured channel, fetch the video list page by page
2. For each video ID not yet in `videos` table, insert with `status='pending'`
3. For each `status='pending'` video, call `yt-transcript` to fetch the transcript
4. On success: store in `transcripts`, update `status='fetched'`
5. On failure: update `status='failed'` with error note; retry on next run

## CLI Interface

```
ytchannel [flags]

Flags:
  -config string    Config file path (default: ~/.ytchannel.yaml)
  -channel string   Channel ID or URL (overrides config)
  -db string        Path to SQLite database (default: ./ytchannel.db)
  -lang string      Language code for transcripts (default: en)
  -limit int        Max new videos to process per run (default: 0 = unlimited)
  -dry-run          List pending videos without fetching
  -verbose          Print progress to stderr
```

## Config File Format (YAML)

```yaml
db: /path/to/ytchannel.db
lang: en
channels:
  - id: UCxxxxxxxxxxxxxxxxxxxxxx
    name: "Channel Name"
  - id: UCyyyyyyyyyyyyyyyyyyyyyy
    name: "Another Channel"
```

## Dependencies

- `github.com/paulstuart/yt-transcript` — transcript fetching
- `modernc.org/sqlite` or `mattn/go-sqlite3` — SQLite driver
- Google YouTube Data API v3 (REST, no SDK needed)

## Success Criteria

- Running `ytchannel` fetches all new videos from configured channels
- Already-fetched videos are skipped (idempotent)
- Failed fetches are recorded and retried on the next run
- Output is compatible with `yt-digest` input format
