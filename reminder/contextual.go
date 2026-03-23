package reminder

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

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
		time.Sleep(2 * time.Hour)
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
	verse, ref := pickBestVerse(result)
	if verse == "" {
		app.Log("reminder", "No suitable Quran verse found in search results")
		return
	}

	// Build the HTML card
	html := buildContextualHTML(verse, ref)

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
		"verse":     verse,
		"reference": ref,
		"query":     query,
		"timestamp": time.Now().Unix(),
	}
	if b, err := json.Marshal(cacheData); err == nil {
		data.SaveFile("reminder_contextual.json", string(b))
	}

	app.Log("reminder", "Updated contextual reminder: %s", truncate(verse, 80))
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

// pickBestVerse finds the highest-scoring Quran verse from search results
func pickBestVerse(result *SearchResponse) (text string, ref string) {
	for _, r := range result.References {
		source, _ := r.Metadata["source"].(string)
		if source != "quran" {
			continue
		}

		chapter, _ := r.Metadata["chapter"].(string)
		name, _ := r.Metadata["name"].(string)
		verse, _ := r.Metadata["verse"].(string)

		if name != "" && chapter != "" && verse != "" {
			ref = fmt.Sprintf("%s (%s:%s)", name, chapter, verse)
		} else if chapter != "" && verse != "" {
			ref = fmt.Sprintf("Quran %s:%s", chapter, verse)
		}

		return r.Text, ref
	}

	// No Quran verse found, try names of Allah
	for _, r := range result.References {
		source, _ := r.Metadata["source"].(string)
		if source == "names" {
			return r.Text, "Names of Allah"
		}
	}

	return "", ""
}

func buildContextualHTML(verse, ref string) string {
	var sb strings.Builder
	sb.WriteString(`<div class="item"><div class="verse">`)
	sb.WriteString(htmlpkg.EscapeString(verse))
	sb.WriteString(`</div>`)
	if ref != "" {
		sb.WriteString(fmt.Sprintf(`<div style="font-size:12px;color:#888;margin-top:4px;">— %s</div>`, htmlpkg.EscapeString(ref)))
	}
	sb.WriteString(`</div>`)

	// Build link to the verse on reminder.dev
	moreURL := "https://reminder.dev"
	sb.WriteString(app.Link("More", moreURL))

	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
