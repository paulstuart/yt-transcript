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
	for i, indexEntry := range result.Indexes {
		// Check that timestamp is non-negative
		if indexEntry.Timestamp < 0 {
			t.Errorf("Index entry %d has negative timestamp: %f", i, indexEntry.Timestamp)
		}

		// Check that offset is non-negative
		if indexEntry.Offset < 0 {
			t.Errorf("Index entry %d has negative offset: %d", i, indexEntry.Offset)
		}

		// Check that length matches text length
		// if indexEntry. != len(indexEntry.Text) {
		// 	t.Errorf("Index entry %d length mismatch: got %d, want %d",
		// 		i, indexEntry.Length, len(indexEntry.Text))
		// }

		// Check that text at offset matches
		// TODO: fix this?
		// if indexEntry.Offset+indexEntry.Offset <= len(result.Text) {
		// 	extractedText := result.Text[indexEntry.Offset : indexEntry.Offset+indexEntry.Length]
		// 	if extractedText != indexEntry.Offset {
		// 		t.Errorf("Index entry %d text mismatch at offset %d: got %q, want %q",
		// 			i, indexEntry.Offset, extractedText, indexEntry.Text)
		// 	}
		// }
	}

	// Validate that smooshed text contains paragraph breaks
	if !strings.Contains(result.Text, "\n\n") {
		t.Log("Warning: ProcessTranscript() smooshed text has no paragraph breaks")
	}

	// Validate first and last entries are properly indexed
	if len(result.Indexes) > 0 {
		firstIndex := result.Indexes[0]
		if firstIndex.Offset != 0 {
			t.Errorf("First index entry offset = %d, want 0", firstIndex.Offset)
		}

		// TODO: re-enable this check after fixing length tracking
		// lastIndex := result.Indexes[len(result.Indexes)-1]
		// expectedEnd := lastIndex.Offset + lastIndex.Length
		// if expectedEnd > len(result.Text) {
		// 	t.Errorf("Last index entry extends beyond text: offset=%d, length=%d, text length=%d",
		// 		lastIndex.Offset, lastIndex.Length, len(result.Text))
		// }
	}
}

// func TestProcessTranscript_EmptyTranscript(t *testing.T) {
// 	ctx := context.Background()
// 	channelID := "test-channel"
// 	videoID := "test-video"

// 	// Test with empty transcript
// 	emptyTranscript := &Transcript{
// 		Language: "en",
// 		Entries:  []TranscriptEntry{},
// 	}

// 	result, err := ProcessTranscript(ctx, channelID, videoID, emptyTranscript)
// 	if err == nil {
// 		t.Error("ProcessTranscript() with empty transcript should return error")
// 	}
// 	if result != nil {
// 		t.Error("ProcessTranscript() with empty transcript should return nil result")
// 	}
// }

// func TestProcessTranscript_EntriesWithEmptyText(t *testing.T) {
// 	ctx := context.Background()
// 	channelID := "test-channel"
// 	videoID := "test-video"

// 	// Test with transcript containing some empty text entries
// 	transcript := &Transcript{
// 		Language: "en",
// 		Entries: []TranscriptEntry{
// 			{Text: "First entry", Start: 0.0, Duration: 1.0},
// 			{Text: "", Start: 1.0, Duration: 0.5}, // Empty text
// 			{Text: "Second entry", Start: 1.5, Duration: 1.0},
// 		},
// 	}

// 	result, err := ProcessTranscript(ctx, channelID, videoID, transcript)
// 	if err != nil {
// 		t.Fatalf("ProcessTranscript() error = %v, want nil", err)
// 	}

// 	// Should only have 2 index entries (empty text should be skipped)
// 	if len(result.Index) != 2 {
// 		t.Errorf("ProcessTranscript() index length = %d, want 2", len(result.Index))
// 	}

// 	// Text should contain both non-empty entries
// 	if !strings.Contains(result.Text, "First entry") {
// 		t.Error("ProcessTranscript() smooshed text missing 'First entry'")
// 	}
// 	if !strings.Contains(result.Text, "Second entry") {
// 		t.Error("ProcessTranscript() smooshed text missing 'Second entry'")
// 	}
// }

// func TestProcessTranscript_ParagraphBreaks(t *testing.T) {
// 	ctx := context.Background()
// 	channelID := "test-channel"
// 	videoID := "test-video"

// 	// Test with transcript that should generate paragraph breaks
// 	transcript := &Transcript{
// 		Language: "en",
// 		Entries: []TranscriptEntry{
// 			{Text: "First sentence.", Start: 0.0, Duration: 2.0},
// 			{Text: "Second sentence.", Start: 2.0, Duration: 2.0},
// 			{Text: "Third sentence.", Start: 4.0, Duration: 2.0},
// 			{Text: "Fourth sentence.", Start: 6.0, Duration: 2.0},
// 		},
// 	}

// 	result, err := ProcessTranscript(ctx, channelID, videoID, transcript)
// 	if err != nil {
// 		t.Fatalf("ProcessTranscript() error = %v, want nil", err)
// 	}

// 	// Should have paragraph breaks
// 	if !strings.Contains(result.Text, "\n\n") {
// 		t.Error("ProcessTranscript() should create paragraph breaks after sentences")
// 	}

// 	// All text should be present
// 	for _, entry := range transcript.Entries {
// 		if !strings.Contains(result.Text, entry.Text) {
// 			t.Errorf("ProcessTranscript() smooshed text missing: %q", entry.Text)
// 		}
// 	}
// }
