package yttranscript

// TranscriptInfo represents the metadata comprising TranscriptRaw
type TranscriptInfo struct {
	VideoID        string
	VideoTitle     string
	Language       string
	LanguageCode   string
	IsGenerated    bool
	IsTranslatable bool
}

// TranscriptConfig holds configuration for fetching transcripts
// TODO: if this is just Lang, consider removing the struct and passing Lang string directly
type TranscriptConfig struct {
	// Lang specifies the language code for the transcript (e.g., "en" for English)
	// If empty, the first available language will be used
	Lang string
	// TimeoutSeconds specifies the timeout for fetching the transcript in seconds
	TimeoutSeconds int
}

// TranscriptResult contains the complete transcript and video metadata
type TranscriptResult struct {
	Info     *TranscriptInfo
	Smooshed *Smooshed
}

// GetInfo extracts TranscriptInfo from a TranscriptRaw
func GetInfo(transcript *TranscriptRaw) *TranscriptInfo {
	info := &TranscriptInfo{
		VideoID:        transcript.VideoID,
		VideoTitle:     transcript.VideoTitle,
		Language:       transcript.Language,
		LanguageCode:   transcript.LanguageCode,
		IsGenerated:    transcript.IsGenerated,
		IsTranslatable: transcript.IsTranslatable,
	}
	return info
}

// TranscriptEntry represents a single transcript segment
type TranscriptEntry struct {
	Text     string
	Start    float64 // seconds
	Duration float64 // seconds
}

// IndexEntry represents a mapping from smooshed text offset to timestamp
type IndexEntry struct {
	// Timestamp is the start time in seconds for this segment
	Timestamp float64 `json:"timestamp"`
	// Duration of the segment source
	Duration float64 `json:"duration"`
	// Offset is the byte offset into the smooshed text at the Timestamp
	Offset int `json:"offset"`
}

// Smooshed represents the transcript text as a string
// and includes index entries
type Smooshed struct {
	Text    string
	Indexes []IndexEntry
}
