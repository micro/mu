package reminder

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
	"unicode"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
	"mu/news"
)

// SearchResult represents a single reference from reminder.dev/api/search
type SearchResult struct {
	Text     string                 `json:"text"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SearchResponse is the response from reminder.dev/api/search
type SearchResponse struct {
	Answer     string         `json:"answer"`
	Query      string         `json:"q"`
	References []SearchResult `json:"references"`
}

// contextual reminder state
var (
	contextualHTML  string
	contextualReady bool
	lastContextHash string
)

// startContextualRefresh runs a background loop that picks a Quran verse
// relevant to the current news cycle and updates the reminder card.
func startContextualRefresh() {
	// Wait for news to load before first attempt
	time.Sleep(2 * time.Minute)

	for {
		refreshContextualReminder()
		time.Sleep(time.Hour)
	}
}

func refreshContextualReminder() {
	query := buildNewsQuery()
	if query == "" {
		app.Log("reminder", "No news context available for contextual reminder")
		return
	}

	// Check if context has meaningfully changed
	if query == lastContextHash {
		app.Log("reminder", "News context unchanged, skipping contextual refresh")
		return
	}

	app.Log("reminder", "Searching for verse relevant to: %s", truncate(query, 100))

	result, err := searchReminder(query)
	if err != nil {
		app.Log("reminder", "Contextual search failed: %v", err)
		return
	}

	// Find the best Quran verse from results
	pick := pickBestVerse(result)
	if pick == nil {
		app.Log("reminder", "No suitable Quran verse found in search results")
		return
	}

	// Build the HTML card
	html := buildContextualHTML(pick)

	reminderMutex.Lock()
	contextualHTML = html
	contextualReady = true
	reminderHTML = html
	data.SaveFile("reminder.html", html)
	reminderMutex.Unlock()

	event.Publish(event.Event{Type: "reminder_updated"})

	lastContextHash = query

	// Cache the contextual result
	cacheData := map[string]interface{}{
		"verse":     pick.text,
		"reference": pick.ref,
		"query":     query,
		"timestamp": time.Now().Unix(),
	}
	if b, err := json.Marshal(cacheData); err == nil {
		data.SaveFile("reminder_contextual.json", string(b))
	}

	app.Log("reminder", "Updated contextual reminder: %s", truncate(pick.text, 80))
}

// buildNewsQuery looks at the current news headlines and builds a thematic
// query for the reminder.dev search API.
func buildNewsQuery() string {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return ""
	}

	// Collect headlines grouped by category, max 3 per category
	byCategory := make(map[string][]string)
	for _, item := range feed {
		cat := item.Category
		if cat == "" {
			cat = "General"
		}
		if len(byCategory[cat]) < 3 {
			byCategory[cat] = append(byCategory[cat], item.Title)
		}
	}

	var sb strings.Builder
	sb.WriteString("What Quran verse is most relevant to today's world events: ")

	count := 0
	for cat, titles := range byCategory {
		if count > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(cat + ": ")
		sb.WriteString(strings.Join(titles, ", "))
		count++
		if count >= 4 {
			break
		}
	}

	query := sb.String()
	if len(query) > 500 {
		query = query[:497] + "..."
	}
	return query
}

func searchReminder(query string) (*SearchResponse, error) {
	body := fmt.Sprintf(`{"q":%q}`, query)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post("https://reminder.dev/api/search", "application/json", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result SearchResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return &result, nil
}

// verseResult holds the picked verse along with its metadata for URL building
type verseResult struct {
	text    string
	ref     string
	chapter string
	verse   string
	source  string // "quran", "names", "bukhari"
	link    string // path on reminder.dev
}

// isTerminal checks if text ends with terminal punctuation (complete thought)
func isTerminal(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return true
	}
	last := text[len(text)-1]
	return last == '.' || last == '!' || last == '?' || last == '"'
}

// pickBestVerse finds the highest-scoring Quran verse from search results.
// Like reminder.dev, if a verse doesn't end with terminal punctuation,
// it looks for consecutive verses from the same chapter to complete the thought.
func pickBestVerse(result *SearchResponse) *verseResult {
	// Collect all Quran verses indexed by chapter:verse for continuation lookup
	type qverse struct {
		text    string
		chapter string
		name    string
		verse   int
	}
	var quranResults []qverse
	for _, r := range result.References {
		source, _ := r.Metadata["source"].(string)
		if source != "quran" {
			continue
		}
		ch, _ := r.Metadata["chapter"].(string)
		nm, _ := r.Metadata["name"].(string)
		vs, _ := r.Metadata["verse"].(string)
		vn := 0
		fmt.Sscanf(vs, "%d", &vn)
		quranResults = append(quranResults, qverse{text: r.Text, chapter: ch, name: nm, verse: vn})
	}

	if len(quranResults) == 0 {
		return nil
	}

	// Start with the best (first) result
	best := quranResults[0]
	combinedText := best.text
	verseStart := best.verse
	verseEnd := best.verse

	// If the verse doesn't end with terminal punctuation, look for
	// consecutive verses from the same chapter to complete the thought.
	// This mirrors getVerse() in reminder.dev which extends up to 10 verses.
	if !isTerminal(combinedText) {
		for i := 1; i <= 10; i++ {
			nextVerse := verseEnd + 1
			found := false
			for _, qv := range quranResults {
				if qv.chapter == best.chapter && qv.verse == nextVerse {
					// Join like reminder.dev: inline if ending with comma/dash/semicolon,
					// paragraph break otherwise
					last := combinedText[len(combinedText)-1]
					if last == ',' || last == ';' || string(last) == "—" || unicode.IsLetter(rune(last)) {
						combinedText += " " + qv.text
					} else {
						combinedText += "\n\n" + qv.text
					}
					verseEnd = nextVerse
					found = true
					break
				}
			}
			if !found || isTerminal(combinedText) {
				break
			}
		}
	}

	// Build the reference string with range if multiple verses
	verseNum := fmt.Sprintf("%d", verseStart)
	if verseStart != verseEnd {
		verseNum = fmt.Sprintf("%d-%d", verseStart, verseEnd)
	}

	ref := ""
	if best.name != "" && best.chapter != "" {
		ref = fmt.Sprintf("%s (%s:%s)", best.name, best.chapter, verseNum)
	} else if best.chapter != "" {
		ref = fmt.Sprintf("Quran %s:%s", best.chapter, verseNum)
	}

	return &verseResult{
		text:    combinedText,
		ref:     ref,
		chapter: best.chapter,
		verse:   fmt.Sprintf("%d", verseStart),
		source:  "quran",
		link:    fmt.Sprintf("/quran/%s#%d", best.chapter, verseStart),
	}
}

func buildContextualHTML(pick *verseResult) string {
	// Format: "{Name} ({Chapter}:{Verse})\n\n{Text}"
	formattedText := pick.text
	if pick.ref != "" {
		formattedText = pick.ref + "\n\n" + pick.text
	}

	var sb strings.Builder
	sb.WriteString(`<div class="item"><div class="verse">`)
	sb.WriteString(htmlpkg.EscapeString(formattedText))
	sb.WriteString(`</div></div>`)

	// Build link to the specific verse on reminder.dev
	moreURL := "https://reminder.dev"
	if pick.link != "" {
		moreURL = "https://reminder.dev" + pick.link
	}
	sb.WriteString(app.Link("More", moreURL))

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
