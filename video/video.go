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

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
	"mu/app"
	"mu/data"
)

//go:embed channels.json
var f embed.FS

var mutex sync.RWMutex

// category to channel mapping
var channels = map[string]string{}

// latest videos from channels
var videos = map[string]Channel{}

// latest video
var latestHtml string

// saved videos
var videosHtml string

// recent searches
var recentSearches []string

const maxRecentSearches = 10

type Channel struct {
	Videos []*Result `json:"videos"`
	Html   string    `json:"html"`
}

type Result struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Html        string    `json:"html"`
	Published   time.Time `json:"published"`
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
  .recent-searches {
    margin-bottom: 20px;
  }
  .recent-search-item {
    display: inline-block;
    margin: 5px 10px 5px 0;
    padding: 5px 10px;
    background-color: #f0f0f0;
    border-radius: 5px;
    text-decoration: none;
    color: #333;
  }
  .recent-search-item:hover {
    background-color: #e0e0e0;
  }
</style>
<form action="/video" method="POST">
  <input name="query" id="query" value="%s">
  <button>Search</button>
</form>
%s
<div id="topics">%s</div>
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
  .recent-searches {
    margin-bottom: 20px;
  }
  .recent-search-item {
    display: inline-block;
    margin: 5px 10px 5px 0;
    padding: 5px 10px;
    background-color: #f0f0f0;
    border-radius: 5px;
    text-decoration: none;
    color: #333;
  }
  .recent-search-item:hover {
    background-color: #e0e0e0;
  }
</style>
<!-- <form action="/video" method="POST" onsubmit="event.preventDefault(); getVideos(this); return false;"> -->
<form action="/video" method="POST">
  <input name="query" id="query" placeholder=Search autocomplete=off autofocus>
  <button>Search</button>
</form>
%s
<div id="topics">%s</div>
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

// loadRecentSearches loads recent searches from disk
func loadRecentSearches() {
	b, err := data.Load("recent_searches.json")
	if err != nil {
		return
	}
	mutex.Lock()
	if err := json.Unmarshal(b, &recentSearches); err != nil {
		fmt.Println("Error parsing recent_searches.json", err)
	}
	mutex.Unlock()
}

