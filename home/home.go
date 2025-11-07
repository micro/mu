package home

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
	"mu/mail"
	"mu/news"
	"mu/video"
)

var Template = `<div id="home">%s</div>`

func Cards(mailStatus, news, markets, reminder, latest string) []string {
	mailStatus += app.Link("More", "/mail")
	news += app.Link("More", "/news")
	latest += app.Link("More", "/video")

	cards := []string{
		app.Card("news", "News", news),
		app.Card("mail", "Mail", mailStatus),
		app.Card("reminder", "Reminder", reminder),
		app.Card("markets", "Markets", markets),
		app.Card("video", "Video", latest),
	}
	return cards
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// get the session
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}

	username := sess.Account
	mailStatus := mail.LatestMail(username)
	headlines := news.Headlines()
	markets := news.Markets()
	reminder := news.Reminder()
	latest := video.Latest()

	// create homepage
	cards := strings.Join(Cards(mailStatus, headlines, markets, reminder, latest), "\n")
	homepage := fmt.Sprintf(Template, cards)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
