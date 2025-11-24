package yttranscript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testTranscriptJSON represents the structure of the test transcript JSON file
type testTranscriptJSON struct {
	LanguageCode string `json:"language_code"`
	Transcripts  []struct {
		Text     string  `json:"text"`
		Start    float64 `json:"start"`
		Duration float64 `json:"duration"`
	} `json:"transcripts"`
}

// loadTestTranscript loads and converts the test transcript JSON to a Transcript
func loadTestTranscript(t *testing.T) *TranscriptRaw {
	t.Helper()

	// Read the test transcript file
	testDataPath := filepath.Join("testdata", "test_transcript.json")
	data, err := os.ReadFile(testDataPath)
	if err != nil {
		t.Fatalf("Failed to read test transcript: %v", err)
	}

	// Unmarshal the JSON
	var rawTranscripts []testTranscriptJSON
	if err := json.Unmarshal(data, &rawTranscripts); err != nil {
		t.Fatalf("Failed to unmarshal test transcript: %v", err)
	}

	if len(rawTranscripts) == 0 {
		t.Fatal("Test transcript is empty")
	}

	// Convert to Transcript type
	raw := rawTranscripts[0]
	transcript := &TranscriptRaw{
		Language: raw.LanguageCode,
		Lines:    make([]TranscriptLine, 0, len(raw.Transcripts)),
	}

	for _, entry := range raw.Transcripts {
		transcript.Lines = append(transcript.Lines, TranscriptLine{
			Text:     entry.Text,
			Start:    entry.Start,
			Duration: entry.Duration,
		})
	}

	return transcript
}

func TestProcessTranscript(t *testing.T) {
	transcript := loadTestTranscript(t)

	// Test with valid transcript
	// ctx := context.Background()
	// channelID := "test-channel-123"
	// videoID := "test-video-456"

	result, err := ProcessTranscript(transcript)
	if err != nil {
		t.Fatalf("ProcessTranscript() error = %v, want nil", err)
	}

	// Validate result is not nil
	if result == nil {
		t.Fatal("ProcessTranscript() returned nil result")
	}

	// Validate text is not empty
	if result.Text == "" {
		t.Error("ProcessTranscript() returned empty smooshed text")
	}

	// Validate index is populated
	if len(result.Indexes) == 0 {
		t.Error("ProcessTranscript() returned empty index")
	}

	// Validate index length matches transcript entries (approximately)
	// Some entries might be skipped if they have empty text
	if len(result.Indexes) > len(transcript.Lines) {
		t.Errorf("ProcessTranscript() index length %d exceeds transcript entries %d",
			len(result.Indexes), len(transcript.Lines))
	}

	// Validate that all text from entries appears in smooshed text
	for i, entry := range transcript.Lines {
		if entry.Text == "" {
			continue
		}
		if !strings.Contains(result.Text, entry.Text) {
			t.Errorf("ProcessTranscript() smooshed text missing entry %d: %q", i, entry.Text)
		}
	}

	// Validate index entries
	// TODO: anything more to check?
	for i, indexEntry := range result.Indexes {
		// Check that timestamp is non-negative
		if indexEntry.Timestamp < 0 {
			t.Errorf("Index entry %d has negative timestamp: %f", i, indexEntry.Timestamp)
		}

		// Check that offset is non-negative
		if indexEntry.Offset < 0 {
			t.Errorf("Index entry %d has negative offset: %d", i, indexEntry.Offset)
		}
	}

	// Validate that smooshed text contains paragraph breaks
	if !strings.Contains(result.Text, "\n\n") {
		t.Log("Warning: ProcessTranscript() smooshed text has no paragraph breaks")
	}
}

