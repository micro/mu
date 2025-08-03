package home

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/micro/mu/app"
	"github.com/micro/mu/news"
)

var Template = `<div id="home">%s</div>`

func Cards(news, markets, reminder string) []string {
	cards := []string{
		app.Card("news", "News", fmt.Sprintf(`%s<a href="/news"><button>Read more</button></a>`, news)),
		app.Card("reminder", "Reminder", reminder),
		app.Card("markets", "Markets", markets+`<a href=https://coinmarketcap.com/><button>Charts</button></a>`),
		app.Card("video", "Video", `
		  <form action="/video" method="POST">
		    <input name="query" id="query" placeholder="Search for videos" autocomplete=off>
		    <button>Search</button>
		  </form>`),
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

	// create homepage
	cards := strings.Join(Cards(headlines, markets, reminder), "\n")
	homepage := fmt.Sprintf(Template, cards)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
