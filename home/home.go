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

	"mu/agent"
	"mu/apps"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/blog"
	"mu/internal/event"
	"mu/news"
	"mu/social"
	"mu/markets"
	"mu/reminder"
	"mu/user"
	"mu/video"
	"mu/weather"
)

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() string {
	return news.Headlines()
}

func ChatCard() string {
	return `<div id="home-chat">
		<form id="home-chat-form" action="/chat" method="GET">
			<input type="text" name="prompt" placeholder="Ask a question" required>
			<button type="submit">Ask</button>
		</form>
	</div>`
}

func AgentCard() string {
	return `<div id="home-agent">
		<form id="home-agent-form" action="/agent" method="GET">
			<div style="display:flex;gap:8px;">
				<input type="text" name="prompt" placeholder="Tell the agent what to do..." required style="flex:1;padding:8px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;">
				<button type="submit" style="padding:8px 16px;font-family:inherit;font-size:14px;border:1px solid #ddd;border-radius:4px;cursor:pointer;">Do</button>
			</div>
			<div style="display:flex;gap:8px;margin-top:6px;align-items:center;">
				<select name="model" style="padding:4px 8px;font-family:inherit;font-size:13px;border:1px solid #ddd;border-radius:4px;">
					<option value="standard">Fast</option>
					<option value="premium">Best</option>
				</select>
				<span style="flex:1;"></span>
				` + agent.ToolsDropdownHTML() + `
			</div>
		</form>
	</div>`
}

type Card struct {
	ID          string
	Title       string
	Icon        string // Optional icon image path (e.g. "/news.png")
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
		"agent":    AgentCard,
		"blog":     blog.Preview,
		"chat":     ChatCard,
		"news":     newsCard,
		"markets":  markets.MarketsHTML,
		"reminder": reminder.ReminderHTML,
		"video":    video.Latest,
		"apps":     apps.Preview,
		"social":   social.CardHTML,
		"weather":  weather.CardHTML,
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

	// Subscribe to blog and apps update events
	go func() {
		sub := event.Subscribe("blog_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("apps_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("social_updated")
		for range sub.Chan {
			ForceRefresh()
		}
	}()
	go func() {
		sub := event.Subscribe("reminder_updated")
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
	// JSON endpoint for auto-refresh polling
	if app.WantsJSON(r) {
		RefreshCards()
		cacheMutex.RLock()
		type cardData struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			HTML    string `json:"html"`
			Column  string `json:"column"`
		}
		var result []cardData
		for _, card := range Cards {
			if strings.TrimSpace(card.CachedHTML) == "" {
				continue
			}
			result = append(result, cardData{
				ID:     card.ID,
				Title:  card.Title,
				HTML:   card.CachedHTML,
				Column: card.Column,
			})
		}
		cacheMutex.RUnlock()
		app.RespondJSON(w, result)
		return
	}

	// Refresh cards if cache expired (2 minute TTL)
	RefreshCards()

	var b strings.Builder

	// Date header + admin actions
	now := time.Now()
	_, viewerAcc := auth.TrySession(r)
	inviteLink := ""
	if viewerAcc != nil && viewerAcc.Admin && auth.InviteOnly() {
		inviteLink = ` <a href="/admin/invite" style="float:right;font-size:13px;color:#555;text-decoration:none">+ Invite user</a>`
	}
	b.WriteString(fmt.Sprintf(`<p id="home-date">%s%s</p>`, now.Format("Monday, 2 January 2006"), inviteLink))

	// Status card content (will be prepended to left column).
	// Built by user.RenderStatusStream so the fragment endpoint and the
	// home card share one code path. The #home-status-wrap element is
	// polled every ~10 seconds for near-real-time updates, and the
	// compose form submits via fetch so the stream refreshes in place.
	var viewerID string
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}
	statusInner := user.RenderStatusStream(viewerID)
	statusCardBody := `<div id="home-status-wrap">` + statusInner + `</div>` + statusCardScript
	statusCardHTML := fmt.Sprintf(
		app.CardTemplate,
		"status", "status", "Status",
		statusCardBody,
	)

	// Feed section — existing home cards below the agent
	var leftHTML []string
	if statusCardHTML != "" {
		leftHTML = append(leftHTML, statusCardHTML)
	}
	var rightHTML []string

	tooltips := map[string]string{
		"blog":     "Microblog posts with daily AI-generated digests",
		"news":     "Headlines from RSS feeds, sorted by time",
		"markets":  "Live crypto, futures, and commodity prices",
		"reminder": "Daily Islamic reminder with verse and hadith",
		"social":   "Public discussion threads",
		"video":    "Latest videos from curated channels",
	}

	for _, card := range Cards {
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		title := card.Title
		if tip, ok := tooltips[card.ID]; ok {
			title += fmt.Sprintf(` <span class="card-tooltip" data-tip="%s" onclick="event.stopPropagation();document.querySelectorAll('.card-tooltip.show').forEach(function(e){e.classList.remove('show')});this.classList.toggle('show')">?</span>`, htmlEsc(tip))
		}
		html := fmt.Sprintf(app.CardTemplate, card.ID, card.ID, title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, html)
		} else {
			rightHTML = append(rightHTML, html)
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n")))
	}

	// Auto-refresh: poll every 2 minutes, update card content in-place
	displayMode := r.URL.Query().Get("mode") == "display"
	refreshInterval := 120000 // 2 minutes
	if displayMode {
		refreshInterval = 60000 // 1 minute in display mode
	}
	wakeLockJS := ""
	if displayMode {
		wakeLockJS = `
  // Screen Wake Lock — keep display on in kiosk mode
  if('wakeLock' in navigator){
    var wl=null;
    function reqWake(){navigator.wakeLock.request('screen').then(function(l){wl=l;l.addEventListener('release',function(){setTimeout(reqWake,1000)})}).catch(function(){})}
    reqWake();document.addEventListener('visibilitychange',function(){if(document.visibilityState==='visible')reqWake()});
  }`
	}
	b.WriteString(fmt.Sprintf(`<script>
(function(){
  var interval = %d;
  setInterval(function(){
    fetch('/home', {headers:{Accept:'application/json'}})
    .then(function(r){return r.json()})
    .then(function(cards){
      cards.forEach(function(c){
        var el = document.getElementById(c.id);
        if(el){
          var content = el.querySelector('.card-body');
          if(content) content.innerHTML = c.html;
        }
      });
    }).catch(function(){});
  }, interval);%s
})();
</script>`, refreshInterval, wakeLockJS))

	// Display mode: hide nav, header, footer for kiosk/wall display
	bodyClass := ` class="page-home"`
	if displayMode {
		bodyClass = ` class="page-home display-mode"`
	}

	lang := app.GetUserLanguage(r)
	html := app.RenderHTMLWithLangAndBody("Home", "The home screen", b.String(), lang, bodyClass)
	w.Write([]byte(html))
}

