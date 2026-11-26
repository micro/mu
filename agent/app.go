package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/origin"
	"mu/internal/pay"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
)

type appSettings = pb.AgentApp

var appTurns [64]sync.Mutex

func appPath(a *Agent) string    { return "/agent/" + Slug(a) + "/app/" }
func appPublished(a *Agent) bool { return a != nil && a.App != nil && a.App.Published }
func appName(a *Agent) string {
	if a.App != nil && a.App.Name != "" {
		return a.App.Name
	}
	return a.Name
}

// Browser and native clients use the authenticated customer identity.
func agentAppHandler(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/agent/"), "/", 3)
	if !strings.HasPrefix(r.URL.Path, "/agent/") || len(parts) < 2 || parts[1] != "app" {
		return false
	}
	a, err := namedAgent(parts[0])
	if err != nil {
		http.Error(w, "App unavailable", 503)
		return true
	}
	if a == nil {
		http.NotFound(w, r)
		return true
	}
	base := appPath(a)
	route := ""
	if len(parts) == 3 {
		route = parts[2]
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	r = api.CredentialRequest(r)
	_, acc := auth.TrySession(r)
	token := auth.TokenFromRequest(r) != nil || r.Header.Get("Authorization") != "" || r.Header.Get(api.TokenHeader) != ""
	if token && !AppRequest(r) {
		http.Error(w, "Sign in to use this app", 403)
		return true
	}
	if token && !api.AuthorizeProduct(w, r, "agent", r.Method == "POST" && route != "threads" && route != "history") {
		return true
	}
	if route == "settings" {
		if acc == nil || acc.ID != a.Owner {
			http.NotFound(w, r)
			return true
		}
		appSettingsHandler(w, r, a)
		return true
	}
	if !appPublished(a) {
		http.NotFound(w, r)
		return true
	}
	if r.Method == "GET" {
		switch route {
		case "":
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, base, 308)
				return true
			}
			appPage(w, r, a, acc != nil)
			return true
		case "manifest.webmanifest":
			w.Header().Set("Content-Type", "application/manifest+json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": base, "name": appName(a), "short_name": appName(a), "description": a.App.Description, "start_url": base, "scope": base, "display": "standalone", "background_color": "#ffffff", "theme_color": "#ffffff", "icons": []map[string]string{{"src": base + "icon-192.png?v=" + a.App.Icon, "sizes": "192x192", "type": "image/png", "purpose": "any maskable"}, {"src": base + "icon-512.png?v=" + a.App.Icon, "sizes": "512x512", "type": "image/png", "purpose": "any maskable"}}})
			return true
		case "icon-192.png", "icon-512.png":
			size := 192
			if route == "icon-512.png" {
				size = 512
			}
			raw, e := data.LoadFile("agent-app/icons/" + a.App.Icon + "-" + strconv.Itoa(size) + ".png")
			if e != nil {
				http.NotFound(w, r)
				return true
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(raw)
			return true
		case "sw.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Service-Worker-Allowed", base)
			// Only this app's public offline shell and shared static assets are cached.
			// Never cache navigation responses, history, bookings or credentials.
			fmt.Fprintf(w, `const base=%q, name='agent-app:'+base+'v1';
self.addEventListener('install',e=>e.waitUntil(caches.open(name).then(c=>c.addAll([base+'offline','/mu.css','/mu.js'])).then(()=>self.skipWaiting())));
self.addEventListener('activate',e=>e.waitUntil(self.clients.claim()));
self.addEventListener('fetch',e=>{const u=new URL(e.request.url);if(e.request.method!=='GET'||u.origin!==location.origin)return;
if(e.request.mode==='navigate'&&u.pathname.startsWith(base)){e.respondWith(fetch(e.request).catch(()=>caches.open(name).then(c=>c.match(base+'offline'))));return;}
if(u.pathname==='/mu.css'||u.pathname==='/mu.js')e.respondWith(fetch(e.request).catch(()=>caches.open(name).then(c=>c.match(u.pathname))));});`, base)
			return true
		case "offline":
			appDocument(w, a, `<p>You’re offline. Connect to the internet to chat. Your conversations remain saved on the server.</p><a class="btn" href="`+base+`">Try again</a>`)
			return true
		}
	}
	if acc == nil {
		http.Error(w, "Sign in to continue", 401)
		return true
	}
	if r.Method == "POST" {
		if !token && !auth.StrictCSRF(r) {
			http.Error(w, "Reload the page and try again", 403)
			return true
		}
		if auth.CheckPostRate(acc.ID) != nil {
			http.Error(w, "Too many requests; try again shortly", 429)
			return true
		}
	}
	if appClientHandler(w, r, a, acc.ID, route) {
		return true
	}
	switch {
	case r.Method == "GET" && route == "chats":
		rows := []map[string]string{}
		for _, th := range thread.List(acc.ID, 0) {
			if th.Client == "agent-app" && th.Agent == a.ID {
				rows = append(rows, map[string]string{"id": th.ID, "title": firstLine(th.Subject, "Conversation")})
			}
		}
		app.RespondJSON(w, rows)
	case r.Method == "GET" && strings.HasPrefix(route, "chats/"):
		id := strings.TrimPrefix(route, "chats/")
		th := appThread(acc.ID, a.ID, id)
		if th == nil {
			http.NotFound(w, r)
			break
		}
		messages := []map[string]string{}
		for _, m := range thread.Messages(acc.ID, id, 0) {
			messages = append(messages, map[string]string{"role": m.Role, "text": m.Text})
		}
		app.RespondJSON(w, messages)
	case r.Method == "POST" && route == "chat":
		var q pb.AppPrompt
		if decodeApp(w, r, &q) != nil || strings.TrimSpace(q.Prompt) == "" || len(q.Prompt) > 8000 {
			http.Error(w, "Enter a message of 1–8000 bytes", 400)
			break
		}
		result, e := askApp(r, a, acc.ID, &q)
		if e != nil {
			http.Error(w, e.Error(), 400)
			break
		}
		app.RespondJSON(w, result)
	case r.Method == "POST" && route == "checkout":
		var q pb.CheckoutRequest
		if decodeApp(w, r, &q) != nil || len(q.Reference) < 16 || len(q.Reference) > 64 {
			http.Error(w, "Invalid booking reference", 400)
			break
		}
		if !a.App.Paid || a.Pay == nil || !a.Pay.Enabled {
			http.Error(w, "Paid sessions are unavailable", 400)
			break
		}
		// Customer binding and namespace are server-owned. No caller can adopt a
		// different customer's checkout by reusing its reference or receipt URL.
		hash := sha256.Sum256([]byte(acc.ID + ":" + q.Reference))
		q.Reference = "app_" + hex.EncodeToString(hash[:])
		q.Participants = 1
		policy := &pay.Policy{Enabled: true, Price: a.Pay.Price, Currency: a.Pay.Currency, Minutes: a.Pay.Minutes, ReturnUrl: origin.URL(r) + base}
		result, e := pay.CheckoutForCustomer(r.Context(), a.Owner, a.ID, origin.URL(r), policy, &q, acc.ID)
		if e != nil {
			http.Error(w, "Could not prepare payment. Retry with the same booking.", 400)
			break
		}
		app.RespondJSON(w, result)
	case (r.Method == "GET" || r.Method == "POST") && strings.HasPrefix(route, "session/"):
		id := strings.TrimPrefix(route, "session/")
		s, e := pay.ReadForCustomer(a.Owner, a.ID, id, acc.ID)
		if e != nil {
			http.NotFound(w, r)
			break
		}
		if r.Method == "POST" {
			s, e = pay.Start(a.Owner, a.ID, id)
		}
		if e != nil {
			http.Error(w, "Payment is not confirmed yet", 402)
			break
		}
		app.RespondJSON(w, s)
	default:
		http.NotFound(w, r)
	}
	return true
}

