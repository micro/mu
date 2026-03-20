package apps

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// Scratch is a temporary code execution sandbox.
type Scratch struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	scratchMu    sync.RWMutex
	scratches    = map[string]*Scratch{}
	maxScratches = 1000
)

func init() {
	// Clean up old scratches every 10 minutes
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			scratchMu.Lock()
			cutoff := time.Now().Add(-1 * time.Hour)
			for id, s := range scratches {
				if s.CreatedAt.Before(cutoff) {
					delete(scratches, id)
				}
			}
			scratchMu.Unlock()
		}
	}()
}

func scratchID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateScratch stores code for temporary execution and returns the ID.
func CreateScratch(code, authorID string) string {
	id := scratchID()
	scratchMu.Lock()
	// Evict oldest if at capacity
	if len(scratches) >= maxScratches {
		var oldest string
		var oldestTime time.Time
		for k, v := range scratches {
			if oldest == "" || v.CreatedAt.Before(oldestTime) {
				oldest = k
				oldestTime = v.CreatedAt
			}
		}
		if oldest != "" {
			delete(scratches, oldest)
		}
	}
	scratches[id] = &Scratch{
		ID:        id,
		Code:      code,
		AuthorID:  authorID,
		CreatedAt: time.Now(),
	}
	scratchMu.Unlock()
	return id
}

// handleExec handles POST /apps/exec (create scratch) and GET /apps/exec?id=xxx (serve it).
func handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		handleExecCreate(w, r)
		return
	}

	// GET — serve the scratch as executable HTML
	id := r.URL.Query().Get("id")
	if id == "" {
		app.Error(w, r, http.StatusBadRequest, "id parameter required")
		return
	}

	scratchMu.RLock()
	s, ok := scratches[id]
	scratchMu.RUnlock()
	if !ok {
		app.Error(w, r, http.StatusNotFound, "Scratch not found or expired")
		return
	}

	// Serve raw HTML if requested (for iframe embedding)
	if r.URL.Query().Get("raw") == "1" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'unsafe-inline' 'self' data: blob:; script-src 'unsafe-inline'; style-src 'unsafe-inline';")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Write([]byte(wrapCodeAsHTML(s.Code)))
		return
	}

	// Render in a page with iframe
	var sb strings.Builder
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h4>Code Execution</h4>`)
	sb.WriteString(fmt.Sprintf(`<iframe src="/apps/exec?id=%s&raw=1" sandbox="allow-scripts" style="width:100%%;min-height:200px;border:1px solid #eee;border-radius:6px;background:#fff;"></iframe>`, id))
	sb.WriteString(`</div>`)

	app.Respond(w, r, app.Response{
		Title:       "Exec",
		Description: "Code execution sandbox",
		HTML:        sb.String(),
	})
}

func handleExecCreate(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := app.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.Code) == "" {
		app.RespondError(w, http.StatusBadRequest, "code is required")
		return
	}

	if len(req.Code) > 64*1024 {
		app.RespondError(w, http.StatusBadRequest, "code exceeds 64KB limit")
		return
	}

	id := CreateScratch(req.Code, acc.ID)
	app.RespondJSON(w, map[string]string{
		"id":  id,
		"url": "/apps/exec?id=" + id,
		"run": "/apps/exec?id=" + id + "&raw=1",
	})
}

// wrapCodeAsHTML wraps JavaScript code in a minimal HTML page that executes it
// and sends the result back via mu.run().
func wrapCodeAsHTML(code string) string {
	// Escape the code for embedding in a script tag
	escaped, _ := json.Marshal(code)

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: 'Nunito Sans', -apple-system, sans-serif; font-size: 14px; color: #333; padding: 12px; }
.output { white-space: pre-wrap; word-break: break-word; }
.error { color: #dc3545; }
pre { background: #f5f5f5; padding: 8px; border-radius: 4px; overflow-x: auto; font-size: 13px; }
table { border-collapse: collapse; width: 100%%; }
th, td { border: 1px solid #ddd; padding: 6px 10px; text-align: left; font-size: 13px; }
th { background: #f5f5f5; font-weight: 600; }
</style>
<script>
window.mu = {
  _id: 0, _cb: {},
  _send: function(t, d) {
    var id = ++this._id;
    return new Promise(function(ok, fail) {
      mu._cb[id] = {ok: ok, fail: fail};
      window.parent.postMessage({type: 'mu:' + t, id: id, data: d}, '*');
    });
  },
  ai: function(p, o) { return this._send('ai', {prompt: p, options: o || {}}); },
  fetch: function(u) { return this._send('fetch', {url: u}); },
  user: function() { return this._send('user', {}); },
  store: {
    set: function(k, v) { return mu._send('store', {op: 'set', key: k, value: v}); },
    get: function(k) { return mu._send('store', {op: 'get', key: k}); },
    del: function(k) { return mu._send('store', {op: 'del', key: k}); },
    keys: function() { return mu._send('store', {op: 'keys'}); }
  },
  run: function(result) {
    window.parent.postMessage({type: 'mu:run', result: result}, '*');
  }
};
window.addEventListener('message', function(e) {
  var d = e.data;
  if (d && d.type && d.type.indexOf('mu:') === 0 && d.id && mu._cb[d.id]) {
    if (d.error) { mu._cb[d.id].fail(new Error(d.error)); }
    else { mu._cb[d.id].ok(d.result); }
    delete mu._cb[d.id];
  }
});
</script>
</head>
<body>
<div id="output" class="output"></div>
<script>
(function() {
  var out = document.getElementById('output');
  function display(val) {
    if (val === undefined || val === null) return;
    if (typeof val === 'object') {
      // Render arrays of objects as tables
      if (Array.isArray(val) && val.length > 0 && typeof val[0] === 'object') {
        var keys = Object.keys(val[0]);
        var html = '<table><tr>';
        keys.forEach(function(k) { html += '<th>' + esc(k) + '</th>'; });
        html += '</tr>';
        val.forEach(function(row) {
          html += '<tr>';
          keys.forEach(function(k) { html += '<td>' + esc(String(row[k])) + '</td>'; });
          html += '</tr>';
        });
        html += '</table>';
        out.innerHTML = html;
      } else {
        out.innerHTML = '<pre>' + esc(JSON.stringify(val, null, 2)) + '</pre>';
      }
    } else {
      out.innerHTML = '<div>' + esc(String(val)) + '</div>';
    }
    // Send result back to parent
    mu.run(val);
  }
  function esc(s) {
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
  }
  try {
    var code = %s;
    var fn = new Function(code);
    var result = fn();
    if (result && typeof result.then === 'function') {
      result.then(display).catch(function(e) {
        out.innerHTML = '<div class="error">' + esc(e.message) + '</div>';
        mu.run({error: e.message});
      });
    } else {
      display(result);
    }
  } catch(e) {
    out.innerHTML = '<div class="error">' + esc(e.message) + '</div>';
    mu.run({error: e.message});
  }
})();
</script>
</body>
</html>`, string(escaped))
}
