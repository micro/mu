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
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/mail"
	"mu/service/news"
)

//go:embed cards.json
var f embed.FS

// Template is the home cards container: two independent columns (so each flows
// at its own height, no ragged gaps) on desktop, a single stack on mobile.
var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() string {
	return news.Headlines()
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

	// Card renderers, derived from the Specs that declare them.
	//
	// This was a map of names kept by hand next to the services it named, so a
	// service could grow a card and never appear here, and a renamed one would
	// silently render nothing. Agent, Chat and News stay written out: they are
	// this package's own cards, not a service's view of itself.
	cardFunctions := map[string]func() string{}
	for _, sp := range service.Cards() {
		cardFunctions[sp.Name] = sp.Card
	}
	// Set after the derived ones so they win. Agent is this package's own card,
	// and news has two renderers — the service's headline list, and this one,
	// built for the home screen.
	//
	// Chat is no longer among them: its card was a second question box that
	// posted to /chat, which meant home had two prompts, only one of which
	// reached the agent. Chat's own Spec renders it now, as what is being
	// discussed.
	cardFunctions["agent"] = AgentCard
	cardFunctions["news"] = newsCard

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

// CardHandler serves individual card HTML fragments at /home/card/{id}.
// Each card loads independently so one slow/broken card can't block
// the entire home page.
func CardHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/home/card/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	// User-specific cards that need session context.
	if id == "mail" || id == "web" {
		viewerID := ""
		if sess, _ := auth.TrySession(r); sess != nil {
			viewerID = sess.Account
		}
		if viewerID == "" {
			w.WriteHeader(204)
			return
		}
		switch id {
		case "mail":
			content := mail.GetRecentThreadsPreview(viewerID, 3)
			content += app.Link("More", "/mail")
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, app.CardTemplate, "mail", "mail", "Mail", content)
		case "web":
			content := `<form method="GET" action="/web"><input type="text" name="q" placeholder="Search the web..." style="width:100%%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box"></form>`
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, app.CardTemplate, "web", "web", "Search", content)
		}
		return
	}

	// App widget cards.
	if strings.HasPrefix(id, "app-") {
		slug := strings.TrimPrefix(id, "app-")
		a := apps.GetApp(slug)
		if a == nil {
			http.NotFound(w, r)
			return
		}
		content := fmt.Sprintf(`<iframe src="/apps/%s" style="width:100%%;height:300px;border:none;border-radius:6px" sandbox="allow-scripts allow-same-origin" loading="lazy"></iframe>`, slug)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, app.CardTemplate, id, id, a.Name, content)
		return
	}

	// Standard cached cards — serve with a 3-second timeout to prevent
	// deadlocks from blocking the response.
	done := make(chan string, 1)
	go func() {
		RefreshCards()
		cacheMutex.RLock()
		defer cacheMutex.RUnlock()
		for _, card := range Cards {
			if card.ID == id {
				content := card.CachedHTML
				if strings.TrimSpace(content) == "" {
					done <- ""
					return
				}
				if card.Link != "" {
					content += app.Link("More", card.Link)
				}
				done <- fmt.Sprintf(app.CardTemplate, card.ID, card.ID, card.Title, content)
				return
			}
		}
		done <- ""
	}()

	select {
	case html := <-done:
		if html == "" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(html))
	case <-time.After(3 * time.Second):
		app.Log("home", "Card %s timed out", id)
		w.WriteHeader(204)
	}
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
			ID     string `json:"id"`
			Title  string `json:"title"`
			HTML   string `json:"html"`
			Column string `json:"column"`
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

	_, viewerAcc := auth.TrySession(r)

	var b strings.Builder

	// Date header + weather + invite — Overview only, built here
	// and injected into the cards div below.
	now := time.Now()
	var dateLine strings.Builder
	inviteHTML := ""
	if viewerAcc != nil {
		// Nothing. Inviting people is an operator's job, so it moved to the
		// account page next to the other things only an admin does — it was
		// sitting at the top of everybody's console, which put an errand nobody
		// runs beside the date.
	} else {
		// Logged out: the home screen IS the landing, so once a visitor has
		// used it, offer a clear free sign-up (primary) plus log in.
		inviteHTML = `<span id="home-date-actions"><a href="/signup" style="color:#111;text-decoration:none;font-weight:700">Sign up free</a> <a href="/login" style="color:#888;text-decoration:none;margin-left:10px">Log in</a></span>`
	}
	// No cog. It toggled a card picker, and there is no card selection to pick
	// any more — the instance chooses the cards and everybody gets the same
	// ones, so there is nothing here for a settings control to open.
	gearHTML := ""
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

	// ── Cards ──
	b.WriteString(`<div id="home-cards">`)

	// Date + invite/settings above the input
	b.WriteString(dateHTML)

	// What is yours and what is in flight, before the world's content — see
	// home/system.go.
	systemHTML := systemStrip(viewerAcc)

	// Inline agent — Home answers here rather than navigating away, and it renders
	// for everyone: logged out, this is the public face of the product. Signed-in
	// users get personalised chips; guests get generic starters and the guest chat.
	{
		var suggestions []string
		if viewerID != "" {
			if unread := mail.GetUnreadCount(viewerID); unread > 0 {
				if unread == 1 {
					suggestions = append(suggestions, "Read my unread email")
				} else {
					suggestions = append(suggestions, fmt.Sprintf("Read my %d unread emails", unread))
				}
			}
		}
		// No chips for things the cards below already show.
		//
		// There were three: top movers, "Today's news" and "What's happening?".
		// Every one of them asked the agent to fetch something already on the
		// screen — the markets card has the prices, the news card has the
		// headlines, the blog digest is what's happening. A chip that spends a
		// model call to re-fetch what you can already see is a worse version of
		// scrolling down.
		//
		// What is left is the one suggestion the cards cannot answer: unread
		// mail, which needs your inbox opened and read rather than displayed.

		// The question is carried in a data attribute and read by a listener,
		// not written into an onclick.
		//
		// It was `onclick="…muChatAsk(` + JSString(s) + `)"`, and JSString
		// returns a JSON string — double-quoted. Dropped into a double-quoted
		// attribute, the attribute ended at that first quote, so the browser
		// received the handler `window.muChatAsk&&window.muChatAsk(` and threw
		// "Unexpected end of input". Every chip was dead, silently, because the
		// `&&` guard made a broken handler look like a missing one.
		//
		// Escaping the literal would have fixed it. Not putting code in an
		// attribute fixes the whole category, and survives a stricter CSP.
		var chips string
		for _, s := range suggestions {
			chips += chipMarkup(s)
		}
		if chips != "" {
			chips += `<script>
(function(){
  document.querySelectorAll('.console-suggest[data-ask]').forEach(function(btn){
    btn.addEventListener('click', function(){
      var q = btn.getAttribute('data-ask');
      if (window.muChatAsk) { window.muChatAsk(q); return; }
      // The chat component has not finished wiring itself up yet; wait for it
      // rather than dropping the click on the floor.
      var tries = 0;
      var again = function(){
        if (window.muChatAsk) { window.muChatAsk(q); return; }
        if (++tries < 40) setTimeout(again, 50);
      };
      again();
    });
  });
})();
</script>`
		}

		b.WriteString(`<div id="home-agent" style="margin:0 0 20px">`)
		b.WriteString(app.ChatComponent(app.ChatConfig{Guest: viewerID == "", HideSuggestions: true,
			OfferAgentPicker: viewerID != ""}))
		if chips != "" {
			b.WriteString(fmt.Sprintf(`<div class="home-chips">%s</div>`, chips))
		}
		b.WriteString(`</div>`)
	}

	// Pinned apps — a quick-launch strip at the top, just below the suggestion
	// chips. Selected in the preferences panel; opens the app on click.
	// Ask first, then what is yours, then the world's content.
	b.WriteString(systemHTML)

	// What your agents actually did. docs/PRODUCT.md puts this third on the
	// console — after what is in flight and what is waiting — and called it the
	// missing piece: a run that happened while you were elsewhere was recorded
	// and never shown, so the product never told you when something worked.
	if viewerAcc != nil {
		if runs := agent.RecentRuns(viewerAcc.ID, 5); runs != "" {
			b.WriteString(runs)
		} else {
			// Nothing has run yet, which is every brand-new account and every
			// fresh instance. The console was three zeros and half a page of
			// white — the first thing the person who just installed this sees.
			// A screen with nothing on it has to say what to do next.
			b.WriteString(firstRunCTA())
		}
	}

	if viewerAcc != nil && len(viewerAcc.Widgets) > 0 {
		var tiles string
		for _, slug := range viewerAcc.Widgets {
			a := apps.GetApp(slug)
			if a == nil {
				continue
			}
			tiles += fmt.Sprintf(`<a class="home-app" href="/apps/%s">%s</a>`, htmlEsc(a.Slug), htmlEsc(a.Name))
		}
		if tiles != "" {
			b.WriteString(fmt.Sprintf(`<div id="home-apps">%s</div>`, tiles))
		}
	}

	// The cards, on Home, where they were.
	//
	// They were moved to a /context page on the argument that Home is a console
	// and the cards are context. That gave the product two home screens, a
	// sidebar entry for a page nobody asked for, and a card picker to justify
	// the page — and it left /account describing a home screen that no longer
	// had any cards on it. Home shows what this instance knows right now, which
	// is the whole demonstration that the tools are real.
	b.WriteString(CardsHTML(r, viewerAcc))

	b.WriteString(`</div>`) // close #home-cards

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
    fetch('/', {headers:{Accept:'application/json'}})
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

	// Deep-link prefill: /?q=... or /home?prompt=... seeds the agent and submits
	// it, so a shared link lands on the home screen with the answer already coming.
	prefill := r.URL.Query().Get("q")
	if prefill == "" {
		prefill = r.URL.Query().Get("prompt")
	}
	if prefill != "" {
		b.WriteString(`<script>(function(){var v=` + app.JSString(prefill) + `;var f=function(){if(window.muChatAsk){window.muChatAsk(v);history.replaceState(null,'','` + r.URL.Path + `');}else{setTimeout(f,60);}};f();})()</script>`)
	}

	// Display mode: hide nav, header, footer for kiosk/wall display
	bodyClass := ` class="page-home"`
	if displayMode {
		bodyClass = ` class="page-home display-mode"`
	}

	lang := app.GetUserLanguage(r)
	// The home screen is where someone lands after signing up, so it is the one
	// page that must carry the invitation to connect an agent.
	body := app.ConnectBanner(r) + b.String()
	html := app.RenderHTMLWithLangAndBody("Home", "The home screen", body, lang, bodyClass, viewerAcc)
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
// consoleScript — Claude-style inline prompt with overlay for responses.
// consoleScript calls the agent API directly. No stream, no polling,
// no overlay. Question → POST /agent/run → response inline.
// consoleScript — AI prompt with persistent last response, contextual
// suggestions, and typing indicator.
// consoleScript — AI prompt with flip layout (input moves below response),
// persistent last response, typing dots, suggestion pills.

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

