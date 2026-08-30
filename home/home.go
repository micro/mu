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

	"mu/agent"
	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/service"
	"mu/service/news"
)

//go:embed cards.json
var f embed.FS

// Template is the home cards container.
//
// It was two columns: the left the stream, everything that happened newest
// first, and the right what is fixed, how things are now. Two independent
// columns rather than a grid, so each flowed at its own height and an empty
// card did not leave a hole in the other.
//
// The cards are tiles now — an icon and a name, no body — so there are no
// heights to flow and nothing for a column to be. The stylesheet dissolves
// both wrappers with display:contents and lays every card out as one grid.
// This and cards.json's column field are therefore inert: they still decide
// which of two lists a card is written into, and nothing downstream reads it.
// Kept because deleting them is a change to the card model rather than to the
// layout, and worth doing on its own.
var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

func newsCard() (string, time.Time) {
	return news.Headlines(), news.CardAt()
}

type Card struct {
	ID    string
	Title string
	Icon  string // Optional icon image path (e.g. "/news.png")
	// Column is "left" or "right", from cards.json. Position orders within it.
	Column   string
	Position int
	Link     string
	// Content renders it, and says whether that render is the same for
	// everybody. A personal one never enters the cache below — see RefreshCards.
	Content service.Renderer
	// CachedHTML is the last render; At is when what it shows happened, zero
	// for a card that shows how things are rather than something that occurred.
	CachedHTML  string
	At          time.Time
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last time the render changed
}

