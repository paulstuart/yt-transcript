package yttranscript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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

	testDataPath := filepath.Join("testdata", "test_transcript.json")
	data, err := os.ReadFile(testDataPath)
	if err != nil {
		t.Fatalf("Failed to read test transcript: %v", err)
	}

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
	// Note: ProcessTranscript trims whitespace, so we check for trimmed text
	for i, entry := range transcript.Lines {
		trimmed := strings.TrimSpace(entry.Text)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(result.Text, trimmed) {
			t.Errorf("ProcessTranscript() smooshed text missing entry %d: %q", i, trimmed)
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

		// The text at this offset should start with the original entry's text (trimmed)
		// ProcessTranscript trims whitespace from entries
		expectedText := strings.TrimSpace(originalEntry.Text)
		if !strings.HasPrefix(remainingText, expectedText) {
			// Show context for debugging
			contextLen := min(len(remainingText), len(expectedText)+50)
			t.Errorf("Index entry %d (timestamp=%.3f, offset=%d): text mismatch\n"+
				"  Expected to start with: %q\n"+
				"  Actually starts with:   %q",
				i, indexEntry.Timestamp, indexEntry.Offset,
				expectedText,
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
	// Note: ProcessTranscript trims whitespace, so we check/count trimmed text
	totalOriginalTextLen := 0
	for _, entry := range transcript.Lines {
		trimmed := strings.TrimSpace(entry.Text)
		if trimmed != "" {
			totalOriginalTextLen += len(trimmed)
			if !strings.Contains(result.Text, trimmed) {
				t.Errorf("Smooshed text missing original entry: %q", trimmed)
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

// TestProcessTranscript_NoDoubleSpaces verifies that ProcessTranscript correctly
// handles entries with leading/trailing whitespace and produces no double spaces.
// Each timestamp should point to the exact start of its corresponding word in the output.
func TestProcessTranscript_NoDoubleSpaces(t *testing.T) {
	// Create test data with various whitespace issues
	transcript := &TranscriptRaw{
		Language: "en",
		Lines: []TranscriptLine{
			{Text: "Hello world", Start: 0.0, Duration: 1.0},        // normal
			{Text: " Leading space", Start: 1.0, Duration: 1.0},     // leading space
			{Text: "Trailing space ", Start: 2.0, Duration: 1.0},    // trailing space
			{Text: " Both spaces ", Start: 3.0, Duration: 1.0},      // both
			{Text: "  Double leading", Start: 4.0, Duration: 1.0},   // double leading
			{Text: "Double trailing  ", Start: 5.0, Duration: 1.0},  // double trailing
			{Text: "Normal again", Start: 6.0, Duration: 1.0},       // normal
			{Text: "   ", Start: 7.0, Duration: 1.0},                // whitespace only (should skip)
			{Text: "", Start: 8.0, Duration: 1.0},                   // empty (should skip)
			{Text: "After empty.", Start: 9.0, Duration: 1.0},       // sentence end
			{Text: "New sentence", Start: 10.0, Duration: 1.0},      // after sentence
		},
	}

	result, err := ProcessTranscript(transcript)
	if err != nil {
		t.Fatalf("ProcessTranscript() error = %v", err)
	}

	// Check for double spaces - there should be none
	doubleSpaceRegex := regexp.MustCompile(`  +`)
	if matches := doubleSpaceRegex.FindAllStringIndex(result.Text, -1); len(matches) > 0 {
		for _, m := range matches {
			start := m[0] - 20
			if start < 0 {
				start = 0
			}
			end := m[1] + 20
			if end > len(result.Text) {
				end = len(result.Text)
			}
			t.Errorf("Found double space at position %d, context: %q", m[0], result.Text[start:end])
		}
	}

	// Check that whitespace-only and empty entries were skipped
	expectedEntries := 9 // 11 total - 2 skipped (whitespace-only and empty)
	if len(result.Indexes) != expectedEntries {
		t.Errorf("Expected %d index entries (skipping empty/whitespace), got %d",
			expectedEntries, len(result.Indexes))
	}

	// Verify each timestamp points to the correct trimmed word
	expectedTexts := []string{
		"Hello world",
		"Leading space",
		"Trailing space",
		"Both spaces",
		"Double leading",
		"Double trailing",
		"Normal again",
		"After empty.",
		"New sentence",
	}

	for i, idx := range result.Indexes {
		if i >= len(expectedTexts) {
			break
		}

		remaining := result.Text[idx.Offset:]
		if !strings.HasPrefix(remaining, expectedTexts[i]) {
			contextLen := min(len(remaining), 50)
			t.Errorf("Index %d (timestamp=%.1f, offset=%d): expected %q, got %q",
				i, idx.Timestamp, idx.Offset, expectedTexts[i], remaining[:contextLen])
		}
	}

	t.Logf("Output text: %q", result.Text)
}

// TestProcessTranscript_WordAlignment validates that each index entry correctly
// points to the first word of its corresponding transcript chunk.
func TestProcessTranscript_WordAlignment(t *testing.T) {
	transcript := loadTestTranscript(t)

	result, err := ProcessTranscript(transcript)
	if err != nil {
		t.Fatalf("ProcessTranscript() error = %v", err)
	}

	// Build a map from timestamp to expected (trimmed) text
	timestampToText := make(map[float64]string)
	for _, line := range transcript.Lines {
		trimmed := strings.TrimSpace(line.Text)
		if trimmed != "" {
			timestampToText[line.Start] = trimmed
		}
	}

	// Verify each index entry points to the correct word
	for i, idx := range result.Indexes {
		expectedText, exists := timestampToText[idx.Timestamp]
		if !exists {
			t.Errorf("Index %d: timestamp %.3f not found in original transcript", i, idx.Timestamp)
			continue
		}

		// Verify offset is valid
		if idx.Offset < 0 || idx.Offset >= len(result.Text) {
			t.Errorf("Index %d: offset %d out of bounds [0, %d)", i, idx.Offset, len(result.Text))
			continue
		}

		// Get the text starting at this offset
		remaining := result.Text[idx.Offset:]

		// The text should start with the expected (trimmed) text
		if !strings.HasPrefix(remaining, expectedText) {
			contextLen := min(len(remaining), len(expectedText)+30)
			t.Errorf("Index %d (timestamp=%.3f, offset=%d):\n"+
				"  Expected to start with: %q\n"+
				"  Actually starts with:   %q",
				i, idx.Timestamp, idx.Offset, expectedText, remaining[:contextLen])
		}

		// Verify the first word matches
		// Extract first word from expected text
		expectedWords := strings.Fields(expectedText)
		if len(expectedWords) > 0 {
			firstExpectedWord := expectedWords[0]
			actualWords := strings.Fields(remaining[:min(len(remaining), len(expectedText)+50)])
			if len(actualWords) > 0 {
				if actualWords[0] != firstExpectedWord {
					t.Errorf("Index %d (timestamp=%.3f): first word mismatch, expected %q, got %q",
						i, idx.Timestamp, firstExpectedWord, actualWords[0])
				}
			}
		}
	}

	// Additional check: no double spaces anywhere in the output
	doubleSpaceRegex := regexp.MustCompile(`  +`)
	if matches := doubleSpaceRegex.FindAllStringIndex(result.Text, -1); len(matches) > 0 {
		t.Errorf("Found %d instances of double spaces in output", len(matches))
		for i, m := range matches {
			if i >= 5 {
				t.Logf("... and %d more", len(matches)-5)
				break
			}
			start := max(0, m[0]-20)
			end := min(len(result.Text), m[1]+20)
			t.Logf("  Position %d: %q", m[0], result.Text[start:end])
		}
	}

	t.Logf("Word alignment validation passed for %d entries", len(result.Indexes))
}
