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
	"mu/wallet"
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
	ChannelID   string    `json:"channel_id,omitempty"`
	Category    string    `json:"category,omitempty"`
	PlaylistID  string    `json:"playlist_id,omitempty"`
	Thumbnail   string    `json:"thumbnail,omitempty"`
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
	.summarize-link {
		color: #666;
		text-decoration: none;
		cursor: pointer;
	}
	.summarize-link:hover {
		color: #333;
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
					const form = document.getElementById('video-search');
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
			// Try to get Channel name and ID from indexed metadata if missing
			if video.Channel == "" || video.ChannelID == "" {
				if indexed := data.GetByID("video_" + video.ID); indexed != nil {
					if video.Channel == "" {
						if ch, ok := indexed.Metadata["channel"].(string); ok {
							video.Channel = ch
						}
					}
					if video.ChannelID == "" {
						if chID, ok := indexed.Metadata["channel_id"].(string); ok {
							video.ChannelID = chID
						}
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

		thumbnailURL := res.Thumbnail
		if thumbnailURL == "" {
			thumbnailURL = fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", res.ID)
		}

		// Build info section with channel and category
		var info string
		if res.Channel != "" {
			channelLink := res.Channel
			if res.ChannelID != "" {
				channelLink = fmt.Sprintf(`<a href="/video?channel=%s">%s</a>`, res.ChannelID, res.Channel)
		info += fmt.Sprintf(` · <a href="%s&summarize=1" class="summarize-link">✨ Summarize</a>`, res.URL)
			}
			info = fmt.Sprintf(`%s · <span data-timestamp="%d">%s</span>`, channelLink, res.Published.Unix(), app.TimeAgo(res.Published))
		} else {
			info = fmt.Sprintf(`<span data-timestamp="%d">%s</span>`, res.Published.Unix(), app.TimeAgo(res.Published))
		}
		if res.Category != "" {
			info += fmt.Sprintf(` · <a href="/video#%s" class="highlight">%s</a>`, res.Category, res.Category)
		}

		latestHtml = fmt.Sprintf(`
	<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a><div class="info">%s</div></div>`,
			res.URL, thumbnailURL, res.Title, info)

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
	b, _ := json.Marshal(vids)
	vidJson := string(b)

	mutex.Lock()
	data.SaveFile("videos.html", vidHtml)
	data.SaveFile("videos.json", vidJson)
	if len(latest) > 0 {
		// Generate proper latest HTML with channel and category links
		res := latest[0]
		thumbnailURL := res.Thumbnail
		if thumbnailURL == "" {
			thumbnailURL = fmt.Sprintf("https://i.ytimg.com/vi/%s/mqdefault.jpg", res.ID)
		}

		var info string
		if res.Channel != "" {
			channelLink := res.Channel
			if res.ChannelID != "" {
		info += fmt.Sprintf(` · <a href="%s&summarize=1" class="summarize-link">✨ Summarize</a>`, res.URL)
				channelLink = fmt.Sprintf(`<a href="/video?channel=%s">%s</a>`, res.ChannelID, res.Channel)
			}
			info = fmt.Sprintf(`%s · <span data-timestamp="%d">%s</span>`, channelLink, res.Published.Unix(), app.TimeAgo(res.Published))
		} else {
			info = fmt.Sprintf(`<span data-timestamp="%d">%s</span>`, res.Published.Unix(), app.TimeAgo(res.Published))
		}
		if res.Category != "" {
			info += fmt.Sprintf(` · <a href="/video#%s" class="highlight">%s</a>`, res.Category, res.Category)
		}

		latestHtml = fmt.Sprintf(`
	<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a><div class="info">%s</div></div>`,
			res.URL, thumbnailURL, res.Title, info)
		data.SaveFile("latest.html", latestHtml)
	}
	videos = vids
	videosHtml = vidHtml
	mutex.Unlock()

	time.Sleep(time.Hour)
	go loadVideos()
}

func embedVideo(id string) string {
	return embedVideoWithAutoplay(id, false)
}

func embedVideoWithAutoplay(id string, autoplay bool) string {
	u := "https://www.youtube.com/embed/" + id
	if autoplay {
		u += "?autoplay=1"
	}
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

		thumbnailURL := ""
		if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
			thumbnailURL = item.Snippet.Thumbnails.Medium.Url
		}

		res := &Result{
			ID:          id,
			Type:        kind,
			Title:       item.Snippet.Title,
			Description: item.Snippet.Description,
			URL:         url,
			Published:   t,
			Channel:     item.Snippet.ChannelTitle,
			ChannelID:   item.Snippet.ChannelId,
			Category:    category,
			PlaylistID:  uploadsPlaylistID,
			Thumbnail:   thumbnailURL,
		}

		// All links are now internal
		html := fmt.Sprintf(`
	<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a><div class="info"><a href="/video?channel=%s">%s</a> · %s · <a href="/video#%s" class="highlight">%s</a> · <a href="%s&summarize=1" class="summarize-link">✨ Summarize</a></div></div>`,
			url, thumbnailURL, item.Snippet.Title, item.Snippet.ChannelId, item.Snippet.ChannelTitle, app.TimeAgo(t), category, category, url)
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
				"url":        url,
				"category":   category,
				"channel":    item.Snippet.ChannelTitle,
				"channel_id": item.Snippet.ChannelId,
				"published":  t,
				"thumbnail":  thumbnailURL,
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

		thumbnailURL := ""
		if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
			thumbnailURL = item.Snippet.Thumbnails.Medium.Url
		}

		res := &Result{
			ID:        id,
			Type:      kind,
			Title:     item.Snippet.Title,
			URL:       url,
			Published: t,
			Channel:   item.Snippet.ChannelTitle,
			ChannelID: item.Snippet.ChannelId,
			Thumbnail: thumbnailURL,
		}

		if kind == "playlist" {
			res.PlaylistID = id
		}

		if kind == "channel" {
			results = append([]*Result{res}, results...)
		} else {
			// returning json results
			results = append(results, res)
		}

		// All links are now internal
		html := fmt.Sprintf(`
			<div class="thumbnail"><a href="%s"><img src="%s"><h3>%s</h3></a><a href="/video?channel=%s">%s</a> · %s · <a href="%s&summarize=1" class="summarize-link">✨ Summarize</a></div>`,
			url, thumbnailURL, item.Snippet.Title, item.Snippet.ChannelId, item.Snippet.ChannelTitle, desc, url)
		sb.WriteString(html)
		res.Html = html
	}

	return sb.String(), results, nil
}