// saveRecentSearch adds a search query to recent searches
func saveRecentSearch(query string) {
	if len(strings.TrimSpace(query)) == 0 {
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Remove query if it already exists
	for i, search := range recentSearches {
		if search == query {
			recentSearches = append(recentSearches[:i], recentSearches[i+1:]...)
			break
		}
	}

	// Add to the beginning
	recentSearches = append([]string{query}, recentSearches...)

	// Keep only maxRecentSearches
	if len(recentSearches) > maxRecentSearches {
		recentSearches = recentSearches[:maxRecentSearches]
	}

	// Save to disk
	b, _ := json.Marshal(recentSearches)
	data.Save("recent_searches.json", string(b))
}

// getRecentSearchesHTML generates HTML for recent searches
func getRecentSearchesHTML() string {
	mutex.RLock()
	defer mutex.RUnlock()

	if len(recentSearches) == 0 {
		return ""
	}

	html := `<div class="recent-searches"><h3>Recent Searches</h3>`
	for _, search := range recentSearches {
		html += fmt.Sprintf(`<a href="#" class="recent-search-item" onclick="event.preventDefault(); document.getElementById('query').value='%s'; document.querySelector('form').submit();">%s</a>`, search, search)
	}
	html += `</div>`

	return html
}

// Load videos
func Load() {
	// load latest video
	b, _ := data.Load("latest.html")
	latestHtml = string(b)

	// load saved videos
	b, _ = data.Load("videos.html")
	videosHtml = string(b)

	b, _ = data.Load("videos.json")
	json.Unmarshal(b, &videos)

	// load channels
	loadChannels()

	// load recent searches
	loadRecentSearches()

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

	var latest []*Result

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
		latest = append(latest, res[0])

		vids[channel] = Channel{
			Videos: res,
			Html:   html,
		}

		// index at this point
		go func() {
			for _, res := range vids[channel].Videos {
				id := res.ID
				md := map[string]string{
					"id":          res.ID,
					"type":        res.Type,
					"topic":       channel,
					"title":       res.Title,
					"description": res.Description,
					"url":         res.URL,
					"posted":      res.Published.String(),
				}
				val := res.Html

				if err := data.Index(id, md, val); err != nil {
					fmt.Println("Failed to index video", err)
				}
			}
		}()
	}

	// sort the latest by date
	sort.Slice(latest, func(i, j int) bool {
		return latest[i].Published.After(latest[j].Published)
	})

	// add to body
	for _, res := range latest {
		body += res.Html
	}

	// get chan names and sort
	for channel, _ := range channels {
		chanNames = append(chanNames, channel)
	}

	// generate head
	head = app.Head("video", chanNames)

	// sort channel names
	sort.Strings(chanNames)

	// create head for channels
	for _, channel := range chanNames {
		body += `<div class=section>`
		body += `<hr id="` + channel + `" class="anchor">`
		body += fmt.Sprintf(`<h1>%s</h1>`, channel)
		body += vids[channel].Html
		body += `</div>`
	}

	vidHtml := app.RenderHTML("Video", "Search for videos", fmt.Sprintf(Template, getRecentSearchesHTML(), head, body))
	b, _ := json.Marshal(videos)
	vidJson := string(b)

	mutex.Lock()
	data.Save("videos.html", vidHtml)
	data.Save("videos.json", vidJson)
	data.Save("latest.html", latest[0].Html)
	latestHtml = latest[0].Html
	videos = vids
	videosHtml = vidHtml
	mutex.Unlock()

	time.Sleep(time.Hour)
	go loadVideos()
}

func embedVideo(id string) string {
	u := "https://www.youtube.com/embed/" + id + "?autoplay=1&mute=0"
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
			url = "/video?id=" + id
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

		desc = fmt.Sprintf(`<span class="highlight">%s</span> | <small>%s</small>`, kind, app.TimeAgo(t))

		res := &Result{
			ID:          id,
			Type:        kind,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
			URL:         url,
			Published:   t,
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
		desc = fmt.Sprintf(`<span class="highlight">%s</span> | <small>Published %s</small>`, kind, app.TimeAgo(t))

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
			ID:        id,
			Type:      kind,
			URL:       url,
			Published: t,
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

func Latest() string {
	mutex.RLock()
	defer mutex.RUnlock()
	return latestHtml
}

func Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	ct := r.Header.Get("Content-Type")

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

		if ct == "application/json" {
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

			// Save recent search
			if len(query) > 0 {
				saveRecentSearch(query)
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

		// Save recent search
		if len(query) > 0 {
			saveRecentSearch(query)
		}

		// fetch results from api
		results, _, err := getResults(query, chanId)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		html := app.RenderHTML("Video", query+" | Results", fmt.Sprintf(Results, query, getRecentSearchesHTML(), head, results))
		w.Write([]byte(html))
		return
	}

	// Watch video
	id := r.Form.Get("id")

	// render watch page
	if len(id) > 0 {
		// get the page
		tmpl := `<html>
  <head>
    <title>Video | Mu</title>
  </head>
  <body>
  %s
  </body>
</html>
`
		html := fmt.Sprintf(`<div class="video" style="padding-top: 100px">%s</div>`, embedVideo(id))
		rhtml := fmt.Sprintf(tmpl, html)
		w.Write([]byte(rhtml))

		return
	}

	// GET

	var b []byte
	mutex.RLock()
	if ct == "application/json" {
		data := map[string]interface{}{
			"channels": videos,
		}

		b, _ = json.Marshal(data)
	} else {
		b = []byte(videosHtml)
	}
	mutex.RUnlock()
	w.Write(b)

}
