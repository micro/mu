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
	Channel     string    `json:"channel,omitempty"`
	Category    string    `json:"category,omitempty"`
}

var Key = os.Getenv("YOUTUBE_API_KEY")

var Client *youtube.Service

func init() {
	var err error
	Client, err = youtube.NewService(context.TODO(), option.WithAPIKey(Key))
	if err != nil {
		app.Log("video", "Failed to initialize YouTube client: %v", err)
	}
	if Key == "" {
		app.Log("video", "WARNING: YOUTUBE_API_KEY environment variable not set")
	}
}

var commonStyles = `
  .thumbnail {
    margin-bottom: 20px;
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
  <button id="video-search-btn">Search</button>
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
  <button id="video-search-btn">Search</button>
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
  <button id="video-search-btn">Search</button>
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
	app.Log("video", "Loaded %d channels from channels.json", len(channels))
	mutex.Unlock()
}

// Load videos
func Load() {
	// load channels
	loadChannels()

	// load saved videos.json
	b, _ := data.LoadFile("videos.json")
	json.Unmarshal(b, &videos)

	app.Log("video", "Loaded %d channels from videos.json", len(videos))

	// Regenerate HTML from cached JSON data
	if len(videos) > 0 {
		regenerateHTML()
		app.Log("video", "Regenerated HTML from cached data")
	} else {
		// load saved HTML files if no JSON data
		b, _ = data.LoadFile("latest.html")
		latestHtml = string(b)

		b, _ = data.LoadFile("videos.html")
		videosHtml = string(b)
		app.Log("video", "No cached JSON, loaded HTML files")
	}

	// load fresh videos
	go loadVideos()
}

// regenerateHTML creates HTML from cached video data
func regenerateHTML() {
	mutex.Lock()
	defer mutex.Unlock()

	var head string
	var body strings.Builder
	var chanNames []string

	var latest []*Result

	// Collect latest from cached data
	for channel, channelData := range videos {
		if len(channelData.Videos) > 0 {
			video := channelData.Videos[0]
			// Populate Category if missing (for old cached data)
			if video.Category == "" {
				video.Category = channel
			}
			// Try to get Channel name from indexed metadata if missing
			if video.Channel == "" {
				if indexed := data.GetByID("video_" + video.ID); indexed != nil {
					if ch, ok := indexed.Metadata["channel"].(string); ok {
						video.Channel = ch
					}
				}
			}
			latest = append(latest, video)
		}
		chanNames = append(chanNames, channel)
	}

	// sort the latest by date
	sort.Slice(latest, func(i, j int) bool {
		return latest[i].Published.After(latest[j].Published)
	})

	// Generate latest HTML
	if len(latest) > 0 {
		res := latest[0]

		// Build category badge if available
		var categoryBadge string
		if res.Category != "" {
			categoryBadge = fmt.Sprintf(`<div style="margin-bottom: 5px;"><a href="/video#%s" class="highlight">%s</a></div>`,
				res.Category,
				res.Category)
		}

		thumbnailURL := fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", res.ID)

		// Build info section with channel if available
		var info string
		if res.Channel != "" {
			info = res.Channel + " · " + app.TimeAgo(res.Published)
		} else {
			info = app.TimeAgo(res.Published)
		}

		latestHtml = fmt.Sprintf(`
	<div class="thumbnail">%s<a href="%s"><img src="%s"><h3>%s</h3></a><div class="info">%s</div></div>`,
			categoryBadge, res.URL, thumbnailURL, res.Title, info)

		// add to body
		for _, res := range latest {
			body.WriteString(res.Html)
		}
	}

	// generate head
	head = app.Head("video", chanNames)

	// sort channel names
	sort.Strings(chanNames)

	// create body for channels
	for _, channel := range chanNames {
		body.WriteString(`<div class=section>`)
		body.WriteString(`<hr id="` + channel + `" class="anchor">`)
		fmt.Fprintf(&body, `<h1>%s</h1>`, channel)
		body.WriteString(videos[channel].Html)
		body.WriteString(`</div>`)
	}

	videosHtml = app.RenderHTML("Video", "Search for videos", fmt.Sprintf(Template, head, body.String()))
}

func loadVideos() {
	app.Log("video", "Loading videos")

	mutex.RLock()
	chans := channels
	mutex.RUnlock()

	vids := make(map[string]Channel)

	// create head
	var head string
	var body strings.Builder
	var chanNames []string

	var latest []*Result

	// get results
	for channel, handle := range chans {
		app.Log("video", "Fetching channel: %s (@%s)", channel, handle)
		html, res, err := getChannel(channel, handle)
		if err != nil {
			app.Log("video", "Error getting channel %s (@%s): %v", channel, handle, err)
			continue
		}
		if len(res) == 0 {
			app.Log("video", "No results for channel %s (@%s)", channel, handle)
			continue
		}
		app.Log("video", "Got %d videos for channel %s", len(res), channel)
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

	// Check if we got any videos
	if len(latest) == 0 {
		app.Log("video", "WARNING: No videos loaded from any channel. Check YouTube API key and channel handles.")
		mutex.Lock()
		videos = vids
		mutex.Unlock()
		time.Sleep(time.Hour)
		go loadVideos()
		return
	}

	// add to body
	for _, res := range latest {
		body.WriteString(res.Html)
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
		body.WriteString(`<div class=section>`)
		body.WriteString(`<hr id="` + channel + `" class="anchor">`)
		fmt.Fprintf(&body, `<h1>%s</h1>`, channel)
		body.WriteString(vids[channel].Html)
		body.WriteString(`</div>`)
	}

	vidHtml := app.RenderHTML("Video", "Search for videos", fmt.Sprintf(Template, head, body.String()))
	b, _ := json.Marshal(videos)
	vidJson := string(b)

	mutex.Lock()
	data.SaveFile("videos.html", vidHtml)
	data.SaveFile("videos.json", vidJson)
	if len(latest) > 0 {
		data.SaveFile("latest.html", latest[0].Html)
		latestHtml = latest[0].Html
	}
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
	var sb strings.Builder

	for _, item := range resp.Items {
		var id, url string
		kind := strings.Split(item.Kind, "#")[1]

		// Parse ISO 8601 timestamp with multiple format attempts
		var t time.Time
		var err error
		if t, err = time.Parse(time.RFC3339, item.Snippet.PublishedAt); err != nil {
			// Try with milliseconds
			if t, err = time.Parse("2006-01-02T15:04:05.000Z", item.Snippet.PublishedAt); err != nil {
				// Try without timezone
				if t, err = time.Parse("2006-01-02T15:04:05", item.Snippet.PublishedAt); err != nil {
					app.Log("video", "Failed to parse ISO 8601 timestamp for %s: '%s' - using current time", item.Snippet.Title, item.Snippet.PublishedAt)
					t = time.Now()
				}
			}
		}

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
		}

		res := &Result{
			ID:          id,
			Type:        kind,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
			URL:         url,
			Published:   t,
			Channel:     item.Snippet.ChannelTitle,
			Category:    category,
		}

		channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

		// Build category badge
		categoryBadge := fmt.Sprintf(`<div style="margin-bottom: 5px;"><a href="/video#%s" class="highlight">%s</a></div>`,
			category, category)

		// All links are now internal
		html := fmt.Sprintf(`
	<div class="thumbnail">%s<a href="%s"><img src="%s"><h3>%s</h3></a><div class="info">%s · %s</div></div>`,
			categoryBadge, url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, app.TimeAgo(t))
		sb.WriteString(html)
		res.Html = html

		// Append to results
		results = append(results, res)

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
				"published": t,
				"thumbnail": item.Snippet.Thumbnails.Medium.Url,
			},
		)
	}

	return sb.String(), results, nil
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
	var sb strings.Builder

	for _, item := range resp.Items {
		var id, url, desc string
		kind := strings.Split(item.Id.Kind, "#")[1]

		// Parse ISO 8601 timestamp with multiple format attempts
		var t time.Time
		var err error
		if t, err = time.Parse(time.RFC3339, item.Snippet.PublishedAt); err != nil {
			// Try with milliseconds
			if t, err = time.Parse("2006-01-02T15:04:05.000Z", item.Snippet.PublishedAt); err != nil {
				// Try without timezone
				if t, err = time.Parse("2006-01-02T15:04:05", item.Snippet.PublishedAt); err != nil {
					app.Log("video", "Failed to parse ISO 8601 timestamp for %s: '%s' - using current time", item.Snippet.Title, item.Snippet.PublishedAt)
					t = time.Now()
				}
			}
		}

		switch kind {
		case "video":
			id = item.Id.VideoId
			url = "/video?id=" + id
			desc = fmt.Sprintf(`%s · <span class="highlight">%s</span>`, app.TimeAgo(t), kind)
		case "playlist":
			id = item.Id.PlaylistId
			url = "/video?playlist=" + id
			desc = fmt.Sprintf(`%s · <span class="highlight">%s</span>`, app.TimeAgo(t), kind)
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
			<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a>%s · %s</div>`,
			url, item.Snippet.Thumbnails.Medium.Url, item.Snippet.Title, channel, desc)
		sb.WriteString(html)
		res.Html = html
	}

	return sb.String(), results, nil
}

