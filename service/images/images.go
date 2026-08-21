// Package images is Mu's image service: on-demand text-to-image generation via
// Atlas Cloud (google/nano-banana-2-lite), plus a calming daily image generated
// once a day and shown on the home card. See dailyThemes for what it is a
// picture of — weather, water, sky, land, geometry, material.
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
	"mu/internal/blob"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/quota"
	"mu/internal/safety"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/userdb"
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
	// File is the local storage key for the downloaded image, empty if the
	// download failed and we are still relying on the provider URL.
	File string `json:"file,omitempty"`
}

var (
	dailyMu  sync.RWMutex
	daily    Daily
	dailyGen sync.Once
)

// dailyThemes rotate day to day — always calm, never ragebait.
//
// There were five and they were the same picture: a serene landscape at soft
// light, in five locations. Rotating them changed the subject and not the
// image, and with five in the list the same one came round every five days, so
// somebody who opens the home page most mornings saw it repeat within a week.
//
// The families below are meant to differ from each other rather than within
// themselves. Light and weather, water, sky, land — those are the original
// idea, done properly. Then the ones that are not photographs at all: geometry,
// pattern and material, which are calm for a different reason. A Voronoi field
// and a beach at dawn have nothing in common except that neither is asking you
// for anything, and that is the actual brief.
//
// Two rules hold across all of them. Calm, because this sits on the home screen
// and nobody needs a jolt with their coffee. And "no text", because a model
// asked for a spiral or a contour map will letter it — axis labels, a caption,
// a signature — and a picture with words in it is a picture that says something
// nobody wrote.
var dailyThemes = []struct {
	name, prompt string
}{
	// Light and weather. What the sky is doing, which is the oldest reason to
	// look out of a window.
	{"sunshine", "Warm sunshine falling across an open meadow, long grass, clear air, deep blue above. Bright, joyful, unhurried, high detail, no text."},
	{"dawn", "First light over low hills, cool blue giving way to warm gold, mist in the hollows. Quiet, expectant, cinematic, high detail, no text."},
	{"dusk", "The last of the light after sunset, deep violet sky, a thin band of amber on the horizon. Still, spacious, cinematic, high detail, no text."},
	{"rain", "Soft rain on a still surface, concentric rings spreading and overlapping, muted greens and greys. Calm, close, high detail, no text."},
	{"snow", "Fresh snow over open ground under a pale sky, blue shadows, every edge softened. Silent, weightless, high detail, no text."},
	{"fog", "Thick fog thinning at the edges, shapes suggested rather than shown, almost monochrome. Hushed, minimal, high detail, no text."},

	// Water, in its several moods, all of them slow.
	{"beach", "An empty beach at low tide, wet sand holding the sky, gentle surf, pale warm light. Open, restful, cinematic, high detail, no text."},
	{"ocean", "A tranquil ocean horizon at dawn, gentle waves, soft pastel sky. Serene, cinematic, high detail, no text."},
	{"lake", "A still mountain lake at first light, a perfect reflection, faint mist on the water. Mirror-calm, high detail, no text."},
	{"tide pools", "Shallow tide pools between dark rocks, clear water, anemones and weed, low sun. Intricate, quiet, high detail, no text."},

	// Sky and space. Far away and indifferent, which is restful.
	{"stars", "A dense field of stars over a dark horizon, the Milky Way arching across, no light pollution. Vast, still, high detail, no text."},
	{"space", "A quiet, awe-inspiring view of deep space — a nebula and distant galaxies in soft colour. Calm, contemplative, high detail, no text."},
	{"aurora", "Green and violet aurora drifting over a snowbound landscape, reflected in ice. Slow, luminous, high detail, no text."},
	{"moon", "A full moon low over calm water, its light laid across the surface in a long path. Cool, spare, high detail, no text."},
	{"clouds", "Towering cumulus in late afternoon light, seen from above, gold on white. Immense, gentle, high detail, no text."},

	// Land. Somewhere to be, with nobody in it.
	{"nature", "A serene natural landscape at golden hour — misty mountains, still water, soft light. Peaceful, cinematic, high detail, no text."},
	{"forest", "Sunlight filtering through a quiet forest, moss and ferns, soft focus. Peaceful, immersive, high detail, no text."},
	{"desert", "Wind-carved dunes at low sun, long shadows, one clean curve after another. Minimal, warm, high detail, no text."},
	{"mountains", "A range of peaks above the cloud line, cold blue distance, snow catching the light. Remote, still, high detail, no text."},
	{"mindful", "A minimal, mindful scene evoking calm — a single tree, gentle fog, muted tones, negative space. Meditative, high detail, no text."},

	// Geometry. Not photographs of anything: the pleasure is the rule being
	// followed, which is a different kind of quiet.
	{"fractals", "A fractal in soft natural colour, self-similar detail receding inward, organic rather than neon. Intricate, hypnotic, high detail, no text, no labels, no numbers."},
	{"spirals", "Nested logarithmic spirals in fine lines, the golden ratio made visible, muted ink on warm paper. Precise, meditative, high detail, no text, no labels, no numbers."},
	{"waves", "Interference patterns from two wave sources, smooth bands crossing and cancelling, soft gradients. Rhythmic, calm, high detail, no text, no labels, no numbers."},
	{"contours", "A topographic contour field in fine concentric lines, elevation implied by spacing alone, two muted colours. Quiet, exact, high detail, no text, no labels, no numbers."},
	{"tessellation", "A tessellation of interlocking shapes drifting slowly out of true, muted earth tones. Ordered, absorbing, high detail, no text, no labels, no numbers."},
	{"voronoi", "A Voronoi field of irregular cells, thin borders, gently varied fill, like dried mud or a leaf. Natural, orderly, quiet, high detail, no text, no labels, no numbers."},
	{"flow", "A flow field of thousands of fine curved lines following an invisible current, single hue on off-white. Smooth, absorbing, calm, high detail, no text, no labels, no numbers."},

	// Material. Close up, where the pattern is something real doing what it
	// does on its own.
	{"ink", "A drop of ink blooming in still water, filaments unfurling, dark against pale. Slow, organic, high detail, no text."},
	{"marble", "Veined marble in cool greys and one thread of gold, polished, lit softly from one side. Rich, still, high detail, no text."},
	{"glass", "Light refracting through thick textured glass, soft caustics thrown on a plain surface. Luminous, quiet, high detail, no text."},
	{"textile", "Handwoven cloth in undyed fibres, the weave visible, raking light across the texture. Warm, tactile, high detail, no text."},
}

