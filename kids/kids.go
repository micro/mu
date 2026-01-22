// Package kids provides a safe, audio-focused experience for children
package kids

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"mu/app"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// Playlist represents a curated playlist
type Playlist struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	ID   string `json:"id"` // YouTube playlist ID
}

// Video represents a video from a playlist
type Video struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Playlist  string `json:"playlist"`
}

var (
	mu        sync.RWMutex
	playlists []Playlist
	videos    map[string][]Video // playlist name -> videos
	client    *youtube.Service
	apiKey    string
)

// Default playlists - curated, not algorithmic
var defaultPlaylists = []Playlist{
	{Name: "Quran", Icon: "📖", ID: "PLYZxc42QNctXcCQZyZs48hAN90YJgnOnJ"}, // Mishary Rashid - Kids Quran
	{Name: "Disney", Icon: "✨", ID: "PLRfhDHeBRBEjfvtPOpTe9AHMJCGr0oDm1"}, // Disney Soundtracks
	{Name: "Nasheed", Icon: "🎵", ID: "PLF48FC0BCA476D6EC"},               // Zain Bhikha
}

func init() {
	apiKey = os.Getenv("YOUTUBE_API_KEY")
	videos = make(map[string][]Video)
}

// Load initializes the kids package
func Load() {
	if apiKey == "" {
		app.Log("kids", "No YouTube API key, kids features limited")
		return
	}

	var err error
	client, err = youtube.NewService(context.TODO(), option.WithAPIKey(apiKey))
	if err != nil {
		app.Log("kids", "Failed to initialize YouTube client: %v", err)
		return
	}

	// Load custom playlists or use defaults
	if err := loadPlaylists(); err != nil {
		app.Log("kids", "Using default playlists: %v", err)
		playlists = defaultPlaylists
	}

	// Initial load
	go refreshVideos()

	// Refresh daily
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			refreshVideos()
		}
	}()

	app.Log("kids", "Kids mode loaded with %d playlists", len(playlists))
}

