package scripture

// Public scripture transport shared by the Islam service and legacy Prayer tools.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client accesses public Reminder references, never its generated answers.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

var Default = Client{BaseURL: "https://reminder.dev/api", HTTP: &http.Client{Timeout: 10 * time.Second}}

func (c Client) request(ctx context.Context, method, path, body string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("reminder API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("reminder API returned status %d", resp.StatusCode)
	}
	const maxBody = 4 << 20
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxBody {
		return "", fmt.Errorf("reminder response is too large")
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return "", fmt.Errorf("empty reminder response")
	}
	if !json.Valid(b) {
		return "", fmt.Errorf("invalid reminder JSON response")
	}
	return string(b), nil
}

// ── Verse ───────────────────────────────────────────────────────

type VerseRequest struct {
	Chapter int `json:"chapter" required:"true" description:"Chapter (surah) number, 1-114"`
	Verse   int `json:"verse" description:"Verse number within the chapter. Omit for the whole chapter"`
}

type VerseResponse struct {
	Text string `json:"text" description:"The verse or chapter, with its reference"`
}

// Verse looks up a chapter of the Quran, or one verse within it.
// @example {"chapter": 2, "verse": 255}
func (c Client) Verse(ctx context.Context, req *VerseRequest, rsp *VerseResponse) error {
	if req.Verse < 0 {
		return fmt.Errorf("verse must not be negative")
	}
	if req.Chapter < 1 || req.Chapter > 114 {
		return fmt.Errorf("chapter must be between 1 and 114")
	}
	path := fmt.Sprintf("/quran/%d", req.Chapter)
	if req.Verse > 0 {
		path += fmt.Sprintf("/%d", req.Verse)
	}
	text, err := c.request(ctx, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	rsp.Text = text
	return nil
}

// ── Saying ──────────────────────────────────────────────────────

type SayingRequest struct {
	Book int `json:"book" description:"Book number within Sahih al-Bukhari. Omit for the collection"`
}

type SayingResponse struct {
	Text string `json:"text" description:"Hadith source records, preserving book and attribution metadata"`
}

// Saying reads a book or the collection of Sahih al-Bukhari.
// @example {"book": 1}
func (c Client) Saying(ctx context.Context, req *SayingRequest, rsp *SayingResponse) error {
	if req.Book < 0 {
		return fmt.Errorf("book must not be negative")
	}
	path := "/hadith"
	if req.Book > 0 {
		path += fmt.Sprintf("/%d", req.Book)
	}
	text, err := c.request(ctx, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	rsp.Text = text
	return nil
}

// ── Search ──────────────────────────────────────────────────────

type SearchRequest struct {
	Query string `json:"query" required:"true" description:"A question in plain language, e.g. \"what is said about patience\""`
}

type SearchResponse struct {
	Text string `json:"text" description:"Passages that answer the question, with their references"`
}

// Search asks the Quran, the hadith and the names of Allah a question in plain
// language, rather than looking up a reference already known.
// @example {"query": "what is said about patience"}
func (c Client) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	result, err := c.References(ctx, req.Query)
	if err != nil {
		return err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	rsp.Text = string(body)
	return nil
}

// Reference preserves source metadata without inventing attribution or grading.
type Reference struct {
	Text     string            `json:"text"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata"`
}
type ReferencesResponse struct {
	Query      string      `json:"q"`
	References []Reference `json:"references"`
}

func (c Client) References(ctx context.Context, query string) (*ReferencesResponse, error) {
	q := strings.TrimSpace(query)
	if q == "" || len(q) > 1000 {
		return nil, fmt.Errorf("query must contain 1-1000 bytes")
	}
	body, err := json.Marshal(struct {
		Query     string `json:"q"`
		Summarise bool   `json:"summarise"`
	}{Query: q})
	if err != nil {
		return nil, err
	}
	raw, err := c.request(ctx, http.MethodPost, "/search", string(body))
	if err != nil {
		return nil, err
	}
	var result ReferencesResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid reminder references: %w", err)
	}
	if result.References == nil {
		return nil, fmt.Errorf("reminder response is missing references")
	}
	for _, ref := range result.References {
		if strings.TrimSpace(ref.Text) == "" || len(ref.Metadata) == 0 {
			return nil, fmt.Errorf("reminder reference is missing text or provenance")
		}
	}
	result.Query = q
	return &result, nil
}