func decodeApp(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("send one object")
	}
	return nil
}
func appThread(customer, agent, id string) *thread.Thread {
	t := thread.Get(customer, id)
	if t == nil || t.Client != "agent-app" || t.Agent != agent {
		return nil
	}
	return t
}
func appQuery(a *Agent, history []QueryMessage) QueryOpts {
	m := a.AsMicro()
	return QueryOpts{Public: true, anonymousTools: true, System: namedSystem(appName(a), m.SystemPrompt), Tools: m.Tools, Model: m.Model, History: history}
}
func askApp(r *http.Request, a *Agent, customer string, q *pb.AppPrompt) (*pb.AppAnswer, error) {
	key := sha256.Sum256([]byte(customer + ":" + a.ID))
	lock := &appTurns[int(key[0])%len(appTurns)]
	lock.Lock()
	defer lock.Unlock()
	a = For(a.Owner, a.ID)
	if !appPublished(a) {
		return nil, errors.New("app is no longer available")
	}
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	if q.Thread != "" && appThread(customer, a.ID, q.Thread) == nil {
		return nil, errors.New("conversation not found")
	}
	if a.App.Paid {
		s, err := pay.ReadForCustomer(a.Owner, a.ID, q.Session, customer)
		if err != nil || s.Status != "active" {
			return nil, errors.New("start a paid session before chatting")
		}
		if q.Thread == "" {
			q.Thread = s.Thread
		}
		if q.Thread != "" && appThread(customer, a.ID, q.Thread) == nil {
			return nil, errors.New("conversation not found")
		}
	}
	if q.Thread == "" {
		t := thread.Open(customer, "agent-app", newFlowID())
		if t == nil {
			return nil, errors.New("could not open conversation")
		}
		q.Thread = t.ID
		thread.SetAgent(customer, t.ID, a.ID)
	}
	if a.App.Paid {
		if err := pay.Bind(a.Owner, a.ID, q.Session, q.Thread); err != nil {
			return nil, err
		}
	}
	opts := appQuery(a, History(customer, q.Thread, historyTurns))
	opts.RunContext = r.Context()
	// Billing and the paid-session gate use the creator; history lives under the
	// customer. No creator context, provider OAuth, memory or credentials reach tools.
	opts.PaidSession = q.Session
	if !a.App.Paid {
		opts.PaidSession = ""
	}
	if err := pay.Check(a.Owner, a.ID, opts.PaidSession, q.Thread); err != nil {
		return nil, err
	}
	answer, err := QueryWithOpts(a.Owner, q.Prompt, opts)
	if err != nil {
		return nil, errors.New("The agent could not answer. Please try again.")
	}
	Said(customer, q.Thread, q.Prompt, "", customer)
	AnsweredAs(customer, q.Thread, answer, "", appName(a))
	thread.MarkSeen(customer, q.Thread)
	return &pb.AppAnswer{Text: answer, Thread: q.Thread}, nil
}

