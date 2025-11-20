package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/news"
	"mu/video"
)

var Template = `<div id="home">%s</div>`

func Cards(news, markets, reminder, posts, latest string) []string {
	news += app.Link("More", "/news")
	markets += app.Link("More", "/markets")
	posts += app.Link("More", "/blog")
	latest += app.Link("More", "/video")

	cards := []string{
		app.Card("news", "News", news),
		app.Card("reminder", "Reminder", reminder),
		app.Card("markets", "Markets", markets),
		app.Card("posts", "Posts", posts),
		app.Card("video", "Video", latest),
	}
	return cards
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
	headlines := news.Headlines()
	markets := news.Markets()
	reminder := news.Reminder()
	posts := blog.Preview()
	latest := video.Latest()

	// create homepage
	cards := strings.Join(Cards(headlines, markets, reminder, posts, latest), "\n")
	homepage := fmt.Sprintf(Template, cards)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
