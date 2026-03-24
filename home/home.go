package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/apps"
	"mu/internal/app"
	"mu/blog"
	"mu/internal/event"
	"mu/news"
	"mu/social"
	"mu/markets"
	"mu/reminder"
	"mu/video"
)

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() string {
	return news.Headlines()
}

func ChatCard() string {
	return `<div id="home-chat">
		<form id="home-chat-form" action="/chat" method="GET">
			<input type="text" name="prompt" placeholder="Ask a question" required>
			<button type="submit">Ask</button>
		</form>
	</div>`
}

func AgentCard() string {
	return `<div id="home-agent">
		<form id="home-agent-form" action="/agent" method="GET">
			<div style="display:flex;gap:8px;">
				<input type="text" name="prompt" placeholder="Tell the agent what to do..." required style="flex:1;padding:8px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;">
				<button type="submit" style="padding:8px 16px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;cursor:pointer;">Do</button>
			</div>
			<div style="display:flex;gap:8px;margin-top:6px;align-items:center;">
				<select name="model" style="padding:4px 8px;font-family:inherit;font-size:13px;border:1px solid #ddd;border-radius:4px;">
					<option value="standard">Fast</option>
					<option value="premium">Best</option>
				</select>
				<span style="flex:1;"></span>
				` + agent.ToolsDropdownHTML() + `
			</div>
		</form>
	</div>`
}

type Card struct {
	ID          string
	Title       string
	Icon        string // Optional icon image path (e.g. "/news.png")
	Column      string // "left" or "right"
	Position    int
	Link        string
	Content     func() string
	CachedHTML  string    // Cached rendered content
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last update timestamp
}

var (
	lastRefresh time.Time
	cacheMutex  sync.RWMutex
	cacheTTL    = 2 * time.Minute
)

type CardConfig struct {
	Left []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"left"`
	Right []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"right"`
}

var Cards []Card

func Load() {
	b, _ := f.ReadFile("cards.json")
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		fmt.Println("Error loading cards.json:", err)
		return
	}

	// Map of card types to their content functions
	cardFunctions := map[string]func() string{
		"agent":    AgentCard,
		"blog":     blog.Preview,
		"chat":     ChatCard,
		"news":     newsCard,
		"markets":  markets.MarketsHTML,
		"reminder": reminder.ReminderHTML,
		"video":    video.Latest,
		"apps":     apps.Preview,
		"social":   social.CardHTML,
	}

	// Build Cards array from config
	Cards = []Card{}

	for _, c := range config.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "left",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	for _, c := range config.Right {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "right",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	// Sort by column and position
	sort.Slice(Cards, func(i, j int) bool {
		if Cards[i].Column != Cards[j].Column {
			return Cards[i].Column < Cards[j].Column
		}
		return Cards[i].Position < Cards[j].Position
	})

	// Do initial refresh
	RefreshCards()

	// Subscribe to blog and apps update events
	go func() {
		sub := event.Subscribe("blog_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("apps_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("social_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("reminder_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
}

// RefreshCards updates card content and timestamps if content changed
func RefreshCards() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	now := time.Now()

	// Check if cache is still valid
	if now.Sub(lastRefresh) < cacheTTL {
		return
	}

	for i := range Cards {
		card := &Cards[i]

		// Get fresh content
		content := card.Content()

		// Calculate hash
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		// Only update if content changed
		if hash != card.ContentHash {
			card.CachedHTML = content
			card.ContentHash = hash
			card.UpdatedAt = now
		}
	}

	lastRefresh = now
}

// ForceRefresh forces an immediate cache refresh (for admin actions)
func ForceRefresh() {
	cacheMutex.Lock()
	lastRefresh = time.Time{} // Reset to zero to force refresh
	cacheMutex.Unlock()
	RefreshCards()
}

// RefreshHandler clears the last_visit cookie to show all cards again
func RefreshHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	cookie := &http.Cookie{
		Name:     "last_visit",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete cookie
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)

	// Redirect back to home
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Refresh cards if cache expired (2 minute TTL)
	RefreshCards()

	var b strings.Builder

	// Feed section — existing home cards below the agent
	var leftHTML []string
	var rightHTML []string

	for _, card := range Cards {
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		html := app.Card(card.ID, card.Title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, html)
		} else {
			rightHTML = append(rightHTML, html)
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n")))
	}

	// Use RenderHTMLWithLang directly to inject a body class that hides the page title,
	// keeping the agent prompt as the primary visual element.
	lang := app.GetUserLanguage(r)
	html := app.RenderHTMLWithLangAndBody("Home", "The home screen", b.String(), lang, ` class="page-home"`)
	w.Write([]byte(html))
}

// htmlEsc escapes HTML special characters.
func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
