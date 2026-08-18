package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/inbox"
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

// Template is the home cards container: two columns on desktop, one stack on a
// phone.
//
// The left is the stream — everything that happened, newest first. The right is
// what is fixed: how things are now, in an order that does not move. Two
// independent columns rather than a grid, so each flows at its own height and
// an empty card does not leave a hole in the other.
var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() (string, time.Time) {
	return news.Headlines(), news.CardAt()
}

type Card struct {
	ID       string
	Title    string
	Icon     string // Optional icon image path (e.g. "/news.png")
	Position int
	Link     string
	Content  func() service.Card
	// CachedHTML is the last render; At is when what it shows happened, zero
	// for a card that shows how things are rather than something that occurred.
	CachedHTML  string
	At          time.Time
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last time the render changed
}

// Streamed reports whether this card belongs in the left column.
//
// Derived rather than configured. cards.json used to say "left" or "right" per
// card, which meant the layout was a list somebody kept in step by hand — and
// the answer was already knowable: a card that can say when what it shows
// happened is a card with a place in a chronology, and one that cannot is a
// standing view of how things are. See service.Card.
func (c Card) Streamed() bool { return !c.At.IsZero() }

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
	// silently render nothing. News stays written out: it is this package's own
	// card, not the service's view of itself.
	cardFunctions := map[string]func() service.Card{}
	for _, sp := range service.Cards() {
		cardFunctions[sp.Name] = sp.Card
	}
	// Set after the derived ones so they win: news has two renderers — the
	// service's headline list, and this one, built for the home screen.
	//
	// Two cards used to be here and are not. Chat's was a second question box
	// posting to /chat, so home had two prompts and only one reached the agent.
	// Agent's was a whole second agent form — a prompt box, a Fast/Best model
	// select /agent never read, and a hand-written dropdown still naming tools
	// called Reminder, Topup and Code Run. It was from when the agent was a
	// service with a card like any other; it is not, and cards.json had already
	// stopped asking for it, so it rendered nowhere while it rotted.
	cardFunctions["news"] = service.Timed(newsCard)

	// Build Cards array from config
	Cards = []Card{}

	for _, c := range config.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Icon:     c.Icon,
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	for _, c := range config.Right {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:    c.ID,
				Title: c.Title,
				Icon:  c.Icon,
				// Positions in the file run from zero per column, so the
				// right-hand ones are pushed past the left to keep the file's
				// own order readable now that the column is derived.
				Position: 100 + c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	// The file's order is the fallback: it is what decides the fixed column,
	// and what orders two stream cards carrying the same time.
	sort.Slice(Cards, func(i, j int) bool { return Cards[i].Position < Cards[j].Position })

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

		// Get fresh content, and when what it shows happened.
		fresh := card.Content()
		card.At = fresh.At

		// Calculate hash
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fresh.HTML)))

		// Only update if content changed
		if hash != card.ContentHash {
			card.CachedHTML = fresh.HTML
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
				Column: card.column(),
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
		inviteHTML = `<span id="home-date-actions"><a href="/signup" style="color:#111;text-decoration:none;font-weight:700">Sign up</a> <a href="/login" style="color:#888;text-decoration:none;margin-left:10px">Log in</a></span>`
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
		// The box is one way in and not the interesting one.
		//
		// Home has always offered a place to type, which makes the agent look
		// like a chat on a page — something you visit. The thing that makes it
		// more than that is that it has an address and answers whether or not
		// anybody has this page open, and there was nowhere on the screen you
		// arrive at saying so.
		//
		// And what is in it, when there is anything. The link on its own was a
		// word pointing at a page, so the one thing the product claims — that
		// the agent answers whether or not you have this open — was visible only
		// to somebody who clicked through to check. Mail that arrived and was
		// answered overnight left Home looking exactly as it had the night
		// before. See inbox.Preview.
		//
		// The address line says where to write and nothing else. It used to end
		// in "Your inbox →" as well, so a screen with the preview on it carried
		// two links to the same page, one directly above the other, under a list
		// of the very conversations they led to. The preview's own link is the
		// way through; when there is no preview there is nothing to go to, and
		// the sidebar carries Inbox regardless.
		if viewerID != "" {
			// The address of whichever agent is answering. The id is the
			// contract with the picker in the chat component, which rewrites it
			// when the selection changes — this said agent@ while the control
			// directly above it said "answering as Test".
			if addr := mail.SharedAgentAddress(); addr != "" {
				b.WriteString(`<p class="home-inbox">Or write to it at <code id="mu-agent-addr">` +
					html.EscapeString(addr) + `</code> — from your mail, your phone, anywhere.</p>`)
			}
			b.WriteString(inbox.Preview(viewerID))
		}
		b.WriteString(`</div>`)
	}

	// No counts strip. Four tiles reading Agents 0, Unread 0, Apps 0, Credits
	// 100 is a dashboard of numbers rather than a thing you can act on, and
	// every one of them duplicates a sidebar row that is already one click
	// away. What actually belongs above the world's content is what your agents
	// did — which is the next block, and which says something a count cannot.

	// No runs block and no mail card here. Both were what-is-yours placed above
	// the world's content, and both said less than the space they cost: one run
	// is a receipt for something you just watched happen, and an inbox preview
	// is three subject lines beside a Mail page one click away. /runs and /mail
	// are the pages for them, and the header already carries an unread badge.

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
	//
	// Labelled, because that demonstration only lands if somebody can tell what
	// they are looking at. Unlabelled, the cards read as a dashboard somebody
	// configured — headlines, prices, weather, arranged on a page. What they
	// actually are is the tools answering, live, right now: the same calls an
	// agent makes, rendered. Two words say so.
	b.WriteString(`<p class="home-section"><small>Live context</small></p>`)
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

	lang := app.UserLanguage(r)
	// The home screen is where someone lands after signing up, so it is the one
	// page that must carry the invitation to connect an agent.
	body := app.ConnectBanner(r) + b.String()
	html := app.RenderHTMLWithLangAndBody("Home", "The home screen", body, lang, bodyClass, viewerAcc)
	w.Write([]byte(html))
}