func Latest() string {
	// Use cached HTML for efficiency
	mutex.RLock()
	defer mutex.RUnlock()

	return latestHtml
}

func Handler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	ct := r.Header.Get("Content-Type")

	// Handle summarize action
	if r.Method == "GET" && r.URL.Query().Get("action") == "summarize" {
		videoID := r.URL.Query().Get("id")
		if videoID == "" {
			app.RespondJSON(w, map[string]string{"error": "Missing video ID"})
			return
		}
		
		// Require authentication
		_, _, err := auth.RequireSession(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			w.Write([]byte(`{"error": "Login required"}`))
			return
		}
		
		// Get summary
		summary, err := SummarizeVideo(videoID, "")
		if err != nil {
			app.RespondJSON(w, map[string]string{"error": err.Error()})
			return
		}
		
		app.RespondJSON(w, map[string]string{"summary": summary.Summary})
		return
	}

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
			sess, _, err := auth.RequireSession(r)
			if err != nil {
				app.Unauthorized(w, r)
				return
			}

			// Limit query length to prevent abuse
			if len(query) > 256 {
				app.BadRequest(w, r, "Search query must not exceed 256 characters")
				return
			}

			// Check quota before search
			canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpVideoSearch)
			if !canProceed {
				content := wallet.QuotaExceededPage(wallet.OpVideoSearch, cost)
				html := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", content, r)
				w.Write([]byte(html))
				return
			}

			// fetch results from api
			results, _, err := getResults(query, "")
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			// Consume quota after successful search
			wallet.ConsumeQuota(sess.Account, wallet.OpVideoSearch)

			html := app.RenderHTML("Video", query+" | Results", fmt.Sprintf(Results, query, head, results))
			w.Write([]byte(html))
			return
		}
	}

	// if r.Method == "POST" {
	if r.Method == "POST" {
		// Require authentication for search
		sess, _, err := auth.RequireSession(r)
		if err != nil {
			app.Unauthorized(w, r)
			return
		}

		var query string
		var channel string

		if ct == "application/json" {
			var reqData map[string]interface{}

			b, _ := ioutil.ReadAll(r.Body)
			json.Unmarshal(b, &reqData)

			if v := reqData["query"]; v != nil {
				query = fmt.Sprintf("%v", v)
			}

			if v := reqData["channel"]; v != nil {
				channel = fmt.Sprintf("%v", v)
			}

			mutex.RLock()
			chanId := channels[channel]
			mutex.RUnlock()

			if len(query) == 0 && len(chanId) == 0 {
				return
			}

			// Check quota before search (only for actual queries, not channel browsing)
			if len(query) > 0 {
				canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpVideoSearch)
				if !canProceed {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(402) // Payment Required
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error":   "quota_exceeded",
						"message": "Daily search limit reached. Please top up credits or upgrade to member.",
						"cost":    cost,
					})
					return
				}
			}

			// fetch results from api
			html, results, err := getResults(query, chanId)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			// Consume quota after successful search (only for actual queries)
			if len(query) > 0 {
				wallet.ConsumeQuota(sess.Account, wallet.OpVideoSearch)
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

		// Check quota before search (only for actual queries, not channel browsing)
		if len(query) > 0 {
			canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpVideoSearch)
			if !canProceed {
				content := wallet.QuotaExceededPage(wallet.OpVideoSearch, cost)
				html := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", content, r)
				w.Write([]byte(html))
				return
			}
		}

		// fetch results from api
		results, _, err := getResults(query, chanId)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Consume quota after successful search (only for actual queries)
		if len(query) > 0 {
			wallet.ConsumeQuota(sess.Account, wallet.OpVideoSearch)
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
			channel := fmt.Sprintf(`<a href="/video?channel=%s">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

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
			channel := fmt.Sprintf(`<a href="/video?channel=%s">%s</a>`, item.Snippet.ChannelId, item.Snippet.ChannelTitle)

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
		youtubeURL := "https://www.youtube.com/watch?v=" + id
		thumbnailURL := "https://img.youtube.com/vi/" + id + "/maxresdefault.jpg"

		// Check if user is authenticated
		sess, _ := auth.TrySession(r)
		isGuest := sess == nil

		// For guests: show thumbnail with options to login or go to YouTube
		if isGuest {
			guestHtml := fmt.Sprintf(`
				<div class="card" style="text-align: center; padding: 40px; max-width: 640px; margin: 40px auto;">
					<img src="%s" style="width: 100%%; border-radius: 8px; margin-bottom: 20px;" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
					<h2>Watch Video</h2>
					<p style="color: #666; margin: 20px 0;">Login to watch ad-free, or view on YouTube.</p>
					<p style="margin: 20px 0;">
						<a href="/login?redirect=/video?id=%s" style="display: inline-block; padding: 10px 20px; background: #000; color: #fff; text-decoration: none; border-radius: 4px; margin-right: 10px;">Login to watch</a>
						<a href="%s" target="_blank" rel="noopener noreferrer" style="display: inline-block; padding: 10px 20px; border: 1px solid #000; text-decoration: none; border-radius: 4px;">Watch on YouTube →</a>
					</p>
					<p style="margin-top: 20px;"><a href="/video">← Back to videos</a></p>
				</div>
			`, thumbnailURL, id, id, youtubeURL)
			pageHTML := fmt.Sprintf(app.Template, "en", "Video", "Video", "", "", "", guestHtml)
			w.Write([]byte(pageHTML))
			return
		}

		// Check quota for watching video
		canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpVideoWatch)
		if !canProceed {
			// Show paywall page with YouTube fallback
			credits := "credit"
			if cost != 1 {
				credits = "credits"
			}
			paywallHtml := fmt.Sprintf(`
				<div class="card" style="text-align: center; padding: 40px; max-width: 640px; margin: 40px auto;">
					<img src="%s" style="width: 100%%; border-radius: 8px; margin-bottom: 20px;" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
					<h2>Watch Video</h2>
					<p>Watching ad-free videos costs %d %s.</p>
					<p style="color: #666; margin: 20px 0;">Your balance: %d credits</p>
					<p style="margin: 20px 0;">
						<a href="/wallet/topup" style="display: inline-block; padding: 10px 20px; background: #000; color: #fff; text-decoration: none; border-radius: 4px; margin-right: 10px;">Top up credits</a>
						<a href="%s" target="_blank" rel="noopener noreferrer" style="display: inline-block; padding: 10px 20px; border: 1px solid #000; text-decoration: none; border-radius: 4px;">Watch on YouTube →</a>
					</p>
					<p style="margin-top: 20px;"><a href="/video">← Back to videos</a></p>
				</div>
			`, thumbnailURL, id, cost, credits, wallet.GetBalance(sess.Account), youtubeURL)
			pageHTML := fmt.Sprintf(app.Template, "en", "Credits Required", "Credits Required", "", "", "", paywallHtml)
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(pageHTML))
			return
		}

		// Consume quota for video watch
		wallet.ConsumeQuota(sess.Account, wallet.OpVideoWatch)

		// Check if autoplay is requested
		autoplay := r.Form.Get("autoplay") == "1"

		// get the page with summarize button
		tmpl := `<!DOCTYPE html>
<html>
  <head>
    <title>Video | Mu</title>
    <link rel="stylesheet" href="/mu.css">
    <style>
      .video-container { max-width: 900px; margin: 20px auto 0; padding: 0 20px; }
      .video-wrapper { position: relative; padding-bottom: 56.25%%; height: 0; overflow: hidden; }
      .video-wrapper iframe { position: absolute; top: 0; left: 0; width: 100%%; height: 100%%; }
      .video-actions { margin-top: 20px; text-align: center; }
      .video-actions button { padding: 10px 20px; font-size: 16px; cursor: pointer; background: #333; color: white; border: none; border-radius: 6px; }
      .video-actions button:hover { background: #555; }
      .video-actions button:disabled { background: #ccc; cursor: not-allowed; }
      #summary { margin-top: 20px; padding: 20px; background: #f9f9f9; border-radius: 8px; display: none; }
      #summary h3 { margin-top: 0; }
      #summary-content { white-space: pre-wrap; line-height: 1.6; }
      .loading { color: #666; font-style: italic; }
      .error { color: #c00; }
    </style>
  </head>
  <body>
    <div class="video-container">
      <div class="video-wrapper">%s</div>
      <div class="video-actions">
        <button id="summarize-btn" onclick="summarizeVideo()">✨ Summarize Video</button>
      </div>
      <div id="summary">
        <h3>Summary</h3>
        <div id="summary-content"></div>
      </div>
    </div>
    <script>
      const videoId = '%s';
      const autoSummarize = %t;
      
      async function summarizeVideo() {
        const btn = document.getElementById('summarize-btn');
        const summary = document.getElementById('summary');
        const content = document.getElementById('summary-content');
        
        btn.disabled = true;
        btn.textContent = 'Generating summary...';
        summary.style.display = 'block';
        content.innerHTML = '<span class="loading">Fetching transcript and generating summary...</span>';
        
        try {
          const resp = await fetch('/video?action=summarize&id=' + videoId);
          const data = await resp.json();
          
          if (data.error) {
            content.innerHTML = '<span class="error">' + data.error + '</span>';
            btn.textContent = '✨ Summarize Video';
            btn.disabled = false;
          } else {
            content.textContent = data.summary;
            btn.textContent = '✓ Summarized';
          }
        } catch (err) {
          content.innerHTML = '<span class="error">Failed to generate summary: ' + err.message + '</span>';
          btn.textContent = '✨ Summarize Video';
          btn.disabled = false;
        }
      }
      
      // Auto-trigger summarize if requested
      if (autoSummarize) {
        summarizeVideo();
      }
    </script>
  </body>
</html>
`
		autoSummarize := r.Form.Get("summarize") == "1"
		html := fmt.Sprintf(tmpl, embedVideoWithAutoplay(id, autoplay), id, autoSummarize)
		w.Write([]byte(html))

		return
	}

	// GET - return video feed

	mutex.RLock()
	currentVideos := videos
	currentHtml := videosHtml
	mutex.RUnlock()

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"channels": currentVideos,
		})
		return
	}

	w.Write([]byte(currentHtml))
}
