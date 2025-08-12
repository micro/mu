package home

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/micro/mu/app"
	"github.com/micro/mu/news"
	"github.com/micro/mu/video"
)

var Template = `<div id="home">%s</div>`

func Cards(news, markets, reminder, latest string) []string {
	cards := []string{
		app.Card("news", "News", fmt.Sprintf(`%s<a href="/news" class="link">Read more</a>`, news)),
		app.Card("reminder", "Reminder", reminder),
		app.Card("markets", "Markets", markets+`<a href="https://coinmarketcap.com/" class="link">Charts</a>`),
		app.Card("video", "Video", latest+`<a href="/video" class="link">More</a>`),
		app.Card("chat", "Chat", `
			<form action="/chat" method="POST" onsubmit="event.preventDefault(); askQuestion(this);">
			  <input name="prompt" id="prompt" placeholder="Ask a question" autocomplete=off>
			  <button>Submit</button>
			</form>`),
	}
	return cards
}

func Handler(w http.ResponseWriter, r *http.Request) {
	headlines := news.Headlines()
	markets := news.Markets()
	reminder := news.Reminder()
	latest := video.Latest()

	// create homepage
	cards := strings.Join(Cards(headlines, markets, reminder, latest), "\n")
	homepage := fmt.Sprintf(Template, cards)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
