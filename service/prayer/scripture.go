package prayer

// Looking things up in the sources: a chapter, a saying, or a question.
//
// These three were tools with no service behind them. They lived in
// internal/api with their own copy of an HTTP client for reminder.dev, because
// that package may not import a service and so had nowhere to put the call —
// which meant the one package in this repo that already talks to reminder.dev,
// this one, was not the package doing it.
//
// The client is small on purpose. reminder.dev answers in prose, and prose is
// what a model wants, so nothing here parses a response: it asks and hands back
// what came out.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// scriptureBase is a variable so a test can point it somewhere it controls.
var scriptureBase = "https://reminder.dev/api"

var scriptureClient = &http.Client{Timeout: 10 * time.Second}

func scriptureGet(path string) (string, error) {
	return scriptureDo(http.MethodGet, path, "", "")
}

func scripturePost(path, contentType, body string) (string, error) {
	return scriptureDo(http.MethodPost, path, contentType, body)
}

func scriptureDo(method, path, contentType, body string) (string, error) {
	req, err := http.NewRequest(method, strings.TrimRight(scriptureBase, "/")+path, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := scriptureClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("reminder API error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("reminder API returned status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading reminder response: %w", err)
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
func (Server) Verse(_ context.Context, req *VerseRequest, rsp *VerseResponse) error {
	if req.Chapter < 1 || req.Chapter > 114 {
		return fmt.Errorf("chapter must be between 1 and 114")
	}
	path := fmt.Sprintf("/quran/%d", req.Chapter)
	if req.Verse > 0 {
		path += fmt.Sprintf("/%d", req.Verse)
	}
	text, err := scriptureGet(path)
	if err != nil {
		return err
	}
	rsp.Text = text
	return nil
}

// ── Saying ──────────────────────────────────────────────────────

type SayingRequest struct {
	Book int `json:"book" description:"Book number within Sahih al-Bukhari. Omit for a saying from any book"`
}

type SayingResponse struct {
	Text string `json:"text" description:"The hadith, with its book and chain"`
}

// Saying looks up a hadith from Sahih al-Bukhari.
// @example {"book": 1}
func (Server) Saying(_ context.Context, req *SayingRequest, rsp *SayingResponse) error {
	path := "/hadith"
	if req.Book > 0 {
		path += fmt.Sprintf("/%d", req.Book)
	}
	text, err := scriptureGet(path)
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
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return fmt.Errorf("query is required")
	}
	text, err := scripturePost("/search", "application/json", fmt.Sprintf(`{"q":%q}`, q))
	if err != nil {
		return err
	}
	rsp.Text = text
	return nil
}
