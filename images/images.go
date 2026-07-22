// Package images is Mu's image service: on-demand text-to-image generation via
// Atlas Cloud (google/nano-banana-2-lite), plus a calming daily image (nature,
// space, or something mindful) generated once a day and shown on the home card.
package images

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/service"
	"mu/internal/userdb"
	"mu/wallet"
)

const (
	ns         = "images"    // userdb namespace for per-user generations
	collection = "generated" // per-user collection
	dailyKey   = "images/daily.json"
)

// Daily is the once-a-day ambient image shown on the home card and /images.
type Daily struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
	Theme  string `json:"theme"`
	Date   string `json:"date"` // YYYY-MM-DD (UTC)
}

var (
	dailyMu  sync.RWMutex
	daily    Daily
	dailyGen sync.Once
)

// dailyThemes rotate day to day — always calm, never ragebait.
var dailyThemes = []struct {
	name, prompt string
}{
	{"nature", "A serene natural landscape at golden hour — misty mountains, still water, soft light. Peaceful, cinematic, high detail, no text."},
	{"space", "A quiet, awe-inspiring view of deep space — a nebula and distant galaxies in soft colour. Calm, contemplative, high detail, no text."},
	{"mindful", "A minimal, mindful scene evoking calm — a single tree, gentle fog, muted tones, negative space. Meditative, high detail, no text."},
	{"ocean", "A tranquil ocean horizon at dawn, gentle waves, soft pastel sky. Serene, cinematic, high detail, no text."},
	{"forest", "Sunlight filtering through a quiet forest, moss and ferns, soft focus. Peaceful, immersive, high detail, no text."},
}

// Load restores the last daily image and starts the daily generator.
func Load() {
	if err := service.Register("images", new(Server)); err != nil {
		app.Log("images", "service register failed: %v", err)
	}
	var d Daily
	if err := data.LoadJSON(dailyKey, &d); err == nil && d.URL != "" {
		dailyMu.Lock()
		daily = d
		dailyMu.Unlock()
	}
	go scheduler()
}

// today returns the current UTC date as YYYY-MM-DD.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// scheduler generates today's image if missing, then wakes each day at 06:00 UTC.
func scheduler() {
	// Small delay so AI settings/env are wired before the first attempt.
	time.Sleep(5 * time.Second)
	for {
		dailyMu.RLock()
		have := daily.Date == today() && daily.URL != ""
		dailyMu.RUnlock()
		if !have {
			generateDaily()
		}
		// If we still don't have today's image (no provider yet, a transient
		// model error), retry within the hour so it self-heals once the Atlas
		// key is set — don't wait a whole day. Otherwise sleep until 06:00 UTC.
		dailyMu.RLock()
		ok := daily.Date == today() && daily.URL != ""
		dailyMu.RUnlock()
		if !ok {
			time.Sleep(time.Hour)
			continue
		}
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		time.Sleep(time.Until(next))
	}
}

// generateDaily creates the ambient image for today and persists it. The theme
// rotates by day so consecutive days differ.
func generateDaily() {
	if !aiReady() {
		return // no provider configured — try again next cycle
	}
	day := time.Now().UTC().YearDay()
	theme := dailyThemes[day%len(dailyThemes)]
	url, err := ai.GenerateImage(theme.prompt)
	if err != nil {
		app.Log("images", "daily image generation failed: %v", err)
		return
	}
	d := Daily{URL: url, Prompt: theme.prompt, Theme: theme.name, Date: today()}
	dailyMu.Lock()
	daily = d
	dailyMu.Unlock()
	if err := data.SaveJSON(dailyKey, d); err != nil {
		app.Log("images", "failed to persist daily image: %v", err)
	}
	app.Log("images", "generated daily %s image", theme.name)
}

// aiReady reports whether an AI provider (and thus image generation) is usable.
func aiReady() bool { return ai.Configured() }

// getDaily returns a copy of the current daily image.
func getDaily() Daily {
	dailyMu.RLock()
	defer dailyMu.RUnlock()
	return daily
}