// chipMarkup renders one starter chip. The question rides in a data attribute,
// escaped once — see the comment where these are built for what putting it in
// an onclick cost.
func chipMarkup(q string) string {
	return fmt.Sprintf(`<button type="button" class="console-suggest" data-ask="%s" `+
		`style="padding:6px 12px;border:1px solid #e0e0e0;border-radius:6px;background:#fff;`+
		`font-size:13px;color:#555;cursor:pointer;white-space:nowrap;font-family:inherit">%s</button>`,
		htmlEsc(q), htmlEsc(q))
}


// CardsHTML renders the cards a reader watches: the live view of each
// service they chose to keep an eye on.
//
// They live on Home, and there is no setting for which ones. There was: a
// picker, in three places, over a per-account allowlist and a stored order —
// a composition step in front of a page whose job is to show you something the
// moment you arrive. Nobody arrives wanting to choose cards. The instance
// picks them, in cards.json, and everybody gets the same ones.
func CardsHTML(r *http.Request, viewerAcc *auth.Account) string {
	var b strings.Builder
	viewerID := ""
	if viewerAcc != nil {
		viewerID = viewerAcc.ID
	}

	// Which cards to show. Default cards (cards.json) show unless the user has
	// deselected them; cards added after the user last customised default to
	// visible (see auth.Account.ShowHomeCard). Order and column come from
	// cards.json. mail/web are opt-in and off unless explicitly enabled.

	tooltips := map[string]string{
		"blog":    "Microblog posts with daily AI-generated digests",
		"news":    "Headlines from RSS feeds, sorted by time",
		"markets": "Live crypto, futures, and commodity prices",
		"prayer":  "Islamic prayer times, and a daily verse, saying and name",
		"social":  "Public discussion threads",
		"video":   "Latest videos from curated channels",
		"chat":    "Discussions happening on this instance right now",
	}

	// One ordered list, and mail and search are in it like everything else.
	//
	// They used to be appended after the loop, so they could only ever land at
	// the bottom of the right column however anybody felt about it.
	type rendered struct{ id, html string }
	build := func(id string) string {
		switch id {
		case "mail":
			if viewerID == "" {
				return ""
			}
			return mail.GetRecentThreadsPreview(viewerID, 3) + app.Link("More", "/mail")
		case "web":
			if viewerID == "" {
				return ""
			}
			return `<form method="GET" action="/web"><input type="text" name="q" placeholder="Search the web..." style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box"></form>`
		}
		for _, card := range Cards {
			if card.ID != id {
				continue
			}
			content := card.CachedHTML
			if strings.TrimSpace(content) == "" {
				return ""
			}
			if card.Link != "" {
				content += app.Link("More", card.Link)
			}
			return content
		}
		return ""
	}
	titleOf := func(id string) string {
		switch id {
		case "mail":
			return "Mail"
		case "web":
			return "Search"
		}
		for _, card := range Cards {
			if card.ID == id {
				return card.Title
			}
		}
		return id
	}

	// The instance's order, from cards.json, for everybody. Mail and search are
	// appended because they need a session to render at all — a guest gets the
	// rest and no empty boxes where their inbox would be.
	order := make([]string, 0, len(Cards)+2)
	for _, card := range Cards {
		order = append(order, card.ID)
	}
	if viewerID != "" {
		order = append(order, "mail", "web")
	}

	var shown []rendered
	for _, id := range order {
		content := build(id)
		if strings.TrimSpace(content) == "" {
			continue
		}
		title := titleOf(id)
		if tip, ok := tooltips[id]; ok {
			title += fmt.Sprintf(` <span class="card-tooltip" data-tip="%s" onclick="event.stopPropagation();document.querySelectorAll('.card-tooltip.show').forEach(function(e){e.classList.remove('show')});this.classList.toggle('show')">?</span>`, htmlEsc(tip))
		}
		shown = append(shown, rendered{id, fmt.Sprintf(app.CardTemplate, id, id, title, content)})
	}

	// Two columns, filled alternately, so first means top-left and second means
	// top-right. Any other distribution would make the stored order unreadable
	// on the page that displays it.
	var leftHTML, rightHTML []string
	for i, c := range shown {
		if i%2 == 0 {
			leftHTML = append(leftHTML, c.html)
		} else {
			rightHTML = append(rightHTML, c.html)
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template, strings.Join(leftHTML, "\n"), strings.Join(rightHTML, "\n")))
	}
	return b.String()
}

