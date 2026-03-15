# Plan: yt-agent — Meta Agent

## Purpose

Act as a personal assistant over the full knowledge base built by the pipeline. The agent analyzes all captured content and surfaces only what the user should be paying attention to — answering questions, generating digests, and proactively flagging topics of interest.

## Repo

`github.com/paulstuart/yt-agent`

## Inputs and Outputs

**Input:**

- `yt-digest` SQLite database (read-only)
- User query or request (via CLI or chat interface)
- Optional: user preference profile (topics, channels, keywords of interest)

**Output:**

- Natural language answers and summaries
- Personalized digest reports (daily/weekly)
- Lists of recommended videos matching user interests

## Core Features

- **Q&A mode**: ask a question, get an answer grounded in the transcript knowledge base
- **Digest mode**: generate a periodic summary of new content across all channels
- **Recommendation mode**: "what should I watch/read this week based on my interests?"
- **Topic tracking**: monitor for new content on specific topics and alert the user
- **Cross-channel synthesis**: summarize what multiple channels say about the same topic

## Technical Approach

### Architecture: RAG Pipeline

Retrieval-Augmented Generation (RAG):

1. Take user query
2. Embed the query (same model used during digestion)
3. Retrieve top-N relevant transcript chunks by semantic similarity
4. Also retrieve top-N by FTS keyword match
5. Merge and de-duplicate retrieved context
6. Send context + user query to LLM for a grounded answer
7. Return answer with source citations (video title, channel, timestamp)

### LLM

Claude (via Anthropic API). Use `claude-sonnet-4-6` for quality responses.

Model selection:

- Simple queries / digest generation: `claude-haiku-4-5` (fast, cheap)
- Complex synthesis / deep Q&A: `claude-sonnet-4-6`

### User Preference Profile

Simple YAML or TOML config:

```yaml
interests:
  - metabolic health
  - longevity
  - strength training
channels:
  preferred:
    - UCxxxxxxxxxxxxxxxxxxxxxx
digest:
  frequency: weekly
  max_items: 10
```

### Modes of Operation

#### Interactive Chat

```
ytai chat
```

Opens a REPL-style chat session. User types questions; agent retrieves relevant context and answers conversationally. Maintains conversation history within the session.

#### One-Shot Query

```
ytai ask "What do these channels say about seed oils?"
```

Returns a single answer with citations, then exits.

#### Digest Generation

```
ytai digest --since 7d --output digest.md
```

Generates a structured Markdown report of new content from the past week, organized by topic cluster.

#### Topic Alert

```
ytai watch --topic "GLP-1" --since last-run
```

Reports any new videos mentioning the topic since the last check. Suitable for running on a schedule.

## CLI Interface

```
ytai <command> [flags]

Commands:
  chat          Interactive Q&A session
  ask <query>   One-shot question
  digest        Generate a content digest
  watch         Check for new content on a topic
  recommend     Suggest videos based on user profile

Global Flags:
  -db string       Path to yt-digest SQLite database (required)
  -config string   Path to user preference config (default: ~/.ytai.yaml)
  -model string    Claude model to use (default: claude-haiku-4-5)
  -verbose         Show retrieval debug info
```

## Dependencies

- `modernc.org/sqlite` — SQLite driver
- `github.com/anthropics/anthropic-sdk-go` — Claude API
- Embedding model (same as `yt-digest`: Ollama or OpenAI)
- Go standard library

## Non-Goals

- Web UI (handled by `yt-ui`)
- Fetching new transcripts (handled by `yt-channel`)
- Persistent multi-session memory (conversation history is per-session only, for now)
- Fine-tuning or training a custom model

## Success Criteria

- `ytai ask "what are the best sources of protein?"` returns a grounded, cited answer from the knowledge base
- `ytai digest --since 7d` produces a readable weekly report without hallucination
- Answers include citations: video title, channel name, and approximate timestamp
- Works fully offline if using a local embedding model (only LLM calls require internet)
- Adding a new channel and running the pipeline makes that content immediately queryable
