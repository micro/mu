// Package kids provides a safe, audio-focused experience for children
package kids

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// Category represents a content category for kids
type Category struct {
	Name     string    `json:"name"`
	Icon     string    `json:"icon"`
	Channels []Channel `json:"channels"`
}

// Channel represents a whitelisted YouTube channel
type Channel struct {
	Name   string `json:"name"`
	Handle string `json:"handle"` // YouTube handle or channel ID
	ID     string `json:"-"`      // Resolved channel ID
}

// Video represents a kid-safe video
type Video struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Channel     string    `json:"channel"`
	Category    string    `json:"category"`
	Thumbnail   string    `json:"thumbnail"`
	PublishedAt time.Time `json:"published_at"`
}

var (
	mu         sync.RWMutex
	categories []Category
	videos     []Video
	client     *youtube.Service
	apiKey     string
)

// Default kid-safe channels
var defaultCategories = []Category{
	{
		Name: "Islamic",
		Icon: "🕌",
		Channels: []Channel{
			{Name: "Omar & Hana", Handle: "OmarAndHanaOfficial"},
			{Name: "FreeQuranEducation", Handle: "FreeQuranEducation"},
			{Name: "Zaky", Handle: "OneIslamProductions"},
		},
	},
	{
		Name: "Arabic",
		Icon: "🔤",
		Channels: []Channel{
			{Name: "Learn Arabic with Maha", Handle: "LearnArabicwithMaha"},
			{Name: "Arabic Fairy Tales", Handle: "ArabicFairyTales"},
		},
	},
	{
		Name: "Songs",
		Icon: "🎵",
		Channels: []Channel{
			{Name: "CoComelon", Handle: "Cocomelon"},
			{Name: "Super Simple Songs", Handle: "SuperSimpleSongs"},
			{Name: "Pinkfong", Handle: "Pinkfong"},
		},
	},
	{
		Name: "Stories",
		Icon: "📚",
		Channels: []Channel{
			{Name: "CBeebies", Handle: "CBeebies"},
			{Name: "Peppa Pig", Handle: "PeppaPigOfficial"},
			{Name: "Bluey", Handle: "BlueyChannel"},
		},
	},
	{
		Name: "Learning",
		Icon: "🎓",
		Channels: []Channel{
			{Name: "National Geographic Kids", Handle: "NatGeoKids"},
			{Name: "SciShow Kids", Handle: "scikidzshow"},
			{Name: "Numberblocks", Handle: "Numberblocks"},
		},
	},
}

func init() {
	apiKey = os.Getenv("YOUTUBE_API_KEY")
}

// Load initializes the kids package
func Load() {
	if apiKey == "" {
		app.Log("kids", "No YouTube API key, kids features disabled")
		return
	}

	var err error
	client, err = youtube.NewService(context.TODO(), option.WithAPIKey(apiKey))
	if err != nil {
		app.Log("kids", "Failed to initialize YouTube client: %v", err)
		return
	}

	// Load custom channels or use defaults
	if err := loadChannels(); err != nil {
		app.Log("kids", "Using default channels: %v", err)
		categories = defaultCategories
	}

	// Resolve channel handles to IDs
	resolveChannelIDs()

	// Initial video load
	go refreshVideos()

	// Refresh videos periodically (every 2 hours)
	go func() {
		for {
			time.Sleep(2 * time.Hour)
			refreshVideos()
		}
	}()

	app.Log("kids", "Kids mode loaded with %d categories", len(categories))
}

func loadChannels() error {
	data, err := os.ReadFile("kids/channels.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &categories)
}

func resolveChannelIDs() {
	for i := range categories {
		for j := range categories[i].Channels {
			ch := &categories[i].Channels[j]
			if ch.ID == "" {
				id, err := getChannelID(ch.Handle)
				if err != nil {
					app.Log("kids", "Failed to resolve channel %s: %v", ch.Handle, err)
					continue
				}
				ch.ID = id
			}
		}
	}
}

func getChannelID(handle string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no client")
	}

	// Try as handle first
	resp, err := client.Channels.List([]string{"id"}).ForHandle(handle).Do()
	if err == nil && len(resp.Items) > 0 {
		return resp.Items[0].Id, nil
	}

	// Try as username
	resp, err = client.Channels.List([]string{"id"}).ForUsername(handle).Do()
	if err == nil && len(resp.Items) > 0 {
		return resp.Items[0].Id, nil
	}

	// Maybe it's already an ID
	if strings.HasPrefix(handle, "UC") {
		return handle, nil
	}

	return "", fmt.Errorf("channel not found: %s", handle)
}