// firstRunCTA is what a console shows before anything has happened on it.
//
// Two doors, in the order the product cares about: point an agent you already
// have at this instance, or build one here. Both are one click, and the copy
// says what you get rather than what to press.
func firstRunCTA() string {
	return `<div class="cards-head"><span>Get started</span>` +
		`<span class="cards-sub">nothing has run yet</span></div>
<div class="cta-row">
  <a class="cta-card" href="/tools#connect">
    <strong>Connect an agent you have</strong>
    <span>Claude, Cursor, or your own program. One endpoint, one token, every tool.</span>
  </a>
  <a class="cta-card" href="/agent/new">
    <strong>Build one here</strong>
    <span>Give it a standing instruction and the services it may reach, then talk to it.</span>
  </a>
</div>
<style>
.cta-row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.cta-card{display:block;border:1px solid #e8e8e8;border-radius:8px;padding:14px 16px;
  text-decoration:none;color:inherit;background:var(--card-background,#fff)}
.cta-card:hover{border-color:#bbb}
.cta-card strong{display:block;font-size:14px;margin:0 0 4px;color:var(--text-primary,#111)}
.cta-card span{display:block;font-size:13px;color:#666;line-height:1.45}
@media only screen and (max-width:600px){.cta-row{grid-template-columns:1fr}}
</style>`
}
