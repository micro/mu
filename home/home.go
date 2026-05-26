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
	"mu/mail"
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
	if viewerAcc != nil {
		label := "+ Invite"
		link := "/invite"
		if viewerAcc.Admin {
			link = "/admin/invite"
		}
		inviteHTML = fmt.Sprintf(`<span id="home-date-actions"><a href="%s" style="color:#555;text-decoration:none">%s</a></span>`, link, label)
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

	// Console prompt — inline at the top, before cards. Claude-style
	// rounded textarea with send button inside.
	if viewerID != "" {
		b.WriteString(fmt.Sprintf(`
<div id="console-prompt" style="margin:0 0 16px">
<form id="console-form" style="position:relative">
<textarea id="console-input" placeholder="Search or look up..." maxlength="%d" rows="1" style="width:100%%;padding:10px 40px 10px 12px;border:1px solid #ddd;border-radius:12px;font-size:14px;font-family:inherit;resize:none;box-sizing:border-box;line-height:1.4;overflow:hidden"></textarea>
<button type="submit" style="position:absolute;right:6px;top:50%%;transform:translateY(-50%%);width:28px;height:28px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:14px;padding:0">&#x2192;</button>
</form>
<div id="console-response" style="display:none;margin-top:12px;padding:14px;background:#f9f9f9;border-radius:10px"></div>
</div>`, stream.MaxContentLength))
		b.WriteString(consoleScript)
	}

	// Inline card preferences panel
	if viewerAcc != nil {
		allCardDefs := []struct{ id, label string }{
			{"reminder", "Reminder"}, {"blog", "Blog"}, {"news", "News"},
			{"markets", "Markets"}, {"social", "Social"}, {"video", "Video"},
			{"mail", "Mail"}, {"web", "Web Search"},
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
			checkboxes += fmt.Sprintf(`<label style="display:flex;align-items:center;gap:8px;padding:6px 0;font-size:14px;border-bottom:1px solid #f0f0f0"><input type="checkbox" name="cards" value="%s"%s style="width:18px;height:18px"> %s</label>`, c.id, checked, c.label)
		}
		// App widget checkboxes — any public app can be pinned as a card.
		var widgetCheckboxes string
		activeWidgets := map[string]bool{}
		if viewerAcc != nil {
			for _, w := range viewerAcc.Widgets {
				activeWidgets[w] = true
			}
		}
		publicApps := apps.GetPublicApps()
		if len(publicApps) > 0 {
			for _, a := range publicApps {
				checked := ""
				if activeWidgets[a.Slug] {
					checked = " checked"
				}
				widgetCheckboxes += fmt.Sprintf(`<label style="display:flex;align-items:center;gap:8px;padding:6px 0;font-size:14px;border-bottom:1px solid #f0f0f0"><input type="checkbox" name="widgets" value="%s"%s style="width:18px;height:18px"> %s</label>`, htmlEsc(a.Slug), checked, htmlEsc(a.Name))
			}
		}

		b.WriteString(fmt.Sprintf(`<div id="home-card-prefs" style="display:none;padding:12px 16px;margin-bottom:12px;background:#f9f9f9;border-radius:8px;border:1px solid #eee">
<p style="font-weight:600;font-size:14px;margin:0 0 4px">Customise home screen</p>
<p style="font-size:12px;color:#999;margin:0 0 8px">Show or hide cards.</p>
<div id="card-checkboxes">%s</div>`, checkboxes))
		if widgetCheckboxes != "" {
			b.WriteString(fmt.Sprintf(`<p style="font-weight:600;font-size:13px;margin:10px 0 4px">App widgets</p>
<p style="font-size:12px;color:#999;margin:0 0 6px">Pin apps as cards.</p>
<div id="widget-checkboxes">%s</div>`, widgetCheckboxes))
		}
		b.WriteString(`<script>
(function(){
  function csrfToken(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
  function savePrefs(type,containerId){
    var checks=document.querySelectorAll('#'+containerId+' input[type=checkbox]');
    var body=new URLSearchParams();
    body.set(type==='cards'?'save_cards':'save_widgets','1');
    checks.forEach(function(c){if(c.checked)body.append(type==='cards'?'cards':'widgets',c.value)});
    var h={'Content-Type':'application/x-www-form-urlencoded'};
    var tok=csrfToken();if(tok)h['X-CSRF-Token']=tok;
    fetch('/account',{method:'POST',credentials:'same-origin',headers:h,body:body.toString()})
    .then(function(){location.reload()});
  }
  document.querySelectorAll('#card-checkboxes input').forEach(function(c){
    c.addEventListener('change',function(){savePrefs('cards','card-checkboxes')});
  });
  document.querySelectorAll('#widget-checkboxes input').forEach(function(c){
    c.addEventListener('change',function(){savePrefs('widgets','widget-checkboxes')});
  });
})();
</script></div>`)
	}

	var userCards map[string]int
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

	// User-specific cards — rendered per-request, not cached.
	if viewerID != "" {
		isCardEnabled := func(id string) bool {
			if userCards == nil {
				return false // mail/web are opt-in, not in default set
			}
			_, ok := userCards[id]
			return ok
		}

		// Mail card — recent messages.
		if isCardEnabled("mail") {
			mailContent := mail.GetRecentThreadsPreview(viewerID, 3)
			mailContent += app.Link("More", "/mail")
			rightHTML = append(rightHTML, fmt.Sprintf(app.CardTemplate, "mail", "mail", "Mail", mailContent))
		}

		// Web search card.
		if isCardEnabled("web") {
			webContent := `<form method="GET" action="/web"><input type="text" name="q" placeholder="Search the web..." style="width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box"></form>`
			rightHTML = append(rightHTML, fmt.Sprintf(app.CardTemplate, "web", "web", "Web Search", webContent))
		}

		// App widgets — user-selected apps rendered as iframe cards.
		if viewerAcc != nil && len(viewerAcc.Widgets) > 0 {
			for _, slug := range viewerAcc.Widgets {
				a := apps.GetApp(slug)
				if a == nil {
					continue
				}
				widgetContent := fmt.Sprintf(`<iframe src="/apps/%s" style="width:100%%;height:300px;border:none;border-radius:6px" sandbox="allow-scripts allow-same-origin" loading="lazy"></iframe>`, htmlEsc(a.Slug))
				rightHTML = append(rightHTML, fmt.Sprintf(app.CardTemplate, "app-"+a.Slug, "app-"+a.Slug, htmlEsc(a.Name), widgetContent))
			}
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n")))
	}

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
// consoleScript — Claude-style inline prompt with overlay for responses.
// consoleScript calls the agent API directly. No stream, no polling,
// no overlay. Question → POST /agent/run → response inline.
const consoleScript = `<script>
(function(){
  var form = document.getElementById('console-form');
  var resp = document.getElementById('console-response');
  var currentFlowId = '';
  if (!form || !resp) return;

  function csrfToken() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }

  var input = document.getElementById('console-input');
  if (input) {
    input.addEventListener('input', function(){ this.style.height='auto'; this.style.height=Math.min(this.scrollHeight,120)+'px'; });
    input.addEventListener('keydown', function(e){ if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();form.dispatchEvent(new Event('submit',{cancelable:true}));} });
  }
  form.addEventListener('submit', function(ev){
    ev.preventDefault();
    var input = document.getElementById('console-input');
    if (!input) return;
    var q = input.value.trim();
    if (!q) return;

    resp.style.display = 'block';
    var qid = 'q' + Date.now();
    resp.innerHTML += '<div id="'+qid+'" style="margin-top:12px;padding-bottom:12px;border-bottom:1px solid #eee"><p style="color:#333;font-weight:600;margin:0 0 6px">' + escHtml(q) + '</p><p style="color:#999;margin:0" id="'+qid+'-a">Working...</p></div>';
    input.value = '';
    input.style.height = 'auto';

    var headers = { 'Content-Type': 'application/json', 'Accept': 'application/json' };
    var tok = csrfToken();
    if (tok) headers['X-CSRF-Token'] = tok;

    fetch('/agent/run', {
      method: 'POST',
      credentials: 'same-origin',
      headers: headers,
      body: JSON.stringify({ prompt: q, context_id: currentFlowId })
    }).then(function(r){
      if (!r.ok) return r.text().then(function(t){ throw new Error(t) });
      return r.json();
    }).then(function(data){
      var answer = (data && data.answer) ? data.answer : (typeof data === 'string' ? data : JSON.stringify(data));
      if (data && data.flow_id) currentFlowId = data.flow_id;
      var ae = document.getElementById(qid+'-a'); if(ae) ae.outerHTML = '<div style="color:#555;line-height:1.6;word-wrap:break-word">' + renderMd(answer) + '</div>';
    }).catch(function(err){
      var ee = document.getElementById(qid+'-a'); if(ee) ee.outerHTML = '<p style="color:#c00;margin:0">' + escHtml(err.message || 'Something went wrong') + '</p>';
    });
  });

  function escHtml(s) {
    var d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }
  function renderMd(s) {
    s = escHtml(s);
    var bt = String.fromCharCode(96);
    var codeBlockRe = new RegExp(bt+bt+bt+'(\\w*)\\n([\\s\\S]*?)'+bt+bt+bt, 'g');
    var inlineCodeRe = new RegExp(bt+'([^'+bt+']+)'+bt, 'g');
    s = s.replace(codeBlockRe, '<pre style="background:#f0f0f0;padding:10px;border-radius:6px;overflow-x:auto;font-size:13px"><code>$2</code></pre>');
    s = s.replace(inlineCodeRe, '<code style="background:#eee;padding:1px 4px;border-radius:3px;font-size:13px">$1</code>');
    s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    s = s.replace(/\*(.+?)\*/g, '<em>$1</em>');
    s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener" style="color:#06c">$1</a>');
    s = s.replace(/^### (.+)$/gm, '<strong style="font-size:14px">$1</strong>');
    s = s.replace(/^## (.+)$/gm, '<strong style="font-size:15px">$1</strong>');
    s = s.replace(/^# (.+)$/gm, '<strong style="font-size:16px">$1</strong>');
    s = s.replace(/^[-*] (.+)$/gm, '<li style="margin-left:16px;list-style:disc">$1</li>');
    s = s.replace(/^\d+\. (.+)$/gm, '<li style="margin-left:16px;list-style:decimal">$1</li>');
    s = s.replace(/\n\n/g, '</p><p style="margin:8px 0">');
    s = s.replace(/\n/g, '<br>');
    return '<p style="margin:8px 0">' + s + '</p>';
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