// htmlEsc escapes text for HTML.
//
// It delegates rather than reimplementing, because this package had its own
// version and it escaped one character fewer than the others: & < > and the
// double quote, but not the single quote. Nothing here puts output in a
// single-quoted attribute today, so it was a hazard rather than a hole — and
// the next single-quoted attribute would have made it one, with nothing to say
// the escaper was weaker than it looked.
func htmlEsc(s string) string { return html.EscapeString(s) }

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

	// Order and column come from cards.json.

	tooltips := map[string]string{
		"blog":    "Microblog posts with daily AI-generated digests",
		"news":    "Headlines from RSS feeds, sorted by time",
		"markets": "Live crypto, futures, and commodity prices",
		"prayer":  "Islamic prayer times, and a daily verse, saying and name",
		"social":  "Public discussion threads",
		"video":   "Latest videos from curated channels",
		"images":  "A picture a day, generated here",
	}

	// Each card renders in the column cards.json puts it in.
	//
	// It used to flatten every card into one list and deal them out
	// alternately — left, right, left, right — which threw the configuration
	// away. The file said blog, prayer, news on the left and markets, images,
	// social, video on the right; the page rendered blog, news, social on the
	// left and prayer, markets, video on the right. Nobody could set the
	// layout, because the only code that read the layout was ignoring it.
	//
	// Dealing also meant an empty card reshuffled everything after it, so the
	// whole page moved depending on whether the daily image had landed yet.
	// The stream first, newest at the top; then what is fixed, in the file's
	// order. Sorted here rather than at load, because a card's time changes as
	// things arrive and the ordering has to change with it.
	ordered := make([]Card, len(Cards))
	copy(ordered, Cards)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Streamed() != ordered[j].Streamed() {
			return ordered[i].Streamed()
		}
		if ordered[i].Streamed() {
			return ordered[i].At.After(ordered[j].At)
		}
		return ordered[i].Position < ordered[j].Position
	})

	var leftHTML, rightHTML []string
	for _, card := range ordered {
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
		// When it is from, on the card, which is the whole point of the stream:
		// a row of headlines with no age on it reads as "now" whether it is an
		// hour old or a week.
		if card.Streamed() {
			title += ` <span class="card-when">` + htmlEsc(app.TimeAgo(card.At)) + `</span>`
		}
		rendered := fmt.Sprintf(app.CardTemplate, card.ID, card.ID, title, content)
		if card.Streamed() {
			leftHTML = append(leftHTML, rendered)
		} else {
			rightHTML = append(rightHTML, rendered)
		}
	}

	if len(leftHTML) > 0 || len(rightHTML) > 0 {
		b.WriteString(fmt.Sprintf(Template, strings.Join(leftHTML, "\n"), strings.Join(rightHTML, "\n")))
	}
	return b.String()
}

// column names where a card renders, for the JSON the page refreshes itself
// with.
func (c Card) column() string {
	if c.Streamed() {
		return "left"
	}
	return "right"
}
