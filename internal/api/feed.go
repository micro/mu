package api

import (
	"html"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

// The optional service feed never blocks navigation on upstream requests.
// Only public renderers enter this cache; personal data stays out of shared HTML.
var feedCache struct {
	sync.Mutex
	html    string
	at      time.Time
	loading bool
}

func serviceFeed(w http.ResponseWriter, r *http.Request) {
	if !app.WantsJSON(r) {
		app.Respond(w, r, app.Response{Title: "Services", HTML: serviceViews(true) + `<div id="service-feed" class="feed-layout" aria-live="polite">Loading feed…</div><script>(function(){var n=0;function load(){var el=document.getElementById('service-feed');if(!el)return;fetch('/services?view=feed',{headers:{Accept:'application/json'}}).then(function(r){if(!r.ok)throw new Error();return r.json()}).then(function(d){el.innerHTML=d.html||'Nothing to show yet.';if(d.loading&&n++<30)setTimeout(load,2000)}).catch(function(){el.textContent='Could not load the feed. Reload to try again.'})}load()})()</script>`})
		return
	}
	feedCache.Lock()
	if !feedCache.loading && time.Since(feedCache.at) > 2*time.Minute {
		feedCache.loading = true
		go refreshFeed()
	}
	body, loading := feedCache.html, feedCache.loading
	feedCache.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	app.RespondJSON(w, map[string]any{"html": body, "loading": loading})
}

func refreshFeed() {
	defer func() {
		if e := recover(); e != nil {
			log.Printf("service feed: %v", e)
			feedCache.Lock()
			feedCache.loading = false
			feedCache.at = time.Now()
			feedCache.Unlock()
		}
	}()
	type entry struct {
		html string
		at   time.Time
	}
	var entries []entry
	for _, sp := range service.Cards() {
		if sp.Card.Personal() || service.AccountScoped(sp.Name) {
			continue
		}
		content := sp.Card.Render(service.Anyone())
		if strings.TrimSpace(content.HTML) == "" {
			continue
		}
		entries = append(entries, entry{app.SectionCard("", `<img class="section-icon" src="/`+html.EscapeString(sp.NavIcon())+`" alt="" aria-hidden="true">`+html.EscapeString(sp.NavLabel()), sp.Page, content.HTML), content.At})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].at.After(entries[j].at) })
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.html)
	}
	feedCache.Lock()
	feedCache.html, feedCache.at, feedCache.loading = b.String(), time.Now(), false
	feedCache.Unlock()
}

func serviceViews(feed bool) string {
	link := func(label, url string, active bool) string {
		current := ""
		if active {
			current = ` aria-current="page"`
		}
		return `<a href="` + html.EscapeString(url) + `"` + current + `>` + label + `</a>`
	}
	return `<nav class="view-switch" aria-label="Service view">` + link("Grid", "/services", !feed) + link("Feed", "/services?view=feed", feed) + `</nav>`
}
