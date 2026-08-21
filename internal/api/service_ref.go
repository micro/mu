package api

// One service, as the thing you call.
//
// /api is every method on this instance in one page — the reference for
// somebody writing a client against the whole surface. This is the same
// derivation cut one service wide, which is the granularity somebody actually
// arrives at: they want the weather, not the API.
//
// Why it is here and not in internal/app, where a product page would belong:
// the page is about the tool registry, and internal/api is the tool registry.
// internal/api already imports internal/app, so the move would be a cycle
// anyway — but the reason it should not move is the first one. What made
// ServicePage sit oddly in this package was that it rendered a *destination*
// from the tool door. A reference page for a callable surface is what this
// package is for.
//
// The one thing here that is not derived is the form, and even that is: the
// inputs come from the declared parameters, and pressing the button makes the
// call through /api/v1 — the real door, the real gate, really charged. A
// playground that mocked the answer would be a fourth place for the truth
// about a call to live.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// ServiceRefHandler serves /services/<name>.
func ServiceRefHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/services/"), "/")
	if name == "" {
		http.Redirect(w, r, "/services", http.StatusSeeOther)
		return
	}
	// One segment. /services/news/anything is not a page, and answering it with
	// the news page would publish a URL that means nothing.
	if strings.Contains(name, "/") {
		app.NotFound(w, r, "No such service")
		return
	}
	spec, ok := service.SpecFor(strings.ToLower(name))
	if !ok {
		app.NotFound(w, r, "No such service")
		return
	}

	who := service.Anyone()
	if _, acc := auth.TrySession(r); acc != nil {
		who = service.For(acc.ID)
	}

	app.Respond(w, r, app.Response{
		Title:       spec.NavLabel(),
		Description: spec.Description,
		HTML:        serviceRef(spec, who, app.BaseURL(r)),
	})
}

func serviceRef(spec service.Spec, who service.Viewer, base string) string {
	var b strings.Builder
	b.WriteString(`<div class="svc-page">`)
	b.WriteString(`<p class="svc-lead">` + html.EscapeString(spec.Description) + `</p>`)

	// The demonstration, first, and it is the argument the whole instance
	// makes: a page showing today's actual forecast is the difference between
	// claiming a service and having one. Everything under it is how to ask for
	// more of the same.
	if spec.Card != nil {
		b.WriteString(`<div class="svc-card">` + spec.Card(who).HTML + `</div>`)
	}

	// Where a person goes. The tile on /services leads here, so this is the way
	// back out to the thing itself — unless this *is* the thing, which is true
	// of a service whose page was derived: weather and hazards have no page but
	// this one, and a button back to the page you are on is furniture.
	if spec.Page != "" && spec.Page != "/services/"+spec.Name {
		b.WriteString(`<p class="svc-open"><a class="btn" href="` + html.EscapeString(spec.Page) +
			`">Open ` + html.EscapeString(spec.NavLabel()) + ` &rarr;</a></p>`)
	}

	b.WriteString(askTheAgent(spec.Name))

	methods := restMethodsFor(spec.Name)
	if len(methods) == 0 {
		b.WriteString(`<p class="card-meta">Nothing on this service is callable from outside.</p>`)
		b.WriteString(`</div>`)
		return b.String()
	}

	b.WriteString(refAuthCard(spec, base))

	b.WriteString(`<h2 class="svc-h">Methods</h2>`)
	for _, m := range methods {
		b.WriteString(refMethodCard(m, base))
	}

	b.WriteString(`<p class="svc-doors">The same methods are tools over ` +
		`<a href="/mcp">MCP</a>, and every service on this instance is in ` +
		`<a href="/api">one reference</a>. Same method, same answer, same price.</p>`)

	b.WriteString(`</div>`)
	b.WriteString(tryScript)
	return b.String()
}

// restMethodsFor is restMethods narrowed to one service. The filter is here
// rather than a parameter on restMethods because /api wants every one of them
// and would otherwise pass a sentinel meaning "all".
func restMethodsFor(name string) []restMethod {
	var out []restMethod
	for _, m := range restMethods() {
		if m.Service == name {
			out = append(out, m)
		}
	}
	return out
}