func refreshVideos() {
	app.Log("kids", "Refreshing kids videos...")

	var newVideos []Video

	for _, cat := range categories {
		for _, ch := range cat.Channels {
			if ch.ID == "" {
				continue
			}

			vids, err := fetchChannelVideos(ch.ID, cat.Name, ch.Name)
			if err != nil {
				app.Log("kids", "Error fetching %s: %v", ch.Name, err)
				continue
			}

			newVideos = append(newVideos, vids...)
		}
	}

	// Sort by publish date
	sort.Slice(newVideos, func(i, j int) bool {
		return newVideos[i].PublishedAt.After(newVideos[j].PublishedAt)
	})

	mu.Lock()
	videos = newVideos
	mu.Unlock()

	app.Log("kids", "Loaded %d kids videos", len(newVideos))
}

func fetchChannelVideos(channelID, category, channelName string) ([]Video, error) {
	if client == nil {
		return nil, fmt.Errorf("no client")
	}

	resp, err := client.Search.List([]string{"id", "snippet"}).
		ChannelId(channelID).
		Type("video").
		Order("date").
		SafeSearch("strict").
		MaxResults(10).
		Do()

	if err != nil {
		return nil, err
	}

	var vids []Video
	for _, item := range resp.Items {
		pubAt, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
		vids = append(vids, Video{
			ID:          item.Id.VideoId,
			Title:       item.Snippet.Title,
			Channel:     channelName,
			Category:    category,
			Thumbnail:   item.Snippet.Thumbnails.High.Url,
			PublishedAt: pubAt,
		})
	}

	return vids, nil
}