// Load restores the last daily image and starts the daily generator.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("images", "service register failed: %v", err)
	}
	loadArchive()
	var d Daily
	if err := data.LoadJSON(dailyKey, &d); err == nil && d.URL != "" {
		dailyMu.Lock()
		daily = d
		dailyMu.Unlock()
		// The archive is newer than the daily record, so an instance upgrading
		// from before it existed has a current image that was never archived.
		// Pull it in, and fetch its bytes if the provider URL still resolves.
		go backfill(d)
	}
	go scheduler()
}

// today returns the current UTC date as YYYY-MM-DD.
func today() string { return time.Now().UTC().Format("2006-01-02") }

// themeStride is how far along the list one day moves.
//
// Not one. The themes are grouped into families so the list can be read and
// added to, and stepping one at a time means six weather days in a row and then
// four of water — a bigger list that is less varied day to day than the small
// one it replaced. Striding jumps families, and any stride coprime with the
// length visits every theme exactly once before repeating any.
//
// TestEveryThemeComesRound holds that, which is the part that breaks silently:
// a stride sharing a factor with the length quietly reduces the rotation to a
// handful of themes and everything still runs.
const themeStride = 7

// themeFor is which theme a day of the year gets.
func themeFor(day int) struct{ name, prompt string } {
	return dailyThemes[(day*themeStride)%len(dailyThemes)]
}

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
	theme := themeFor(time.Now().UTC().YearDay())
	url, err := ai.GenerateImage(theme.prompt)
	if err != nil {
		app.Log("images", "daily image generation failed: %v", err)
		return
	}
	d := Daily{URL: url, Prompt: theme.prompt, Theme: theme.name, Date: today()}
	// Take our own copy of the bytes. The provider URL can expire, and without
	// this the archive would fill up with links that stop resolving.
	d.File = storeImage(url, dailyPrefix(d.Date))

	dailyMu.Lock()
	daily = d
	dailyMu.Unlock()
	if err := data.SaveJSON(dailyKey, d); err != nil {
		app.Log("images", "failed to persist daily image: %v", err)
	}
	for _, old := range archiveDaily(d) {
		if old.File != "" {
			blob.Delete(old.File) //nolint:errcheck
		}
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
	// What this instance will not make, whoever asks. Before the quota check
	// and before the model, so a refused request costs nobody anything and
	// leaves no half-charge to explain.
	if reason, refused := safety.Refused(prompt); refused {
		app.Log("images", "refused a generation for %s", owner)
		return "", fmt.Errorf("%s", reason)
	}
	// Affordability, before spending time on the model. The charge itself is
	// not here any more: a tool call is charged by the gateway every service
	// call goes through (internal/service/gateway.go), and the page below
	// charges its own, because a page still reaches past the endpoint into
	// this function. When pages call endpoints, the line below goes too.
	canProceed, _, cost, err := quota.CheckQuota(owner, quota.OpImageGenerate)
	if err != nil {
		return "", err
	}
	if !canProceed {
		return "", fmt.Errorf("this costs %d credits — top up at /account/topup", cost)
	}

	url, err := ai.GenerateImage(prompt)
	if err != nil {
		return "", err
	}

	rec, err := userdb.Create(ns, owner, collection, map[string]interface{}{
		"prompt": prompt,
		"url":    url,
	}, false)
	if err != nil {
		// The image exists and was paid for; a storage hiccup shouldn't fail
		// the call — just log and return the provider's URL.
		app.Log("images", "failed to save generation for %s: %v", owner, err)
		return url, nil
	}

	// Take our own copy of the bytes, exactly as the daily image does. Someone
	// who has paid 15 credits owns a picture, not a link to somebody's CDN: the
	// provider URL expires, and until it does it is a cross-origin embed that
	// any content blocker, hotlink rule or resource policy can refuse — which
	// renders as a broken image on a page that has just charged for it.
	if key := storeImage(url, genPrefix(rec.ID)); key != "" {
		rec.Data["file"] = key
		if _, err := userdb.Update(ns, owner, collection, rec.ID, rec.Data, rec.Public); err != nil {
			app.Log("images", "failed to record stored image for %s: %v", owner, err)
		}
	}

	// Theirs, so the timeline shows it to them and to nobody else. The prompt
	// is the only description there is and people put private things in it.
	event.Announce("images", "Generated an image: "+prompt, DisplayURL(rec.ID), owner)

	return DisplayURL(rec.ID), nil
}

