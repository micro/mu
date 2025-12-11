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
	"mu/auth"
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

var commonStyles = `
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
  .recent-searches h3 {
    margin-bottom: 10px;
    white-space: normal;
  }
  .recent-search-item {
    display: inline-block;
    margin: 5px 10px 5px 0;
    padding: 5px 10px;
    background-color: #f0f0f0;
    border-radius: 5px;
    text-decoration: none;
    color: #333;
    cursor: pointer;
    white-space: nowrap;
  }
  .recent-search-item:hover {
    background-color: #e0e0e0;
  }
  .recent-search-item.active {
    background-color: #333;
    color: white;
  }
  .recent-search-item.active .recent-search-close {
    color: #ccc;
  }
  .recent-search-item.active .recent-search-close:hover {
    color: white;
  }
	.recent-search-label {
		margin-right: 8px;
	}
	.recent-search-close {
		display: inline-block;
		padding: 0 6px;
		color: #666;
		cursor: pointer;
		font-weight: bold;
	}
	.recent-search-close:hover {
		color: #000;
	}
`

var recentSearchesScript = `
<script>
  const MAX_RECENT_SEARCHES = 10;
  const STORAGE_KEY = 'mu_recent_video_searches';

  function escapeHTML(text) {
    return text.replace(/&/g, '&amp;')
               .replace(/</g, '&lt;')
               .replace(/>/g, '&gt;')
               .replace(/"/g, '&quot;')
               .replace(/'/g, '&#039;');
  }

  function loadRecentSearches() {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch (e) {
      console.error('Error loading recent searches:', e);
      return [];
    }
  }

  function saveRecentSearch(query) {
    if (!query || !query.trim()) return;
    
    try {
      let searches = loadRecentSearches();
      
      // Remove if already exists
      searches = searches.filter(s => s !== query);
      
      // Add to beginning
      searches.unshift(query);
      
      // Keep only MAX_RECENT_SEARCHES
      if (searches.length > MAX_RECENT_SEARCHES) {
        searches = searches.slice(0, MAX_RECENT_SEARCHES);
      }
      
      localStorage.setItem(STORAGE_KEY, JSON.stringify(searches));
    } catch (e) {
      console.error('Error saving recent search:', e);
    }
  }

  function displayRecentSearches() {
    const searches = loadRecentSearches();
    const container = document.getElementById('recent-searches-container');
    
    if (!container) return;
    
    if (searches.length === 0) {
      container.innerHTML = '';
      return;
    }
    
		// Get current query from input to highlight active search
		const queryInput = document.getElementById('query');
		const currentQuery = queryInput ? queryInput.value.trim() : '';
    
		let html = '<div class="recent-searches"><h3>Recent Searches</h3><div class="recent-searches-scroll">';
		searches.forEach(search => {
			const escaped = escapeHTML(search);
			const isActive = currentQuery && search === currentQuery;
			const activeClass = isActive ? ' active' : '';
			// each item contains a label and a close button
			html += '<span class="recent-search-item' + activeClass + '" data-query="' + escaped + '">'
					 + '<span class="recent-search-label">' + escaped + '</span>'
					 + '<span class="recent-search-close" title="Remove">&times;</span>'
					 + '</span>';
		});
		html += '</div></div>';
    
    container.innerHTML = html;
    
    // Add click handlers
		// Clicking the label triggers a search, clicking the close removes it
		container.querySelectorAll('.recent-search-item').forEach(item => {
			const label = item.querySelector('.recent-search-label');
			const close = item.querySelector('.recent-search-close');

			if (label) {
				label.addEventListener('click', function(e) {
					e.preventDefault();
					e.stopPropagation();
					const query = item.getAttribute('data-query');
					
					// Move clicked search to front
					saveRecentSearch(query);
					
					const queryInput = document.getElementById('query');
					const form = item.closest('form') || document.querySelector('form');
					if (queryInput && form) {
						queryInput.value = query;
						form.submit();
					}
				});
			}

			if (close) {
				close.addEventListener('click', function(e) {
					e.preventDefault();
					e.stopPropagation();
					const q = item.getAttribute('data-query');
					removeRecentSearch(q);
				});
			}
		});
  }

	function removeRecentSearch(query) {
		try {
			let searches = loadRecentSearches();
			searches = searches.filter(s => s !== query);
			localStorage.setItem(STORAGE_KEY, JSON.stringify(searches));
			displayRecentSearches();
		} catch (e) {
			console.error('Error removing recent search:', e);
		}
	}

  // Save search when form is submitted
  document.addEventListener('DOMContentLoaded', function() {
    displayRecentSearches();
    
    const form = document.querySelector('form[action="/video"]');
    if (form) {
      form.addEventListener('submit', function() {
        const queryInput = document.getElementById('query');
        if (queryInput && queryInput.value && queryInput.value.trim()) {
          saveRecentSearch(queryInput.value.trim());
        }
      });
    }
  });
</script>
`

