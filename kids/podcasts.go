package kids

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/app"

	"github.com/mmcdole/gofeed"
)

// Podcast represents a podcast feed
type Podcast struct {
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	FeedURL  string `json:"feed_url"`
	Category string `json:"category"` // learn, stories, faith
}

// Episode represents a podcast episode
type Episode struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AudioURL    string `json:"audio_url"`
	Duration    string `json:"duration"`
	Published   string `json:"published"`
	Image       string `json:"image"`
	Podcast     string `json:"podcast"`
}

// Category groups content for the home page
type Category struct {
	Name  string
	Icon  string
	Items []CategoryItem
}

// CategoryItem is either a playlist or podcast
type CategoryItem struct {
	Name     string
	Icon     string
	Type     string // "playlist" or "podcast"
	ID       string // playlist ID or podcast name
	Count    int
}

var (
	podcastMu   sync.RWMutex
	podcasts    []Podcast
	episodes    map[string][]Episode // podcast name -> episodes
	categories  []Category
)

// Default podcasts - curated for kids
var defaultPodcasts = []Podcast{
	// Learn
	{Name: "Science", Icon: "🔬", Category: "learn", FeedURL: "https://feeds.simplecast.com/hl6Dj3hR"},           // Brains On
	{Name: "Nature", Icon: "🌿", Category: "learn", FeedURL: "https://feeds.simplecast.com/ePf5jMqS"},            // Wow in the World
	
	// Stories
	{Name: "Folktales", Icon: "📚", Category: "stories", FeedURL: "https://feeds.simplecast.com/2zFK5GNG"},       // Circle Round
}

func initPodcasts() {
	episodes = make(map[string][]Episode)
	podcasts = defaultPodcasts
	
	// Build categories
	buildCategories()
	
	// Initial fetch
	go refreshPodcasts()
	
	// Refresh every 6 hours
	go func() {
		for {
			time.Sleep(6 * time.Hour)
			refreshPodcasts()
		}
	}()
}

func buildCategories() {
	// Music category (existing playlists)
	musicItems := []CategoryItem{}
	for _, pl := range playlists {
		mu.RLock()
		count := len(videos[pl.Name])
		mu.RUnlock()
		musicItems = append(musicItems, CategoryItem{
			Name:  pl.Name,
			Icon:  pl.Icon,
			Type:  "playlist",
			ID:    pl.Name,
			Count: count,
		})
	}
	
	// Learn category (podcasts)
	learnItems := []CategoryItem{}
	for _, p := range podcasts {
		if p.Category == "learn" {
			podcastMu.RLock()
			count := len(episodes[p.Name])
			podcastMu.RUnlock()
			learnItems = append(learnItems, CategoryItem{
				Name:  p.Name,
				Icon:  p.Icon,
				Type:  "podcast",
				ID:    p.Name,
				Count: count,
			})
		}
	}
	
	// Stories category (podcasts)
	storiesItems := []CategoryItem{}
	for _, p := range podcasts {
		if p.Category == "stories" {
			podcastMu.RLock()
			count := len(episodes[p.Name])
			podcastMu.RUnlock()
			storiesItems = append(storiesItems, CategoryItem{
				Name:  p.Name,
				Icon:  p.Icon,
				Type:  "podcast",
				ID:    p.Name,
				Count: count,
			})
		}
	}
	
	categories = []Category{
		{Name: "Music", Icon: "🎵", Items: musicItems},
		{Name: "Learn", Icon: "🔬", Items: learnItems},
		{Name: "Stories", Icon: "📚", Items: storiesItems},
	}
}