// DisplayURL is where an image is rendered from: this instance, always. The
// handler behind it serves our stored copy, or fetches it on first sight for an
// image generated before they were stored.
func DisplayURL(id string) string { return "/images/file/" + id }

// AbsoluteURL is DisplayURL with this instance's public origin in front, for
// callers outside a browser on this site: an agent handed "/images/file/abc"
// has nowhere to send it.
func AbsoluteURL(id string) string {
	if base := app.PublicURL(); base != "" {
		return base + DisplayURL(id)
	}
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" && d != "localhost" {
		return "https://" + d + DisplayURL(id)
	}
	return DisplayURL(id)
}

// hasImage reports whether a record has an image to show — our own copy, or a
// provider URL the handler can still fall back to.
func hasImage(rec userdb.Record) bool {
	if file, _ := rec.Data["file"].(string); file != "" {
		return true
	}
	url, _ := rec.Data["url"].(string)
	return url != ""
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
	// No image, no card. See the comment on chat.Card: a card that reports
	// nothing to report is worse than absent.
	if d.URL == "" {
		return ""
	}
	theme := html.EscapeString(strings.Title(d.Theme))
	return `<a href="/images" class="no-underline inherit-color">
<img src="` + html.EscapeString(d.displayURL()) + `" alt="Daily ` + theme + ` image" class="w-full rounded-lg d-block" loading="lazy">
<p class="text-sm text-muted mt-2 m-0">Daily image · ` + theme + `</p></a>`
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
	out := map[string]interface{}{
		"daily":   getDaily(),
		"archive": Archive(60),
		"stock":   Search("", ""),
	}
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
	// Charged here rather than inside Generate, which the tool door also calls
	// and which the gateway now charges for. Only once we have an image.
	if err := quota.Charge(acc.ID, quota.OpImageGenerate, nil); err != nil {
		app.Log("images", "image generated but not charged: %v", err)
	}
	// id lets the page show the new image with its share button without
	// reloading — the reload is what used to throw the result away.
	app.RespondJSON(w, map[string]string{
		"url":    url,
		"id":     strings.TrimPrefix(url, "/images/file/"),
		"prompt": strings.TrimSpace(req.Prompt),
	})
}

