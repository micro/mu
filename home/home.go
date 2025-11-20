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

var Template = `<div id="home"><div id="home-layout">%s</div></div>`

func Cards(news, markets, reminder, latest string) []string {
	news += app.Link("More", "/news")
	markets += app.Link("More", "/markets")
	latest += app.Link("More", "/video")

	cards := []string{
		app.Card("news", "News", news),
		app.Card("reminder", "Reminder", reminder),
		app.Card("markets", "Markets", markets),
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
	latest := video.Latest()

	// Get the posting form and limited feed
	postForm := blog.PostingForm("/home")
	homeFeed := blog.HomeFeed()

	// Create the main feed column with More link
	feedColumn := fmt.Sprintf(`<div id="feed-column">
		<h2>Posts</h2>
		%s
		<div id="posts-list">%s</div>
		<div style="text-align: center; margin-top: 30px;">
			<a href="/blog" style="padding: 10px 20px; background: #333; color: white; border-radius: 5px; text-decoration: none; display: inline-block;">View All Posts</a>
		</div>
	</div>`, postForm, homeFeed)

	// Create info cards sidebar
	cards := strings.Join(Cards(headlines, markets, reminder, latest), "\n")
	sidebar := fmt.Sprintf(`<div id="info-sidebar">%s</div>`, cards)
	
	// Combine feed and sidebar
	homepage := fmt.Sprintf(Template, feedColumn+"\n"+sidebar)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