func refreshPodcasts() {
	app.Log("kids", "Refreshing podcasts...")
	parser := gofeed.NewParser()
	
	for _, p := range podcasts {
		feed, err := parser.ParseURL(p.FeedURL)
		if err != nil {
			app.Log("kids", "Error fetching %s: %v", p.Name, err)
			continue
		}
		
		var eps []Episode
		for i, item := range feed.Items {
			if i >= 50 { // Limit to 50 episodes
				break
			}
			
			// Find audio enclosure
			audioURL := ""
			for _, enc := range item.Enclosures {
				if strings.HasPrefix(enc.Type, "audio/") {
					audioURL = enc.URL
					break
				}
			}
			if audioURL == "" {
				continue
			}
			
			// Get image
			image := ""
			if item.Image != nil {
				image = item.Image.URL
			} else if feed.Image != nil {
				image = feed.Image.URL
			}
			
			// Get duration
			duration := ""
			if item.ITunesExt != nil && item.ITunesExt.Duration != "" {
				duration = item.ITunesExt.Duration
			}
			
			// Published date
			published := ""
			if item.PublishedParsed != nil {
				published = item.PublishedParsed.Format("Jan 2, 2006")
			}
			
			eps = append(eps, Episode{
				ID:          fmt.Sprintf("%s-%d", p.Name, i),
				Title:       item.Title,
				Description: truncateText(stripHTML(item.Description), 200),
				AudioURL:    audioURL,
				Duration:    duration,
				Published:   published,
				Image:       image,
				Podcast:     p.Name,
			})
		}
		
		podcastMu.Lock()
		episodes[p.Name] = eps
		podcastMu.Unlock()
		
		app.Log("kids", "Loaded %d episodes for %s", len(eps), p.Name)
	}
	
	// Rebuild categories with updated counts
	buildCategories()
}

