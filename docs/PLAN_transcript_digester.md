# Plan: yt-digest — Transcript Digester

## Purpose

Transform raw transcripts (produced by `yt-channel`) into a searchable knowledge base. For each transcript: generate a summary, extract keywords, and store everything in SQLite with full-text search and vector embeddings. This is the processing and indexing layer.

## Repo

`github.com/paulstuart/yt-digest`

## Inputs and Outputs

**Input:**

- Raw transcript JSON from the `yt-channel` SQLite DB (or stdin/files)
- LLM API credentials for summarization and embedding

**Output:**

- SQLite database with:
  - Raw transcripts
  - Generated summaries
  - Keywords per video
  - FTS5 index over transcript text and summaries
  - Vector embeddings per chunk for semantic search

## Core Features

- Read unprocessed transcripts from the channel DB
- Generate a concise summary per video (via LLM)
- Extract keywords/topics per video (via LLM)
- Chunk transcript text and generate vector embeddings
- Store all of the above in SQLite
- FTS5 index for keyword search
- Vector index for semantic similarity search
- Idempotent: skip already-digested videos

## Technical Approach

### LLM Integration

Use the Claude API (via the Anthropic Go SDK or direct HTTP) for:

- **Summarization**: prompt with full transcript text, return 3–5 sentence summary
- **Keyword extraction**: prompt with transcript, return JSON array of topic keywords

For long transcripts that exceed the context window, chunk the text, summarize chunks, then summarize the summaries.

Accept the API key via `ANTHROPIC_API_KEY` env var.

### Vector Embeddings

Use a local embedding model or the Claude embeddings endpoint to generate vectors for each ~500-token chunk. Store in SQLite using the `sqlite-vec` extension (or equivalent).

Embedding model priority:

1. Local: `nomic-embed-text` via Ollama (no API cost, private)
2. Remote: OpenAI `text-embedding-3-small` (cheap, good quality)

### Database Schema

```sql
-- Summaries and keywords per video
CREATE TABLE digests (
    video_id      TEXT PRIMARY KEY,
    channel_id    TEXT,
    title         TEXT,
    published     DATETIME,
    lang          TEXT,
    summary       TEXT,
    keywords      TEXT,  -- JSON array of strings
    digested_at   DATETIME
);

-- Full transcript text for FTS
CREATE VIRTUAL TABLE transcripts_fts USING fts5(
    video_id UNINDEXED,
    text,
    content='raw_transcripts',
    content_rowid='rowid'
);

-- Chunked embeddings for semantic search
CREATE TABLE embeddings (
    id          INTEGER PRIMARY KEY,
    video_id    TEXT,
    chunk_index INTEGER,
    chunk_text  TEXT,
    embedding   BLOB   -- float32 array
);
```

### Processing Pipeline

1. Query channel DB for videos with `status='fetched'` not yet in `digests`
2. For each video:
   a. Load raw transcript JSON
   b. Join timed segments into plain text
   c. Send to LLM for summary + keywords
   d. Chunk text and generate embeddings
   e. Insert into `digests` and `embeddings`
   f. Rebuild FTS5 index

## CLI Interface

```
ytdigest [flags]

Flags:
  -channel-db string   Path to yt-channel SQLite database (required)
  -digest-db string    Path to digest output database (default: ./ytdigest.db)
  -limit int           Max videos to process per run (default: 0 = unlimited)
  -embeddings          Generate vector embeddings (default: true)
  -model string        LLM model for summarization (default: claude-haiku-4-5)
  -embed-model string  Embedding model/endpoint (default: nomic-embed-text via ollama)
  -dry-run             Show pending videos without processing
  -verbose             Print progress to stderr
```

## Dependencies

- `github.com/paulstuart/yt-channel` — input DB schema reference
- `modernc.org/sqlite` — SQLite driver with FTS5
- `github.com/anthropics/anthropic-sdk-go` — Claude API for summarization
- `sqlite-vec` or equivalent — vector storage in SQLite
- Ollama (optional, local embeddings)

## Success Criteria

- Running `ytdigest` processes all undigested transcripts
- Each video has a human-quality summary and relevant keywords
- Full-text search returns relevant videos for a keyword query
- Semantic search returns relevant videos for a natural-language query
- Re-running is idempotent (no duplicate processing)
