// Package kids provides a safe, audio-focused experience for children
package kids

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// Playlist represents a curated playlist
type Playlist struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	ID   string `json:"id"` // YouTube playlist ID
}

// SavedPlaylist is a user-created playlist
type SavedPlaylist struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	Videos    []Video   `json:"videos"`
	CreatedAt time.Time `json:"created_at"`
}

// Video represents a video from a playlist
type Video struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Playlist  string `json:"playlist"`
}

var (
	mu             sync.RWMutex
	playlists      []Playlist
	videos         map[string][]Video // playlist name -> videos
	savedPlaylists map[string]*SavedPlaylist
	client         *youtube.Service
	apiKey         string
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
	savedPlaylists = make(map[string]*SavedPlaylist)
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

	// Load saved playlists
	loadSavedPlaylists()

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
	case path == "search":
		handleSearch(w, r)
	case strings.HasPrefix(path, "play/"):
		id := strings.TrimPrefix(path, "play/")
		handlePlay(w, r, id)
	case strings.HasPrefix(path, "list/"):
		name := strings.TrimPrefix(path, "list/")
		handleList(w, r, name)
	case strings.HasPrefix(path, "saved/"):
		rest := strings.TrimPrefix(path, "saved/")
		handleSaved(w, r, rest)
	case path == "playlists":
		handlePlaylists(w, r)
	case path == "playlist":
		handlePlaylistAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	// Get current user to show their saved playlists
	sess, _ := auth.GetSession(r)
	userID := ""
	if sess != nil {
		userID = sess.Account
	}

	var content strings.Builder

	// Search bar
	content.WriteString(`<form class="search-bar" action="/kids/search" method="get">
		<input type="text" name="q" placeholder="Search...">
		<button type="submit">Search</button>
	</form>`)

	content.WriteString(`<div class="kids-home">`)
	content.WriteString(`<div class="kids-categories">`)

	// Built-in playlists
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

	// User's saved playlists only (if logged in)
	mu.RLock()
	for _, sp := range savedPlaylists {
		if sp.UserID != userID {
			continue // Skip playlists owned by other users
		}
		icon := sp.Icon
		if icon == "" {
			icon = "🎵"
		}
		content.WriteString(fmt.Sprintf(`
			<a href="/kids/saved/%s" class="kids-category-btn">
				<span class="kids-icon">%s</span>
				<span class="kids-label">%s</span>
				<span class="kids-count">%d</span>
			</a>`,
			sp.ID, icon, html.EscapeString(sp.Name), len(sp.Videos)))
	}
	mu.RUnlock()

	// Add playlist button
	content.WriteString(`
		<a href="/kids/playlists" class="kids-category-btn kids-category-btn-dashed">
			<span class="kids-icon">➕</span>
			<span class="kids-label">New</span>
		</a>`)

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

	// Play All button
	mu.RLock()
	vids := videos[name]
	mu.RUnlock()

	if len(vids) > 0 {
		content.WriteString(fmt.Sprintf(`<div class="kids-play-all">
			<a href="/kids/play/%s?playlist=%s&idx=0" class="btn">▶ Play All</a>
		</div>`, vids[0].ID, name))
	}

	content.WriteString(`<div class="kids-videos">`)
	for i, v := range vids {
		content.WriteString(renderVideoCardWithIndex(v, name, i))
	}
	content.WriteString(`</div>`)

	html := app.RenderHTMLForRequest(playlist.Name, fmt.Sprintf("%s for kids", playlist.Name), content.String(), r)
	w.Write([]byte(html))
}

