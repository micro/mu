package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/app"
	"mu/user"
	"mu/blog"
	"mu/news"
	"mu/video"
)

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

type Card struct {
	ID          string
	Title       string
	Column      string // "left" or "right"
	Position    int
	Link        string
	Content     func() string
	CachedHTML  string    // Cached rendered content
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last update timestamp
}

type CardConfig struct {
	Left  []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
	} `json:"left"`
	Right []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
	} `json:"right"`
}

var Cards []Card

func Load() {
	data, _ := f.ReadFile("cards.json")
	var config CardConfig
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Println("Error loading cards.json:", err)
		return
	}
	
	// Map of card types to their content functions
	cardFunctions := map[string]func() string{
		"news": news.Headlines,
		"markets": news.Markets,
		"reminder": news.Reminder,
		"posts": blog.Preview,
		"video": video.Latest,
	}
	
	// Build Cards array from config
	Cards = []Card{}
	
	for _, c := range config.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
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
	// Start card refresh goroutine
	go refreshCardsLoop()
}

// refreshCardsLoop regenerates card content hourly
func refreshCardsLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	// Initial refresh
	RefreshCards()
	
	for range ticker.C {
		RefreshCards()
	}
}

// RefreshCards updates card content and timestamps if content changed
func RefreshCards() {
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
			card.UpdatedAt = time.Now()
		}
	}
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
	// Handle POST requests for creating posts
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		title := strings.TrimSpace(r.FormValue("title"))
		content := strings.TrimSpace(r.FormValue("content"))

		if content == "" {
			http.Error(w, "Content is required", http.StatusBadRequest)
			return
		}

		// Get the authenticated user
		author := "Anonymous"
		sess, err := user.GetSession(r)
		if err == nil {
			acc, err := user.GetAccount(sess.Account)
			if err == nil {
				author = acc.Name
			}
		}

		// Create the post
		if err := blog.CreatePost(title, content, author); err != nil {
			http.Error(w, "Failed to save post", http.StatusInternalServerError)
			return
		}

		// Redirect back to home page
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	// GET request - render the page
	
	// Get last visit time from cookie
	var lastVisit time.Time
	if cookie, err := r.Cookie("last_visit"); err == nil {
		if ts, err := time.Parse(time.RFC3339, cookie.Value); err == nil {
			lastVisit = ts
		}
	}
	
	// Set cookie for this visit
	cookie := &http.Cookie{
		Name:     "last_visit",
		Value:    time.Now().Format(time.RFC3339),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365, // 1 year
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
	
	var leftHTML []string
	var rightHTML []string
	
	for _, card := range Cards {
		// Skip cards that haven't updated since last visit
		if !lastVisit.IsZero() && !card.UpdatedAt.After(lastVisit) {
			continue
		}
		
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}
		
		// Add "More" link if card has a link URL
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

	// create homepage
	var homepage string
	if len(leftHTML) == 0 && len(rightHTML) == 0 {
		// No new content - show caught up message with refresh link
		homepage = `<div id="home"><div class="home-left">` + 
			app.Card("caught-up", "All Caught Up", "<p>You're all caught up! Check back later for new updates.</p><p><a href='/home/refresh'>Refresh now</a> to see all cards again.</p>") + 
			`</div><div class="home-right"></div></div>`
	} else {
		homepage = fmt.Sprintf(Template, 
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n"))
	}

	// render html using user's language preference
	html := app.RenderHTMLForRequest("Home", "The Mu homescreen", homepage, r)

	w.Write([]byte(html))
}
