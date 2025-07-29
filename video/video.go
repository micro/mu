package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/micro/mu/app"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Result struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

var Key = os.Getenv("YOUTUBE_API_KEY")
var Client, _ = youtube.NewService(context.TODO(), option.WithAPIKey(Key))

func embedVideo(id string) string {
	u := "https://www.youtube.com/embed/" + id
	style := `style="position: absolute; top: 0; left: 0; right: 0; width: 100%; height: 100%; border: none;"`
	return `<iframe width="560" height="315" ` + style + ` src="` + u + `" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`
}

func getResults(q string) (string, []*Result, error) {
	if Client == nil {
		return "", nil, fmt.Errorf("No client")
	}
	resp, err := Client.Search.List([]string{"id", "snippet"}).Q(q).MaxResults(25).Do()
	if err != nil {
		return "", nil, err
	}

	var data []*Result
	var results string

	for _, item := range resp.Items {
		var id, url, desc string
		kind := strings.Split(item.Id.Kind, "#")[1]
		t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
		desc = fmt.Sprintf(`[%s] published on %s`, kind, t.Format(time.RFC822))

		switch kind {
		case "video":
			id = item.Id.VideoId
			url = "/video?id=" + id
		case "playlist":
			id = item.Id.PlaylistId
			url = "https://youtube.com/playlist?list=" + id
		case "channel":
			id = item.Id.ChannelId
			url = "https://www.youtube.com/channel/" + id
			desc = "[channel]"
		}

		// returning json results
		data = append(data, &Result{
			ID:   id,
			Type: kind,
			URL:  url,
		})

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)
		results += fmt.Sprintf(`
			<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
	}

	return results, data, nil
}

var Results = `
<style>
  .thumbnail {
    margin-bottom: 50px;
  }
  img {
    border-radius: 10px;
  }
  h3 {
    margin-bottom: 5px;
  }
</style>
<form action="/video" method="POST">
  <input name="q" id="q" value="%s">
  <button>-></button>
</form>
<h1>Results</h1>
<div id="results">
%s
</div>`

var Template = `
<style>
.video {
  width: 100%;
  overflow: hidden;
  padding-top: 56.25%;
  position: relative;
}
</style>
<form action="/video" method="POST">
  <input name="q" id="q" placeholder=Search autocomplete=off autofocus>
  <button>-></button>
</form>`

func Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// if r.Method == "POST" {
	if r.Method == "POST" {
		var q string

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			var data map[string]interface{}

			b, _ := ioutil.ReadAll(r.Body)
			json.Unmarshal(b, &data)

			if v := data["q"]; v != nil {
				q = fmt.Sprintf("%v", v)
			} else {
				return
			}

			// fetch results from api
			_, results, err := getResults(q)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			b, _ = json.Marshal(results)
			w.Write(b)
			return
		}

		q = r.Form.Get("q")

		// fetch results from api
		results, _, err := getResults(q)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		html := app.RenderHTML("Video", q+" | Results", fmt.Sprintf(Results, q, results))
		w.Write([]byte(html))
		return
	}

	// Watch video
	id := r.Form.Get("id")

	// render watch page
	if len(id) > 0 {
		// get the page
		html := fmt.Sprintf(`<div class="video" style="padding-top: 100px">%s</div>`, embedVideo(id))

		rhtml := app.RenderHTML("Video", id, html)
		w.Write([]byte(rhtml))

		return
	}

	// GET
	html := app.RenderHTML("Video", "Search for videos", Template)
	w.Write([]byte(html))
}
