// Package cards provides reusable UI card components.
// Cards are self-contained HTML/CSS/JS templates that render data.
// They can be used server-side (RenderHTML) or client-side (fetch + fill).
package cards

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	templates     map[string]*template.Template
	templatesOnce sync.Once
)

// Card types
const (
	TypeNews    = "news"
	TypeVideo   = "video"
	TypeNote    = "note"
	TypeMail    = "mail"
	TypeBlog    = "blog"
	TypeChat    = "chat"
	TypeApp     = "app"
	TypeMarket  = "market"
)

// loadTemplates loads all card templates from embedded files
func loadTemplates() {
	templates = make(map[string]*template.Template)
	
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		
		name := strings.TrimSuffix(entry.Name(), ".html")
		content, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			continue
		}
		
		tmpl, err := template.New(name).Parse(string(content))
		if err != nil {
			continue
		}
		
		templates[name] = tmpl
	}
}

// RenderHTML renders a card with the given data, returning HTML string
func RenderHTML(cardType string, data any) string {
	templatesOnce.Do(loadTemplates)
	
	tmpl, ok := templates[cardType]
	if !ok {
		return "<!-- unknown card type: " + cardType + " -->"
	}
	
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "<!-- render error: " + err.Error() + " -->"
	}
	
	return buf.String()
}

// RenderList renders multiple items of the same type
func RenderList(cardType string, items []any) string {
	var buf strings.Builder
	for _, item := range items {
		buf.WriteString(RenderHTML(cardType, item))
	}
	return buf.String()
}

// Handler serves card templates and renders cards via API
// Routes:
//   GET /card/{type}         - returns the HTML template
//   POST /card/{type}/render - renders template with JSON body data
func Handler(w http.ResponseWriter, r *http.Request) {
	templatesOnce.Do(loadTemplates)
	
	path := strings.TrimPrefix(r.URL.Path, "/card/")
	parts := strings.Split(path, "/")
	
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "card type required", http.StatusBadRequest)
		return
	}
	
	cardType := parts[0]
	
	// GET /card/{type} - return template source
	if r.Method == "GET" && len(parts) == 1 {
		content, err := templateFS.ReadFile("templates/" + cardType + ".html")
		if err != nil {
			http.Error(w, "card not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
		return
	}
	
	// POST /card/{type}/render - render with data
	if r.Method == "POST" && len(parts) == 2 && parts[1] == "render" {
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		
		html := RenderHTML(cardType, data)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}
	
	http.Error(w, "not found", http.StatusNotFound)
}

// List returns available card types
func List() []string {
	templatesOnce.Do(loadTemplates)
	
	var types []string
	for name := range templates {
		types = append(types, name)
	}
	return types
}

// Grid wraps content in a card-grid container
func Grid(content string) string {
	return `<div class="card-grid">` + content + `</div>`
}

// ListLayout wraps content in a card-list container
func ListLayout(content string) string {
	return `<div class="card-list">` + content + `</div>`
}

// Row wraps content in a card-row container
func Row(content string) string {
	return `<div class="card-row">` + content + `</div>`
}

// Common data structures for cards

// NewsData for news card
type NewsData struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Summary     string `json:"summary"`
	Time        string `json:"time"`
}

// VideoData for video card
type VideoData struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Channel   string `json:"channel"`
	Duration  string `json:"duration"`
	Views     string `json:"views"`
}

// NoteData for note card
type NoteData struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Color   string   `json:"color"`
	Pinned  bool     `json:"pinned"`
	Time    string   `json:"time"`
}

// MailData for mail card
type MailData struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Preview string `json:"preview"`
	Time    string `json:"time"`
	Unread  bool   `json:"unread"`
}

// BlogData for blog card
type BlogData struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Author  string   `json:"author"`
	Preview string   `json:"preview"`
	Tags    []string `json:"tags"`
	Time    string   `json:"time"`
}

// AppData for app card
type AppData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Author  string `json:"author"`
}

// ChatData for chat card
type ChatData struct {
	User   string `json:"user"`
	Text   string `json:"text"`
	Time   string `json:"time"`
	IsUser bool   `json:"is_user"`
}

// MarketData for market card
type MarketData struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
	Change string `json:"change"`
	Up     bool   `json:"up"`
}
