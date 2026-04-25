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

	// Date header + weather + invite — Overview only, built here
	// and injected into the cards div below.
	now := time.Now()
	_, viewerAcc := auth.TrySession(r)
	var dateLine strings.Builder
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
	gearHTML := ""
	if viewerAcc != nil {
		gearHTML = ` <a href="#" id="home-gear" onclick="var p=document.getElementById('home-card-prefs');p.style.display=p.style.display==='none'?'block':'none';return false" style="color:#bbb;text-decoration:none;font-size:16px" title="Customise cards">⚙</a>`
	}
	dateLine.WriteString(fmt.Sprintf(`<div id="home-date"><span id="home-date-text">%s</span><span id="home-date-weather"></span>%s%s</div>`, now.Format("Monday, 2 January 2006"), inviteHTML, gearHTML))
	// Inline weather: reads cached summary, and refreshes it in the
	// background if stale (>1 hour). This runs independently of the
	// weather card — even if the card is hidden, the date-line temp
	// stays current.
	dateLine.WriteString(`<script>(function(){
var KEY='mu_weather_now',KEY_TS='mu_weather_now_ts',KEY_LAT='mu_weather_lat',KEY_LON='mu_weather_lon',TTL=3600000;
var emoji={'clear':'☀️','sunny':'☀️','cloud':'☁️','overcast':'☁️','partly':'⛅','rain':'🌧️','drizzle':'🌧️','snow':'❄️','thunder':'⛈️','storm':'⛈️','fog':'🌫️','mist':'🌫️','haze':'🌫️','wind':'💨'};
function show(w){
  if(!w||w.temp==null)return;
  var e='';var d=(w.desc||'').toLowerCase();for(var k in emoji){if(d.indexOf(k)>=0){e=emoji[k];break}}
  document.getElementById('home-date-weather').textContent=w.temp+'°C '+(e||'');
}
// Show cached immediately.
var cached;try{cached=JSON.parse(localStorage.getItem(KEY))}catch(e){}
show(cached);
// Refresh in background if stale or missing.
var ts=parseInt(localStorage.getItem(KEY_TS)||'0');
if(Date.now()-ts<TTL)return;
var lat=localStorage.getItem(KEY_LAT);
var lon=localStorage.getItem(KEY_LON);
if(!lat||!lon){
  if(!navigator.geolocation)return;
  navigator.geolocation.getCurrentPosition(function(pos){
    lat=pos.coords.latitude.toFixed(4);lon=pos.coords.longitude.toFixed(4);
    localStorage.setItem(KEY_LAT,lat);localStorage.setItem(KEY_LON,lon);
    fetchW(lat,lon);
  },function(){},{timeout:5000});
  return;
}
fetchW(lat,lon);
function fetchW(la,lo){
  fetch('/weather?lat='+la+'&lon='+lo,{headers:{'Accept':'application/json'},credentials:'same-origin'})
  .then(function(r){return r.ok?r.json():null})
  .then(function(d){
    if(!d||!d.forecast||!d.forecast.Current)return;
    var c=d.forecast.Current;
    var w={temp:Math.round(c.TempC),desc:c.Description||''};
    localStorage.setItem(KEY,JSON.stringify(w));
    localStorage.setItem(KEY_TS,String(Date.now()));
    show(w);
  }).catch(function(){});
}
})()</script>`)
	dateHTML := dateLine.String()

	var viewerID string
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}

	// ── Cards (always visible) ──
	b.WriteString(`<div id="home-cards">`)
	b.WriteString(dateHTML)

	// Inline card preferences panel — toggled by the ⚙ icon.
	if viewerAcc != nil {
		allCardDefs := []struct{ id, label string }{
			{"reminder", "Reminder"}, {"blog", "Blog"}, {"news", "News"},
			{"markets", "Markets"}, {"social", "Social"}, {"video", "Video"},
		}
		activeSet := map[string]bool{}
		if len(viewerAcc.HomeCards) > 0 {
			for _, id := range viewerAcc.HomeCards {
				activeSet[id] = true
			}
		} else {
			for _, c := range allCardDefs {
				activeSet[c.id] = true
			}
		}
		var checkboxes string
		for _, c := range allCardDefs {
			checked := ""
			if activeSet[c.id] {
				checked = " checked"
			}
			checkboxes += fmt.Sprintf(`<label style="display:flex;align-items:center;gap:8px;padding:6px 0;font-size:14px;border-bottom:1px solid #f0f0f0"><input type="checkbox" name="cards" value="%s"%s onchange="this.form.submit()" style="width:18px;height:18px"> %s</label>`, c.id, checked, c.label)
		}
		b.WriteString(fmt.Sprintf(`<div id="home-card-prefs" style="display:none;padding:12px 16px;margin-bottom:12px;background:#f9f9f9;border-radius:8px;border:1px solid #eee">
<p style="font-weight:600;font-size:14px;margin:0 0 4px">Customise home screen</p>
<p style="font-size:12px;color:#999;margin:0 0 8px">Show or hide cards on your overview.</p>
<form method="POST" action="/account">
<input type="hidden" name="save_cards" value="1">
%s
</form>
</div>`, checkboxes))
	}

	// User card preferences — if set, only show cards in the user's
	// chosen order. Empty = show all in default order.
	var userCards map[string]int // card ID → display order
	if viewerAcc != nil && len(viewerAcc.HomeCards) > 0 {
		userCards = make(map[string]int, len(viewerAcc.HomeCards))
		for i, id := range viewerAcc.HomeCards {
			userCards[id] = i
		}
	}

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
		// If user has card preferences, skip cards not in their list.
		if userCards != nil {
			if _, ok := userCards[card.ID]; !ok {
				continue
			}
		}
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

	// ── Console: fixed bottom bar + full-screen overlay ──
	// Always visible as a bar at the bottom. Tap to expand into a
	// full-screen command overlay. Like Spotlight or Cmd+K.
	if viewerID != "" {
		b.WriteString(fmt.Sprintf(`
<div id="console-bar" onclick="muOpenConsole()" style="position:fixed;bottom:0;left:0;right:0;padding:10px 16px;background:#fff;border-top:1px solid #e0e0e0;cursor:pointer;z-index:900;display:flex;align-items:center;gap:8px;box-shadow:0 -1px 4px rgba(0,0,0,0.05)">
<span style="color:#bbb;font-size:14px;flex:1">Ask Micro anything...</span>
<span style="background:#000;color:#fff;padding:4px 10px;border-radius:6px;font-size:12px">⌘</span>
</div>

<div id="console-overlay" style="display:none;position:fixed;top:0;left:0;right:0;bottom:0;z-index:1000;background:#fff;flex-direction:column;height:100dvh;height:100vh">
<div style="display:flex;align-items:center;padding:12px 16px;border-bottom:1px solid #eee">
<span style="font-weight:600;font-size:15px;flex:1">Console</span>
<a href="#" onclick="muCloseConsole();return false" style="color:#999;text-decoration:none;font-size:20px;padding:4px 8px">✕</a>
</div>
<div id="console-response" style="flex:1;overflow-y:auto;-webkit-overflow-scrolling:touch;padding:16px;display:flex;align-items:center;justify-content:center">
<p style="color:#bbb;font-size:15px">Ask Micro anything</p>
</div>
<form id="console-form" style="display:flex;gap:6px;padding:10px 16px;border-top:1px solid #eee;background:#fff;flex-shrink:0;min-width:0">
<input type="text" id="console-input" placeholder="What's the BTC price? Summarise the news..." maxlength="%d" autocomplete="off" style="flex:1;min-width:0;padding:10px 12px;border:1px solid #ddd;border-radius:8px;font-size:14px;box-sizing:border-box">
<button type="submit" style="padding:10px 14px;background:#000;color:#fff;border:none;border-radius:8px;cursor:pointer;font-size:14px;flex-shrink:0">Ask</button>
</form>
</div>
`, stream.MaxContentLength))
		b.WriteString(consoleScript)
	}

	// Pad the bottom of the page so the fixed console bar doesn't
	// overlap the last card.
	if viewerID != "" {
		b.WriteString(`<div style="height:56px"></div>`)
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
// consoleScript — stateless command prompt. Fixed bottom bar opens a
// full-screen overlay. Ask a question, get an answer. Escape closes.
const consoleScript = `<script>
window.muOpenConsole=function(){
  document.getElementById('console-overlay').style.display='flex';
  document.getElementById('console-bar').style.display='none';
  setTimeout(function(){document.getElementById('console-input').focus()},50);
};
window.muCloseConsole=function(){
  document.getElementById('console-overlay').style.display='none';
  document.getElementById('console-bar').style.display='flex';
};
document.addEventListener('keydown',function(e){
  if(e.key==='Escape'&&document.getElementById('console-overlay').style.display==='flex') muCloseConsole();
});
(function(){
  var form = document.getElementById('console-form');
  var resp = document.getElementById('console-response');
  if (!form || !resp) return;

  function csrfToken() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }

  form.addEventListener('submit', function(ev){
    ev.preventDefault();
    var input = document.getElementById('console-input');
    if (!input) return;
    var q = input.value.trim();
    if (!q) return;

    // Show thinking state.
    resp.style.alignItems = 'flex-start';
    resp.style.justifyContent = 'flex-start';
    resp.innerHTML = '<div style="padding:12px 0"><p style="color:#333;font-weight:600;margin-bottom:8px">' + escHtml(q) + '</p><p style="color:#999">Working...</p></div>';
    input.value = '';

    var headers = { 'Content-Type': 'application/json', 'Accept': 'application/json' };
    var tok = csrfToken();
    if (tok) headers['X-CSRF-Token'] = tok;

    fetch('/stream', {
      method: 'POST',
      credentials: 'same-origin',
      headers: headers,
      body: JSON.stringify({ content: '@micro ' + q })
    }).then(function(r) {
      if (!r.ok) return r.text().then(function(t){ throw new Error(t) });
      // Poll for the agent response — it's async, so we check every 2s.
      var attempts = 0;
      var maxAttempts = 30;
      function poll() {
        attempts++;
        fetch('/stream?format=json&since=' + Math.floor(Date.now()/1000 - 60), { credentials: 'same-origin' })
          .then(function(r){ return r.json() })
          .then(function(data){
            if (!data.events) { if (attempts < maxAttempts) setTimeout(poll, 2000); return; }
            // Find the latest agent response.
            for (var i = 0; i < data.events.length; i++) {
              if (data.events[i].type === 'agent') {
                resp.innerHTML = '<div style="padding:12px 0"><p style="color:#333;font-weight:600;margin-bottom:8px">' + escHtml(q) + '</p><div style="color:#555;line-height:1.6;white-space:pre-wrap;word-wrap:break-word">' + escHtml(data.events[i].content) + '</div></div>';
                return;
              }
            }
            if (attempts < maxAttempts) setTimeout(poll, 2000);
            else resp.innerHTML = '<div style="padding:12px 0"><p style="color:#333;font-weight:600;margin-bottom:8px">' + escHtml(q) + '</p><p style="color:#c00">Timed out waiting for a response. Try again.</p></div>';
          })
          .catch(function(){ if (attempts < maxAttempts) setTimeout(poll, 2000); });
      }
      setTimeout(poll, 2000);
    }).catch(function(err){
      resp.innerHTML = '<div style="padding:12px 0"><p style="color:#c00">' + escHtml(err.message || 'Something went wrong') + '</p></div>';
    });
  });

  function escHtml(s) {
    var d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }
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
