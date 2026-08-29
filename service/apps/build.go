package apps

// Writing an app, rather than choosing one of three.
//
// # What this replaces
//
// apps_build did not write an app. It asked a flash model, in eight hundred
// tokens, to pick one of three shapes — tracker, checklist, counter — and fill
// in a title, an emoji and some field names; a renderer turned that spec into
// markup. The model never wrote a line of HTML.
//
// So "build me a unit converter" produced a checklist called Unit Converter,
// and every complaint that the button makes bad apps was really the observation
// that it can only make three things. Somebody sitting with a model and writing
// the HTML by hand got a good app, from the same model, because they were not
// choosing from a menu.
//
// That design was right once. Its own comment says why: "a tiny, checkable
// output the renderer can always turn into a working app" — which is exactly
// the trade you want when the model cannot reliably produce a working page.
// That has not been the constraint for some time.
//
// # The loop is what makes it work
//
// A model writing a whole document unaided is the other failure, and it is the
// one micro.Generate was avoiding. What closes the gap is the thing a person
// does by hand: look at what came back, say what is wrong, ask again. Every
// piece of that already existed here and none of it was wired to anything.
//
//   - ScanApp reads HTML for the patterns an app must not contain. It had a
//     test suite and no caller at all — a scanner nothing ran, which is worse
//     than none, because it reads as protection.
//   - TestHTML checks structure and then *executes the app's mu. calls
//     server-side*, so "it calls a service that does not exist" or "it reads a
//     field the response does not have" is known before anybody opens it.
//
// Three attempts, with the problems fed back each time, and the three shapes
// underneath as the floor: if the model cannot produce a document that passes,
// a checklist that works beats a broken page.
//
// # Why the model is not named here
//
// Model is left empty, so this takes the instance's default rather than the
// cheap one micro.Generate asks for by name. The whole complaint was quality
// and this is the one operation where the output is a program somebody keeps.
// It is metered — quota.OpAppBuild — so the cost is already accounted for.

import (
	"fmt"
	"regexp"
	"strings"

	"mu/internal/ai"
)

// buildAttempts is how many times the model is asked before falling back to a
// shape. Three, which is what micro.Generate already uses for its own repair
// loop: a fourth try on the same complaint is rarely a different answer.
const buildAttempts = 3

// buildTokens bounds one document. A single-page app with its styles and its
// script is a few thousand tokens; this is room for a generous one and short of
// the 256KB an app may be.
const buildTokens = 8000

// BuildApp writes an app from a description and saves it.
//
// The fallback is the whole of the old behaviour, kept because it is a good
// floor rather than a good ceiling. Build is a tool an agent calls in the
// middle of doing something else, and returning an error to it means the person
// who asked for a tracker gets an apology.
func BuildApp(description, authorID, authorName string) (*App, error) {
	written, err := writeApp(description, authorID)
	if err != nil {
		return BuildMicroApp(description, authorID, authorName)
	}

	a, err := CreateApp(authorID, written.Title, "", strings.TrimSpace(description),
		written.Tags, written.HTML, emojiSVG(written.Emoji), 0, true)
	if err != nil {
		return BuildMicroApp(description, authorID, authorName)
	}
	return a, nil
}

// written is one finished attempt: the document, and what to call it.
type written struct {
	Title string
	Emoji string
	Tags  string
	HTML  string
}

// refinement is one ask, check, ask-again-with-the-problems loop.
//
// Named and separated because there are two of these now and they differ in
// four strings. Writing an app and changing one are the same operation with a
// different opening question: produce a document, run the scanner and the
// tests over it, hand back what they said, ask again. Two copies of a loop
// that decides whether a program is fit to keep is two places for the checks
// to drift apart — and the one that drifts is always the one somebody added
// later, which here is the edit.
type refinement struct {
	system string // what the model is
	caller string // how the spend is attributed
	first  string // the question, the first time
	again  func(problems []string) string
	// describes is the app in a few words, used for a title when the model
	// returns a document without one.
	describes string
	author    string // whose app, so the tests run its calls as that account
}

// run asks until the checks pass, and reports how many turns that took.
func (rf refinement) run() (written, int, error) {
	question := rf.first

	var last []string
	for attempt := 1; attempt <= buildAttempts; attempt++ {
		raw, err := ai.Ask(&ai.Prompt{
			System:    rf.system,
			Question:  question,
			Caller:    rf.caller,
			MaxTokens: buildTokens,
		})
		if err != nil {
			return written{}, attempt, fmt.Errorf("could not reach the model: %w", err)
		}

		out := splitHeader(raw, rf.describes)
		problems := buildProblems(out.HTML, rf.author)
		if len(problems) == 0 {
			return out, attempt, nil
		}
		last = problems
		question = rf.again(problems)
	}
	return written{}, buildAttempts, fmt.Errorf("three attempts, still: %s",
		strings.Join(last, "; "))
}

// writeApp asks for a document, checks it, and asks again with the problems.
func writeApp(description, authorID string) (written, error) {
	description = strings.TrimSpace(description)
	out, _, err := refinement{
		system:    buildSystem,
		caller:    "app-build",
		first:     "Build this app: " + description,
		describes: description,
		author:    authorID,
		// The correction, said as a list rather than as prose. The model is
		// being asked to fix a document it wrote, so it gets the description
		// again — a repair prompt without the original goal drifts towards
		// satisfying the complaints and away from the app.
		again: func(problems []string) string {
			return "Build this app: " + description +
				"\n\nYour previous attempt had these problems:\n- " +
				strings.Join(problems, "\n- ") +
				"\n\nReturn the whole corrected document, in the same format."
		},
	}.run()
	return out, err
}

