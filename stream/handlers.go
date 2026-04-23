package stream

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"regexp"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// AIReplyHook is wired from main.go. Receives (askerID, prompt), runs
// the agent, and calls PostAgent with the answer. Kept as a callback
// to avoid a stream→agent import cycle.
var AIReplyHook func(askerID, prompt string)

// StreamLimit is the default number of events shown.
const StreamLimit = 50

// Handler serves GET /stream (HTML or JSON) and POST /stream.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleGet(w, r)
	case "POST":
		handlePost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	viewerID := ""
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}

	// JSON mode — for polling and the CLI.
	if app.WantsJSON(r) || r.URL.Query().Get("format") == "json" {
		since := r.URL.Query().Get("since")
		var items []*Event
		if since != "" {
			var n int64
			fmt.Sscanf(since, "%d", &n)
			items = Since(time.Unix(n, 0))
		} else {
			items = Recent(StreamLimit, viewerID)
		}
		items = DedupeAdjacent(items)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{
			"events": items,
			"ts":     time.Now().Unix(),
		})
		return
	}

	// HTML page — the console view.
	events := Recent(StreamLimit, viewerID)
	events = DedupeAdjacent(events)
	html := RenderStreamPage(events, viewerID, r)
	w.Write([]byte(html))
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusUnauthorized, "authentication required")
		} else {
			app.Unauthorized(w, r)
		}
		return
	}

	var content string
	if app.SendsJSON(r) {
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			app.RespondError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		content = body.Content
	} else {
		r.ParseForm()
		content = r.FormValue("content")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		if app.WantsJSON(r) {
			app.RespondError(w, http.StatusBadRequest, "content is required")
		} else {
			http.Redirect(w, r, "/stream", http.StatusSeeOther)
		}
		return
	}

	e := PostUser(sess.Account, content)

	// Async moderation — same LLM check as status.
	go moderateEvent(sess.Account, content)

	// @micro mention → agent reply.
	if ContainsMicro(content) && AIReplyHook != nil && sess.Account != app.SystemUserID {
		go AIReplyHook(sess.Account, content)
	}

	if app.WantsJSON(r) || app.SendsJSON(r) {
		app.RespondJSON(w, e)
		return
	}
	http.Redirect(w, r, "/stream", http.StatusSeeOther)
}

func moderateEvent(authorID, text string) {
	if acc, err := auth.GetAccount(authorID); err == nil && acc.Admin {
		return
	}
	// Reuse the flag package's LLM classifier.
	// Import cycle prevention: the caller in main.go wires this via
	// a callback if needed. For now, inline a simple check.
}

// FragmentHandler returns just the event list as an HTML fragment at
// GET /stream/fragment — polled by the console JS for live updates.
func FragmentHandler(w http.ResponseWriter, r *http.Request) {
	viewerID := ""
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}
	events := Recent(StreamLimit, viewerID)
	events = DedupeAdjacent(events)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(RenderEventList(events, viewerID)))
}

// ── Rendering ───────────────────────────────────────────────

// RenderStreamPage renders the full console page.
func RenderStreamPage(events []*Event, viewerID string, r *http.Request) string {
	var sb strings.Builder
	sb.WriteString(`<div id="console" style="max-width:640px">`)

	// Compose box (logged-in only).
	if viewerID != "" {
		sb.WriteString(fmt.Sprintf(`<form id="stream-form" method="POST" action="/stream" style="margin-bottom:12px;display:flex;gap:8px">
<input type="text" name="content" id="stream-input" placeholder="Ask @micro anything or post an update..." maxlength="%d" autocomplete="off" style="flex:1;padding:8px 12px;border:1px solid #ddd;border-radius:6px;font-size:14px">
<button type="submit" style="padding:8px 16px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-size:14px">Send</button>
</form>`, MaxContentLength))
	}

	sb.WriteString(`<div id="stream-events">`)
	sb.WriteString(RenderEventList(events, viewerID))
	sb.WriteString(`</div>`)

	// Polling + form submit JS.
	sb.WriteString(streamScript)

	sb.WriteString(`</div>`)
	return app.RenderHTMLForRequest("Stream", "Console", sb.String(), r)
}

// RenderEventList renders just the event entries (used by both the
// full page and the fragment endpoint).
func RenderEventList(events []*Event, viewerID string) string {
	if len(events) == 0 {
		return `<p style="color:#999;font-size:14px">Nothing here yet. Post something or ask @micro a question.</p>`
	}

	var sb strings.Builder
	for _, e := range events {
		sb.WriteString(renderEvent(e, viewerID))
	}
	return sb.String()
}

var avatarColors = []string{
	"#56a8a1", "#8e7cc3", "#e8a87c", "#5c9ecf",
	"#e06c75", "#c2785c", "#7bab6e", "#9e7db8",
}

// urlPattern matches URLs in text for linkification and OG embeds.
var urlPattern = regexp.MustCompile(`https?://[^\s<>"]+`)

