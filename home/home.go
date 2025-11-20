package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
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