// imageGrid renders a responsive grid of image records (link to full image,
// prompt as the hover title). Used for search results and the stock pool.
func imageGrid(recs []userdb.Record) string {
	var b strings.Builder
	b.WriteString(`<div class="thumb-grid">`)
	for _, rec := range recs {
		prompt, _ := rec.Data["prompt"].(string)
		if !hasImage(rec) {
			continue
		}
		url := DisplayURL(rec.ID)
		b.WriteString(`<a href="` + html.EscapeString(url) + `" target="_blank" title="` + html.EscapeString(prompt) + `"><img src="` + html.EscapeString(url) + `" alt="" class="w-full rounded-lg d-block" loading="lazy"></a>`)
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
	price := quota.OperationCost(quota.OpImageGenerate)
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var b strings.Builder

	// Search box — searches your images plus the public stock pool.
	b.WriteString(`<div class="card"><form method="GET" action="/images" class="d-flex gap-2 m-0">`)
	b.WriteString(`<input name="q" value="` + html.EscapeString(q) + `" placeholder="Search images by description…" class="form-input grow text-base">`)
	b.WriteString(`<button type="submit" class="text-base">Search</button>`)
	b.WriteString(`</form></div>`)

	// Search results.
	if q != "" {
		res := Search(caller, q)
		b.WriteString(`<div class="card"><h3>Results for &ldquo;` + html.EscapeString(q) + `&rdquo;</h3>`)
		if len(res) == 0 {
			b.WriteString(`<p class="text-muted text-base">No matching images.</p>`)
		} else {
			b.WriteString(imageGrid(res))
		}
		b.WriteString(`</div>`)
		b.WriteString(`<p class="m-0 mb-3"><a href="/images">← Back to Images</a></p>`)
		app.Respond(w, r, app.Response{Title: "Images", Description: "Search generated images", HTML: b.String()})
		return
	}

	// Daily image hero.
	d := getDaily()
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Image of the day</h3>`)
	if d.URL != "" {
		b.WriteString(`<img src="` + html.EscapeString(d.displayURL()) + `" alt="Daily image" class="img-full my-2">`)
		b.WriteString(`<p class="card-meta text-muted text-sm">` + html.EscapeString(strings.Title(d.Theme)) + ` · generated ` + html.EscapeString(d.Date) + `</p>`)
	} else {
		b.WriteString(`<p class="text-muted">Today's image is being generated — check back shortly.</p>`)
	}
	b.WriteString(`</div>`)

	// Past dailies — the archive, newest first, today's excluded since it is
	// already the hero above.
	if past := pastDailies(d.Date, 60); len(past) > 0 {
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Past dailies</h3>`)
		b.WriteString(`<p class="card-desc">Every daily image Mu has generated, kept on this server.</p>`)
		b.WriteString(`<div class="thumb-grid">`)
		for _, e := range past {
			title := strings.Title(e.Theme) + " · " + e.Date
			b.WriteString(`<a href="` + html.EscapeString(e.displayURL()) + `" target="_blank" title="` + html.EscapeString(title) + `">`)
			b.WriteString(`<img src="` + html.EscapeString(e.displayURL()) + `" alt="Daily image for ` + html.EscapeString(e.Date) + `" class="w-full rounded-lg d-block" loading="lazy">`)
			b.WriteString(`<span class="d-block text-xs text-muted mt-1">` + html.EscapeString(e.Date) + `</span></a>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Generate panel.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Generate an image</h3>`)
	b.WriteString(fmt.Sprintf(`<p class="card-desc">Describe an image and Mu creates it with nano-banana. %d credits per image.</p>`, price))
	if acc == nil {
		b.WriteString(`<p><a href="/login">Sign in</a> to generate images.</p>`)
	} else {
		b.WriteString(`<textarea id="img-prompt" rows="3" placeholder="a cat astronaut drifting past Saturn, watercolour" class="form-area"></textarea>`)
		b.WriteString(`<button id="img-go" onclick="imgGenerate()" class="mt-2 text-base">Generate</button>`)
		b.WriteString(`<span id="img-status" class="ml-3 text-sm text-muted"></span>`)
		b.WriteString(`<div id="img-result" class="mt-3"></div>`)
	}
	b.WriteString(`</div>`)

	// Your images — each with a share-to-stock toggle.
	if acc != nil {
		recs := gallery(acc.ID)
		b.WriteString(`<div class="card">`)
		b.WriteString(`<h3>Your images</h3>`)
		b.WriteString(`<p class="card-desc">Share an image to the public stock pool so others (and their agents) can find and reuse it.</p>`)
		b.WriteString(`<div id="img-gallery" class="thumb-grid wide">`)
		if len(recs) == 0 {
			b.WriteString(`<p class="text-muted text-base span-all" id="img-empty">Nothing yet — generate your first image above.</p>`)
		}
		for _, rec := range recs {
			prompt, _ := rec.Data["prompt"].(string)
			if !hasImage(rec) {
				continue
			}
			url := DisplayURL(rec.ID)
			label, next := "Share", "true"
			if rec.Public {
				label, next = "Shared ✓", "false"
			}
			b.WriteString(`<div class="relative">`)
			b.WriteString(`<a href="` + html.EscapeString(url) + `" target="_blank" title="` + html.EscapeString(prompt) + `"><img src="` + html.EscapeString(url) + `" alt="" class="w-full rounded-lg d-block" loading="lazy"></a>`)
			b.WriteString(`<button data-id="` + html.EscapeString(rec.ID) + `" data-next="` + next + `" onclick="imgShare(this)" class="overlay-btn">` + label + `</button>`)
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
 btn.disabled=true;st.textContent='Generating… this takes up to a minute.';
 fetch('/images',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':imgCookie('csrf_token')},credentials:'same-origin',body:JSON.stringify({prompt:p})})
 .then(function(r){return r.json().then(function(j){return {ok:r.ok,j:j}})})
 .then(function(res){
  btn.disabled=false;
  if(!res.ok||res.j.error){st.textContent=res.j.error||'Failed';return;}
  st.textContent='';
  // Show it here, where the person is looking. This used to render the
  // image and then immediately reload the page, so nobody ever saw it —
  // you landed back on /images and had to scroll to find your own picture.
  var r=document.getElementById('img-result');
  r.innerHTML='<a href="'+res.j.url+'" target="_blank"><img src="'+res.j.url+'" alt="" class="img-full"></a>'+
              '<p class="text-sm text-muted mt-half m-0">'+
              '<button data-id="'+res.j.id+'" data-next="true" onclick="imgShare(this)" class="text-xs mr-2">Share</button>'+
              'Saved to your images.</p>';
  r.scrollIntoView({block:'nearest'});
  // Add it to the gallery too, so the page matches what a reload would show.
  var g=document.getElementById('img-gallery'),e=document.getElementById('img-empty');if(e)e.remove();
  if(g){
   var d=document.createElement('div');d.style.position='relative';
   d.innerHTML='<a href="'+res.j.url+'" target="_blank"><img src="'+res.j.url+'" alt="" class="w-full rounded-lg d-block"></a>';
   g.insertBefore(d,g.firstChild);
  }
  document.getElementById('img-prompt').value='';
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

// DeleteAll removes everything images holds for an owner.
//
// Called when the account is deleted (internal/server/hooks.go). Without it
// the records outlived the account that made them: there was no way to ask
// this store for everything one owner had, so the deletion hooks had nothing
// to call and somebody's generated images was simply left behind.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("images", "deleting %s's records: %v", owner, err)
	} else if n > 0 {
		app.Log("images", "deleted %d records for %s", n, owner)
	}
}