func stripHTML(s string) string {
	// Simple HTML stripping
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// handlePodcast shows episodes for a podcast
func handlePodcast(w http.ResponseWriter, r *http.Request, name string) {
	// Find podcast
	var podcast *Podcast
	for i := range podcasts {
		if podcasts[i].Name == name {
			podcast = &podcasts[i]
			break
		}
	}
	
	if podcast == nil {
		http.NotFound(w, r)
		return
	}
	
	var content strings.Builder
	content.WriteString(`<p><a href="/kids">← Back</a></p>`)
	content.WriteString(fmt.Sprintf(`<div class="kids-header"><span class="kids-icon-lg">%s</span></div>`, podcast.Icon))
	
	podcastMu.RLock()
	eps := episodes[name]
	podcastMu.RUnlock()
	
	if len(eps) == 0 {
		content.WriteString(`<p class="text-muted">No episodes available</p>`)
	} else {
		// Play All button
		content.WriteString(fmt.Sprintf(`<div class="kids-play-all">
			<a href="/kids/episode/%s?idx=0" class="btn">▶ Play All</a>
		</div>`, name))
		
		content.WriteString(`<div class="kids-episodes">`)
		for i, ep := range eps {
			content.WriteString(renderEpisodeCard(ep, name, i))
		}
		content.WriteString(`</div>`)
	}
	
	html := app.RenderHTMLForRequest(podcast.Name, podcast.Name+" podcast", content.String(), r)
	w.Write([]byte(html))
}

func renderEpisodeCard(ep Episode, podcastName string, idx int) string {
	image := ep.Image
	if image == "" {
		image = "/placeholder.png"
	}
	
	meta := ep.Published
	if ep.Duration != "" {
		if meta != "" {
			meta += " · "
		}
		meta += ep.Duration
	}
	
	return fmt.Sprintf(`
		<a href="/kids/episode/%s?idx=%d" class="kids-episode-card">
			<img src="%s" alt="%s">
			<div class="kids-episode-info">
				<div class="kids-episode-title">%s</div>
				<div class="kids-episode-meta">%s</div>
			</div>
		</a>`,
		podcastName, idx,
		image,
		html.EscapeString(ep.Title),
		html.EscapeString(truncate(ep.Title, 60)),
		meta,
	)
}

// handleEpisode plays a podcast episode
func handleEpisode(w http.ResponseWriter, r *http.Request, podcastName string) {
	idxStr := r.URL.Query().Get("idx")
	idx := 0
	if idxStr != "" {
		fmt.Sscanf(idxStr, "%d", &idx)
	}
	autoplay := r.URL.Query().Get("auto") == "1"
	
	podcastMu.RLock()
	eps := episodes[podcastName]
	podcastMu.RUnlock()
	
	if idx < 0 || idx >= len(eps) {
		http.NotFound(w, r)
		return
	}
	
	ep := eps[idx]
	
	// Find podcast for icon
	icon := "🎧"
	for _, p := range podcasts {
		if p.Name == podcastName {
			icon = p.Icon
			break
		}
	}
	
	// Calculate prev/next
	var prevURL, nextURL string
	if idx > 0 {
		prevURL = fmt.Sprintf("/kids/episode/%s?idx=%d&auto=1", podcastName, idx-1)
	}
	if idx < len(eps)-1 {
		nextURL = fmt.Sprintf("/kids/episode/%s?idx=%d&auto=1", podcastName, idx+1)
	}
	
	backURL := "/kids/podcast/" + podcastName
	
	renderEpisodePlayer(w, r, &ep, icon, backURL, prevURL, nextURL, autoplay)
}

func renderEpisodePlayer(w http.ResponseWriter, r *http.Request, ep *Episode, icon, backURL, prevURL, nextURL string, autoplay bool) {
	var content strings.Builder
	
	content.WriteString(fmt.Sprintf(`<p><a href="%s">← Back</a></p>`, backURL))
	
	// Episode image
	image := ep.Image
	if image == "" {
		image = "/placeholder.png"
	}
	content.WriteString(fmt.Sprintf(`<div class="kids-episode-player">
		<img src="%s" alt="%s" class="kids-episode-image">
		<h3>%s</h3>
		<p class="text-muted">%s</p>
	</div>`, image, html.EscapeString(ep.Title), html.EscapeString(ep.Title), ep.Published))
	
	// Audio player
	autoplayAttr := ""
	if autoplay {
		autoplayAttr = "autoplay"
	}
	content.WriteString(fmt.Sprintf(`<audio id="audioPlayer" src="%s" %s style="width:100%%; margin: 1rem 0;"></audio>`,
		ep.AudioURL, autoplayAttr))
	
	// Controls
	playBtnIcon := "▶"
	if autoplay {
		playBtnIcon = "⏸"
	}
	prevBtn := `<span class="btn btn-secondary disabled">⏮</span>`
	if prevURL != "" {
		prevBtn = fmt.Sprintf(`<a href="%s" class="btn btn-secondary">⏮</a>`, prevURL)
	}
	nextBtn := `<span class="btn btn-secondary disabled">⏭</span>`
	if nextURL != "" {
		nextBtn = fmt.Sprintf(`<a href="%s" class="btn btn-secondary">⏭</a>`, nextURL)
	}
	
	content.WriteString(fmt.Sprintf(`<div class="kids-controls">
		%s
		<button onclick="togglePlay()" id="playBtn">%s</button>
		%s
	</div>`, prevBtn, playBtnIcon, nextBtn))
	
	// Description
	if ep.Description != "" {
		content.WriteString(fmt.Sprintf(`<div class="kids-episode-desc mt-3">
			<p>%s</p>
		</div>`, html.EscapeString(ep.Description)))
	}
	
	// JavaScript
	content.WriteString(fmt.Sprintf(`<script>
		let playing = %t;
		const audio = document.getElementById('audioPlayer');
		const nextURL = '%s';
		
		audio.addEventListener('ended', function() {
			if (nextURL) window.location.href = nextURL;
		});
		
		audio.addEventListener('play', function() {
			playing = true;
			document.getElementById('playBtn').textContent = '⏸';
		});
		
		audio.addEventListener('pause', function() {
			playing = false;
			document.getElementById('playBtn').textContent = '▶';
		});
		
		function togglePlay() {
			if (playing) {
				audio.pause();
			} else {
				audio.play();
			}
		}
	</script>`, autoplay, nextURL))
	
	html := app.RenderHTMLForRequest(ep.Title, "Playing: "+ep.Title, content.String(), r)
	w.Write([]byte(html))
}