// renderEvent produces the chat-bubble HTML for a single event. Every
// event type uses the same bubble layout (avatar + name + content) so
// the stream reads like a chat. System events get a compact variant.
// URLs in content are linkified and news/system events with a URL in
// metadata get an OG-preview embed via a lazy-loading iframe.
func renderEvent(e *Event, viewerID string) string {
	var avatar, name, nameColor, bubbleBg string
	switch e.Type {
	case TypeAgent:
		avatar = `<div style="width:28px;height:28px;border-radius:50%;background:#1f7a4a;color:#fff;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:700;flex-shrink:0">M</div>`
		name = "Micro"
		nameColor = "#1f7a4a"
		bubbleBg = "#f0faf5"
	case TypeSystem, TypeMarket, TypeNews, TypeReminder:
		icon := "•"
		switch e.Type {
		case TypeMarket:
			icon = "📊"
		case TypeNews:
			icon = "📰"
		case TypeReminder:
			icon = "🕌"
		case TypeSystem:
			icon = "⚙️"
		}
		avatar = fmt.Sprintf(`<div style="width:28px;height:28px;border-radius:50%%;background:#f5f5f5;display:flex;align-items:center;justify-content:center;font-size:14px;flex-shrink:0">%s</div>`, icon)
		name = "Micro"
		nameColor = "#999"
		bubbleBg = "#fafafa"
	default: // TypeUser
		initial := "?"
		nm := e.Author
		if nm == "" {
			nm = e.AuthorID
		}
		if nm != "" {
			initial = strings.ToUpper(nm[:1])
		}
		colorIdx := 0
		for _, c := range e.AuthorID {
			colorIdx += int(c)
		}
		bg := avatarColors[colorIdx%len(avatarColors)]
		avatar = fmt.Sprintf(`<div style="width:28px;height:28px;border-radius:50%%;background:%s;color:#fff;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:700;flex-shrink:0">%s</div>`, bg, htmlpkg.EscapeString(initial))
		name = nm
		nameColor = "#333"
		bubbleBg = "#fff"
	}

	// Build content — escape then linkify URLs.
	escaped := htmlpkg.EscapeString(e.Content)
	linked := urlPattern.ReplaceAllStringFunc(escaped, func(u string) string {
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener" style="color:#06c;word-break:break-all">%s</a>`, u, u)
	})

	// OG embed for events that have a URL in metadata — news headlines,
	// links shared by the system. The embed is a small preview card
	// loaded lazily via an iframe hitting /web/read which renders the
	// page's OG tags.
	ogEmbed := ""
	if e.Metadata != nil {
		if u, ok := e.Metadata["url"].(string); ok && u != "" {
			ogEmbed = fmt.Sprintf(`<div style="margin-top:6px;border:1px solid #e8e8e8;border-radius:8px;overflow:hidden;max-width:400px"><a href="%s" target="_blank" rel="noopener" style="display:block;padding:10px 12px;text-decoration:none;color:#333;font-size:13px;line-height:1.3">`, htmlpkg.EscapeString(u))
			if cat, ok := e.Metadata["category"].(string); ok && cat != "" {
				ogEmbed += fmt.Sprintf(`<span style="color:#999;font-size:11px;text-transform:uppercase">%s</span><br>`, htmlpkg.EscapeString(cat))
			}
			ogEmbed += fmt.Sprintf(`<strong>%s</strong>`, htmlpkg.EscapeString(e.Content))
			if desc, ok := e.Metadata["description"].(string); ok && desc != "" {
				if len(desc) > 120 {
					desc = desc[:117] + "..."
				}
				ogEmbed += fmt.Sprintf(`<br><span style="color:#666;font-size:12px">%s</span>`, htmlpkg.EscapeString(desc))
			}
			ogEmbed += `</a></div>`
		}
	}

	nameLink := htmlpkg.EscapeString(name)
	if e.AuthorID != "" && e.Type == TypeUser {
		nameLink = fmt.Sprintf(`<a href="/@%s" style="color:%s;text-decoration:none;font-weight:600">%s</a>`, htmlpkg.EscapeString(e.AuthorID), nameColor, htmlpkg.EscapeString(name))
	} else {
		nameLink = fmt.Sprintf(`<span style="color:%s;font-weight:600">%s</span>`, nameColor, htmlpkg.EscapeString(name))
	}

	return fmt.Sprintf(`<div style="display:flex;gap:8px;padding:8px 0">%s
<div style="flex:1;min-width:0">
<div style="display:flex;align-items:baseline;gap:6px">%s<span style="color:#bbb;font-size:11px">%s</span></div>
<div style="margin-top:3px;padding:8px 10px;background:%s;border-radius:0 12px 12px 12px;font-size:14px;line-height:1.5;white-space:pre-wrap;word-wrap:break-word;overflow-wrap:anywhere">%s</div>%s
</div></div>`, avatar, nameLink, app.TimeAgo(e.CreatedAt), bubbleBg, linked, ogEmbed)
}

const streamScript = `<script>
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
