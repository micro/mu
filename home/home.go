package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/data"
	"mu/markets"
	"mu/news"
	"mu/reminder"
	"mu/video"
)

// landingTemplate is the full HTML template for the public landing page.
// It mirrors the structure of app/html/index.html but includes dynamic content sections.
var landingTemplate = `<html lang="en">
  <head>
    <title>Mu - The Micro Network</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="description" content="The Micro Network. Apps without ads, algorithms, or tracking.">
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css?%s">
  </head>
  <body>
    <div style="float: right; padding: 20px;">
      <a href="/about"><b>About</b></a>&nbsp;
      <a href="/docs"><b>Docs</b></a>&nbsp;
      <a href="/api"><b>API</b></a>&nbsp;
      <a href="/mcp"><b>MCP</b></a>&nbsp;
      <a href="/plans"><b>Plans</b></a>&nbsp;
      <a href="/login"><b>Login</b></a>
    </div>
    <div id="main">
      <div id="title">Mu</div>
      <div id="desc">The Micro Network</div>
      <p style="font-size: 18px; font-weight: 800; color: #333; margin: 20px 0; text-align: center; max-width: 800px;">
      Apps without ads, algorithms, or tracking.
      </p>

      <div style="height: 40px;"></div>

      <!-- Live preview: cards showing real content -->
      <h3>What&#39;s Available</h3>
      <p style="color:#555;max-width:600px;margin:0 auto 24px;">A glimpse of what&#39;s live right now — sign up to get full access.</p>
      <div style="max-width:900px;margin:0 auto;display:flex;gap:20px;flex-wrap:wrap;justify-content:center;text-align:left;">
        %s
        %s
      </div>

      <div style="height: 40px;"></div>

      <!-- API & MCP section -->
      <h3>API &amp; MCP</h3>
      <p style="color:#555;max-width:600px;margin:0 auto 24px;">Every feature is available via REST API and <a href="/mcp">Model Context Protocol</a> for AI clients.</p>
      <div style="max-width:900px;margin:0 auto;display:flex;gap:20px;flex-wrap:wrap;justify-content:center;text-align:left;">
        <div class="card" style="flex:1;min-width:280px;">
          <h4>REST API</h4>
          <p class="card-desc">Access news, video, chat and more programmatically. Pass <code>Accept: application/json</code> to any endpoint.</p>
          <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">GET /news?q=technology HTTP/1.1
Accept: application/json</pre>
          <a href="/api" class="link">API Docs &#x2192;</a>
        </div>
        <div class="card" style="flex:1;min-width:280px;">
          <h4>MCP (Model Context Protocol)</h4>
          <p class="card-desc">Connect AI agents (e.g. Claude) to Mu via MCP. Use the <code>signup</code> or <code>login</code> tool to obtain a token.</p>
          <pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;border-radius:4px;">{"jsonrpc":"2.0","id":1,"method":"tools/call",
 "params":{"name":"search_news",
           "arguments":{"query":"technology"}}}</pre>
          <a href="/mcp" class="link">MCP Server &#x2192;</a>
        </div>
      </div>

      <div style="height: 60px;"></div>

      <h3>Our Mission</h3>
      <p style="max-width: 600px">
      Mu is built with the intention that tools should serve humanity, enabling consumption without addiction, exploitation or manipulation.
      </p>

      <div style="height: 60px;"></div>

      <h3>Featured Apps</h3>
      <p>See what&#39;s included</p>
      <div id="links">
        <a href="/blog" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/post.png" alt="Blog" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Blog</b>
            <div class="small">Share thoughts and updates with the community</div>
          </div>
        </a>
        <a href="/chat" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/chat.png" alt="Chat" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Chat</b>
            <div class="small">Discussions powered by an AI knowledge assistant</div>
          </div>
        </a>
        <a href="/mail" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/mail.png" alt="Mail" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Mail</b>
            <div class="small">Message other users directly or send an email</div>
          </div>
        </a>
        <a href="/news" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/news.png" alt="News" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>News</b>
            <div class="small">Source of truth for news events around the world</div>
          </div>
        </a>
        <a href="/video" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/video.png" alt="Video" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Video</b>
            <div class="small">Watch YouTube without ads, algorithms or shorts</div>
          </div>
        </a>
        <a href="/places" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/places.png" alt="Places" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Places</b>
            <div class="small">Search and discover places on an ad-free map</div>
          </div>
        </a>
        <a href="/weather" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/weather.png" alt="Weather" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Weather</b>
            <div class="small">Local weather forecasts without ads or tracking</div>
          </div>
        </a>
        <a href="/markets" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/markets.png" alt="Markets" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Markets</b>
            <div class="small">Live crypto, futures and commodity prices</div>
          </div>
        </a>
        <a href="/reminder" style="text-decoration: none; color: inherit;">
          <div class="block">
            <img src="/reminder.png" alt="Reminder" style="width: 32px; height: 32px; margin-bottom: 8px; filter: brightness(0);">
            <b>Reminder</b>
            <div class="small">Daily Islamic verse, hadith, and name of Allah</div>
          </div>
        </a>
      </div>

      <div style="height: 60px;"></div>

      <hr />

      <div style="text-align: center; margin: 20px 0;">
        <a href="/login"><button class="btn" style="font-size:1em;padding:10px 28px;height:auto;">Get Started</button></a>
        <button id="install-pwa" style="display: none;">Install App</button>
      </div>

      <div style="height: 60px;"></div>

      <hr />

      <h3>FAQ</h3>

      <div style="height: 20px;"></div>

      <p><strong>Is Mu free to use?</strong><br>
      Yes! Create an account and start using Mu immediately at no cost.</p>

      <div style="height: 20px;"></div>

      <p><strong>Can I self-host Mu?</strong><br>
      Absolutely. Mu is open source and runs as a single Go binary. Check <a href="https://github.com/micro/mu" target="_blank">GitHub</a> for install instructions.</p>

      <div style="height: 20px;"></div>

      <p><strong>What about pricing?</strong><br>
      Mu is free with 10 credits/day. Need more? Top up and pay as you go from 1p per query. No subscriptions, no tricks. See our <a href="/plans">plans</a> for details.</p>

      <div style="height: 20px;"></div>

      <p><strong>How is this different from big tech platforms?</strong><br>
      No ads, no algorithmic feeds, no data mining. Just simple, useful tools that work for you.</p>

      <div style="height: 20px;"></div>

      <p><strong>Can AI agents use Mu?</strong><br>
      Yes. Mu supports the <a href="/mcp">Model Context Protocol (MCP)</a>. See the <a href="/mcp">MCP page</a> for setup and available tools.</p>

      <div style="height: 60px;"></div>
    </div>
  <script>
      let deferredPrompt;

      if (navigator.serviceWorker) {
        navigator.serviceWorker.register(
          '/mu.js',
          {scope: '/'}
        )
      }

      window.addEventListener('beforeinstallprompt', (e) => {
        e.preventDefault();
        deferredPrompt = e;
        const installButton = document.getElementById('install-pwa');
        if (installButton) {
          installButton.style.display = 'inline-block';
        }
      });

      document.getElementById('install-pwa')?.addEventListener('click', async () => {
        if (!deferredPrompt) return;

        deferredPrompt.prompt();
        const { outcome } = await deferredPrompt.userChoice;

        deferredPrompt = null;
        document.getElementById('install-pwa').style.display = 'none';
      });
  </script>
  </body>
</html>`