var Results = `
<style>` + commonStyles + `
</style>
<form id="video-search" action="/video" method="GET">
  <input name="query" id="query" value="%s">
  <button id="video-search-btn">Search</button>
</form>
<div id="topics">%s</div>
<div id="recent-searches-container"></div>
<h1>Results</h1>
<div id="results">
%s
</div>
` + recentSearchesScript

var PlaylistView = `
<style>` + commonStyles + `
</style>
<form id="video-search" action="/video" method="GET">
  <input name="query" id="query" placeholder="Search">
  <button>Search</button>
</form>
<div id="topics">%s</div>
<div id="recent-searches-container"></div>
<h1>Playlist</h1>
<div id="results">
%s
</div>
` + recentSearchesScript

var ChannelView = `
<style>` + commonStyles + `
</style>
<form id="video-search" action="/video" method="GET">
  <input name="query" id="query" placeholder="Search">
  <button>Search</button>
</form>
<div id="topics">%s</div>
<div id="recent-searches-container"></div>
<h1>Channel</h1>
%s
<div id="results">
%s
</div>
` + recentSearchesScript

var Template = `
<style>` + commonStyles + `
</style>
<!-- <form action="/video" method="POST" onsubmit="event.preventDefault(); getVideos(this); return false;"> -->
<form id="video-search" action="/video" method="GET">
  <input name="query" id="query" placeholder=Search autocomplete=off>
  <button>Search</button>
</form>
<div id="topics">%s</div>
<div id="recent-searches-container"></div>
<div>%s</div>
` + recentSearchesScript

func loadChannels() {
	// load the feeds file
	data, _ := f.ReadFile("channels.json")
	// unpack into feeds
	mutex.Lock()
	if err := json.Unmarshal(data, &channels); err != nil {
		app.Log("video", "Error parsing channels.json", err)
	}
	mutex.Unlock()
}

// Load videos
func Load() {
	// load latest video
	b, _ := data.LoadFile("latest.html")
	latestHtml = string(b)

	// load saved videos
	b, _ = data.LoadFile("videos.html")
	videosHtml = string(b)

	b, _ = data.LoadFile("videos.json")
	json.Unmarshal(b, &videos)

	// load channels
	loadChannels()

	// load fresh videos
	go loadVideos()
}

func loadVideos() {
	app.Log("video", "Loading videos")

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
			app.Log("video", "Error getting channel", channel, handle, err)
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

	vidHtml := app.RenderHTML("Video", "Search for videos", fmt.Sprintf(Template, head, body))
	b, _ := json.Marshal(videos)
	vidJson := string(b)

	mutex.Lock()
	data.SaveFile("videos.html", vidHtml)
	data.SaveFile("videos.json", vidJson)
	data.SaveFile("latest.html", latest[0].Html)
	latestHtml = latest[0].Html
	videos = vids
	videosHtml = vidHtml
	mutex.Unlock()

	time.Sleep(time.Hour)
	go loadVideos()
}

