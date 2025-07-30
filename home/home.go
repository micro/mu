package home

import (
	"fmt"
	"net/http"

	"github.com/micro/mu/app"
	"github.com/micro/mu/news"
)

var Home = `
<div id="home">
	<!-- news -->
	<div id="news" class="card">
	%s
	<a href="/news"><button>Read more</button></a>
	</div>

	<!-- video -->
	<div id="video" class="card">
	  <h1>Search Videos</h1>
	  <form action="/video" method="POST">
	    <input name="query" id="query" placeholder="Search for videos" autocomplete=off>
	    <button>Search</button>
	  </form>
	</div>

	<!-- chat -->
	<div id="chat" class="card">
	<h1>Chat with AI</h1>
	  <form action="/chat" method="POST" onsubmit="event.preventDefault(); askQuestion(this);">
	    <input name="prompt" id="prompt" placeholder="Ask a question" autocomplete=off>
	    <button>Submit</button>
	  </form>
	</div>
</div>
`

func Handler(w http.ResponseWriter, r *http.Request) {
	headlines := news.Headlines()

	// create homepage
	homepage := fmt.Sprintf(Home, headlines)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
