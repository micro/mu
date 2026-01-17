package video

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/app"
	"mu/chat"
)

// TranscriptEntry represents a single caption segment
type TranscriptEntry struct {
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Text     string  `json:"text"`
}

// TranscriptResult holds the transcript and metadata
type TranscriptResult struct {
	VideoID    string            `json:"video_id"`
	Language   string            `json:"language"`
	Entries    []TranscriptEntry `json:"entries"`
	FullText   string            `json:"full_text"`
	FetchedAt  time.Time         `json:"fetched_at"`
}

// VideoSummary holds a cached summary
type VideoSummary struct {
	VideoID   string    `json:"video_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	KeyPoints []string  `json:"key_points"`
	CreatedAt time.Time `json:"created_at"`
}

// captionTrack from YouTube page JSON
type captionTrack struct {
	BaseURL      string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Name         struct {
		SimpleText string `json:"simpleText"`
	} `json:"name"`
	Kind string `json:"kind"` // "asr" for auto-generated
}

// timedText XML structure from YouTube
type timedText struct {
	XMLName xml.Name   `xml:"transcript"`
	Texts   []textNode `xml:"text"`
}

type textNode struct {
	Start    string `xml:"start,attr"`
	Duration string `xml:"dur,attr"`
	Text     string `xml:",chardata"`
}

// GetTranscript fetches the transcript for a YouTube video
func GetTranscript(videoID string) (*TranscriptResult, error) {
	// Fetch the video page
	pageURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set user agent to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch video page: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	pageContent := string(body)
	
	// Extract caption tracks JSON from page
	tracks, err := extractCaptionTracks(pageContent)
	if err != nil {
		return nil, err
	}
	
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no captions available for this video")
	}
	
	// Prefer English, then first available
	var selectedTrack *captionTrack
	for i := range tracks {
		if strings.HasPrefix(tracks[i].LanguageCode, "en") {
			selectedTrack = &tracks[i]
			break
		}
	}
	if selectedTrack == nil {
		selectedTrack = &tracks[0]
	}
	
	// Fetch the transcript XML
	entries, err := fetchTranscriptFromURL(selectedTrack.BaseURL)
	if err != nil {
		return nil, err
	}
	
	// Build full text
	var fullText strings.Builder
	for _, e := range entries {
		fullText.WriteString(e.Text)
		fullText.WriteString(" ")
	}
	
	return &TranscriptResult{
		VideoID:   videoID,
		Language:  selectedTrack.LanguageCode,
		Entries:   entries,
		FullText:  strings.TrimSpace(fullText.String()),
		FetchedAt: time.Now(),
	}, nil
}

// playerResponse represents the YouTube player config JSON
type playerResponse struct {
	Captions struct {
		PlayerCaptionsTracklistRenderer struct {
			CaptionTracks []captionTrack `json:"captionTracks"`
		} `json:"playerCaptionsTracklistRenderer"`
	} `json:"captions"`
}

// extractCaptionTracks parses caption track info from YouTube page HTML
func extractCaptionTracks(pageContent string) ([]captionTrack, error) {
	// Find ytInitialPlayerResponse JSON
	marker := "ytInitialPlayerResponse = "
	startIdx := strings.Index(pageContent, marker)
	if startIdx == -1 {
		return nil, fmt.Errorf("no player response found in page")
	}
	startIdx += len(marker)
	
	// Find the end of the JSON object by matching braces
	depth := 0
	endIdx := startIdx
	for endIdx < len(pageContent) {
		switch pageContent[endIdx] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				endIdx++
				goto found
			}
		}
		endIdx++
	}
	return nil, fmt.Errorf("malformed player response JSON")
	
found:
	jsonStr := pageContent[startIdx:endIdx]
	
	var player playerResponse
	if err := json.Unmarshal([]byte(jsonStr), &player); err != nil {
		return nil, fmt.Errorf("failed to parse player response: %w", err)
	}
	
	tracks := player.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no caption tracks in player response")
	}
	
	return tracks, nil
}

// fetchTranscriptFromURL fetches and parses the transcript XML
func fetchTranscriptFromURL(url string) ([]TranscriptEntry, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transcript: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}
	
	// Parse XML
	var tt timedText
	if err := xml.Unmarshal(body, &tt); err != nil {
		return nil, fmt.Errorf("failed to parse transcript XML: %w", err)
	}
	
	entries := make([]TranscriptEntry, 0, len(tt.Texts))
	for _, t := range tt.Texts {
		var start, dur float64
		fmt.Sscanf(t.Start, "%f", &start)
		fmt.Sscanf(t.Duration, "%f", &dur)
		
		// Decode HTML entities
		text := decodeHTMLEntities(t.Text)
		
		entries = append(entries, TranscriptEntry{
			Start:    start,
			Duration: dur,
			Text:     text,
		})
	}
	
	return entries, nil
}

// decodeHTMLEntities decodes common HTML entities in transcript text
func decodeHTMLEntities(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
		"&#x27;", "'",
		"&nbsp;", " ",
	)
	return replacer.Replace(s)
}

// formatTimestamp converts seconds to MM:SS or HH:MM:SS format
func formatTimestamp(seconds float64) string {
	total := int(seconds)
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

// SummarizeVideo generates an AI summary of a video transcript
func SummarizeVideo(videoID, title string) (*VideoSummary, error) {
	// Get transcript
	transcript, err := GetTranscript(videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcript: %w", err)
	}
	
	// Truncate if too long (keep first ~15000 chars for context window)
	text := transcript.FullText
	if len(text) > 15000 {
		text = text[:15000] + "..."
	}
	
	// Build prompt
	promptText := fmt.Sprintf(`Summarize this video transcript. Provide:
1. A brief 2-3 sentence summary
2. 5-7 key points with timestamps (format: [MM:SS] point)

Video Title: %s

Transcript:
%s`, title, text)
	
	// Call LLM
	prompt := &chat.Prompt{
		Question: promptText,
		Priority: chat.PriorityHigh,
	}
	response, err := chat.AskLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}
	
	app.Log("video", "Generated summary for video %s", videoID)
	
	return &VideoSummary{
		VideoID:   videoID,
		Title:     title,
		Summary:   response,
		CreatedAt: time.Now(),
	}, nil
}
