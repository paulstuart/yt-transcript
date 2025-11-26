package yttranscript

import (
	"fmt"
	"strings"
)

// ProcessTranscript converts Youtube's chunky transcript into a smooshed text version with an index.
func ProcessTranscript(transcript *TranscriptRaw) (*Smooshed, error) {
	// Check if transcript is empty (no captions available)
	if len(transcript.Lines) == 0 {
		return nil, fmt.Errorf("no captions available for this video")
	}

	// Create smooshed text version and index
	var textBuilder strings.Builder
	var index = make([]IndexEntry, 0, len(transcript.Lines))

	currentOffset := 0
	var prevEntry *TranscriptLine
	sentenceCount := 0 // Track sentences for paragraph breaks

	for i, entry := range transcript.Lines {
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}

		// Add space or paragraph break between entries
		if currentOffset > 0 {
			addParagraphBreak := false

			// Check if previous entry ended with sentence-ending punctuation
			// Use trimmed text since original may have trailing whitespace
			if prevEntry != nil {
				prevTrimmed := strings.TrimSpace(prevEntry.Text)
				if strings.HasSuffix(prevTrimmed, ".") ||
					strings.HasSuffix(prevTrimmed, "?") ||
					strings.HasSuffix(prevTrimmed, "!") {

					sentenceCount++

					// Strategy 1: Check for timing gaps (rare but meaningful when they exist)
					prevEnd := prevEntry.Start + prevEntry.Duration
					gap := entry.Start - prevEnd
					if gap < 0 {
						gap = 0
					}

					// If there's a gap > 0.5 seconds, treat as paragraph break
					if gap > 0.5 {
						addParagraphBreak = true
					} else if sentenceCount >= 2 {
						// Strategy 2: Add paragraph break every 2-3 sentences
						// This creates readable paragraphs even without timing gaps
						addParagraphBreak = true
						sentenceCount = 0
					}
				}
			}

			if addParagraphBreak {
				textBuilder.WriteString("\n\n")
				currentOffset += 2
			} else {
				textBuilder.WriteString(" ")
				currentOffset++
			}
		}

		// Record index entry
		index = append(index, IndexEntry{
			Timestamp: entry.Start,
			Duration:  entry.Duration,
			Offset:    currentOffset,
		})

		// Write text
		textBuilder.WriteString(text)
		currentOffset += len(text)

		prevEntry = &transcript.Lines[i]
	}

	smsh := &Smooshed{
		Indexes: index,
		Text:    textBuilder.String(),
	}

	return smsh, nil
}