func handlePlay(w http.ResponseWriter, r *http.Request, id string) {
	// Get playlist context for prev/next navigation
	playlistName := r.URL.Query().Get("playlist")
	idxStr := r.URL.Query().Get("idx")
	idx, _ := strconv.Atoi(idxStr)
	savedID := r.URL.Query().Get("saved") // for saved playlists

	// Get the playlist videos
	var playlistVideos []Video
	var video *Video
	var backURL string

	if savedID != "" {
		// Saved playlist
		mu.RLock()
		if sp, ok := savedPlaylists[savedID]; ok {
			playlistVideos = sp.Videos
			backURL = "/kids/saved/" + savedID
		}
		mu.RUnlock()
	} else if playlistName != "" {
		// Built-in playlist
		mu.RLock()
		playlistVideos = videos[playlistName]
		mu.RUnlock()
		backURL = "/kids/list/" + playlistName
	}

	// Find video in playlist or search all
	if len(playlistVideos) > 0 && idx >= 0 && idx < len(playlistVideos) {
		video = &playlistVideos[idx]
	} else {
		// Fallback: search all playlists
		mu.RLock()
		for name, vids := range videos {
			for i := range vids {
				if vids[i].ID == id {
					video = &vids[i]
					playlistVideos = vids
					idx = i
					playlistName = name
					backURL = "/kids/list/" + name
					break
				}
			}
			if video != nil {
				break
			}
		}
		// Also check saved playlists
		if video == nil {
			for sid, sp := range savedPlaylists {
				for i := range sp.Videos {
					if sp.Videos[i].ID == id {
						video = &sp.Videos[i]
						playlistVideos = sp.Videos
						idx = i
						savedID = sid
						backURL = "/kids/saved/" + sid
						break
					}
				}
				if video != nil {
					break
				}
			}
		}
		mu.RUnlock()
	}

	if video == nil {
		http.Error(w, "Video not available", http.StatusForbidden)
		return
	}

	if backURL == "" {
		backURL = "/kids"
	}

	// Calculate prev/next
	var prevURL, nextURL string
	if len(playlistVideos) > 0 {
		if idx > 0 {
			prev := playlistVideos[idx-1]
			if savedID != "" {
				prevURL = fmt.Sprintf("/kids/play/%s?saved=%s&idx=%d", prev.ID, savedID, idx-1)
			} else {
				prevURL = fmt.Sprintf("/kids/play/%s?playlist=%s&idx=%d", prev.ID, playlistName, idx-1)
			}
		}
		if idx < len(playlistVideos)-1 {
			next := playlistVideos[idx+1]
			if savedID != "" {
				nextURL = fmt.Sprintf("/kids/play/%s?saved=%s&idx=%d", next.ID, savedID, idx+1)
			} else {
				nextURL = fmt.Sprintf("/kids/play/%s?playlist=%s&idx=%d", next.ID, playlistName, idx+1)
			}
		}
	}

	renderPlayer(w, r, video, id, backURL, prevURL, nextURL)
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

func renderVideoCardWithIndex(v Video, playlist string, idx int) string {
	return fmt.Sprintf(`
		<a href="/kids/play/%s?playlist=%s&idx=%d" class="kids-video-card">
			<img src="%s" alt="%s" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
			<div class="kids-video-title">%s</div>
		</a>`,
		v.ID, playlist, idx,
		v.Thumbnail,
		html.EscapeString(v.Title),
		v.ID,
		html.EscapeString(truncate(v.Title, 50)),
	)
}

func renderVideoCardForSaved(v Video, savedID string, idx int) string {
	return fmt.Sprintf(`
		<a href="/kids/play/%s?saved=%s&idx=%d" class="kids-video-card">
			<img src="%s" alt="%s" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
			<div class="kids-video-title">%s</div>
		</a>`,
		v.ID, savedID, idx,
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


func renderPlayer(w http.ResponseWriter, r *http.Request, video *Video, id, backURL, prevURL, nextURL string) {
	var content strings.Builder
	
	// Back link at top like other pages
	content.WriteString(fmt.Sprintf(`<p><a href="%s">← Back</a></p>`, backURL))
	
	// Video container with thumbnail overlay
	content.WriteString(fmt.Sprintf(`<div class="kids-video-container">
		<div id="player"></div>
		<img src="%s" class="kids-thumb-overlay" id="thumbnail" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
	</div>`, video.Thumbnail, id))
	
	// Controls
	prevBtn := `<span class="btn btn-secondary disabled">⏮</span>`
	if prevURL != "" {
		prevBtn = fmt.Sprintf(`<a href="%s" class="btn btn-secondary" id="prevBtn">⏮</a>`, prevURL)
	}
	nextBtn := `<span class="btn btn-secondary disabled">⏭</span>`
	if nextURL != "" {
		nextBtn = fmt.Sprintf(`<a href="%s" class="btn btn-secondary" id="nextBtn">⏭</a>`, nextURL)
	}
	
	content.WriteString(fmt.Sprintf(`<div class="kids-controls">
		%s
		<button onclick="togglePlay()" id="playBtn">▶</button>
		%s
	</div>`, prevBtn, nextBtn))
	
	// Video button
	content.WriteString(`<div class="text-center mt-3">
		<button onclick="toggleVideo()" id="videoBtn">📺 Show Video</button>
	</div>`)
	
	// JavaScript for YouTube player
	content.WriteString(fmt.Sprintf(`<script>
		let playing = false;
		let videoVisible = false;
		const videoId = '%s';
		const nextURL = '%s';
		let player;
		
		const tag = document.createElement('script');
		tag.src = 'https://www.youtube.com/iframe_api';
		document.head.appendChild(tag);
		
		function onYouTubeIframeAPIReady() {
			player = new YT.Player('player', {
				width: '100%%',
				height: '300',
				videoId: videoId,
				playerVars: { autoplay: 0, rel: 0, modestbranding: 1 },
				events: { 'onStateChange': onPlayerStateChange }
			});
		}
		
		function onPlayerStateChange(event) {
			if (event.data === 0 && nextURL) {
				window.location.href = nextURL;
			}
		}
		
		function togglePlay() {
			const btn = document.getElementById('playBtn');
			if (!playing) {
				if (player && player.playVideo) player.playVideo();
				btn.textContent = '⏸';
				playing = true;
			} else {
				if (player && player.pauseVideo) player.pauseVideo();
				btn.textContent = '▶';
				playing = false;
			}
		}
		
		function toggleVideo() {
			const thumb = document.getElementById('thumbnail');
			const btn = document.getElementById('videoBtn');
			
			if (!videoVisible) {
				thumb.classList.add('hidden');
				btn.textContent = '🖼 Hide Video';
				if (player && player.playVideo) player.playVideo();
				document.getElementById('playBtn').textContent = '⏸';
				playing = true;
				videoVisible = true;
			} else {
				thumb.classList.remove('hidden');
				btn.textContent = '📺 Show Video';
				videoVisible = false;
			}
		}
	</script>`, id, nextURL))
	
	html := app.RenderHTMLForRequest(video.Title, "Playing: "+video.Title, content.String(), r)
	w.Write([]byte(html))
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	
	// Check if user is logged in
	sess, _ := auth.GetSession(r)
	isLoggedIn := sess != nil
	
	var content strings.Builder
	content.WriteString(`<p><a href="/kids">← Back</a></p>`)
	content.WriteString(`<form class="search-bar" action="/kids/search" method="get">
		<input type="text" name="q" placeholder="Search..." value="` + html.EscapeString(query) + `">
		<button type="submit">Search</button>
	</form>`)
	
	if query != "" && client != nil {
		results, err := searchYouTube(query)
		if err != nil {
			content.WriteString(fmt.Sprintf(`<p class="text-error">Search error: %v</p>`, err))
		} else if len(results) == 0 {
			content.WriteString(`<p class="text-muted">No results found</p>`)
		} else {
			content.WriteString(`<div class="kids-videos">`)
			for _, v := range results {
				content.WriteString(renderSearchResult(v, isLoggedIn))
			}
			content.WriteString(`</div>`)
		}
	} else if query != "" {
		content.WriteString(`<p class="text-muted">Search not available</p>`)
	}
	
	// Add modal and script for add-to-playlist
	content.WriteString(`
	<div id="addModal" class="modal" hidden>
		<div class="modal-content" style="max-width:320px;">
			<h3>Add to Playlist</h3>
			<div id="debugInfo" class="text-xs text-muted mb-2"></div>
			<div id="playlistList"></div>
			<div id="createForm" hidden>
				<div class="mb-2">
					<input type="text" id="newPlaylistName" placeholder="Playlist name" class="w-full">
				</div>
				<button onclick="createAndAdd()" class="btn w-full">Create & Add</button>
			</div>
			<div class="mt-3 d-flex gap-2">
				<button onclick="showCreateForm()" id="newBtn" class="btn btn-outline flex-1">+ New</button>
				<button onclick="closeModal()" class="btn flex-1">Cancel</button>
			</div>
		</div>
	</div>
	<script>
		let pendingVideo = null;
		
		function addToPlaylist(videoId, title, thumbnail) {
			pendingVideo = {id: videoId, title: title, thumbnail: thumbnail};
			document.getElementById('newPlaylistName').value = '';
			const endpoint = '/kids/playlist';
			console.log('Fetching:', endpoint, 'cookies:', document.cookie);
			fetch(endpoint, {credentials: 'include'})
				.then(r => {
					console.log('Response status:', r.status);
					return r.json();
				})
				.then(playlists => {
					const count = playlists ? playlists.length : 0;
					document.getElementById('debugInfo').textContent = 'Found ' + count + ' playlists';
					const list = document.getElementById('playlistList');
					const createForm = document.getElementById('createForm');
					const newBtn = document.getElementById('newBtn');
					
					if (!playlists || playlists.length === 0) {
						// No playlists - show create form directly
						list.innerHTML = '';
						createForm.hidden = false;
						newBtn.style.display = 'none';
						setTimeout(() => document.getElementById('newPlaylistName').focus(), 100);
					} else {
						// Has playlists - show list with + New option
						list.innerHTML = playlists.map(p => 
							'<button onclick="doAdd(\'' + p.id + '\')" class="btn btn-outline w-full mb-2 text-left">' +
							(p.icon || '🎵') + ' ' + p.name + '</button>'
						).join('');
						createForm.hidden = true;
						newBtn.style.display = '';
					}
					document.getElementById('addModal').hidden = false;
				});
		}
		
		function showCreateForm() {
			document.getElementById('createForm').hidden = false;
			document.getElementById('newBtn').style.display = 'none';
			document.getElementById('newPlaylistName').focus();
		}
		
		function createAndAdd() {
			const name = document.getElementById('newPlaylistName').value.trim();
			if (!name) {
				alert('Please enter a playlist name');
				return;
			}
			// Create playlist first
			const createForm = new FormData();
			createForm.append('action', 'create');
			createForm.append('name', name);
			createForm.append('icon', '🎵');
			
			fetch('/kids/playlist', {
				method: 'POST',
				body: createForm,
				headers: {'Accept': 'application/json'}, credentials: 'include'
			}).then(r => r.json()).then(result => {
				if (result && result.error) {
					alert(result.error);
				} else if (result && result.id) {
					// Now add the video to the new playlist
					doAdd(result.id);
				} else {
					alert('Failed to create playlist');
				}
			}).catch(() => alert('Please login to create playlists'));
		}
		
		function doAdd(playlistId) {
			const form = new FormData();
			form.append('action', 'add');
			form.append('playlist_id', playlistId);
			form.append('video_id', pendingVideo.id);
			form.append('title', pendingVideo.title);
			form.append('thumbnail', pendingVideo.thumbnail);
			
			fetch('/kids/playlist', {
				method: 'POST',
				body: form,
				headers: {'Accept': 'application/json'}, credentials: 'include'
			}).then(r => r.json()).then(() => {
				closeModal();
				alert('Added!');
			});
		}
		
		function closeModal() {
			document.getElementById('addModal').hidden = true;
			pendingVideo = null;
		}
	</script>
	`)
	
	html := app.RenderHTMLForRequest("Search", "Search for kids music", content.String(), r)
	w.Write([]byte(html))
}

func searchYouTube(query string) ([]Video, error) {
	resp, err := client.Search.List([]string{"snippet"}).
		Q(query + " kids").
		Type("video").
		SafeSearch("strict").
		MaxResults(20).
		Do()
	if err != nil {
		return nil, err
	}
	
	var results []Video
	for _, item := range resp.Items {
		thumbnail := ""
		if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.High != nil {
			thumbnail = item.Snippet.Thumbnails.High.Url
		}
		results = append(results, Video{
			ID:        item.Id.VideoId,
			Title:     item.Snippet.Title,
			Thumbnail: thumbnail,
			Playlist:  "search",
		})
	}
	return results, nil
}

func renderSearchResult(v Video, showAddButton bool) string {
	titleEscaped := html.EscapeString(strings.ReplaceAll(v.Title, "'", "\\'"))
	addBtn := ""
	if showAddButton {
		addBtn = fmt.Sprintf(`<button onclick="addToPlaylist('%s', '%s', '%s')" class="kids-add-btn">+</button>`,
			v.ID, titleEscaped, v.Thumbnail)
	}
	return fmt.Sprintf(`
		<div class="kids-video-card">
			<a href="/kids/play/%s">
				<img src="%s" alt="%s" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
				<div class="kids-video-title">%s</div>
			</a>
			%s
		</div>`,
		v.ID,
		v.Thumbnail,
		html.EscapeString(v.Title),
		v.ID,
		html.EscapeString(truncate(v.Title, 50)),
		addBtn,
	)
}

func handleSaved(w http.ResponseWriter, r *http.Request, rest string) {
	mu.RLock()
	sp, ok := savedPlaylists[rest]
	mu.RUnlock()
	
	if !ok {
		http.NotFound(w, r)
		return
	}
	
	var content strings.Builder
	content.WriteString(`<p><a href="/kids">← Back</a></p>`)
	
	icon := sp.Icon
	if icon == "" {
		icon = "🎵"
	}
	content.WriteString(fmt.Sprintf(`<div class="kids-header"><span class="kids-icon-lg">%s</span></div>`, icon))
	
	if len(sp.Videos) > 0 {
		content.WriteString(fmt.Sprintf(`<div class="kids-play-all">
			<a href="/kids/play/%s?saved=%s&idx=0" class="btn">▶ Play All</a>
		</div>`, sp.Videos[0].ID, sp.ID))
	}
	
	// Current playlist videos
	if len(sp.Videos) > 0 {
		content.WriteString(`<h3>Videos</h3>`)
		content.WriteString(`<div class="kids-videos">`)
		for i, v := range sp.Videos {
			content.WriteString(renderVideoCardForSaved(v, sp.ID, i))
		}
		content.WriteString(`</div>`)
	}
	
	// Add from existing playlists section
	content.WriteString(`<h3 class="mt-4">Add Videos</h3>`)
	content.WriteString(`<p class="text-muted mb-3">Select a playlist to add videos from:</p>`)
	
	// Playlist selector tabs
	content.WriteString(`<div class="kids-playlist-tabs">`)
	for _, pl := range playlists {
		content.WriteString(fmt.Sprintf(`<button class="kids-tab" onclick="showPlaylist('%s')">%s %s</button>`,
			pl.Name, pl.Icon, pl.Name))
	}
	content.WriteString(`</div>`)
	
	// Videos from each playlist (hidden by default)
	mu.RLock()
	for _, pl := range playlists {
		content.WriteString(fmt.Sprintf(`<div class="kids-add-videos" id="add-%s" style="display:none;">`, pl.Name))
		if vids, ok := videos[pl.Name]; ok {
			for _, v := range vids {
				titleEscaped := html.EscapeString(strings.ReplaceAll(v.Title, "'", "\\'"))
				content.WriteString(fmt.Sprintf(`
					<div class="kids-video-card kids-video-mini">
						<img src="%s" alt="%s" onerror="this.src='https://img.youtube.com/vi/%s/hqdefault.jpg'">
						<div class="kids-video-title">%s</div>
						<button onclick="addVideo('%s', '%s', '%s')" class="kids-add-btn">+</button>
					</div>`,
					v.Thumbnail, html.EscapeString(v.Title), v.ID,
					html.EscapeString(truncate(v.Title, 40)),
					v.ID, titleEscaped, v.Thumbnail))
			}
		}
		content.WriteString(`</div>`)
	}
	mu.RUnlock()
	
	// JavaScript for adding videos
	content.WriteString(fmt.Sprintf(`<script>
		let activeTab = null;
		
		function showPlaylist(name) {
			// Hide all
			document.querySelectorAll('.kids-add-videos').forEach(el => el.style.display = 'none');
			document.querySelectorAll('.kids-tab').forEach(el => el.classList.remove('active'));
			
			// Show selected
			const panel = document.getElementById('add-' + name);
			if (panel) {
				panel.style.display = 'grid';
			}
			event.target.classList.add('active');
			activeTab = name;
		}
		
		function addVideo(videoId, title, thumbnail) {
			const form = new FormData();
			form.append('action', 'add');
			form.append('playlist_id', '%s');
			form.append('video_id', videoId);
			form.append('title', title);
			form.append('thumbnail', thumbnail);
			
			fetch('/kids/playlist', {
				method: 'POST',
				body: form,
				headers: {'Accept': 'application/json'}, credentials: 'include'
			}).then(r => r.json()).then(() => {
				location.reload();
			});
		}
	</script>`, sp.ID))
	
	html := app.RenderHTMLForRequest(sp.Name, sp.Name+" playlist", content.String(), r)
	w.Write([]byte(html))
}

func handlePlaylists(w http.ResponseWriter, r *http.Request) {
	// Get current user
	sess, _ := auth.GetSession(r)
	userID := ""
	if sess != nil {
		userID = sess.Account
	}

	var content strings.Builder
	content.WriteString(`<p><a href="/kids">← Back</a></p>`)
	
	if userID == "" {
		content.WriteString(`<p><a href="/login">Login</a> to create and manage playlists.</p>`)
	} else {
		content.WriteString(`<h2>Create Playlist</h2>`)
		content.WriteString(`<form method="post" action="/kids/playlist" class="max-w-sm">
			<input type="hidden" name="action" value="create">
			<div class="mb-3">
				<label>Name</label>
				<input type="text" name="name" required placeholder="e.g. Journey Songs">
			</div>
			<div class="mb-3">
				<label>Icon (emoji)</label>
				<input type="text" name="icon" placeholder="🎸" maxlength="4">
			</div>
			<button type="submit" class="btn">Create</button>
		</form>`)
		
		// List user's playlists only
		mu.RLock()
		hasPlaylists := false
		for _, sp := range savedPlaylists {
			if sp.UserID == userID {
				if !hasPlaylists {
					content.WriteString(`<h2 class="mt-5">Your Playlists</h2>`)
					hasPlaylists = true
				}
				icon := sp.Icon
				if icon == "" {
					icon = "🎵"
				}
				content.WriteString(fmt.Sprintf(`<div class="d-flex items-center gap-2 mb-2">
					<span class="text-xl">%s</span>
					<a href="/kids/saved/%s">%s</a>
					<span class="text-muted">(%d songs)</span>
					<form method="post" action="/kids/playlist" class="m-0">
						<input type="hidden" name="action" value="delete">
						<input type="hidden" name="id" value="%s">
						<button type="submit" class="btn-danger text-sm">Delete</button>
					</form>
				</div>`, icon, sp.ID, html.EscapeString(sp.Name), len(sp.Videos), sp.ID))
			}
		}
		mu.RUnlock()
	}
	
	html := app.RenderHTMLForRequest("Playlists", "Manage playlists", content.String(), r)
	w.Write([]byte(html))
}

func handlePlaylistAPI(w http.ResponseWriter, r *http.Request) {
	// Get current user (optional for GET, required for POST)
	sess, err := auth.GetSession(r)
	userID := ""
	if sess != nil {
		userID = sess.Account
	}
	app.Log("kids", "PlaylistAPI %s: userID=%q, err=%v, savedPlaylists=%d", r.Method, userID, err, len(savedPlaylists))

	if r.Method == "POST" {
		// Require auth for creating/modifying playlists
		if userID == "" {
			if r.Header.Get("Accept") == "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Please login to create playlists"})
				return
			}
			http.Error(w, "Login required", http.StatusUnauthorized)
			return
		}

		// Support both multipart (JS FormData) and urlencoded form data
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			r.ParseMultipartForm(10 << 20) // 10MB max
		} else {
			r.ParseForm()
		}
		action := r.FormValue("action")
		
		switch action {
		case "create":
			name := r.FormValue("name")
			icon := r.FormValue("icon")
			if name == "" {
				http.Error(w, "Name required", http.StatusBadRequest)
				return
			}
			id := fmt.Sprintf("%d", time.Now().UnixNano())
			sp := &SavedPlaylist{
				ID:        id,
				UserID:    userID,
				Name:      name,
				Icon:      icon,
				Videos:    []Video{},
				CreatedAt: time.Now(),
			}
			mu.Lock()
			savedPlaylists[id] = sp
			mu.Unlock()
			saveSavedPlaylists()
			
			// Return JSON for AJAX
			if r.Header.Get("Accept") == "application/json" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":   true,
					"id":   id,
					"name": name,
				})
				return
			}
			http.Redirect(w, r, "/kids/saved/"+id, http.StatusSeeOther)
			return
			
		case "delete":
			id := r.FormValue("id")
			mu.Lock()
			// Only allow deletion if user owns the playlist
			if sp, ok := savedPlaylists[id]; ok && sp.UserID == userID {
				delete(savedPlaylists, id)
			}
			mu.Unlock()
			saveSavedPlaylists()
			http.Redirect(w, r, "/kids/playlists", http.StatusSeeOther)
			return
			
		case "add":
			id := r.FormValue("playlist_id")
			videoID := r.FormValue("video_id")
			title := r.FormValue("title")
			thumbnail := r.FormValue("thumbnail")
			
			mu.Lock()
			// Only allow adding to playlists the user owns
			if sp, ok := savedPlaylists[id]; ok && sp.UserID == userID {
				// Check for duplicate
				found := false
				for _, v := range sp.Videos {
					if v.ID == videoID {
						found = true
						break
					}
				}
				if !found {
					sp.Videos = append(sp.Videos, Video{
						ID:        videoID,
						Title:     title,
						Thumbnail: thumbnail,
						Playlist:  sp.Name,
					})
				}
			}
			mu.Unlock()
			saveSavedPlaylists()
			
			// Return JSON for AJAX
			if r.Header.Get("Accept") == "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"ok":true}`))
				return
			}
			http.Redirect(w, r, "/kids/saved/"+id, http.StatusSeeOther)
			return
		}
	}
	
	// GET - return list of playlists as JSON (for add-to-playlist modal)
	// Only return playlists owned by the current user
	mu.RLock()
	var list []map[string]interface{}
	for _, sp := range savedPlaylists {
		if sp.UserID == userID {
			list = append(list, map[string]interface{}{
				"id":   sp.ID,
				"name": sp.Name,
				"icon": sp.Icon,
			})
		}
	}
	mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func loadSavedPlaylists() {
	data, err := data.LoadFile("kids/playlists.json")
	if err != nil {
		return
	}
	var list []*SavedPlaylist
	if err := json.Unmarshal(data, &list); err != nil {
		app.Log("kids", "Error loading saved playlists: %v", err)
		return
	}
	mu.Lock()
	for _, sp := range list {
		savedPlaylists[sp.ID] = sp
	}
	mu.Unlock()
	app.Log("kids", "Loaded %d saved playlists", len(list))
}

func saveSavedPlaylists() {
	mu.RLock()
	var list []*SavedPlaylist
	for _, sp := range savedPlaylists {
		list = append(list, sp)
	}
	mu.RUnlock()
	
	b, _ := json.Marshal(list)
	data.SaveFile("kids/playlists.json", string(b))
}