// htmlEsc escapes HTML special characters.
func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// statusCardScript wires the status card for live updates:
//
//   - Polls /user/status/stream every 10 seconds and swaps the inner
//     markup of #home-status-wrap, preserving whatever the user is
//     currently typing in the compose input.
//   - Intercepts the compose form submit so it POSTs via fetch and
//     then refreshes the stream in place (no full page reload).
//   - Keeps the stream scrolled to the top after a refresh so new
//     messages are always visible.
//
// The script is defensive: if anything throws, the form still falls
// back to its native POST + redirect behaviour.
const statusCardScript = `<script>
(function(){
  var wrap = document.getElementById('home-status-wrap');
  if (!wrap) return;
  var pollInterval = 10000;
  var inflight = false;

  function csrfToken() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }

  function currentInput() {
    var el = document.getElementById('home-status-input');
    return el ? { value: el.value, focused: document.activeElement === el } : null;
  }
  function restoreInput(saved) {
    if (!saved) return;
    var el = document.getElementById('home-status-input');
    if (!el) return;
    el.value = saved.value;
    if (saved.focused) {
      el.focus();
      try { el.setSelectionRange(el.value.length, el.value.length); } catch(e){}
    }
  }

  function refresh(scrollToTop) {
    if (inflight) return;
    inflight = true;
    fetch('/user/status/stream', { credentials: 'same-origin', cache: 'no-store' })
      .then(function(r){ return r.ok ? r.text() : null; })
      .then(function(html){
        if (html == null) return;
        var saved = currentInput();
        // Preserve scroll position unless we explicitly want to
        // scroll to top (e.g. after posting a new status).
        var feed = document.getElementById('home-statuses');
        var scrollPos = feed ? feed.scrollTop : 0;
        wrap.innerHTML = html;
        restoreInput(saved);
        var newFeed = document.getElementById('home-statuses');
        if (newFeed) {
          newFeed.scrollTop = scrollToTop ? 0 : scrollPos;
        }
        bindForm();
      })
      .catch(function(){})
      .then(function(){ inflight = false; });
  }

  function bindForm() {
    var form = document.getElementById('home-status-form');
    if (!form || form.dataset.bound) return;
    form.dataset.bound = '1';
    form.addEventListener('submit', function(ev){
      ev.preventDefault();
      var input = document.getElementById('home-status-input');
      if (!input) return;
      var text = input.value.trim();
      if (!text) return;
      var body = new URLSearchParams();
      body.set('status', text);
      var headers = { 'Content-Type': 'application/x-www-form-urlencoded' };
      var tok = csrfToken();
      if (tok) headers['X-CSRF-Token'] = tok;
      fetch('/user/status', {
        method: 'POST',
        credentials: 'same-origin',
        headers: headers,
        body: body.toString()
      }).then(function(){
        input.value = '';
        refresh(true);
      }).catch(function(){
        // Fall back to a native form submit on network error.
        form.submit();
      });
    });
  }

  bindForm();

  // Poll while the tab is visible.
  setInterval(function(){
    if (document.hidden) return;
    refresh();
  }, pollInterval);

  // Fetch immediately when the tab regains focus.
  document.addEventListener('visibilitychange', function(){
    if (!document.hidden) refresh();
  });
})();
</script>`
