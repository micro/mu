package stream

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Limit is how many entries a page shows.
const Limit = 50

// Handler serves GET /stream, as HTML or JSON.
//
// There is no POST. The console had one and it was the only thing that ever
// wrote here; a timeline of what happened is not somewhere you type.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		app.MethodNotAllowed(w, r)
		return
	}
	viewer := viewerOf(r)

	if app.WantsJSON(r) || r.URL.Query().Get("format") == "json" {
		items := Recent(Limit, viewer)
		if s := r.URL.Query().Get("since"); s != "" {
			var n int64
			fmt.Sscanf(s, "%d", &n)
			items = Since(time.Unix(n, 0), viewer)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"entries": items,
			"ts":      time.Now().Unix(),
		})
		return
	}

	app.Respond(w, r, app.Response{
		Title:       "Stream",
		Description: "What has been happening here",
		HTML:        body(Recent(Limit, viewer)),
	})
}

// FragmentHandler returns the list alone at GET /stream/fragment, so a page
// showing the timeline can refresh it without redrawing itself.
func FragmentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(RenderList(Recent(Limit, viewerOf(r)))))
}

func viewerOf(r *http.Request) string {
	if sess, _ := auth.TrySession(r); sess != nil {
		return sess.Account
	}
	return ""
}

// ── Rendering ───────────────────────────────────────────────

func body(items []*Entry) string {
	return `<div id="stream-entries">` + RenderList(items) + `</div>` + script
}

// RenderList renders the entries alone, used by the page and the fragment.
func RenderList(items []*Entry) string {
	if len(items) == 0 {
		return `<p class="text-muted text-base">Nothing yet. When a post is published, a video found or a headline breaks, it turns up here.</p>`
	}
	var b strings.Builder
	for _, e := range items {
		b.WriteString(renderEntry(e))
	}
	return b.String()
}

// source is what a service is called and where its icon lives, read off the
// registry so a new service needs nothing here. Falls back to the bare name,
// because an entry outliving the service that announced it should still say
// where it came from.
func source(name string) (label, icon, page string) {
	for _, s := range service.Specs() {
		if s.Name == name {
			return s.NavLabel(), s.NavIcon(), s.Page
		}
	}
	return name, "", ""
}

func renderEntry(e *Entry) string {
	label, icon, page := source(e.Service)

	var b strings.Builder
	b.WriteString(`<div class="stream-row">`)

	if icon != "" {
		b.WriteString(`<img class="stream-icon" src="/` + htmlpkg.EscapeString(icon) +
			`?` + app.Version + `" alt="">`)
	} else {
		b.WriteString(`<span class="stream-icon"></span>`)
	}

	b.WriteString(`<div class="stream-body">`)

	text := htmlpkg.EscapeString(e.Text)
	if e.URL != "" {
		text = `<a class="stream-link" href="` + htmlpkg.EscapeString(e.URL) + `">` + text + `</a>`
	}
	b.WriteString(`<div class="stream-text">` + text + `</div>`)

	b.WriteString(`<div class="stream-meta">`)
	if page != "" {
		b.WriteString(`<a href="` + htmlpkg.EscapeString(page) + `">` +
			htmlpkg.EscapeString(label) + `</a>`)
	} else {
		b.WriteString(htmlpkg.EscapeString(label))
	}
	b.WriteString(`<span>` + app.TimeAgo(e.At) + `</span>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div></div>`)
	return b.String()
}

const script = `<script>
(function(){
  var el = document.getElementById('stream-entries');
  if (!el) return;
  var inflight = false;
  function refresh() {
    if (inflight) return;
    inflight = true;
    fetch('/stream/fragment', { credentials: 'same-origin', cache: 'no-store' })
      .then(function(r){ return r.ok ? r.text() : null; })
      .then(function(html){ if (html != null) el.innerHTML = html; })
      .catch(function(){})
      .then(function(){ inflight = false; });
  }
  setInterval(function(){ if (!document.hidden) refresh(); }, 30000);
  document.addEventListener('visibilitychange', function(){ if (!document.hidden) refresh(); });
})();
</script>`