func Latest() string {
	// Generate fresh HTML with current timestamps from cached data
	mutex.RLock()
	defer mutex.RUnlock()

	// Collect all latest videos from each channel
	var latest []*Result
	for _, channel := range videos {
		if len(channel.Videos) > 0 {
			latest = append(latest, channel.Videos[0])
		}
	}

	// Sort by published date
	sort.Slice(latest, func(i, j int) bool {
		return latest[i].Published.After(latest[j].Published)
	})

	// Get the most recent
	if len(latest) == 0 {
		return ""
	}

	// Use cached HTML but update timestamp
	// Parse the HTML to extract thumbnail and title, regenerate info section
	res := latest[0]

	// Build fresh description with current timestamp, channel, and category
	var desc string
	if res.Category != "" {
		desc = fmt.Sprintf(`%s · <a href="/video#%s" class="highlight">%s</a>`,
			app.TimeAgo(res.Published),
			res.Category,
			res.Category)
	} else {
		desc = app.TimeAgo(res.Published)
	}

	// Extract thumbnail URL from the cached HTML (simpler than storing it separately)
	// Or just use YouTube's thumbnail API which is predictable
	thumbnailURL := fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", res.ID)

	// Build info section with channel if available
	var info string
	if res.Channel != "" {
		info = res.Channel + " · " + desc
	} else {
		info = desc
	}

	html := fmt.Sprintf(`
	<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a><div class="info">%s</div></div>`,
		res.URL, thumbnailURL, res.Title, info)

	return html
}

func Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	ct := r.Header.Get("Content-Type")

	// create head
	var chanNames []string
	for channel, _ := range channels {
		chanNames = append(chanNames, channel)
	}
	sort.Strings(chanNames)
	
	var headSB strings.Builder
	for _, channel := range chanNames {
		fmt.Fprintf(&headSB, `<a href="/video#%s" class="head">%s</a>`, channel, channel)
	}
	head := headSB.String()

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

		var resultsSB strings.Builder
		for _, item := range resp.Items {
			if item.Snippet == nil || item.Snippet.ResourceId == nil {
				continue
			}
			videoID := item.Snippet.ResourceId.VideoId
			if videoID == "" {
				continue
			}

			t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			desc := fmt.Sprintf(`<span class="highlight">video</span> · <small>%s</small>`, app.TimeAgo(t))
			channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

			thumbnailURL := ""
			if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
				thumbnailURL = item.Snippet.Thumbnails.Medium.Url
			}

			fmt.Fprintf(&resultsSB, `
		<div class="thumbnail"><a href="/video?id=%s"><img src="%s"><h3>%s</h3></a>%s · %s</div>`,
				videoID, thumbnailURL, item.Snippet.Title, channel, desc)
		}

		content := fmt.Sprintf(PlaylistView, head, playlistDesc+resultsSB.String())
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

		var resultsSB strings.Builder
		for _, item := range resp.Items {
			if item.Snippet == nil || item.Snippet.ResourceId == nil {
				continue
			}
			videoID := item.Snippet.ResourceId.VideoId
			if videoID == "" {
				continue
			}

			t, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
			desc := fmt.Sprintf(`<span class="highlight">video</span> · <small>%s</small>`, app.TimeAgo(t))
			channel := fmt.Sprintf(`<a href="https://youtube.com/channel/%s" target="_blank">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

			thumbnailURL := ""
			if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
				thumbnailURL = item.Snippet.Thumbnails.Medium.Url
			}

			fmt.Fprintf(&resultsSB, `
		<div class="thumbnail"><a href="/video?id=%s"><img src="%s"><h3>%s</h3></a>%s · %s</div>`,
				videoID, thumbnailURL, item.Snippet.Title, channel, desc)
		}

		content := fmt.Sprintf(ChannelView, head, channelInfo, resultsSB.String())
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