// buildProblems is everything wrong with a candidate document, in the words the
// model will be given.
//
// Two sources, and they answer different questions. ScanApp is what an app may
// not contain; TestHTML is whether this one works, including running its mu.
// calls for real. Neither is new — see the package comment on why they were
// both sitting unused.
func buildProblems(html, authorID string) []string {
	var out []string
	out = append(out, ScanApp(html)...)

	result := TestHTML(html, authorID)
	out = append(out, result.Issues...)
	for _, c := range result.APITests {
		switch {
		case c.Error != "":
			out = append(out, c.Call+" failed: "+c.Error)
		case c.Status >= 400:
			out = append(out, fmt.Sprintf("%s returned HTTP %d", c.Call, c.Status))
		}
	}
	if len(html) > MaxHTMLSize {
		out = append(out, "the document is over the 256KB limit — make it smaller")
	}
	return out
}

// splitHeader takes the metadata line off the front and returns the document.
//
// A comment rather than a JSON envelope with the HTML inside it. A whole
// document as a JSON string is a wall of escapes that models truncate and
// mangle, and a truncated envelope loses the app as well as the title. This
// way the worst case is a missing title, and there is a <title> to fall back
// on.
func splitHeader(raw, description string) written {
	body := stripBuildFences(strings.TrimSpace(raw))

	out := written{}
	if m := buildHeaderRe.FindStringSubmatch(body); m != nil {
		out.Title = strings.TrimSpace(jsonField(m[1], "title"))
		out.Emoji = strings.TrimSpace(jsonField(m[1], "emoji"))
		out.Tags = strings.TrimSpace(jsonField(m[1], "tags"))
		body = strings.TrimSpace(strings.Replace(body, m[0], "", 1))
	}
	out.HTML = body

	if out.Title == "" {
		if m := buildTitleRe.FindStringSubmatch(body); m != nil {
			out.Title = strings.TrimSpace(m[1])
		}
	}
	if out.Title == "" {
		out.Title = trimTitle(description)
	}
	if out.Emoji == "" {
		out.Emoji = "🔧"
	}
	return out
}

// jsonField pulls one string value out of the header object.
//
// By hand rather than through encoding/json, because a model that emits a
// trailing comma or a smart quote would otherwise cost the whole attempt over a
// title. The header is three known keys; the document is the part that matters.
func jsonField(obj, key string) string {
	m := regexp.MustCompile(`"` + key + `"\s*:\s*"([^"]*)"`).FindStringSubmatch(obj)
	if m == nil {
		return ""
	}
	return m[1]
}

// trimTitle makes a name out of a description when nothing better exists.
func trimTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "App"
	}
	if i := strings.IndexAny(s, ".\n"); i > 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 40 {
		s = strings.TrimSpace(string(r[:40]))
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// stripBuildFences removes a markdown code fence around the whole answer.
func stripBuildFences(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

var (
	buildHeaderRe = regexp.MustCompile(`(?s)^<!--\s*mu\s*(\{.*?\})\s*-->`)
	buildTitleRe  = regexp.MustCompile(`(?is)<title>(.*?)</title>`)
)

// buildSystem is what the model is told. It carries the API surface, because
// an app that invents mu.calendar() is one TestHTML will reject and a fault the
// prompt could have prevented.
const buildSystem = `You write small, self-contained web apps. Each one is a single HTML document that does one thing well.

Answer with the metadata line, then the document, and nothing else — no prose, no explanation, no markdown fences:

<!-- mu {"title":"Unit Converter","emoji":"📐","tags":"tools,convert"} -->
<!doctype html>
<html>...</html>

Rules for the document:
- Everything inline. No external scripts, stylesheets, fonts or images — they are blocked, and the app will simply fail.
- localStorage works and is per-user: it survives a reload and follows the person, not the browser. Use it the ordinary way.
- fetch works for this instance only: fetch('/api/v1/<service>/<method>') with query string or JSON body, and it returns JSON. Any other URL is refused.
- No <script src="/apps/sdk.js">. The API below is injected for you.
- Plain JavaScript. No build step, no framework, no import from a CDN.
- Works on a phone first. It runs in a frame, so size to the width it is given.
- No document.cookie, no redirecting to another site, no eval of anything built from input.
- Style it yourself, simply and legibly. A system font stack, generous spacing, one accent colour.
- Handle the empty state: an app with nothing in it yet should say what to do.

Beyond those two there is a window.mu API, for the things the web has no name for. Use it only when the app needs it — a calculator needs none of this:

  mu.db.create(coll,data) / get(coll,id) / list(coll,{scope,where,sort,limit}) / update(coll,id,data) / del(coll,id)
  mu.user()                                      the signed-in user
  mu.ai(prompt)                                  a model call, returns text
  mu.agent(prompt)                               the agent, with tools, returns an answer
  mu.web.fetch(url,{method,headers,body})        fetch a page from the open web
  mu.services()                                  what services exist here

Every one returns a Promise. localStorage and fetch, and everything above, only work while the app is open on this site — an app meant to work anywhere should hold its state in memory and do its own work.

Prefer doing the work in the page. Reach for mu.ai or mu.agent only when the app genuinely needs a model — a converter, a timer and a tracker do not.`