// TestProcessTranscript_OffsetAlignment verifies that index offsets correctly align
// with the original transcript entries in the smooshed text
func TestProcessTranscript_OffsetAlignment(t *testing.T) {
	transcript := loadTestTranscript(t)

	result, err := ProcessTranscript(transcript)
	if err != nil {
		t.Fatalf("ProcessTranscript() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("ProcessTranscript() returned nil result")
	}

	// Validate we have the expected number of indexes
	if len(result.Indexes) == 0 {
		t.Fatal("ProcessTranscript() returned empty index")
	}

	// Create a map of timestamps to original transcript entries for quick lookup
	timestampToEntry := make(map[float64]*TranscriptLine)
	for i := range transcript.Lines {
		entry := &transcript.Lines[i]
		if entry.Text != "" {
			timestampToEntry[entry.Start] = entry
		}
	}

	// Track previous offset to ensure monotonically increasing
	prevOffset := -1

	for i, indexEntry := range result.Indexes {
		// Validate offset is within bounds
		if indexEntry.Offset < 0 {
			t.Errorf("Index entry %d has negative offset: %d", i, indexEntry.Offset)
			continue
		}
		if indexEntry.Offset >= len(result.Text) {
			t.Errorf("Index entry %d offset %d exceeds text length %d",
				i, indexEntry.Offset, len(result.Text))
			continue
		}

		// Validate offsets are monotonically increasing (or equal for same position)
		if indexEntry.Offset < prevOffset {
			t.Errorf("Index entry %d offset %d is less than previous offset %d",
				i, indexEntry.Offset, prevOffset)
		}
		prevOffset = indexEntry.Offset

		// Find the corresponding original transcript entry by timestamp
		originalEntry, exists := timestampToEntry[indexEntry.Timestamp]
		if !exists {
			t.Errorf("Index entry %d has timestamp %.3f not found in original transcript",
				i, indexEntry.Timestamp)
			continue
		}

		// Verify duration matches
		if indexEntry.Duration != originalEntry.Duration {
			t.Errorf("Index entry %d duration mismatch: got %.3f, want %.3f",
				i, indexEntry.Duration, originalEntry.Duration)
		}

		// Extract text starting from the offset
		remainingText := result.Text[indexEntry.Offset:]

		// The text at this offset should start with the original entry's text
		// Account for potential Unicode characters by checking if it starts with the expected text
		if !strings.HasPrefix(remainingText, originalEntry.Text) {
			// Show context for debugging
			contextLen := min(len(remainingText), len(originalEntry.Text)+50)
			t.Errorf("Index entry %d (timestamp=%.3f, offset=%d): text mismatch\n"+
				"  Expected to start with: %q\n"+
				"  Actually starts with:   %q",
				i, indexEntry.Timestamp, indexEntry.Offset,
				originalEntry.Text,
				remainingText[:contextLen])
		}

		// Additional validation: if this is not the first entry,
		// verify there's proper spacing/separator before this entry
		if i > 0 && indexEntry.Offset > 0 {
			// Check what precedes this entry (should be space or newline)
			precedingChar := result.Text[indexEntry.Offset-1]
			if precedingChar != ' ' && precedingChar != '\n' {
				t.Errorf("Index entry %d at offset %d: expected space or newline before entry, got %q",
					i, indexEntry.Offset, precedingChar)
			}
		}
	}

	// Validate that all text from original entries appears in smooshed text
	// and that total smooshed length is reasonable
	totalOriginalTextLen := 0
	for _, entry := range transcript.Lines {
		if entry.Text != "" {
			totalOriginalTextLen += len(entry.Text)
			if !strings.Contains(result.Text, entry.Text) {
				t.Errorf("Smooshed text missing original entry: %q", entry.Text)
			}
		}
	}

	// The smooshed text should be longer than original (due to spaces/breaks)
	// but not excessively longer
	if len(result.Text) < totalOriginalTextLen {
		t.Errorf("Smooshed text length %d is less than total original text length %d",
			len(result.Text), totalOriginalTextLen)
	}

	// Calculate expected overhead: one separator per entry (space or \n\n)
	maxExpectedLen := totalOriginalTextLen + (len(result.Indexes) * 2) // 2 chars max per separator
	if len(result.Text) > maxExpectedLen {
		t.Errorf("Smooshed text length %d exceeds reasonable maximum %d (might indicate duplication)",
			len(result.Text), maxExpectedLen)
	}

	// Log some statistics for visibility
	t.Logf("Validation successful:")
	t.Logf("  - Original entries: %d", len(transcript.Lines))
	t.Logf("  - Index entries: %d", len(result.Indexes))
	t.Logf("  - Original text length: %d", totalOriginalTextLen)
	t.Logf("  - Smooshed text length: %d", len(result.Text))
	t.Logf("  - Overhead: %d bytes (%.1f%%)",
		len(result.Text)-totalOriginalTextLen,
		float64(len(result.Text)-totalOriginalTextLen)/float64(totalOriginalTextLen)*100)
}
