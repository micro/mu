package video

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/micro/mu/app"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

//go:embed channels.json
var f embed.FS

var mutex sync.RWMutex

var channels = map[string]string{}

var videos = map[string]Channel{}

// saved videos
var videosHtml string

type Channel struct {
	Videos []*Result
	Html   string
}

type Result struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	Html string `json:"html"`
}

var Key = os.Getenv("YOUTUBE_API_KEY")
var Client, _ = youtube.NewService(context.TODO(), option.WithAPIKey(Key))

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
  <input name="query" id="query" value="%s">
  <button>Search</button>
</form>
<div>%s</div>
<h1>Results</h1>
<div id="results">
%s
</div>`

var Template = `
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
<!-- <form action="/video" method="POST" onsubmit="event.preventDefault(); getVideos(this); return false;"> -->
<form action="/video" method="POST">
  <input name="query" id="query" placeholder=Search autocomplete=off autofocus>
  <button>Search</button>
</form>
<div>%s</div>
<div>%s</div>
`

func loadChannels() {
	// load the feeds file
	data, _ := f.ReadFile("channels.json")
	// unpack into feeds
	mutex.Lock()
	if err := json.Unmarshal(data, &channels); err != nil {
		fmt.Println("Error parsing channels.json", err)
	}
	mutex.Unlock()
}

// Load videos
func Load() {
	// load saved videos
	b, _ := app.Load("videos.html")
	videosHtml = string(b)

	// load channels
	loadChannels()

	// load fresh videos
	go loadVideos()
}

func loadVideos() {
	fmt.Println("Loading videos")

	mutex.RLock()
	chans := channels
	mutex.RUnlock()

	vids := make(map[string]Channel)

	// create head
	var head string
	var body string
	var chanNames []string
	body += `<h1>Latest</h1>`

	// get results
	for channel, handle := range chans {
		html, res, err := getChannel(channel, handle)
		if err != nil {
			fmt.Println("Error getting channel", channel, handle, err)
			continue
		}
		if len(res) == 0 {
			continue
		}
		// latest
		body += res[0].Html

		vids[channel] = Channel{
			Videos: res,
			Html:   html,
		}
	}

	// get chan names and sort
	for channel, _ := range channels {
		chanNames = append(chanNames, channel)
	}

	sort.Strings(chanNames)

	// create head for channels
	for _, channel := range chanNames {
		head += fmt.Sprintf(`<a href="#%s" class="head">%s</a>`, channel, channel)
		body += `<div class=section>`
		body += `<hr id="` + channel + `" class="anchor">`
		body += fmt.Sprintf(`<h1>%s</h1>`, channel)
		body += vids[channel].Html
		body += `</div>`
	}
	head += `<hr>`

	vidHtml := app.RenderHTML("Video", "Search for videos", fmt.Sprintf(Template, head, body))

	mutex.Lock()
	app.Save("videos.html", vidHtml)
	videosHtml = vidHtml
	mutex.Unlock()

	time.Sleep(time.Hour)
	go loadVideos()
}

func embedVideo(id string) string {
	u := "https://www.youtube.com/embed/" + id
	style := `style="position: absolute; top: 0; left: 0; right: 0; width: 100%; height: 100%; border: none;"`
	return `<iframe width="560" height="315" ` + style + ` src="` + u + `" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`
}

func getChannel(category, handle string) (string, []*Result, error) {
	if Client == nil {
		return "", nil, fmt.Errorf("No client")
	}

	// Get the channel details using the handle
	call := Client.Channels.List([]string{"contentDetails"}).ForHandle(handle)
	response, err := call.Do()
	if err != nil {
		return "", nil, err
	}

	if len(response.Items) == 0 {
		return "", nil, errors.New("no items")
	}

	channel := response.Items[0]
	uploadsPlaylistID := channel.ContentDetails.RelatedPlaylists.Uploads
	channelID := channel.Id

	fmt.Printf("Channel ID for @%s: %s\n", handle, channelID)
	fmt.Printf("Uploads Playlist ID: %s\n", uploadsPlaylistID)

	listVideosCall := Client.PlaylistItems.List([]string{"id", "snippet"}).PlaylistId(uploadsPlaylistID).MaxResults(25)
	resp, err := listVideosCall.Do()
	if err != nil {
		return "", nil, err
	}

	var data []*Result
	var results string

	for _, item := range resp.Items {
		var id, url, desc string
		kind := strings.Split(item.Kind, "#")[1]
		t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)

		switch kind {
		case "playlistItem":
			id = item.Snippet.ResourceId.VideoId
			kind = category
		case "video":
			id = item.Snippet.ResourceId.VideoId
			url = "/video?id=" + id
		case "playlist":
			id = item.Snippet.PlaylistId
			url = "https://youtube.com/playlist?list=" + id
		case "channel":
			id = item.Snippet.ChannelId
			url = "https://www.youtube.com/channel/" + id
			desc = `<span class="highlight">channel</span>`
		}

		desc = fmt.Sprintf(`<span class="highlight">%s</span> | <small>Published %s</small>`, kind, timeAgo(t))

		res := &Result{
			ID:   id,
			Type: kind,
			URL:  url,
		}

		if kind == "channel" {
			data = append([]*Result{res}, data...)
		} else {
			// returning json results
			data = append(data, res)
		}

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)
		html := fmt.Sprintf(`
			<div class="thumbnail"><a href="%s" target="_blank"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
		results += html
		res.Html = html
	}

	return results, data, nil
}

