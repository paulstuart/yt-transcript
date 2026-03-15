package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	yttranscript "github.com/paulstuart/yt-transcript"
	"github.com/paulstuart/yt-transcript/channels"
)

func main() {
	var (
		dbPath    string
		logLevel  string
		logFile   string
		batchSize int
		delay     time.Duration
		jitter    time.Duration
		lang      string
		timeout   int
	)

	flag.StringVar(&dbPath, "db", "channels.db", "path to SQLite database")
	flag.StringVar(&logLevel, "log", "info", "log level: debug, info, warn, error")
	flag.StringVar(&logFile, "logfile", "", "log output file (default: stderr)")
	flag.IntVar(&batchSize, "batch", 10, "number of transcripts to fetch per run")
	flag.DurationVar(&delay, "delay", 10*time.Second, "base delay between fetches")
	flag.DurationVar(&jitter, "jitter", 3*time.Second, "maximum random jitter added to delay")
	flag.StringVar(&lang, "lang", "en", "preferred transcript language")
	flag.IntVar(&timeout, "timeout", 30, "per-fetch timeout in seconds")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: transcripts [flags] <channel>\n\n")
		fmt.Fprintf(os.Stderr, "  <channel>  channel ID (UCxxx), handle (@name), channel name, or URL\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	channelRef := flag.Arg(0)

	// --- logging ---
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level %q: %v\n", logLevel, err)
		os.Exit(1)
	}
	logW := os.Stderr
	if logFile != "" {
		f, err := os.Create(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		logW = f
	}
	logger := slog.New(slog.NewTextHandler(logW, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// --- repo ---
	repo, err := channels.NewSQLiteRepo(dbPath)
	if err != nil {
		slog.Error("open database", "db", dbPath, "err", err)
		os.Exit(1)
	}
	defer repo.Close()
	slog.Debug("database opened", "db", dbPath)

	// --- interrupt ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- resolve channel ID from DB ---
	channelID, err := resolveChannelID(ctx, repo, channelRef)
	if err != nil {
		slog.Error("resolve channel", "ref", channelRef, "err", err)
		os.Exit(1)
	}
	slog.Debug("resolved channel", "id", channelID)

	// --- find pending videos ---
	pending, err := repo.ListVideosMissingTranscripts(ctx, channelID, batchSize)
	if err != nil {
		slog.Error("list pending videos", "err", err)
		os.Exit(1)
	}
	if len(pending) == 0 {
		slog.Info("no pending transcripts", "channel", channelID)
		return
	}
	slog.Info("fetching transcripts", "pending", len(pending), "batch", batchSize,
		"delay", delay, "jitter", jitter)

	// --- fetch loop ---
	client := yttranscript.NewClient(timeout)
	config := &yttranscript.TranscriptConfig{Lang: lang}

	succeeded, failed := 0, 0
	for i, video := range pending {
		if ctx.Err() != nil {
			slog.Info("interrupted", "fetched", succeeded, "failed", failed)
			break
		}

		slog.Info("fetching transcript", "video", video.ID, "title", video.Title,
			"progress", fmt.Sprintf("%d/%d", i+1, len(pending)))

		raw, err := client.FetchRawTranscript(video.ID, config)
		if err != nil {
			slog.Warn("transcript fetch failed", "video", video.ID, "title", video.Title, "err", err)
			failed++
		} else {
			smooshed, err := yttranscript.ProcessTranscript(raw)
			if err != nil {
				slog.Warn("transcript process failed", "video", video.ID, "err", err)
				failed++
			} else {
				t := channels.TranscriptFromRaw(raw, smooshed.Text)
				if err := repo.UpsertTranscript(ctx, t); err != nil {
					slog.Error("save transcript", "video", video.ID, "err", err)
					failed++
				} else {
					slog.Debug("transcript saved", "video", video.ID,
						"lang", t.Lang, "generated", t.IsGenerated,
						"lines", len(t.Lines), "chars", len(t.Text))
					succeeded++
				}
			}
		}

		// Delay between fetches, except after the last one.
		if i < len(pending)-1 {
			wait := delay
			if jitter > 0 {
				wait += time.Duration(rand.Int64N(int64(jitter)))
			}
			slog.Debug("waiting", "duration", wait.Round(time.Millisecond))
			select {
			case <-time.After(wait):
			case <-ctx.Done():
			}
		}
	}

	slog.Info("done", "succeeded", succeeded, "failed", failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// resolveChannelID returns the UCxxx channel ID from any reference format.
// It checks if the ref looks like a channel ID directly, otherwise looks it
// up in the DB by handle or name.
func resolveChannelID(ctx context.Context, repo channels.Repository, ref string) (string, error) {
	// Strip URL down to the meaningful part.
	ref = strings.TrimSpace(ref)
	for _, prefix := range []string{
		"https://www.youtube.com/channel/",
		"http://www.youtube.com/channel/",
	} {
		if after, ok := strings.CutPrefix(ref, prefix); ok {
			ref = strings.SplitN(after, "/", 2)[0]
			break
		}
	}
	for _, prefix := range []string{
		"https://www.youtube.com/",
		"http://www.youtube.com/",
	} {
		if after, ok := strings.CutPrefix(ref, prefix); ok {
			ref = strings.SplitN(after, "/", 2)[0]
			break
		}
	}

	// Looks like a channel ID already.
	if strings.HasPrefix(ref, "UC") && len(ref) >= 20 {
		ch, err := repo.GetChannel(ctx, ref)
		if err == nil {
			return ch.ID, nil
		}
		if !errors.Is(err, channels.ErrNotFound) {
			return "", err
		}
		return "", fmt.Errorf("channel %q not found in database; run 'channels' first to populate it", ref)
	}

	// Handle or name lookup.
	ch, err := repo.FindChannelByHandle(ctx, ref)
	if errors.Is(err, channels.ErrNotFound) {
		return "", fmt.Errorf("channel %q not found in database; run 'channels' first to populate it", ref)
	}
	return ch.ID, err
}