func loadPlaylists() error {
	data, err := os.ReadFile("kids/playlists.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &playlists)
}

func refreshVideos() {
	app.Log("kids", "Refreshing kids playlists...")

	for _, pl := range playlists {
		vids, err := fetchPlaylist(pl.ID, pl.Name)
		if err != nil {
			app.Log("kids", "Error fetching %s: %v", pl.Name, err)
			continue
		}

		mu.Lock()
		videos[pl.Name] = vids
		mu.Unlock()

		app.Log("kids", "Loaded %d videos for %s", len(vids), pl.Name)
	}
}

func fetchPlaylist(playlistID, playlistName string) ([]Video, error) {
	if client == nil {
		return nil, fmt.Errorf("no client")
	}

	var vids []Video
	pageToken := ""

	for {
		resp, err := client.PlaylistItems.List([]string{"snippet"}).
			PlaylistId(playlistID).
			MaxResults(50).
			PageToken(pageToken).
			Do()

		if err != nil {
			return vids, err
		}

		for _, item := range resp.Items {
			thumbnail := ""
			if item.Snippet.Thumbnails != nil {
				if item.Snippet.Thumbnails.High != nil {
					thumbnail = item.Snippet.Thumbnails.High.Url
				} else if item.Snippet.Thumbnails.Medium != nil {
					thumbnail = item.Snippet.Thumbnails.Medium.Url
				}
			}

			vids = append(vids, Video{
				ID:        item.Snippet.ResourceId.VideoId,
				Title:     item.Snippet.Title,
				Thumbnail: thumbnail,
				Playlist:  playlistName,
			})
		}

		pageToken = resp.NextPageToken
		if pageToken == "" || len(vids) >= 100 {
			break
		}
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
	case strings.HasPrefix(path, "list/"):
		name := strings.TrimPrefix(path, "list/")
		handleList(w, r, name)
	default:
		http.NotFound(w, r)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	var content strings.Builder

	content.WriteString(`<div class="kids-home">`)
	content.WriteString(`<div class="kids-categories">`)

	for _, pl := range playlists {
		mu.RLock()
		count := len(videos[pl.Name])
		mu.RUnlock()

		content.WriteString(fmt.Sprintf(`
			<a href="/kids/list/%s" class="kids-category-btn">
				<span class="kids-icon">%s</span>
				<span class="kids-label">%s</span>
				<span class="kids-count">%d</span>
			</a>`,
			pl.Name, pl.Icon, pl.Name, count))
	}

	content.WriteString(`</div></div>`)

	html := app.RenderHTMLForRequest("Kids", "Safe audio and videos for children", content.String(), r)
	w.Write([]byte(html))
}

func handleList(w http.ResponseWriter, r *http.Request, name string) {
	// Find playlist
	var playlist *Playlist
	for i := range playlists {
		if playlists[i].Name == name {
			playlist = &playlists[i]
			break
		}
	}

	if playlist == nil {
		http.NotFound(w, r)
		return
	}

	var content strings.Builder

	content.WriteString(`<p><a href="/kids">← Back</a></p>`)
	content.WriteString(fmt.Sprintf(`<div class="kids-header"><span class="kids-icon-lg">%s</span></div>`, playlist.Icon))
	content.WriteString(`<div class="kids-videos">`)

	mu.RLock()
	for _, v := range videos[name] {
		content.WriteString(renderVideoCard(v))
	}
	mu.RUnlock()

	content.WriteString(`</div>`)

	html := app.RenderHTMLForRequest(playlist.Name, fmt.Sprintf("%s for kids", playlist.Name), content.String(), r)
	w.Write([]byte(html))
}

func handlePlay(w http.ResponseWriter, r *http.Request, id string) {
	// Find video in our playlists
	mu.RLock()
	var video *Video
	for _, vids := range videos {
		for i := range vids {
			if vids[i].ID == id {
				video = &vids[i]
				break
			}
		}
		if video != nil {
			break
		}
	}
	mu.RUnlock()

	if video == nil {
		http.Error(w, "Video not available", http.StatusForbidden)
		return
	}

	// Simple player - audio focus with thumbnail
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>%s | Kids</title>
	<link rel="stylesheet" href="/mu.css">
	<style>
		.kids-player { max-width: 600px; margin: 0 auto; padding: 20px; text-align: center; }
		.kids-player h2 { font-size: 1.3em; margin: 20px 0; }
		.kids-thumb { width: 100%%; max-width: 400px; border-radius: 16px; margin: 0 auto; }
		.kids-audio { display: none; }
		.kids-controls { margin-top: 30px; display: flex; gap: 15px; justify-content: center; flex-wrap: wrap; }
		.kids-btn { padding: 15px 30px; font-size: 1.2em; border-radius: 30px; border: none; cursor: pointer; text-decoration: none; display: inline-block; }
		.kids-btn-play { background: var(--accent-color, #0d7377); color: white; font-size: 1.5em; }
		.kids-btn-back { background: #eee; color: #333; }
		.kids-btn-video { background: #333; color: white; }
	</style>
</head>
<body>
	<div class="kids-player">
		<img src="%s" class="kids-thumb" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
		<h2>%s</h2>
		<div class="kids-controls">
			<a href="/kids/list/%s" class="kids-btn kids-btn-back">← Back</a>
			<button onclick="togglePlay()" id="playBtn" class="kids-btn kids-btn-play">▶</button>
			<button onclick="showVideo()" class="kids-btn kids-btn-video">📺 Video</button>
		</div>
		<div id="videoContainer" style="display:none; margin-top: 20px;">
			<iframe id="player" width="100%%" height="300" src="" frameborder="0" allow="autoplay; encrypted-media" allowfullscreen style="border-radius: 12px;"></iframe>
		</div>
	</div>
	<script>
		let playing = false;
		const videoId = '%s';
		
		function togglePlay() {
			const btn = document.getElementById('playBtn');
			const container = document.getElementById('videoContainer');
			const player = document.getElementById('player');
			
			if (!playing) {
				// Start audio-only (video hidden)
				player.src = 'https://www.youtube.com/embed/' + videoId + '?autoplay=1&rel=0';
				container.style.display = 'none';
				btn.textContent = '⏸';
				playing = true;
			} else {
				player.src = '';
				btn.textContent = '▶';
				playing = false;
			}
		}
		
		function showVideo() {
			const container = document.getElementById('videoContainer');
			const player = document.getElementById('player');
			const btn = document.getElementById('playBtn');
			
			container.style.display = 'block';
			player.src = 'https://www.youtube.com/embed/' + videoId + '?autoplay=1&rel=0';
			btn.textContent = '⏸';
			playing = true;
		}
	</script>
</body>
</html>`,
		html.EscapeString(video.Title),
		video.Thumbnail,
		id,
		html.EscapeString(video.Title),
		video.Playlist,
		id,
	)

	w.Write([]byte(html))
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
