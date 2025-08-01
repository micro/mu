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
	<h4>News</h4>
	%s
	<a href="/news"><button>Read more</button></a>
	</div>

	<!-- markets -->
	<div id="markets" class="card">
	  <h4>Markets</h4>
	   %s
	</div>

	<!-- video -->
	<div id="video" class="card">
 	  <h4>Video</h4>
	  <form action="/video" method="POST">
	    <input name="query" id="query" placeholder="Search for videos" autocomplete=off>
	    <button>Search</button>
	  </form>
	</div>

	<!-- chat -->
	<div id="chat" class="card">
 	  <h4>Chat</h4>
	  <form action="/chat" method="POST" onsubmit="event.preventDefault(); askQuestion(this);">
	    <input name="prompt" id="prompt" placeholder="Ask a question" autocomplete=off>
	    <button>Submit</button>
	  </form>
	</div>
</div>
`

func Handler(w http.ResponseWriter, r *http.Request) {
	headlines := news.Headlines()
	markets := news.Markets()

	// create homepage
	homepage := fmt.Sprintf(Home, headlines, markets)

	// render html
	html := app.RenderHTML("Home", "The Mu homescreen", homepage)

	w.Write([]byte(html))
}
