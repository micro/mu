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
	"mu/stream"
	"mu/markets"
	"mu/reminder"
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

	// Date header: date + inline weather summary + admin actions
	now := time.Now()
	_, viewerAcc := auth.TrySession(r)
	inviteHTML := ""
	if viewerAcc != nil && viewerAcc.Admin && auth.InviteOnly() {
		pending := 0
		for _, req := range auth.ListInviteRequests() {
			if !req.Invited {
				pending++
			}
		}
		label := "+ Invite"
		if pending > 0 {
			label = fmt.Sprintf("+ Invite (%d waiting)", pending)
		}
		inviteHTML = fmt.Sprintf(`<span id="home-date-actions"><a href="/admin/invite" style="color:#555;text-decoration:none">%s</a></span>`, label)
	}
	b.WriteString(fmt.Sprintf(`<div id="home-date"><span id="home-date-text">%s</span><span id="home-date-weather"></span>%s</div>`, now.Format("Monday, 2 January 2006"), inviteHTML))
	// Inline script reads cached weather summary from localStorage
	// and renders "10°C ☁️" next to the date. No fetch — reads what
	// the weather card already cached.
	b.WriteString(`<script>(function(){
var w;try{w=JSON.parse(localStorage.getItem('mu_weather_now'))}catch(e){}
if(!w||w.temp==null)return;
var emoji={'clear':'☀️','sunny':'☀️','cloud':'☁️','overcast':'☁️','partly':'⛅','rain':'🌧️','drizzle':'🌧️','snow':'❄️','thunder':'⛈️','storm':'⛈️','fog':'🌫️','mist':'🌫️','haze':'🌫️','wind':'💨'};
var e='';var d=(w.desc||'').toLowerCase();for(var k in emoji){if(d.indexOf(k)>=0){e=emoji[k];break}}
document.getElementById('home-date-weather').textContent=w.temp+'°C '+(e||'');
})()</script>`)

	// View toggle — Console (stream) or Cards (dashboard)
	var viewerID string
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}
	b.WriteString(`<div id="home-tabs" style="display:flex;gap:6px;margin-bottom:14px">`)
	for _, t := range []struct{ id, label string }{{"console", "Console"}, {"cards", "Overview"}} {
		b.WriteString(fmt.Sprintf(`<a href="#" data-tab="%s" class="home-tab" style="padding:4px 14px;border-radius:14px;font-size:13px;text-decoration:none;color:#555">%s</a>`, t.id, t.label))
	}
	b.WriteString(`</div>`)

	// ── Console view (stream) ──
	consoleEvents := stream.Recent(stream.StreamLimit, viewerID)
	consoleEvents = stream.DedupeAdjacent(consoleEvents)
	b.WriteString(`<div id="home-console" style="display:none">`)
	// Compose box (logged-in only).
	if viewerID != "" {
		b.WriteString(fmt.Sprintf(`<form id="stream-form" method="POST" action="/stream" style="margin-bottom:12px;display:flex;gap:8px">
<input type="text" name="content" id="stream-input" placeholder="Ask @micro anything or post an update..." maxlength="%d" autocomplete="off" style="flex:1;padding:8px 12px;border:1px solid #ddd;border-radius:6px;font-size:14px">
<button type="submit" style="padding:8px 16px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-size:14px">Send</button>
</form>`, stream.MaxContentLength))
	}
	b.WriteString(`<div id="stream-events" style="max-height:min(70vh,600px);overflow-y:auto;-webkit-overflow-scrolling:touch">`)
	b.WriteString(stream.RenderEventList(consoleEvents, viewerID))
	b.WriteString(`</div>`)
	b.WriteString(consoleScript)
	b.WriteString(`</div>`)

	// ── Cards view (dashboard) ──
	b.WriteString(`<div id="home-cards">`)

	var leftHTML []string
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

	b.WriteString(`</div>`) // close #home-cards

	// Tab toggle JS — persists choice in localStorage.
	b.WriteString(`<script>
(function(){
  var tabs=document.querySelectorAll('.home-tab');
  var console=document.getElementById('home-console');
  var cards=document.getElementById('home-cards');
  var key='mu_home_view';
  function show(id){
    console.style.display=id==='console'?'block':'none';
    cards.style.display=id==='cards'?'block':'none';
    tabs.forEach(function(t){t.style.background=t.dataset.tab===id?'#000':'';t.style.color=t.dataset.tab===id?'#fff':'#555'});
    try{localStorage.setItem(key,id)}catch(e){}
  }
  tabs.forEach(function(t){t.addEventListener('click',function(e){e.preventDefault();show(t.dataset.tab)})});
  show(localStorage.getItem(key)||'cards');
})();
</script>`)

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
// consoleScript handles polling + form submit for the console stream
// embedded on the home page.
const consoleScript = `<script>
(function(){
  var eventsEl = document.getElementById('stream-events');
  var formEl = document.getElementById('stream-form');
  if (!eventsEl) return;
  var pollInterval = 10000;
  var inflight = false;

  function csrfToken() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }

  function refresh(clear) {
    if (inflight) return;
    inflight = true;
    fetch('/stream/fragment', { credentials: 'same-origin', cache: 'no-store' })
      .then(function(r){ return r.ok ? r.text() : null; })
      .then(function(html){
        if (html == null) return;
        var scroll = eventsEl.scrollTop;
        eventsEl.innerHTML = html;
        if (!clear) eventsEl.scrollTop = scroll;
      })
      .catch(function(){})
      .then(function(){ inflight = false; });
  }

  if (formEl) {
    formEl.addEventListener('submit', function(ev){
      ev.preventDefault();
      var input = document.getElementById('stream-input');
      if (!input) return;
      var text = input.value.trim();
      if (!text) return;
      var body = new URLSearchParams();
      body.set('content', text);
      var headers = { 'Content-Type': 'application/x-www-form-urlencoded' };
      var tok = csrfToken();
      if (tok) headers['X-CSRF-Token'] = tok;
      input.value = '';
      fetch('/stream', {
        method: 'POST',
        credentials: 'same-origin',
        headers: headers,
        body: body.toString()
      }).then(function(){ refresh(true); })
        .catch(function(){ formEl.submit(); });
    });
  }

  setInterval(function(){ if (!document.hidden) refresh(); }, pollInterval);
  document.addEventListener('visibilitychange', function(){ if (!document.hidden) refresh(); });
})();
</script>`

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
        // After posting (scrollToTop=true), don't restore input — we
        // want it cleared. On background polls, preserve what the
        // user is typing.
        var saved = scrollToTop ? null : currentInput();
        var feed = document.getElementById('home-statuses');
        var scrollPos = feed ? feed.scrollTop : 0;
        wrap.innerHTML = html;
        if (saved) restoreInput(saved);
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

  // Unified poll via /updates — only refreshes the status stream when
  // there are actual new entries, and updates mail badge from the same
  // call. Much cheaper than fetching the full HTML fragment every 10s.
  var lastTS = Math.floor(Date.now() / 1000);

  function checkUpdates() {
    if (document.hidden) return;
    fetch('/updates?since=' + lastTS, { credentials: 'same-origin', cache: 'no-store' })
      .then(function(r){ return r.ok ? r.json() : null; })
      .then(function(data){
        if (!data) return;
        lastTS = data.ts || lastTS;
        // Refresh status stream only when new entries exist.
        if (data.status > 0) refresh();
        // Update mail badges in the header/nav.
        var badges = [document.getElementById('head-mail-badge'), document.getElementById('nav-mail-badge')];
        for (var i = 0; i < badges.length; i++) {
          if (badges[i]) badges[i].textContent = data.mail > 0 ? data.mail : '';
        }
      })
      .catch(function(){});
  }

  setInterval(checkUpdates, pollInterval);
  document.addEventListener('visibilitychange', function(){
    if (!document.hidden) checkUpdates();
  });
})();
</script>`
