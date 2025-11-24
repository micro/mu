package home

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/app"
	"mu/auth"
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
	ID       string
	Title    string
	Column   string // "left" or "right"
	Position int
	Link     string
	Content  func() string
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
		sess, err := auth.GetSession(r)
		if err == nil {
			acc, err := auth.GetAccount(sess.Account)
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
	var leftHTML []string
	var rightHTML []string
	
	for _, card := range Cards {
		content := card.Content()
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
	homepage := fmt.Sprintf(Template, 
		strings.Join(leftHTML, "\n"),
		strings.Join(rightHTML, "\n"))

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