// Streamed reports whether what this card shows happened at a time.
//
// It used to decide the column too, on the argument that the answer was already
// knowable: a card that can say when its contents happened has a place in a
// chronology, one that cannot is a standing view of how things are. True about
// the cards, and not a layout. It sorted blog, news, social, video and images
// into one column and left markets and prayer alone in the other — four against
// two, and the two halves of the page no longer the same length.
//
// cards.json says which column, and now decides it again. What is on the left
// and what is on the right is a judgement about how the page looks, and there is
// no property of a card that computes it. See Column.
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
	cardFunctions := map[string]service.Renderer{}
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

	// Positions run from zero within each column, because each column is its
	// own list in the file. They were offset by 100 on the right to flatten
	// both into one sequence, back when the column was computed and the file's
	// halves were only a way of writing the order down. The halves mean what
	// they say again, so a position means its place in its own column.
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

	// The file's order, within the file's column.
	sort.SliceStable(Cards, func(i, j int) bool {
		if Cards[i].Column != Cards[j].Column {
			return Cards[i].Column == "left"
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

		// A personal card is not cached at all.
		//
		// This cache is one set of strings shared by every viewer — see
		// CachedHTML — so a card that answered for whoever triggered the
		// refresh would be served to everybody after them. That was the whole
		// reason every card was rendered here as Anyone, and the cost was that
		// prayer — the one personal card cards.json puts on this page — showed
		// its next-prayer mark to nobody, because that mark is computed from
		// where the reader is. The sharing rule was being enforced by throwing
		// away the reader, on every card, because the type could not say which
		// ones minded.
		//
		// It can now. A personal card is left out of the cache and drawn per
		// request instead, against the reader who asked — see cardsHTMLFor.
		// Skipping it here also skips a render nobody will read, which for a
		// card that fetches is a request saved every two minutes.
		if card.Content.Personal() {
			continue
		}

		// Get fresh content, and when what it shows happened.
		fresh := card.Content.Render(service.Anyone())
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
		// The poll refreshes the page in place, so it renders for the same
		// reader the page did — otherwise a signed-in reader's weather reverted
		// to the signed-out card two minutes after arriving.
		pollWho := service.Anyone()
		if _, acc := auth.TrySession(r); acc != nil {
			pollWho = service.For(acc.ID)
		}
		var result []cardData
		for _, card := range Cards {
			// The same two halves the page builds, from the same two
			// functions. Sending CachedHTML raw is what took the More link off
			// every card on the first refresh, and sending the bare Title is
			// why the age on a card never moved while its contents did.
			body := cardBody(card, pollWho)
			if body == "" {
				continue
			}
			result = append(result, cardData{
				ID:     card.ID,
				Title:  cardHead(card),
				HTML:   body,
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
		inviteHTML = `<span id="home-date-actions"><a href="/signup" class="no-underline bold">Sign up</a> <a href="/login" class="text-muted no-underline ml-3">Log in</a></span>`
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

	// The box asks, the same as the signed-out page does.
	//
	// It searched, and the reason search won was that it is the half that works
	// with no model — which is a constraint about a fresh install, and it got
	// treated as a statement about the product. Every instance that has a model
	// had its front control pointed at its own memory instead of at the thing
	// the memory is for. The README never said that: "services and the archive
	// become tools for agents to use".
	//
	// Still one control doing one thing on both pages, which was the property
	// worth keeping from the previous answer. Where there is no model it
	// renders the search box and says why — a degrade, not a second product.
	{
		b.WriteString(`<div id="home-agent">`)
		b.WriteString(app.ChatComponent(app.ChatConfig{
			Ask:              true,
			HideSuggestions:  true,
			Placeholder:      "What do you need?",
			OfferAgentPicker: viewerID != "",
		}))

		// The address, under the box. Quiet, because it is a fact about the
		// agent rather than a call to action: the thing that makes this more
		// than a chat on a page is that it answers whether or not anybody has
		// the page open, and there was nowhere on the screen you arrive at
		// saying so.
		//
		// There were suggestion chips here and they have gone. The machinery
		// was a data attribute, a listener with a forty-try retry loop, a flex
		// container and a class defined twice in the stylesheet — to render at
		// most one button, only when there was unread mail, saying "read my
		// unread email". One centred pill under a chat box, and an inbox fact
		// sitting above the inbox section. It is a line in that section now.
		//
		// The other three chips had already gone for a related reason: they
		// asked the agent to fetch things the cards below already show.
		// No address under the box.
		//
		// It said "Or write to it at agent@… — from your mail, your phone,
		// anywhere", directly beneath the thing you type into. Somebody on this
		// screen is already talking to the agent; the address is for reaching it
		// when you are not here, which is exactly the moment this line is not on
		// the screen.
		b.WriteString(`</div>`)

		// How things are, before you look anywhere.
		//
		// Between the box and the inbox on purpose: somebody arrives with one
		// question — is there anything I need to know — and answering it used
		// to mean three pages.
		//
		// Labelled like the three blocks under it — see briefHTML, which draws
		// its own heading for the same reason it decides its own silence.
		if viewerID != "" {
			b.WriteString(briefHTML(viewerID))
		}

		// What arrived, under a heading that looks like one.
		//
		// Both halves of this screen are labelled the same way and each label
		// carries a rule across the page, because two words in small caps over
		// a list read as a caption rather than as a section — which is how the
		// conversations came to look like loose links under the address line.
		if viewerID != "" {
			if peek := inbox.Preview(viewerID); peek != "" {
				b.WriteString(sectionRule("Inbox") + peek)
			}
		}
	}

	// Your agents, between what arrived and what the instance knows.
	//
	// Which is the order the three read in: something came in, here is who you
	// have working on it, here is what they can reach. Without this the page
	// was a mailbox above a content grid and the agents were somewhere else
	// entirely — on a roster you had to go and find, on the one screen whose
	// job is to say how things are.
	//
	// Not the runs block that was removed below. A run is an event and ages
	// out; an agent is a standing thing, and this is the roster with a sign of
	// life against each. See agent.Preview.
	if viewerID != "" {
		if who := agent.Preview(viewerID); who != "" {
			b.WriteString(sectionRule("Agents") + who)
		}
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
	// "Live context" was two claims where one was needed: everything on this
	// screen is live, so the word did no work here that the inbox above did not
	// also deserve.
	//
	// And then "Context" was a word from inside the machine. It names what these
	// cards are *for* — the material a run is assembled from — which is a fact
	// about the agent, not about what somebody is looking at. What they are
	// looking at is the services, answering: news, markets, weather, each card
	// one service's own view of itself. That word is already the nav label, the
	// route and the tool prefix, so using it here costs nothing and says where
	// to go next. Naming the parts after the parts is the rule everywhere else
	// in this repo; the cards were the exception.
	b.WriteString(sectionRule("Services"))
	if cards := CardsHTML(r, viewerAcc); cards != "" {
		b.WriteString(cards)
		// And the way to the rest of them. The cards are a handful of services
		// answering, not the catalogue — the same relationship the three inbox
		// rows and the five agents above have to their pages, so it ends the
		// same way. Only when there are cards: a link out from under a heading
		// with nothing beneath it reads as the block having failed to render.
		b.WriteString(app.Link("Go to services", "/services"))
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
    fetch('/', {headers:{Accept:'application/json'}})
    .then(function(r){return r.json()})
    .then(function(cards){
      cards.forEach(function(c){
        var el = document.getElementById(c.id);
        if(el){
          var content = el.querySelector('.card-body');
          if(content) content.innerHTML = c.html;
          // The head too, or the age on a card stands still while its
          // contents move. c.title was sent and never used.
          var head = el.querySelector('h4');
          if(head) head.innerHTML = c.title;
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

	// No ConnectBanner here. This page prepended one itself, from when it was
	// the only page that carried the invitation — and the shell prepends it to
	// every page now, so /home drew it twice, one directly under the other,
	// with the same words and the same button. On the screen somebody lands on
	// after signing up.
	//
	// That is what a banner moving from one page into the chrome looks like
	// when the call site it left behind is not removed. See app.renderForRequest,
	// which is the only place any of the three banners is added.
	app.Respond(w, r, app.Response{Title: "Home", Description: "The home screen",
		HTML: b.String(), BodyClass: bodyClass})
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

// sectionRule is a heading that delimits: the label, then a hairline across the
// rest of the width.
//
// Home is a stack of unrelated blocks — a chat, what arrived, what the tools
// know — and it carried two words in small caps to tell them apart. That reads
// as a caption on the thing below it rather than as a break between two things,
// which is why the sections did not look like sections.
func sectionRule(label string) string {
	return `<p class="home-section"><small>` + htmlEsc(label) + `</small></p>`
}

// cardTips is the one-line explanation behind the "?" on a card.
//
// Package-level because both the page and the refresh build a card's title now,
// and the two disagreeing about what a card is called is the bug below.
var cardTips = map[string]string{
	"blog":    "Microblog posts with daily AI-generated digests",
	"news":    "Headlines from RSS feeds, sorted by time",
	"markets": "Live crypto, futures, and commodity prices",
	"prayer":  "Islamic prayer times, and a daily verse, saying and name",
	"social":  "Public discussion threads",
	"video":   "Latest videos from curated channels",
	"images":  "A picture a day, generated here",
}

// cardBody is a card's contents as a reader sees them: what the service
// rendered, and the way through to the whole of it.
//
// One builder, because there were two. The page appended the More link to
// CachedHTML; the JSON the page polls itself with sent CachedHTML alone, and
// the script replaces .card-body with it wholesale. So every card lost its More
// link on the first refresh and got it back on the next full page load, which
// is exactly what "sometimes the More buttons disappear" looks like from the
// outside.
func cardBody(c Card, who service.Viewer) string {
	body := strings.TrimSpace(cardRender(c, who))
	if body == "" {
		return ""
	}
	if c.Link != "" {
		body += app.Link("More", c.Link)
	}
	return body
}

// cardRender is a card's HTML for one reader: the shared cache for a card that
// says the same thing to everybody, and a fresh render for one that does not.
//
// The personal ones are not in the cache at all — see RefreshCards — so this is
// the only place they are drawn, and it is per request, which is what makes
// them personal in the first place. A card that answers for whoever is looking
// cannot be memoised into a string shared by every viewer, and pretending
// otherwise is how prayer ended up showing its signed-out branch — no next
// prayer, the one thing it is for — to signed-in readers.
func cardRender(c Card, who service.Viewer) string {
	if c.Content.Personal() {
		return c.Content.Render(who).HTML
	}
	return c.CachedHTML
}

// cardHead is a card's title: its name, a way through to the service, what it
// is, and how old what it shows is.
//
// The name is a link where the card has somewhere to go, which is all of them.
// A card is a window onto a service and the title is the name of that service —
// it was the one part of the card that looked like a label and behaved like
// one, so the only way through was the More link at the very bottom, past
// whatever the card was showing.
func cardHead(c Card) string {
	title := htmlEsc(c.Title)
	if c.Icon != "" {
		// cards.json has carried an icon per card the whole time and Home has
		// never drawn one. It is inside the anchor rather than beside it so
		// that on a phone, where a card is a tile in a grid, the icon and the
		// name are one tap target rather than a picture next to a link.
		title = `<img class="card-icon" src="` + htmlEsc(c.Icon) + `" alt="">` + title
	}
	if c.Link != "" {
		title = `<a class="card-head-link" href="` + htmlEsc(c.Link) + `">` + title + `</a>`
	}
	if tip, ok := cardTips[c.ID]; ok {
		title += fmt.Sprintf(` <span class="card-tooltip" data-tip="%s" onclick="event.stopPropagation();document.querySelectorAll('.card-tooltip.show').forEach(function(e){e.classList.remove('show')});this.classList.toggle('show')">?</span>`, htmlEsc(tip))
	}
	// When it is from, on the card, which is the whole point of the stream:
	// a row of headlines with no age on it reads as "now" whether it is an
	// hour old or a week.
	if c.Streamed() {
		title += ` <span class="card-when">` + htmlEsc(app.TimeAgo(c.At)) + `</span>`
	}
	return title
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

	// Each card renders in the column cards.json puts it in, in the order the
	// file lists it. That is the third answer this has had.
	//
	// It flattened every card into one list and dealt them out alternately —
	// left, right, left, right — which threw the configuration away. Then the
	// column was computed from whether a card carries a time, which threw it
	// away again and worse: blog, news, social, video and images all carry
	// times, so five went left and two went right, and the page was lopsided in
	// a way nothing in the file could correct.
	//
	// Cards is already in this order, sorted at load. Nothing is re-sorted here
	// on the way out, because the only thing that changes between loads is a
	// card's contents, and contents do not decide where a card goes. That was
	// the other cost of computing it: an empty card reshuffled everything after
	// it, so the whole page moved depending on whether the daily image had
	// landed yet.
	who := service.Anyone()
	if viewerAcc != nil {
		who = service.For(viewerAcc.ID)
	}

	var leftHTML, rightHTML []string
	for _, card := range Cards {
		body := cardBody(card, who)
		if body == "" {
			continue
		}
		rendered := fmt.Sprintf(app.CardTemplate, card.ID, card.ID, cardHead(card), body)
		if card.column() == "left" {
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

// column names where a card renders — for the page, and for the JSON it
// refreshes itself with, which must agree with it or a refresh moves cards
// about.
//
// Right is the default for a card whose config named no column, because a card
// that fell through is better stacked with the standing views than dropped into
// the reading column at the top of the page.
func (c Card) column() string {
	if c.Column == "left" {
		return "left"
	}
	return "right"
}