// Generate creates an image for a user, charging the image-generation credit
// cost to their wallet, and stores it in their gallery. Returns the image URL.
// Charging lives here so every path (web form, MCP/REST tool) bills once.
func Generate(owner, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if owner == "" {
		return "", fmt.Errorf("sign in to generate images")
	}
	// Affordability check before spending time on the model.
	canProceed, _, cost, err := wallet.CheckQuota(owner, wallet.OpImageGenerate)
	if err != nil {
		return "", err
	}
	if !canProceed {
		return "", fmt.Errorf("this costs %d credits — top up at /wallet", cost)
	}

	url, err := ai.GenerateImage(prompt)
	if err != nil {
		return "", err
	}

	// Only charge once we actually have an image.
	if err := wallet.ConsumeQuota(owner, wallet.OpImageGenerate); err != nil {
		return "", err
	}

	if _, err := userdb.Create(ns, owner, collection, map[string]interface{}{
		"prompt": prompt,
		"url":    url,
	}, false); err != nil {
		// The image exists and was paid for; a storage hiccup shouldn't fail
		// the call — just log and return the URL.
		app.Log("images", "failed to save generation for %s: %v", owner, err)
	}
	return url, nil
}

// gallery returns a user's recent generations, newest first.
func gallery(owner string) []userdb.Record {
	if owner == "" {
		return nil
	}
	recs, err := userdb.List(ns, owner, collection, "mine", nil, "", "desc", 24)
	if err != nil {
		return nil
	}
	return recs
}

// Search finds generated images by prompt text. With an empty caller it
// searches only the public stock pool; with a caller it searches that user's
// own images plus everyone's public ones. An empty query lists recent images.
func Search(caller, query string) []userdb.Record {
	query = strings.TrimSpace(query)
	var where map[string]interface{}
	if query != "" {
		where = map[string]interface{}{"prompt": map[string]interface{}{"contains": query}}
	}
	scope := "all"
	if caller == "" {
		scope = "public"
	}
	recs, err := userdb.List(ns, caller, collection, scope, where, "", "desc", 48)
	if err != nil {
		return nil
	}
	return recs
}

// SetPublic shares one of the caller's images into the stock pool (or pulls it
// back private). Owner-only, enforced by userdb.
func SetPublic(owner, id string, public bool) error {
	rec, err := userdb.Get(ns, owner, collection, id)
	if err != nil {
		return err
	}
	_, err = userdb.Update(ns, owner, collection, id, rec.Data, public)
	return err
}

// CardHTML renders the home card: today's ambient image with its theme.
func CardHTML() string {
	d := getDaily()
	if d.URL == "" {
		return `<p style="color:#888;font-size:14px;margin:0">Today's image is on its way.</p>`
	}
	theme := html.EscapeString(strings.Title(d.Theme))
	return `<a href="/images" style="text-decoration:none;color:inherit">
<img src="` + html.EscapeString(d.URL) + `" alt="Daily ` + theme + ` image" style="width:100%;border-radius:8px;display:block" loading="lazy">
<p style="font-size:13px;color:#888;margin:8px 0 0">Daily image · ` + theme + `</p></a>`
}

// Handler serves /images: GET renders the page (or JSON), POST generates.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if app.WantsJSON(r) {
			handleJSON(w, r)
			return
		}
		handleHTML(w, r)
	case http.MethodPost:
		handlePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleJSON(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	caller := ""
	if acc != nil {
		caller = acc.ID
	}
	// Search mode: /images?q=... searches own + public (or public-only for guests).
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		app.RespondJSON(w, map[string]interface{}{"query": q, "results": Search(caller, q)})
		return
	}
	out := map[string]interface{}{"daily": getDaily(), "stock": Search("", "")}
	if acc != nil {
		out["images"] = gallery(acc.ID)
	}
	app.RespondJSON(w, out)
}

