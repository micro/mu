package text

// The page at /text.
//
// A service with no page is headless, which is right for something only a
// machine would call. This is not that: seeing a page turn into JSON, or a
// paragraph turn into three bullets, is how a person works out what the tool is
// for — and the four here are hard to imagine from a description alone.
//
// It posts to itself and renders the answer inline rather than through the
// agent, because what is being demonstrated is the tool, not the assistant. An
// answer that arrived via a model deciding which tool to call would prove
// something else.

import (
	"context"
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
)

// job is one of the four, as the page needs it: what it is called and the
// second field it takes beyond the text itself.
type job struct {
	Name  string
	Label string
	Hint  string // placeholder for the second field
	Field string // name of the second field, "" for summarise
	Note  string
}

var jobs = []job{
	{"summarise", "Summarise", "bullets (optional)", "style",
		"Shortens text to its substance. Leave the box empty for prose."},
	{"extract", "Extract", `{"name":"string","total":"number"}`, "schema",
		"Returns JSON only, with null for anything the text does not say."},
	{"classify", "Classify", "billing, technical, sales", "labels",
		"Picks one of your labels, or 'none' if none fit."},
	{"translate", "Translate", "French", "to",
		"Keeps line breaks, lists and markdown as they are."},
}

// lede is what the page is, for the tab title and the service listing.
const lede = "Summarise, extract, classify and translate"

// Handler serves /text.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		run(w, r)
		return
	}
	//nolint:errcheck
	app.Respond(w, r, app.Response{Title: "Text", Description: lede, HTML: page("")})
}

// run does the work and renders the page with the answer in place.
func run(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.Error(w, r, http.StatusBadRequest, "That form did not arrive intact")
		return
	}
	which := r.FormValue("job")
	body := r.FormValue("text")
	second := r.FormValue("arg")

	out, err := do(r.Context(), which, body, second)
	if err != nil {
		out = err.Error()
	}
	//nolint:errcheck
	app.Respond(w, r, app.Response{Title: "Text", Description: lede, HTML: page(result(which, body, second, out))})
}

// do dispatches to the service, so the page and an agent go through exactly the
// same code — a demonstration that took a different path would be a
// demonstration of something else.
func do(ctx context.Context, which, body, second string) (string, error) {
	var s Server
	switch which {
	case "extract":
		var rsp ExtractResponse
		err := s.Extract(ctx, &ExtractRequest{Text: body, Schema: second}, &rsp)
		return rsp.Text, err
	case "classify":
		var rsp ClassifyResponse
		err := s.Classify(ctx, &ClassifyRequest{Text: body, Labels: second}, &rsp)
		return rsp.Text, err
	case "translate":
		var rsp TranslateResponse
		err := s.Translate(ctx, &TranslateRequest{Text: body, To: second}, &rsp)
		return rsp.Text, err
	default:
		var rsp SummariseResponse
		err := s.Summarise(ctx, &SummariseRequest{Text: body, Style: second}, &rsp)
		return rsp.Text, err
	}
}

// result renders the answer, and the call that produced it.
//
// The tool call is shown because the page's job is to teach the tool: somebody
// who sees the arguments that produced this answer can make the same call from
// their own agent, which is the point of the whole catalogue.
func result(which, body, second, out string) string {
	j := jobs[0]
	for _, c := range jobs {
		if c.Name == which {
			j = c
		}
	}

	args := `{"text": "…"`
	if j.Field != "" && strings.TrimSpace(second) != "" {
		args += `, "` + j.Field + `": ` + strconv.Quote(second)
	}
	args += "}"

	var b strings.Builder
	b.WriteString(`<div class="card tresult"><h3>` + html.EscapeString(j.Label) + `</h3>`)
	b.WriteString(`<pre class="tout">` + html.EscapeString(out) + `</pre>`)
	b.WriteString(`<p class="tcall">Same call from an agent: <code>text_` +
		html.EscapeString(j.Name) + ` ` + html.EscapeString(args) + `</code></p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// page renders the form, with any answer above it.
func page(answer string) string {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><h2>Text</h2>`)
	b.WriteString(`<p class="tlede">Four things done to a piece of text. ` +
		`An agent can call these over MCP with no account — see <a href="/tools">Tools</a>.</p></div>`)

	b.WriteString(answer)

	b.WriteString(`<form method="post" class="card tform">`)
	b.WriteString(`<div class="tjobs">`)
	for i, j := range jobs {
		checked := ""
		if i == 0 {
			checked = " checked"
		}
		b.WriteString(`<label class="tjob"><input type="radio" name="job" value="` +
			j.Name + `"` + checked + ` data-hint="` + html.EscapeString(j.Hint) +
			`" data-note="` + html.EscapeString(j.Note) + `"> ` +
			html.EscapeString(j.Label) + `</label>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`<p class="tnote" id="tnote">` + html.EscapeString(jobs[0].Note) + `</p>`)
	b.WriteString(`<textarea name="text" rows="9" placeholder="Paste text here" required></textarea>`)
	b.WriteString(`<input type="text" name="arg" id="targ" placeholder="` +
		html.EscapeString(jobs[0].Hint) + `">`)
	b.WriteString(`<button type="submit">Run</button>`)
	b.WriteString(`<p class="tcap">Up to ` + strconv.Itoa(maxInput/1000) + `,000 characters a call.</p>`)
	b.WriteString(`</form></div>`)

	b.WriteString(pageStyle + pageScript)
	return b.String()
}

const pageStyle = `<style>
.tlede{color:#666;font-size:15px;margin:0}
.tjobs{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:10px}
.tjob{display:flex;align-items:center;gap:6px;border:1px solid var(--border-color,#e3e3e3);
  border-radius:var(--border-radius,8px);padding:8px 12px;cursor:pointer;font-size:15px}
.tjob:has(input:checked){border-color:#111;font-weight:600}
.tnote{color:#666;font-size:14px;margin:0 0 10px}
.tform textarea,.tform input[type=text]{width:100%;margin-bottom:10px;font-family:inherit}
.tform button{width:100%}
.tcap{color:#888;font-size:13px;margin:10px 0 0;text-align:center}
.tresult h3{margin-top:0}
.tout{white-space:pre-wrap;word-break:break-word;background:var(--hover-background,#f6f6f6);
  padding:12px;border-radius:var(--border-radius,8px);font-size:14px;margin:0}
.tcall{color:#666;font-size:13px;margin:10px 0 0;word-break:break-all}
</style>`

// The second field changes meaning with the job — a schema, some labels, a
// language — so its placeholder and the note follow the choice. Re-wired on
// mu:navigated because a soft navigation swaps the content and leaves any
// listener bound to the old nodes.
const pageScript = `<script>
(function(){
  function wire(){
    var jobs = document.querySelectorAll('.tjob input[name=job]');
    var arg = document.getElementById('targ');
    var note = document.getElementById('tnote');
    if (!jobs.length || !arg) return;
    jobs.forEach(function(j){
      j.addEventListener('change', function(){
        arg.placeholder = j.dataset.hint || '';
        arg.value = '';
        if (note) note.textContent = j.dataset.note || '';
      });
    });
  }
  wire();
  document.addEventListener('mu:navigated', wire);
})();
</script>`
