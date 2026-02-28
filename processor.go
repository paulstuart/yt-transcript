package yttranscript

import (
	"fmt"
	"strings"
)

var ParagraphGap = 3.0 // seconds to consider a gap in transcript as a new paragraph

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
	var prevStart float64
	endOfSentence := false
	sentenceCount := 0 // Track sentences for paragraph breaks

	for _, entry := range transcript.Lines {
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}

		// Add space or paragraph break between entries
		if currentOffset > 0 {
			addParagraphBreak := false

			// Check if previous entry ended with sentence-ending punctuation
			if endOfSentence {
				sentenceCount++

				// Check for timing gaps between segment starts
				gap := entry.Start - prevStart
				if gap < 0 {
					gap = 0
				}

				// If there's a significant gap, treat as paragraph break
				if gap > ParagraphGap {
					addParagraphBreak = true
				} else if sentenceCount >= 2 {
					// Add paragraph break every 2-3 sentences
					// This creates readable paragraphs even without timing gaps
					addParagraphBreak = true
					sentenceCount = 0
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

		index = append(index, IndexEntry{
			Timestamp: entry.Start,
			Offset:    currentOffset,
		})

		textBuilder.WriteString(text)
		currentOffset += len(text)

		// Track state for next iteration using already-trimmed text
		endOfSentence = strings.HasSuffix(text, ".") ||
			strings.HasSuffix(text, "?") ||
			strings.HasSuffix(text, "!")
		prevStart = entry.Start
	}

	smsh := &Smooshed{
		Indexes: index,
		Text:    textBuilder.String(),
	}

	return smsh, nil
}
