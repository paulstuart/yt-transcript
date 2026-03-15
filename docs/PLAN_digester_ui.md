# Plan: yt-ui — Digester UI

## Purpose

Provide a browser-based interface for exploring the digest database built by `yt-digest`. The UI is the human-facing layer for searching, browsing, and reading the indexed knowledge base.

## Repo

`github.com/paulstuart/yt-ui`

## Inputs and Outputs

**Input:**

- `yt-digest` SQLite database (read-only)

**Output:**

- Local HTTP server serving a web UI

## Core Features

- Full-text search across all transcripts and summaries
- Semantic (vector) search via natural language query
- Filter results by channel, date range, and keyword/topic
- Video card view: title, channel, publish date, summary, keyword tags
- Detail view: full summary, keyword list, full transcript text with timestamps
- Pagination for large result sets
- No login required — local-only, single-user

## Technical Approach

### Server

Minimal Go HTTP server. No framework required — use `net/http` with a small router (e.g., `chi` or plain `ServeMux`). Serve HTML with server-side rendering using Go templates.

Keep JavaScript minimal — use plain HTML forms for search and filter. Avoid a JS build pipeline.

### Search

- **Keyword search**: SQL `WHERE transcripts_fts MATCH ?` via FTS5 BM25 ranking
- **Semantic search**: compute query embedding, rank by cosine similarity against stored vectors, retrieve top N video IDs
- Results merged and de-duplicated, sorted by relevance score

### Pages

```
GET /                   Home: recent videos, search bar
GET /search?q=...       Search results (keyword or semantic)
GET /video/:id          Detail: summary, keywords, full transcript
GET /channel/:id        All videos for a channel
GET /channels           Channel list with video counts
```

### Templates

Server-side Go templates (`.html` files embedded via `embed.FS`). No separate build step.

Minimal styling: plain CSS, no framework. Readable, functional, low-maintenance.

## CLI Interface

```
ytui [flags]

Flags:
  -db string       Path to yt-digest SQLite database (required)
  -addr string     Listen address (default: localhost:8080)
  -open            Open browser on startup
```

## Dependencies

- `modernc.org/sqlite` — SQLite driver
- Go standard library (`net/http`, `html/template`, `embed`)
- `github.com/go-chi/chi` (optional, lightweight router)

## Non-Goals

- User accounts or authentication (local tool, not a web service)
- Video playback (link out to YouTube instead)
- Mobile-optimized design (desktop-first is fine)
- Real-time updates (refresh manually or on a timer)

## Success Criteria

- Searching "insulin resistance" returns relevant videos with ranked results
- Clicking a video shows its summary, keywords, and full transcript
- Filtering by channel narrows results correctly
- Server starts in under one second with no external dependencies
- No JavaScript build step required
