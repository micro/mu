// Package saved provides bookmarking/read-later functionality
package saved

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
)

// Item represents a saved/bookmarked item
type Item struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // blog, news, video, chat
	Title     string    `json:"title"`
	Content   string    `json:"content,omitempty"` // snippet/summary
	URL       string    `json:"url,omitempty"`
	Source    string    `json:"source,omitempty"`
	SavedAt   time.Time `json:"saved_at"`
	UserID    string    `json:"user_id"`
}

var (
	mu    sync.RWMutex
	items = make(map[string][]Item) // userID -> items
)

func init() {
	load()
}

func load() {
	b, err := data.LoadFile("saved.json")
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	json.Unmarshal(b, &items)
	app.Log("saved", "Loaded saved items for %d users", len(items))
}

func save() {
	mu.RLock()
	defer mu.RUnlock()
	b, _ := json.Marshal(items)
	data.SaveFile("saved.json", string(b))
}

// Save adds an item to user's saved list
func Save(userID string, item Item) {
	mu.Lock()
	defer mu.Unlock()
	
	item.UserID = userID
	item.SavedAt = time.Now()
	
	// Check if already saved
	for _, existing := range items[userID] {
		if existing.ID == item.ID && existing.Type == item.Type {
			return // Already saved
		}
	}
	
	items[userID] = append(items[userID], item)
	go save()
}

// Remove removes an item from user's saved list
func Remove(userID, itemID, itemType string) {
	mu.Lock()
	defer mu.Unlock()
	
	userItems := items[userID]
	for i, item := range userItems {
		if item.ID == itemID && item.Type == itemType {
			items[userID] = append(userItems[:i], userItems[i+1:]...)
			go save()
			return
		}
	}
}

// Get returns user's saved items
func Get(userID string) []Item {
	mu.RLock()
	defer mu.RUnlock()
	
	userItems := make([]Item, len(items[userID]))
	copy(userItems, items[userID])
	
	// Sort by saved date, newest first
	sort.Slice(userItems, func(i, j int) bool {
		return userItems[i].SavedAt.After(userItems[j].SavedAt)
	})
	
	return userItems
}

// IsSaved checks if an item is saved by user
func IsSaved(userID, itemID, itemType string) bool {
	mu.RLock()
	defer mu.RUnlock()
	
	for _, item := range items[userID] {
		if item.ID == itemID && item.Type == itemType {
			return true
		}
	}
	return false
}

// Handler serves the saved items page
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	
	userItems := Get(sess.Account)
	
	var content string
	if len(userItems) == 0 {
		content = app.Empty("No saved items yet")
	} else {
		for _, item := range userItems {
			var link string
			switch item.Type {
			case "blog":
				link = "/blog/" + item.ID
			case "news":
				link = item.URL
			case "video":
				link = "/video/play/" + item.ID
			case "chat":
				link = "/chat?id=" + item.ID
			default:
				link = item.URL
			}
			
			typeLabel := item.Type
			if item.Source != "" {
				typeLabel = item.Source
			}
			
			content += app.CardDiv(
				fmt.Sprintf(`<span class="category">%s</span>`, typeLabel) +
				app.Link(link, "<strong>"+item.Title+"</strong>") +
				app.Desc(item.Content) +
				app.Meta(app.TimeAgo(item.SavedAt)+" · "+
					`<a href="#" onclick="unsave('`+item.Type+`','`+item.ID+`'); return false;">Remove</a>`),
			)
		}
	}
	
	html := app.RenderHTMLForRequest("Saved", "Read later", 
		"<h1>Saved</h1>"+app.List(content)+
		`<script>
		function unsave(type, id) {
			fetch('/api/saved?type=' + type + '&id=' + id, {method: 'DELETE'})
				.then(() => location.reload());
		}
		</script>`, r)
	w.Write([]byte(html))
}

// APIHandler handles save/unsave API requests
func APIHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	switch r.Method {
	case "GET":
		// Check if item is saved
		itemType := r.URL.Query().Get("type")
		itemID := r.URL.Query().Get("id")
		if itemType != "" && itemID != "" {
			app.RespondJSON(w, map[string]bool{"saved": IsSaved(sess.Account, itemID, itemType)})
			return
		}
		// Return all saved items
		app.RespondJSON(w, Get(sess.Account))
		
	case "POST":
		var item Item
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		Save(sess.Account, item)
		app.RespondJSON(w, map[string]string{"status": "saved"})
		
	case "DELETE":
		itemType := r.URL.Query().Get("type")
		itemID := r.URL.Query().Get("id")
		Remove(sess.Account, itemID, itemType)
		app.RespondJSON(w, map[string]string{"status": "removed"})
	}
}