// handlePost handles POST /images: {"prompt":"..."} generates a new image;
// {"id":"...","public":true} shares/unshares an existing one to the stock pool.
func handlePost(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.WriteHeader(http.StatusUnauthorized)
		app.RespondJSON(w, map[string]string{"error": "Sign in to generate images."})
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
		ID     string `json:"id"`
		Public bool   `json:"public"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Publish toggle.
	if strings.TrimSpace(req.ID) != "" {
		if err := SetPublic(acc.ID, req.ID, req.Public); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			app.RespondJSON(w, map[string]string{"error": err.Error()})
			return
		}
		app.RespondJSON(w, map[string]interface{}{"id": req.ID, "public": req.Public})
		return
	}

	url, err := Generate(acc.ID, req.Prompt)
	if err != nil {
		w.WriteHeader(http.StatusPaymentRequired)
		app.RespondJSON(w, map[string]string{"error": err.Error()})
		return
	}
	app.RespondJSON(w, map[string]string{"url": url, "prompt": strings.TrimSpace(req.Prompt)})
}

// imageGrid renders a responsive grid of image records (link to full image,
// prompt as the hover title). Used for search results and the stock pool.
func imageGrid(recs []userdb.Record) string {
	var b strings.Builder
	b.WriteString(`<div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:10px;margin-top:8px">`)
	for _, rec := range recs {
		url, _ := rec.Data["url"].(string)
		prompt, _ := rec.Data["prompt"].(string)
		if url == "" {
			continue
		}
		b.WriteString(`<a href="` + html.EscapeString(url) + `" target="_blank" title="` + html.EscapeString(prompt) + `"><img src="` + html.EscapeString(url) + `" alt="" style="width:100%;border-radius:8px;display:block" loading="lazy"></a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func handleHTML(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	caller := ""
	if acc != nil {
		caller = acc.ID
	}
	price := wallet.CostImageGenerate
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var b strings.Builder

	// Search box — searches your images plus the public stock pool.
	b.WriteString(`<div class="card"><form method="GET" action="/images" style="display:flex;gap:8px;margin:0">`)
	b.WriteString(`<input name="q" value="` + html.EscapeString(q) + `" placeholder="Search images by description…" style="flex:1;padding:8px;font-size:14px;border:1px solid #ddd;border-radius:6px">`)
	b.WriteString(`<button type="submit" style="padding:8px 16px;font-size:14px">Search</button>`)
	b.WriteString(`</form></div>`)

	// Search results.
	if q != "" {
		res := Search(caller, q)
		b.WriteString(`<div class="card"><h3>Results for &ldquo;` + html.EscapeString(q) + `&rdquo;</h3>`)
		if len(res) == 0 {
			b.WriteString(`<p style="color:#888;font-size:14px">No matching images.</p>`)
		} else {
			b.WriteString(imageGrid(res))
		}
		b.WriteString(`</div>`)
		b.WriteString(`<p style="margin:0 0 12px"><a href="/images">← Back to Images</a></p>`)
		app.Respond(w, r, app.Response{Title: "Images", Description: "Search generated images", HTML: b.String()})
		return
	}

	// Daily image hero.
	d := getDaily()
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Image of the day</h3>`)
	if d.URL != "" {
		b.WriteString(`<img src="` + html.EscapeString(d.URL) + `" alt="Daily image" style="width:100%;border-radius:10px;display:block;margin:8px 0">`)
		b.WriteString(`<p class="card-meta" style="color:#888;font-size:13px">` + html.EscapeString(strings.Title(d.Theme)) + ` · generated ` + html.EscapeString(d.Date) + `</p>`)
	} else {
		b.WriteString(`<p style="color:#888">Today's image is being generated — check back shortly.</p>`)
	}
	b.WriteString(`</div>`)

	// Generate panel.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Generate an image</h3>`)
	b.WriteString(fmt.Sprintf(`<p class="card-desc">Describe an image and Mu creates it with nano-banana. %d credits per image.</p>`, price))
	if acc == nil {
		b.WriteString(`<p><a href="/login">Sign in</a> to generate images.</p>`)
	} else {
		b.WriteString(`<textarea id="img-prompt" rows="3" placeholder="a cat astronaut drifting past Saturn, watercolour" style="width:100%;padding:8px;font-size:14px;border:1px solid #ddd;border-radius:6px;box-sizing:border-box;font-family:inherit;resize:vertical"></textarea>`)
		b.WriteString(`<button id="img-go" onclick="imgGenerate()" style="margin-top:8px;padding:8px 20px;font-size:14px">Generate</button>`)
		b.WriteString(`<span id="img-status" style="margin-left:10px;font-size:13px;color:#888"></span>`)
		b.WriteString(`<div id="img-result" style="margin-top:12px"></div>`)
	}
	b.WriteString(`</div>`)

	// Your images — each with a share-to-stock toggle.
	if acc != nil {
		recs := gallery(acc.ID)
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Your images</h3>`)
		b.WriteString(`<p class="card-desc">Share an image to the public stock pool so others (and their agents) can find and reuse it.</p>`)
		b.WriteString(`<div id="img-gallery" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:10px;margin-top:8px">`)
		if len(recs) == 0 {
			b.WriteString(`<p style="color:#888;font-size:14px;grid-column:1/-1" id="img-empty">Nothing yet — generate your first image above.</p>`)
		}
		for _, rec := range recs {
			url, _ := rec.Data["url"].(string)
			prompt, _ := rec.Data["prompt"].(string)
			if url == "" {
				continue
			}
			label, next := "Share", "true"
			if rec.Public {
				label, next = "Shared ✓", "false"
			}
			b.WriteString(`<div style="position:relative">`)
			b.WriteString(`<a href="` + html.EscapeString(url) + `" target="_blank" title="` + html.EscapeString(prompt) + `"><img src="` + html.EscapeString(url) + `" alt="" style="width:100%;border-radius:8px;display:block" loading="lazy"></a>`)
			b.WriteString(`<button data-id="` + html.EscapeString(rec.ID) + `" data-next="` + next + `" onclick="imgShare(this)" style="position:absolute;bottom:6px;right:6px;font-size:11px;padding:3px 8px;border:none;border-radius:5px;background:rgba(0,0,0,.6);color:#fff;cursor:pointer">` + label + `</button>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Community stock — public images anyone can reuse.
	stock := Search("", "")
	if len(stock) > 0 {
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Community stock</h3>`)
		b.WriteString(`<p class="card-desc">Public images shared by the community — free to reuse.</p>`)
		b.WriteString(imageGrid(stock))
		b.WriteString(`</div>`)
	}

	// JS: generate, and toggle sharing to the stock pool.
	b.WriteString(`<script>
function imgCookie(n){var m=document.cookie.match('(^|;)\\s*'+n+'\\s*=\\s*([^;]+)');return m?m.pop():'';}
function imgGenerate(){
 var p=document.getElementById('img-prompt').value.trim();
 if(!p){return;}
 var btn=document.getElementById('img-go'),st=document.getElementById('img-status');
 btn.disabled=true;st.textContent='Generating…';
 fetch('/images',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':imgCookie('csrf_token')},credentials:'same-origin',body:JSON.stringify({prompt:p})})
 .then(function(r){return r.json().then(function(j){return {ok:r.ok,j:j}})})
 .then(function(res){
  btn.disabled=false;
  if(!res.ok||res.j.error){st.textContent=res.j.error||'Failed';return;}
  st.textContent='';
  var g=document.getElementById('img-gallery'),e=document.getElementById('img-empty');if(e)e.remove();
  document.getElementById('img-result').innerHTML='<img src="'+res.j.url+'" style="width:100%;border-radius:10px;display:block">';
  if(g){location.reload();}
 }).catch(function(err){btn.disabled=false;st.textContent='Error: '+err;});
}
function imgShare(btn){
 var id=btn.dataset.id,next=btn.dataset.next==='true';
 btn.disabled=true;
 fetch('/images',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':imgCookie('csrf_token')},credentials:'same-origin',body:JSON.stringify({id:id,public:next})})
 .then(function(r){return r.json().then(function(j){return {ok:r.ok,j:j}})})
 .then(function(res){
  btn.disabled=false;
  if(!res.ok||res.j.error){return;}
  if(next){btn.textContent='Shared ✓';btn.dataset.next='false';}
  else{btn.textContent='Share';btn.dataset.next='true';}
 }).catch(function(){btn.disabled=false;});
}
</script>`)

	app.Respond(w, r, app.Response{
		Title:       "Images",
		Description: "Generate images, search your library, and browse community stock",
		HTML:        b.String(),
	})
}