func appSettingsHandler(w http.ResponseWriter, r *http.Request, a *Agent) {
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
		if !auth.StrictCSRF(r) {
			http.Error(w, "Reload the page and try again", 403)
			return
		}
		if err := r.ParseMultipartForm(3 << 20); err != nil {
			http.Error(w, "Upload a PNG or JPEG under 2 MB", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		cfg := &appSettings{Published: r.PostFormValue("published") == "on", Name: strings.TrimSpace(r.PostFormValue("name")), Description: strings.TrimSpace(r.PostFormValue("description")), Paid: r.PostFormValue("access") == "paid"}
		if a.App != nil {
			cfg.Icon = a.App.Icon
		}
		if cfg.Name == "" || len(cfg.Name) > 60 || len(cfg.Description) > 280 {
			http.Error(w, "App name is required (60 bytes maximum); description is limited to 280 bytes", 400)
			return
		}
		if cfg.Paid && (a.Pay == nil || !a.Pay.Enabled) {
			http.Error(w, "Enable Mu Pay in agent settings first", 400)
			return
		}
		f, _, err := r.FormFile("icon")
		if err == nil {
			defer f.Close()
			cfg.Icon, err = saveAppIcon(f)
		} else if err == http.ErrMissingFile {
			err = nil
		}
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if cfg.Published && cfg.Icon == "" {
			http.Error(w, "Upload an app icon before publishing", 400)
			return
		}
		rosterMutation.Lock()
		fresh := For(a.Owner, a.ID)
		if fresh == nil {
			err = errors.New("agent not found")
		} else {
			fresh.App = cfg
			err = fresh.save()
		}
		rosterMutation.Unlock()
		if err != nil {
			http.Error(w, "Could not save app settings", 500)
			return
		}
		http.Redirect(w, r, appPath(a)+"settings", 303)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	cfg := a.App
	if cfg == nil {
		cfg = &appSettings{Name: a.Name, Description: a.Description}
	}
	checked := ""
	if cfg.Published {
		checked = " checked"
	}
	body := `<form class="form record-editor" data-agent-app-settings method="post" enctype="multipart/form-data"><p>Publish a dedicated app for this agent. Customers sign in with their own Mu account. You fund usage; paid sessions add credits to your account. Only public tools are available; your private accounts and files are not shared.</p>`
	body += app.Field{Name: "name", Label: "App name", Value: cfg.Name, Required: true, Max: 60, Wide: true}.HTML() + app.Field{Name: "description", Label: "Description", Value: cfg.Description, Max: 280, Wide: true}.HTML()
	body += `<label>App icon (PNG or JPEG, up to 2 MB)<input type="file" name="icon" accept="image/png,image/jpeg"></label>`
	body += app.Field{Name: "access", Label: "Access", Options: []app.Option{{Value: "free", Label: "Free for customers", On: !cfg.Paid}, {Value: "paid", Label: "Paid private sessions (Mu Pay)", On: cfg.Paid}}, Wide: true}.HTML()
	body += `<label><input type="checkbox" name="published"` + checked + `> Publish app</label><div class="form-actions"><button class="btn" type="submit">Save app</button><a href="/agent/new?id=` + url.QueryEscape(a.ID) + `">Edit agent</a></div></form>`
	if cfg.Published {
		body += `<div class="record-card"><p>Share this install link:</p><a href="` + appPath(a) + `">` + html.EscapeString(origin.URL(r)+appPath(a)) + `</a></div>`
	}
	app.Respond(w, r, app.Response{Title: "App settings", HTML: body})
}

// Decode dimensions before pixel allocation; re-encode to strip metadata and
// active formats. The central 56% fits Android maskable icon safe zones.
func saveAppIcon(reader io.Reader) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return "", errors.New("icon must be under 2 MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16000000 {
		return "", errors.New("use a PNG or JPEG no larger than 4096 × 4096 pixels")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	id := hex.EncodeToString(sum[:])
	for _, size := range []int{192, 512} {
		dst := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		side := size * 56 / 100
		w, h := side, side
		if cfg.Width > cfg.Height {
			h = side * cfg.Height / cfg.Width
		} else {
			w = side * cfg.Width / cfg.Height
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		scaled := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				scaled.Set(x, y, src.At(src.Bounds().Min.X+x*cfg.Width/w, src.Bounds().Min.Y+y*cfg.Height/h))
			}
		}
		draw.Draw(dst, image.Rect((size-w)/2, (size-h)/2, (size-w)/2+w, (size-h)/2+h), scaled, image.Point{}, draw.Over)
		var b bytes.Buffer
		if err = png.Encode(&b, dst); err != nil {
			return "", err
		}
		if err = data.SaveFile("agent-app/icons/"+id+"-"+strconv.Itoa(size)+".png", b.String()); err != nil {
			return "", err
		}
	}
	return id, nil
}

func appDocument(w http.ResponseWriter, a *Agent, body string) {
	base := appPath(a)
	name := html.EscapeString(appName(a))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="referrer" content="no-referrer"><meta name="theme-color" content="#ffffff"><meta name="apple-mobile-web-app-capable" content="yes"><meta name="apple-mobile-web-app-title" content="`+name+`"><title>`+name+`</title><link rel="manifest" href="`+base+`manifest.webmanifest"><link rel="icon" href="`+base+`icon-192.png"><link rel="apple-touch-icon" href="`+base+`icon-192.png"><link rel="stylesheet" href="/mu.css"><script defer src="/mu.js"></script></head><body data-agent-app="`+base+`"><main class="page-col agent-app"><header class="form-actions"><img class="agent-app-icon" src="`+base+`icon-192.png" alt=""><h1>`+name+`</h1><button class="btn" data-app-install hidden>Install app</button></header>`+body+`</main></body></html>`)
}
func appPage(w http.ResponseWriter, r *http.Request, a *Agent, signedIn bool) {
	base := appPath(a)
	body := `<p>` + html.EscapeString(a.App.Description) + `</p><p data-app-status role="status"></p><details><summary>Install on your device</summary><p>Use Install app when offered, or your browser’s Add to Home Screen menu. On iPhone or iPad, open in Safari and choose Share → Add to Home Screen.</p></details>`
	if !signedIn {
		body += `<p>Sign in to keep your conversations private and available across devices.</p><a class="btn" href="/login?next=` + url.QueryEscape(base) + `">Sign in to continue</a>`
		appDocument(w, a, body)
		return
	}
	if a.App.Paid && a.Pay != nil {
		body += `<section class="record-card"><p>` + html.EscapeString(sessionPrice(a.Pay)) + `</p><div class="form-actions"><button class="btn" data-app-book>Book a private session</button><button class="btn" data-app-check hidden>Check payment</button><button class="btn" data-app-start hidden>Start session</button></div><p data-app-session role="status">Payment unlocks a timed private conversation. The clock starts when you press Start session.</p></section>`
	}
	body += `<div class="form-actions"><button class="btn" data-app-new>New chat</button><label>Conversations <select data-app-chats><option value="">New chat</option></select></label><a href="/account">Account</a></div><div data-app-messages class="agent-app-messages" role="log" aria-live="polite"></div><form data-app-chat class="form" method="post"><label for="app-prompt">Message</label><div class="search-bar"><textarea id="app-prompt" name="prompt" rows="3" maxlength="8000" required></textarea><button class="btn" type="submit">Send</button></div></form>`
	appDocument(w, a, body)
}

func appSettingsLink(owner, id string) string {
	a := For(owner, id)
	if a == nil {
		return `<p>Save the agent to configure its installable app.</p>`
	}
	return `<fieldset><legend>App</legend><p>Give customers a dedicated app with your name and icon.</p><a class="btn" href="` + appPath(a) + `settings">Configure app</a></fieldset>`
}
