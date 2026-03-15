# Project Goal

The overarching goal is to gather lessons and information shared in video format, condense them to text, summarize them, and index the whole as a kind of personal meta-TL;DR knowledge base.

Many channels contain a wealth of information that one simply doesn't have the time to watch in full. So instead:

- Identify all the videos of each channel of interest
- Gather their transcripts
- Condense each individually for concise readability
- Index the whole — transcripts, summaries, and keywords — to provide a high-level view across all channels: basically a personal assistant for managing information of interest

## Philosophy

Follow the Unix philosophy wherever possible: each unit of functionality should be simple, robust, and easy to reason about. Composing small, single-focused command-line tools into a data pipeline is how you build a powerful and maintainable workflow.

Each part of the pipeline is planned here but will live in its own repo.

## Pipeline Overview

This repo (`yt-transcript`) handles only step one: capturing the transcript from a single YouTube video. The full pipeline requires four additional tools, each in its own repo:

| Step | Repo | Purpose |
| --- | --- | --- |
| 1 | `yt-transcript` (this repo) | Fetch transcript from a single video |
| 2 | `yt-channel` | Vacuum all videos from a channel into the transcript store |
| 3 | `yt-digest` | Summarize, keyword-index, and store transcripts in SQLite |
| 4 | `yt-ui` | Browser UI for searching and browsing the digest |
| 5 | `yt-agent` | Meta agent: personal assistant over the full knowledge base |

## Sub-Projects

### 1. yt-transcript (this repo) — Status: Working

Fetches a transcript from a single YouTube video by ID or URL. Outputs raw timed segments or a processed "smooshed" text block. Pure Go, no external dependencies.

### 2. yt-channel — Channel Vacuum

Monitors all videos of one or more channels and ensures each transcript is captured into the transcript store. Tracks which videos have been seen and processed, handles pagination, and runs on a schedule or on demand.

See: [PLAN_channel_vacuum.md](PLAN_channel_vacuum.md)

### 3. yt-digest — Transcript Digester

Stores all transcripts with associated metadata, generates summaries and keywords for each, and loads everything into a SQLite database with FTS and vector embeddings for further analysis.

See: [PLAN_transcript_digester.md](PLAN_transcript_digester.md)

### 4. yt-ui — Digester UI

A browser-based interface for the digester database. Supports full-text search, filtering by channel, topic, and date, and reading individual summaries or full transcripts.

See: [PLAN_digester_ui.md](PLAN_digester_ui.md)

### 5. yt-agent — Meta Agent

Analyzes all captured content and acts as a personal assistant — surfacing only what the user should be paying attention to, generating personalized digests, and answering questions across the full knowledge base.

See: [PLAN_meta_agent.md](PLAN_meta_agent.md)

## Prior Art

This goal was the original intent of `github.com/paulstuart/healthweb`, which became too unwieldy. This pipeline approach is the restart, designed to stay focused and composable.
