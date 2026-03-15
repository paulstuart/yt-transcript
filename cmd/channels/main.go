package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/paulstuart/yt-transcript/channels"
)

func main() {
	var (
		dbPath   string
		logLevel string
		logFile  string
	)

	flag.StringVar(&dbPath, "db", "channels.db", "path to SQLite database")
	flag.StringVar(&logLevel, "log", "info", "log level: debug, info, warn, error")
	flag.StringVar(&logFile, "logfile", "", "log output file (default: stderr)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: channels [flags] <channel>\n\n")
		fmt.Fprintf(os.Stderr, "  <channel>  YouTube channel ID (UCxxx), handle (@name), or URL\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	channelRef := flag.Arg(0)

	// --- logging setup ---
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

	// --- repository ---
	repo, err := channels.NewSQLiteRepo(dbPath)
	if err != nil {
		slog.Error("open database", "db", dbPath, "err", err)
		os.Exit(1)
	}
	defer repo.Close()
	slog.Debug("database opened", "db", dbPath)

	// --- context with interrupt support ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- fetch ---
	fetcher := channels.NewFetcher()
	slog.Info("fetching channel", "ref", channelRef)

	ch, videos, err := fetcher.FetchChannel(ctx, channelRef)
	if err != nil {
		slog.Error("fetch channel", "ref", channelRef, "err", err)
		os.Exit(1)
	}
	slog.Info("channel fetched", "id", ch.ID, "name", ch.Name, "handle", ch.Handle, "videos", len(videos))

	// --- persist ---
	if ch.ID == "" {
		slog.Error("could not determine channel ID; skipping save")
		os.Exit(1)
	}

	if err := repo.UpsertChannel(ctx, ch); err != nil {
		slog.Error("save channel", "id", ch.ID, "err", err)
		os.Exit(1)
	}
	slog.Debug("channel saved", "id", ch.ID)

	if err := repo.UpsertVideos(ctx, videos); err != nil {
		slog.Error("save videos", "count", len(videos), "err", err)
		os.Exit(1)
	}
	slog.Info("videos saved", "count", len(videos), "db", dbPath)
}
