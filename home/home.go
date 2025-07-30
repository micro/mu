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
	    <input name="query" id="query" placeholder="Search for videos" autocomplete=off autofocus>
	    <button>Find Video</button>
	  </form>
	</div>

	<!-- chat -->
	<div class="card">
	<h1>Recent Chat</h1>
	<div id="chat">No messages</div>
	<a href="/chat"><button>Load Chat</button></a>
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