// LandingHandler serves the public-facing landing page with live content previews.
func LandingHandler(w http.ResponseWriter, r *http.Request) {
	// Refresh cards so news/markets content is up to date
	RefreshCards()

	newsContent := news.Headlines()
	marketsContent := markets.MarketsHTML()

	var newsCard, marketsCard string
	if strings.TrimSpace(newsContent) != "" {
		newsCard = fmt.Sprintf(`<div class="card" style="flex:1;min-width:280px;max-width:430px;">
      <h4><img src="/news.png" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">News</h4>
      %s
      <a href="/news" class="link">More news &#x2192;</a>
    </div>`, newsContent)
	}
	if strings.TrimSpace(marketsContent) != "" {
		marketsCard = fmt.Sprintf(`<div class="card" style="flex:1;min-width:280px;max-width:430px;">
      <h4><img src="/markets.png" style="width:20px;height:20px;vertical-align:middle;margin-right:6px;">Markets</h4>
      %s
      <a href="/markets" class="link">More &#x2192;</a>
    </div>`, marketsContent)
	}

	html := fmt.Sprintf(landingTemplate, app.Version, newsCard, marketsCard)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func ChatCard() string {
	return `<div id="home-chat">
		<form id="home-chat-form" action="/chat" method="GET">
			<input type="text" name="prompt" placeholder="Ask a question" required>
			<button type="submit">Ask</button>
		</form>
	</div>`
}

type Card struct {
	ID          string
	Title       string
	Icon        string    // Optional icon image path (e.g. "/news.png")
	Column      string // "left" or "right"
	Position    int
	Link        string
	Content     func() string
	CachedHTML  string    // Cached rendered content
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last update timestamp
}

var (
	lastRefresh time.Time
	cacheMutex  sync.RWMutex
	cacheTTL    = 2 * time.Minute
)

type CardConfig struct {
	Left []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"left"`
	Right []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
		Icon     string `json:"icon"`
	} `json:"right"`
}

var Cards []Card

func Load() {
	b, _ := f.ReadFile("cards.json")
	var config CardConfig
	if err := json.Unmarshal(b, &config); err != nil {
		fmt.Println("Error loading cards.json:", err)
		return
	}

	// Map of card types to their content functions
	cardFunctions := map[string]func() string{
		"blog":     blog.Preview,
		"chat":     ChatCard,
		"news":     news.Headlines,
		"markets":  markets.MarketsHTML,
		"reminder": reminder.ReminderHTML,
		"video":    video.Latest,
	}

	// Build Cards array from config
	Cards = []Card{}

	for _, c := range config.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "left",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	for _, c := range config.Right {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Column:   "right",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	// Sort by column and position
	sort.Slice(Cards, func(i, j int) bool {
		if Cards[i].Column != Cards[j].Column {
			return Cards[i].Column < Cards[j].Column
		}
		return Cards[i].Position < Cards[j].Position
	})

	// Do initial refresh
	RefreshCards()

	// Subscribe to blog update events
	go func() {
		sub := data.Subscribe("blog_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
}

// RefreshCards updates card content and timestamps if content changed
func RefreshCards() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	now := time.Now()

	// Check if cache is still valid
	if now.Sub(lastRefresh) < cacheTTL {
		return
	}

	for i := range Cards {
		card := &Cards[i]

		// Get fresh content
		content := card.Content()

		// Calculate hash
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

		// Only update if content changed
		if hash != card.ContentHash {
			card.CachedHTML = content
			card.ContentHash = hash
			card.UpdatedAt = now
		}
	}

	lastRefresh = now
}

// ForceRefresh forces an immediate cache refresh (for admin actions)
func ForceRefresh() {
	cacheMutex.Lock()
	lastRefresh = time.Time{} // Reset to zero to force refresh
	cacheMutex.Unlock()
	RefreshCards()
}

// RefreshHandler clears the last_visit cookie to show all cards again
func RefreshHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	cookie := &http.Cookie{
		Name:     "last_visit",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete cookie
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)

	// Redirect back to home
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Refresh cards if cache expired (2 minute TTL)
	RefreshCards()

	var leftHTML []string
	var rightHTML []string

	// Check if user is logged in (for future use)
	sess, _ := auth.TrySession(r)
	_ = sess

	for _, card := range Cards {
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}

		// Add "More" link if card has a link URL
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		html := app.Card(card.ID, card.Title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, html)
		} else {
			rightHTML = append(rightHTML, html)
		}
	}

	// create homepage
	if len(leftHTML) == 0 && len(rightHTML) == 0 {
		// No content - show welcome message
		leftHTML = append(leftHTML, app.Card("no-content", "Welcome", "<p>Welcome to Mu! Your personalized content will appear here.</p>"))
	}

	homepage := fmt.Sprintf(Template,
		strings.Join(leftHTML, "\n"),
		strings.Join(rightHTML, "\n"))

	// render html using user's language preference
	html := app.RenderHTMLForRequest("Home", "The Mu homescreen", homepage, r)

	w.Write([]byte(html))
}