func getResults(query, channel string) (string, []*Result, error) {
	if Client == nil {
		return "", nil, fmt.Errorf("No client")
	}

	scall := Client.Search.List([]string{"id", "snippet"}).SafeSearch("strict").MaxResults(25)

	if len(channel) > 0 {
		scall = scall.ChannelId(channel)
	}

	if len(query) > 0 {
		scall = scall.Q(query)
	}

	resp, err := scall.Do()
	if err != nil {
		return "", nil, err
	}

	var data []*Result
	var results string

	for _, item := range resp.Items {
		var id, url, desc string
		kind := strings.Split(item.Id.Kind, "#")[1]
		t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
		desc = fmt.Sprintf(`<span class="highlight">%s</span> | <small>Published %s</small>`, kind, timeAgo(t))

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
			desc = `<span class="highlight">channel</span>`
		}

		res := &Result{
			ID:   id,
			Type: kind,
			URL:  url,
		}

		if kind == "channel" {
			data = append([]*Result{res}, data...)
		} else {
			// returning json results
			data = append(data, res)
		}

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)
		html := fmt.Sprintf(`
			<div class="thumbnail"><a href="%s" target="_blank"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
		results += html
		res.Html = html
	}

	return results, data, nil
}

func Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	// create head
	var head string
	var chanNames []string
	for channel, _ := range channels {
		chanNames = append(chanNames, channel)
	}
	sort.Strings(chanNames)
	for _, channel := range chanNames {
		head += fmt.Sprintf(`<a href="#%s" class="head">%s</a>`, channel, channel)
	}
	head += `<hr>`

	// if r.Method == "POST" {
	if r.Method == "POST" {
		var query string
		var channel string

		if ct := r.Header.Get("Content-Type"); ct == "application/json" {
			var data map[string]interface{}

			b, _ := ioutil.ReadAll(r.Body)
			json.Unmarshal(b, &data)

			if v := data["query"]; v != nil {
				query = fmt.Sprintf("%v", v)
			}

			if v := data["channel"]; v != nil {
				channel = fmt.Sprintf("%v", v)
			}

			mutex.RLock()
			chanId := channels[channel]
			mutex.RUnlock()

			if len(query) == 0 && len(chanId) == 0 {
				return
			}

			// fetch results from api
			html, results, err := getResults(query, chanId)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			res := map[string]interface{}{
				"results": results,
				"html":    html,
			}
			b, _ = json.Marshal(res)
			w.Write(b)
			return
		}

		query = r.Form.Get("query")
		channel = r.Form.Get("channel")
		mutex.RLock()
		chanId := channels[channel]
		mutex.RUnlock()

		if len(query) == 0 && len(chanId) == 0 {
			return
		}

		// fetch results from api
		results, _, err := getResults(query, chanId)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		html := app.RenderHTML("Video", query+" | Results", fmt.Sprintf(Results, query, head, results))
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
	mutex.RLock()
	html := videosHtml
	mutex.RUnlock()

	w.Write([]byte(html))
}
