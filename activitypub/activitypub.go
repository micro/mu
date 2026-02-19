package activitypub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"mu/app"
	"mu/auth"
	"mu/blog"
)

// Domain returns the configured domain for ActivityPub URLs.
// Uses MU_DOMAIN env var, falls back to MAIL_DOMAIN, then "localhost".
func Domain() string {
	if d := os.Getenv("MU_DOMAIN"); d != "" {
		return d
	}
	if d := os.Getenv("MAIL_DOMAIN"); d != "" {
		return d
	}
	return "localhost"
}

// baseURL returns the base URL for the instance (e.g. "https://mu.xyz").
func baseURL() string {
	d := Domain()
	if d == "localhost" {
		return "http://localhost:8080"
	}
	return "https://" + d
}

// WantsActivityPub returns true if the request prefers ActivityPub JSON-LD.
func WantsActivityPub(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/activity+json") ||
		strings.Contains(accept, "application/ld+json")
}

// WebFingerHandler handles /.well-known/webfinger requests for user discovery.
func WebFingerHandler(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Query().Get("resource")
	if resource == "" {
		http.Error(w, "missing resource parameter", http.StatusBadRequest)
		return
	}

	// Parse acct:user@domain format
	if !strings.HasPrefix(resource, "acct:") {
		http.Error(w, "unsupported resource format", http.StatusBadRequest)
		return
	}

	acct := strings.TrimPrefix(resource, "acct:")
	parts := strings.SplitN(acct, "@", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid account format", http.StatusBadRequest)
		return
	}

	username := parts[0]
	domain := parts[1]

	if domain != Domain() {
		http.Error(w, "unknown domain", http.StatusNotFound)
		return
	}

	// Verify user exists
	_, err := auth.GetAccount(username)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	base := baseURL()

	response := map[string]interface{}{
		"subject": resource,
		"links": []map[string]string{
			{
				"rel":  "self",
				"type": "application/activity+json",
				"href": fmt.Sprintf("%s/@%s", base, username),
			},
		},
	}

	w.Header().Set("Content-Type", "application/jrd+json")
	json.NewEncoder(w).Encode(response)
}

// ActorHandler serves an ActivityPub actor profile as JSON-LD.
// It should be called when /@username receives an Accept: application/activity+json request.
func ActorHandler(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/@")
	username = strings.TrimSuffix(username, "/")

	if username == "" {
		http.Error(w, "missing username", http.StatusBadRequest)
		return
	}

	acc, err := auth.GetAccount(username)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	base := baseURL()
	actorID := fmt.Sprintf("%s/@%s", base, acc.ID)

	actor := map[string]interface{}{
		"@context": []string{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
		},
		"id":                actorID,
		"type":              "Person",
		"preferredUsername":  acc.ID,
		"name":              acc.Name,
		"url":               actorID,
		"inbox":             fmt.Sprintf("%s/@%s/inbox", base, acc.ID),
		"outbox":            fmt.Sprintf("%s/@%s/outbox", base, acc.ID),
		"summary":           fmt.Sprintf("@%s on Mu", acc.ID),
		"published":         acc.Created.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/activity+json")
	json.NewEncoder(w).Encode(actor)
}

// OutboxHandler serves a user's blog posts as an ActivityPub OrderedCollection.
func OutboxHandler(w http.ResponseWriter, r *http.Request) {
	// Extract username from path: /@username/outbox
	path := strings.TrimSuffix(r.URL.Path, "/outbox")
	username := strings.TrimPrefix(path, "/@")

	if username == "" {
		http.Error(w, "missing username", http.StatusBadRequest)
		return
	}

	acc, err := auth.GetAccount(username)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	base := baseURL()
	userPosts := blog.GetPostsByAuthor(acc.Name)

	// Filter out private posts
	var publicPosts []*blog.Post
	for _, post := range userPosts {
		if !post.Private {
			publicPosts = append(publicPosts, post)
		}
	}

	// Build ordered items
	items := make([]map[string]interface{}, 0, len(publicPosts))
	for _, post := range publicPosts {
		items = append(items, postToObject(base, acc, post))
	}

	collection := map[string]interface{}{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           fmt.Sprintf("%s/@%s/outbox", base, acc.ID),
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	}

	w.Header().Set("Content-Type", "application/activity+json")
	json.NewEncoder(w).Encode(collection)
}

// PostObjectHandler serves a single blog post as an ActivityPub Note object.
func PostObjectHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	post := blog.GetPost(id)
	if post == nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}

	if post.Private {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}

	base := baseURL()

	// Look up author account
	acc, err := auth.GetAccount(post.AuthorID)
	if err != nil {
		// Use minimal author info
		acc = &auth.Account{
			ID:   post.AuthorID,
			Name: post.Author,
		}
	}

	obj := postToObject(base, acc, post)

	w.Header().Set("Content-Type", "application/activity+json")
	json.NewEncoder(w).Encode(obj)
}

// InboxHandler accepts incoming ActivityPub messages (minimal stub).
// A full implementation would handle Follow, Undo, Create, etc.
func InboxHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	app.Log("activitypub", "Received inbox message from %s", r.RemoteAddr)

	// Accept the message but don't process it yet.
	// A full implementation would verify HTTP signatures and handle activities.
	w.WriteHeader(http.StatusAccepted)
}

// postToObject converts a blog post to an ActivityPub Note object.
func postToObject(base string, acc *auth.Account, post *blog.Post) map[string]interface{} {
	content := post.Content

	// Render markdown to HTML for the content field
	rendered := string(app.Render([]byte(content)))

	obj := map[string]interface{}{
		"@context":     "https://www.w3.org/ns/activitystreams",
		"id":           fmt.Sprintf("%s/post?id=%s", base, post.ID),
		"type":         "Note",
		"attributedTo": fmt.Sprintf("%s/@%s", base, acc.ID),
		"content":      rendered,
		"published":    post.CreatedAt.Format(time.RFC3339),
		"url":          fmt.Sprintf("%s/post?id=%s", base, post.ID),
		"to":           []string{"https://www.w3.org/ns/activitystreams#Public"},
	}

	if post.Title != "" {
		obj["name"] = post.Title
	}

	if post.Tags != "" {
		var tags []map[string]interface{}
		for _, tag := range strings.Split(post.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, map[string]interface{}{
					"type": "Hashtag",
					"name": "#" + strings.ToLower(tag),
				})
			}
		}
		if len(tags) > 0 {
			obj["tag"] = tags
		}
	}

	return obj
}