// refAuthCard says what this particular service needs, rather than restating
// the whole authentication story. A service where nothing needs an account and
// nothing costs anything should say so in one line and get out of the way.
func refAuthCard(spec service.Spec, base string) string {
	needsAuth, costs := false, false
	for _, m := range restMethodsFor(spec.Name) {
		needsAuth = needsAuth || m.NeedsAuth
		costs = costs || m.Cost > 0
	}

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Calling it</h3>`)

	switch {
	case !needsAuth && !costs:
		b.WriteString(`<p class="card-desc">Everything here is public and free. No token, ` +
			`no account, no payment.</p>`)
	case needsAuth:
		b.WriteString(`<p class="card-desc">This is your own data, so a call has to say who ` +
			`you are. Make a token at <a href="/token">/token</a> and send it as a header.</p>`)
	default:
		b.WriteString(`<p class="card-desc">Metered methods answer an unauthenticated call ` +
			`with <code>402</code> and an x402 challenge — pay it and the same request ` +
			`succeeds, with no account at all. A <a href="/token">token</a> works too.</p>`)
	}

	if needsAuth || costs {
		b.WriteString(`<pre class="bg-soft p-2 text-xs scroll-x">` +
			html.EscapeString(`curl -H "Authorization: Bearer $MU_TOKEN" \
  `+base+RESTPrefix+spec.Name+`/list`) + `</pre>`)
	}
	b.WriteString(`<p class="card-meta"><a href="/api">How every call behaves &rarr;</a></p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// refMethodCard is restMethodCard with a form under it.
func refMethodCard(m restMethod, base string) string {
	var b strings.Builder
	// The card itself is the /api card, unchanged. Two renderings of one method
	// is how a reference starts disagreeing with itself.
	card := restMethodCard(m, base)
	// Splice the form in before the card closes, so it sits inside.
	if i := strings.LastIndex(card, `</div>`); i >= 0 {
		b.WriteString(card[:i])
		b.WriteString(tryForm(m))
		b.WriteString(card[i:])
	} else {
		b.WriteString(card)
	}
	return b.String()
}

// tryForm is the declared parameters as inputs, and a button that makes the
// call for real.
func tryForm(m restMethod) string {
	var b strings.Builder
	b.WriteString(`<form class="try" data-path="` + html.EscapeString(m.Path) +
		`" data-method="` + map[bool]string{true: "POST", false: "GET"}[m.Destructive] + `">`)

	for _, p := range m.Params {
		req := ""
		if p.Required {
			req = ` <span class="text-error">*</span>`
		}
		b.WriteString(`<label class="try-field"><span>` + html.EscapeString(p.Name) + req + `</span>`)
		switch p.Type {
		case "boolean":
			b.WriteString(`<select name="` + html.EscapeString(p.Name) + `" data-type="boolean">` +
				`<option value=""></option><option value="true">true</option>` +
				`<option value="false">false</option></select>`)
		case "number", "integer":
			b.WriteString(`<input type="number" name="` + html.EscapeString(p.Name) +
				`" data-type="` + p.Type + `" autocomplete="off">`)
		default:
			b.WriteString(`<input type="text" name="` + html.EscapeString(p.Name) +
				`" data-type="string" autocomplete="off">`)
		}
		b.WriteString(`</label>`)
	}

	b.WriteString(`<div class="try-actions"><button type="submit" class="btn">Send</button>`)
	if m.Cost > 0 {
		// Said before the button is pressed, because this one spends money.
		b.WriteString(`<span class="card-meta">Costs ` + strconv.Itoa(m.Cost) + ` ` +
			creditWord(m.Cost) + `</span>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<pre class="try-out bg-soft p-2 text-xs scroll-x" hidden></pre>`)
	b.WriteString(`</form>`)
	return b.String()
}

// tryScript sends the form through /api/v1 — the same door a program uses, so
// what happens here is what happens there. A signed-in session carries the
// call, which is why the CSRF header goes on the POST: api.StrictCSRF refuses a
// cookie-authenticated write without it.
const tryScript = `<script>
(function(){
  function csrf() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }
  function values(form) {
    var out = {};
    form.querySelectorAll('[data-type]').forEach(function(el){
      var v = el.value.trim();
      if (v === '') return;
      if (el.dataset.type === 'number') out[el.name] = parseFloat(v);
      else if (el.dataset.type === 'integer') out[el.name] = parseInt(v, 10);
      else if (el.dataset.type === 'boolean') out[el.name] = v === 'true';
      else out[el.name] = v;
    });
    return out;
  }
  document.querySelectorAll('form.try').forEach(function(form){
    form.addEventListener('submit', function(ev){
      ev.preventDefault();
      var out = form.querySelector('.try-out');
      var args = values(form);
      var path = form.dataset.path;
      var opts = { credentials: 'same-origin', cache: 'no-store', headers: {} };
      if (form.dataset.method === 'POST') {
        opts.method = 'POST';
        opts.headers['Content-Type'] = 'application/json';
        var t = csrf();
        if (t) opts.headers['X-CSRF-Token'] = t;
        opts.body = JSON.stringify(args);
      } else {
        var q = new URLSearchParams();
        Object.keys(args).forEach(function(k){ q.set(k, args[k]); });
        var s = q.toString();
        if (s) path += '?' + s;
      }
      out.hidden = false;
      out.textContent = '…';
      fetch(path, opts)
        .then(function(r){ return r.text().then(function(t){ return {status: r.status, text: t}; }); })
        .then(function(res){
          var body = res.text;
          try { body = JSON.stringify(JSON.parse(body), null, 2); } catch (e) {}
          out.textContent = res.status + '\n\n' + body;
        })
        .catch(function(e){ out.textContent = String(e); });
    });
  });
})();
</script>`