// Handler serves /kids routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/kids")
	path = strings.TrimPrefix(path, "/")

	switch {
	case path == "" || path == "/":
		handleHome(w, r)
	case strings.HasPrefix(path, "play/"):
		id := strings.TrimPrefix(path, "play/")
		handlePlay(w, r, id)
	case strings.HasPrefix(path, "category/"):
		cat := strings.TrimPrefix(path, "category/")
		handleCategory(w, r, cat)
	case path == "api/videos":
		handleAPIVideos(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	var content strings.Builder

	content.WriteString(`<div class="kids-home">`)

	// Category buttons
	content.WriteString(`<div class="kids-categories">`)
	for _, cat := range categories {
		content.WriteString(fmt.Sprintf(`
			<a href="/kids/category/%s" class="kids-category-btn">
				<span class="kids-icon">%s</span>
				<span class="kids-label">%s</span>
			</a>`,
			cat.Name, cat.Icon, cat.Name))
	}
	content.WriteString(`</div>`)

	// Recent videos
	content.WriteString(`<h3 class="mt-5">Recent</h3>`)
	content.WriteString(`<div class="kids-videos">`)

	mu.RLock()
	count := 0
	for _, v := range videos {
		if count >= 12 {
			break
		}
		content.WriteString(renderVideoCard(v))
		count++
	}
	mu.RUnlock()

	if count == 0 {
		content.WriteString(`<p class="text-muted">No videos loaded yet. Check back soon!</p>`)
	}

	content.WriteString(`</div></div>`)

	html := app.RenderHTMLForRequest("Kids", "Safe videos and audio for children", content.String(), r)
	w.Write([]byte(html))
}

func handleCategory(w http.ResponseWriter, r *http.Request, catName string) {
	var content strings.Builder

	// Find category
	var category *Category
	for i := range categories {
		if categories[i].Name == catName {
			category = &categories[i]
			break
		}
	}

	if category == nil {
		http.NotFound(w, r)
		return
	}

	content.WriteString(fmt.Sprintf(`<p><a href="/kids">← Back</a></p>`))
	content.WriteString(fmt.Sprintf(`<div class="kids-header"><span class="kids-icon-lg">%s</span></div>`, category.Icon))

	content.WriteString(`<div class="kids-videos">`)

	mu.RLock()
	for _, v := range videos {
		if v.Category == catName {
			content.WriteString(renderVideoCard(v))
		}
	}
	mu.RUnlock()

	content.WriteString(`</div>`)

	html := app.RenderHTMLForRequest(category.Name, fmt.Sprintf("%s videos for kids", category.Name), content.String(), r)
	w.Write([]byte(html))
}

func handlePlay(w http.ResponseWriter, r *http.Request, id string) {
	// Verify video is in our whitelist
	mu.RLock()
	var video *Video
	for i := range videos {
		if videos[i].ID == id {
			video = &videos[i]
			break
		}
	}
	mu.RUnlock()

	if video == nil {
		// Not in whitelist - don't play
		http.Error(w, "Video not available", http.StatusForbidden)
		return
	}

	// Audio-only player page
	audioOnly := r.URL.Query().Get("audio") == "1"

	var playerStyle string
	if audioOnly {
		// Hide video, show thumbnail
		playerStyle = `
			.video-container iframe { display: none; }
			.video-container { background: url('https://img.youtube.com/vi/` + id + `/maxresdefault.jpg') center/cover; height: 300px; border-radius: 12px; }
		`
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>%s | Kids</title>
	<link rel="stylesheet" href="/mu.css">
	<style>
		.kids-player { max-width: 800px; margin: 0 auto; padding: 20px; }
		.kids-player h2 { font-size: 1.5em; margin-bottom: 20px; }
		.video-container { position: relative; width: 100%%; padding-bottom: 56.25%%; border-radius: 12px; overflow: hidden; }
		.video-container iframe { position: absolute; top: 0; left: 0; width: 100%%; height: 100%%; border: none; }
		.kids-controls { margin-top: 20px; display: flex; gap: 10px; flex-wrap: wrap; }
		.kids-btn { padding: 15px 30px; font-size: 1.2em; border-radius: 25px; border: none; cursor: pointer; }
		.kids-btn-back { background: #eee; color: #333; }
		.kids-btn-audio { background: var(--accent-color, #0d7377); color: white; }
		%s
	</style>
</head>
<body>
	<div class="kids-player">
		<h2>%s</h2>
		<p class="text-muted mb-3">%s</p>
		<div class="video-container">
			<iframe src="https://www.youtube.com/embed/%s?autoplay=1&rel=0" allow="autoplay; encrypted-media" allowfullscreen></iframe>
		</div>
		<div class="kids-controls">
			<a href="/kids" class="kids-btn kids-btn-back">← Back</a>
			<a href="/kids/play/%s?audio=%s" class="kids-btn kids-btn-audio">%s</a>
		</div>
	</div>
</body>
</html>`,
		html.EscapeString(video.Title),
		playerStyle,
		html.EscapeString(video.Title),
		html.EscapeString(video.Channel),
		id,
		id,
		map[bool]string{true: "0", false: "1"}[audioOnly],
		map[bool]string{true: "🎬 Show Video", false: "🎵 Audio Only"}[audioOnly],
	)

	w.Write([]byte(html))
}

func handleAPIVideos(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")

	mu.RLock()
	defer mu.RUnlock()

	var result []Video
	for _, v := range videos {
		if cat == "" || v.Category == cat {
			result = append(result, v)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func renderVideoCard(v Video) string {
	return fmt.Sprintf(`
		<a href="/kids/play/%s" class="kids-video-card">
			<img src="%s" alt="%s" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
			<div class="kids-video-title">%s</div>
		</a>`,
		v.ID,
		v.Thumbnail,
		html.EscapeString(v.Title),
		v.ID,
		html.EscapeString(truncate(v.Title, 50)),
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