func embedVideo(id string) string {
	u := "https://www.youtube.com/embed/" + id
	style := `style="position: absolute; top: 0; left: 0; right: 0; width: 100%; height: 100%; border: none;"`
	return `<iframe width="560" height="315" ` + style + ` src="` + u + `" title="YouTube video player" frameborder="0" allow="accelerometer; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`
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

	app.Log("video", "Channel ID for @%s: %s\n", handle, channelID)
	app.Log("video", "Uploads Playlist ID: %s\n", uploadsPlaylistID)

	listVideosCall := Client.PlaylistItems.List([]string{"id", "snippet"}).PlaylistId(uploadsPlaylistID).MaxResults(25)
	resp, err := listVideosCall.Do()
	if err != nil {
		return "", nil, err
	}

	var results []*Result
	var resultsHtml string

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
			url = "/video?playlist=" + id
		case "channel":
			id = item.Snippet.ChannelId
			url = "/video?channel=" + id
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
			results = append([]*Result{res}, results...)
		} else {
			// returning json results
			results = append(results, res)
		}

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

		// All links are now internal
		html := fmt.Sprintf(`
		<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
		resultsHtml += html
		res.Html = html

		// Index the video for search/RAG
		data.Index(
			"video_"+id,
			"video",
			item.Snippet.Title,
			item.Snippet.Description,
			map[string]interface{}{
				"url":       url,
				"category":  category,
				"channel":   item.Snippet.ChannelTitle,
				"published": item.Snippet.PublishedAt,
				"thumbnail": item.Snippet.Thumbnails.Medium.Url,
			},
		)
	}

	return resultsHtml, results, nil
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

	var results []*Result
	var resultsHtml string

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
			url = "/video?playlist=" + id
		case "channel":
			id = item.Id.ChannelId
			url = "/video?channel=" + id
			desc = `<span class="highlight">channel</span>`
		}

		res := &Result{
			ID:        id,
			Type:      kind,
			URL:       url,
			Published: t,
		}

		if kind == "channel" {
			results = append([]*Result{res}, results...)
		} else {
			// returning json results
			results = append(results, res)
		}

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

		// All links are now internal
		html := fmt.Sprintf(`
			<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
		resultsHtml += html
		res.Html = html
	}

	return resultsHtml, results, nil
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
		head += fmt.Sprintf(`<a href="/video#%s" class="head">%s</a>`, channel, channel)
	}

	// Handle GET with query parameter (search)
	if r.Method == "GET" {
		query := r.URL.Query().Get("query")
		if len(query) > 0 {
			// Require authentication for search
			if _, err := auth.GetSession(r); err != nil {
				http.Error(w, "Authentication required to search", http.StatusUnauthorized)
				return
			}

			// Limit query length to prevent abuse
			if len(query) > 256 {
				http.Error(w, "Search query must not exceed 256 characters", http.StatusBadRequest)
				return
			}

			// fetch results from api
			results, _, err := getResults(query, "")
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			html := app.RenderHTML("Video", query+" | Results", fmt.Sprintf(Results, query, head, results))
			w.Write([]byte(html))
			return
		}
	}

	// if r.Method == "POST" {
	if r.Method == "POST" {
		// Require authentication for search
		if _, err := auth.GetSession(r); err != nil {
			http.Error(w, "Authentication required to search", http.StatusUnauthorized)
			return
		}

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

		head = ""

		html := app.RenderHTML("Video", query+" | Results", fmt.Sprintf(Results, query, head, results))
		w.Write([]byte(html))
		return
	}

	// Watch video
	id := r.Form.Get("id")
	playlistID := r.Form.Get("playlist")
	channelID := r.Form.Get("channel")

	// Handle playlist view
	if len(playlistID) > 0 {
		if Client == nil {
			http.Error(w, "YouTube API not available", 500)
			return
		}

		// Get playlist details
		playlistCall := Client.Playlists.List([]string{"snippet"}).Id(playlistID)
		playlistResp, err := playlistCall.Do()

		playlistTitle := "Playlist"
		playlistDesc := ""
		if err == nil && playlistResp != nil && len(playlistResp.Items) > 0 {
			playlistTitle = playlistResp.Items[0].Snippet.Title
			playlistDesc = playlistResp.Items[0].Snippet.Description
			if len(playlistDesc) > 500 {
				playlistDesc = playlistDesc[:500] + "..."
			}
			if playlistDesc != "" {
				playlistDesc = "<p>" + playlistDesc + "</p>"
			}
		}

		listVideosCall := Client.PlaylistItems.List([]string{"id", "snippet"}).PlaylistId(playlistID).MaxResults(50)
		resp, err := listVideosCall.Do()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var resultsHtml string
		for _, item := range resp.Items {
			if item.Snippet == nil || item.Snippet.ResourceId == nil {
				continue
			}
			videoID := item.Snippet.ResourceId.VideoId
			if videoID == "" {
				continue
			}

			t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			desc := fmt.Sprintf(`<span class="highlight">video</span> | <small>%s</small>`, app.TimeAgo(t))
			channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

			thumbnailURL := ""
			if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
				thumbnailURL = item.Snippet.Thumbnails.Medium.Url
			}

			html := fmt.Sprintf(`
		<div class="thumbnail"><a href="/video?id=%s"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
				videoID, thumbnailURL, item.Snippet.Title, channel, desc)
			resultsHtml += html
		}

		content := fmt.Sprintf(PlaylistView, head, playlistDesc+resultsHtml)
		html := app.RenderHTML("Video", playlistTitle, content)
		w.Write([]byte(html))
		return
	}

	// Handle channel view
	if len(channelID) > 0 {
		if Client == nil {
			http.Error(w, "YouTube API not available", 500)
			return
		}

		// Get channel uploads playlist
		channelCall := Client.Channels.List([]string{"contentDetails", "snippet"}).Id(channelID)
		channelResp, err := channelCall.Do()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		if len(channelResp.Items) == 0 {
			http.Error(w, "Channel not found", 404)
			return
		}

		channelTitle := channelResp.Items[0].Snippet.Title
		channelDesc := channelResp.Items[0].Snippet.Description
		if len(channelDesc) > 500 {
			channelDesc = channelDesc[:500] + "..."
		}
		channelInfo := ""
		if channelDesc != "" {
			channelInfo = "<p>" + channelDesc + "</p>"
		}

		uploadsPlaylistID := channelResp.Items[0].ContentDetails.RelatedPlaylists.Uploads

		listVideosCall := Client.PlaylistItems.List([]string{"id", "snippet"}).PlaylistId(uploadsPlaylistID).MaxResults(50)
		resp, err := listVideosCall.Do()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var resultsHtml string
		for _, item := range resp.Items {
			if item.Snippet == nil || item.Snippet.ResourceId == nil {
				continue
			}
			videoID := item.Snippet.ResourceId.VideoId
			if videoID == "" {
				continue
			}

			t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			desc := fmt.Sprintf(`<span class="highlight">video</span> | <small>%s</small>`, app.TimeAgo(t))
			channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

			thumbnailURL := ""
			if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
				thumbnailURL = item.Snippet.Thumbnails.Medium.Url
			}

			html := fmt.Sprintf(`
		<div class="thumbnail"><a href="/video?id=%s"><img src="%s"><h3>%s</h3></a>%s | %s</div>`,
				videoID, thumbnailURL, item.Snippet.Title, channel, desc)
			resultsHtml += html
		}

		content := fmt.Sprintf(ChannelView, head, channelInfo, resultsHtml)
		html := app.RenderHTML("Video", channelTitle, content)
		w.Write([]byte(html))
		return
	}

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
	if accept := r.Header.Get("Accept"); accept == "application/json" {
		data := map[string]interface{}{
			"channels": videos,
		}

		b, _ = json.Marshal(data)
		w.Header().Set("Content-Type", "application/json")
	} else {
		b = []byte(videosHtml)
	}
	mutex.RUnlock()
	w.Write(b)

}
