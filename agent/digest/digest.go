// Package digest writes the daily briefing: headlines, market data and video,
// synthesised by a model and published as a blog post tagged "digest".
//
// It was service/news/digest, and it was the same mistake agent/blog was made
// to fix — an agent living inside a service. It read three services by name,
// called a model and published a post, which is deciding what to say, not
// answering a question about state. Being one directory below service/news is
// what kept it: the sideways rule globbed service/<name>/*.go and its pattern
// ended at the closing quote, so "mu/service/markets" from here was never seen.
// A rule you can get out of by making a subdirectory is not a rule, and both
// halves are fixed in test/layering_test.go now.
//
// Moving it up paid for itself immediately. Three function variables in
// internal/server/hooks.go existed only so a service could reach the blog
// without importing it; an agent may import what it publishes to, so they are
// gone and this calls blog.CreatePost like any other caller.
//
// It names its sources, and that is the difference between it and agent/blog,
// deliberately. The opinion asks the registry what exists, because it is
// deciding what is worth saying and a service that shipped yesterday might be
// it. The digest does a fixed piece of work — here is what happened today —
// and a fixed job wants a fixed list, read in one place and widened on purpose
// rather than by whatever happened to register.
//
// What it does not do any more is name *packages*. It called news.GetFeed,
// markets.AllPriceData and video.LatestVideos, which is what made it a
// service importing services. It calls tools now, by name, through the same
// door every other caller uses — so the work is attributed to Micro's account,
// counted in usage, and priced if it ever stops being free. Widening the list
// is one line in digestSources.
//
// # It reads the pieces before it reads the feeds
//
// agent/blog writes one piece per topic across the day: it takes a subject,
// reads what the feeds and the web have on it, and forms a view. This ran
// beside that and re-summarised the same headlines from scratch — two model
// passes over the same material, the second starting from nothing the first
// had learned, and the second one calling itself the briefing.
//
// So the pieces are the input now, and the feeds are underneath them for
// whatever the pieces did not reach. That is the order a person works in, and
// it means the briefing is built on something this instance has already
// understood rather than on rows. See opinionContext.
//
// One agent reading another agent's output is the composition, not a
// shortcut: agent/blog exports OpinionsSince and this imports it, which is
// allowed in a way a service reaching sideways is not — see
// test/layering_test.go.
package digest

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	agentblog "mu/agent/blog"
	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/service/blog"
)

var (
	mu         sync.Mutex
	running    bool
	runStarted time.Time
	lastDigest time.Time
	lastError  string
	lastStatus string // "ok", "error", "running", "pending"
)

// Load starts the daily digest scheduler.
func Load() {
	if b, err := data.LoadFile("digest_last.txt"); err == nil {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
		if err == nil {
			lastDigest = t
		}
	}

	if lastDigest.IsZero() {
		lastStatus = "pending"
	} else {
		lastStatus = "ok"
	}

	go scheduler()
}

// Status returns the current digest state for the status page.
func Status() (ok bool, details string) {
	mu.Lock()
	defer mu.Unlock()

	switch lastStatus {
	case "running":
		return true, "Generating..."
	case "error":
		if lastDigest.IsZero() {
			return false, fmt.Sprintf("Failed: %s", lastError)
		}
		return false, fmt.Sprintf("Failed: %s (last success: %s ago)", lastError, time.Since(lastDigest).Round(time.Minute))
	case "ok":
		ago := time.Since(lastDigest).Round(time.Minute)
		return true, fmt.Sprintf("Last: %s (%s ago)", lastDigest.Format("2 Jan 15:04"), ago)
	default:
		return false, "Never run"
	}
}

// Generate triggers digest generation. Returns false if already running.
func Generate() bool {
	mu.Lock()
	if running {
		if time.Since(runStarted) > 5*time.Minute {
			app.Log("digest", "Resetting stuck running state (started %s ago)", time.Since(runStarted))
			running = false
		} else {
			mu.Unlock()
			return false
		}
	}
	mu.Unlock()
	go generate()
	return true
}

// Today returns today's digest from the blog, or nil.
func Today() *blog.Post { return blog.FindTodayDigest() }

// TestGenerate runs the digest pipeline synchronously and returns the
// result or error. Used by diagnostics to test without publishing.
func TestGenerate() (string, error) {
	pieces := opinionContext()
	context, _ := gatherContext()
	if context == "" && pieces == "" {
		return "", fmt.Errorf("no content available (news feed may be empty)")
	}
	return generateDigestContent(pieces+context, pieces != "")
}

func scheduler() {
	// Wait for blog callbacks to be wired in main.go
	time.Sleep(5 * time.Second)
	// Only create a digest on startup if one doesn't exist for today
	if Today() == nil && ready() {
		generate()
	}
	for {
		// Run once per day — sleep until next 6am UTC
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		time.Sleep(time.Until(next))
		if ready() {
			generate()
		}
	}
}

// ready reports whether there is anybody to write a briefing for.
//
// The digest runs as the instance's own agent, and that account deliberately
// does not exist until a human admin does — see auth.EnsureMicro, which will
// not bootstrap it on an empty instance because the first account created
// becomes admin and it must be a person's. So on a fresh install every source
// here failed with "account does not exist", six lines of it, five seconds
// after the first boot. Nothing was wrong; there was simply nobody here yet.
//
// Asked each time rather than once, because the answer changes the moment
// somebody signs up and this loop outlives that.
func ready() bool {
	if _, err := auth.GetAccount(auth.MicroID); err != nil {
		return false
	}
	return true
}

func generate() {
	mu.Lock()
	if running {
		mu.Unlock()
		return
	}
	running = true
	runStarted = time.Now()
	mu.Unlock()

	defer func() {
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	mu.Lock()
	lastStatus = "running"
	mu.Unlock()

	existing := Today()
	if existing == nil {
		app.Log("digest", "No existing digest for today, creating new one")
		createDigest()
	} else {
		app.Log("digest", "Digest already exists for today (%s), skipping", existing.ID)
		setSuccess()
	}
}

func createDigest() {
	app.Log("digest", "Creating new daily digest")

	pieces := opinionContext()
	context, refs := gatherContext()
	if context == "" && pieces == "" {
		setError("no content available")
		app.Log("digest", "No content available for digest")
		return
	}
	if pieces != "" {
		app.Log("digest", "Reading %d chars of the instance's own pieces", len(pieces))
	}

	response, err := generateDigestContent(pieces+context, pieces != "")
	if err != nil {
		setError(err.Error())
		app.Log("digest", "AI generation failed: %v", err)
		return
	}

	response += buildReferences(refs)

	// Published through the door, like the pieces it was written from.
	//
	// This called blog.CreatePost directly while agent/blog published the same
	// kind of post through blog_create — one publish free and uncounted, the
	// other attributed and priced, for two posts by the same account on the
	// same blog. Every other thing this agent does already goes through
	// RunPlannedAs; the one write did not, which is the half that costs money.
	title := "Daily Digest — " + time.Now().Format("2 Jan 2006")
	_, failed, err := api.ExecuteToolAs(auth.MicroID, "blog_create", map[string]any{
		"title": title, "content": response, "tags": "digest",
	})
	if err == nil && failed {
		err = fmt.Errorf("blog_create refused the post")
	}
	if err != nil {
		setError(err.Error())
		app.Log("digest", "Failed to publish digest blog post: %v", err)
		return
	}

	// The blog post is the whole of it.
	//
	// This also opened a conversation in every account's inbox each morning and
	// pushed a notification about it. The argument was that an inbox holding
	// only conversations you started is a chat history rather than an inbox —
	// which is true, and this was the wrong thing to fill it with. The brief is
	// the same text for everybody: it is not addressed to you, it is not about
	// anything you asked for, and it cannot be replied to usefully. A daily
	// arrival that is identical for every account is a feed, and it has one —
	// the blog. What belongs in an inbox is what arrived *for you*.
	setSuccess()
	app.Log("digest", "Daily digest published as blog post: %s", title)
}

// opinionWindow is how far back the briefing looks for the instance's own
// pieces: since the last briefing.
//
// Not "today". The digest goes out at 06:00 UTC and the pieces are written
// across the sixteen hours before that, so "published today" is empty every
// single morning — the failure mode where the feature is implemented, never
// runs, and nothing says so because falling back to the feeds looks exactly
// like working.
const opinionWindow = 24 * time.Hour

// maxPieces bounds what one briefing reads. Newest first, which is the order
// blog.PostsByAuthorID answers in.
const maxPieces = 10

// opinionContext is what this instance has already worked out, put in front of
// the raw feeds.
//
// This is the whole point of the change. Research is reading the links and
// forming a view; the instance does that per topic already, and then the
// briefing ignored it and re-summarised the same feeds from scratch — two AI
// passes over the same headlines, the second one starting from nothing the
// first had learned. A person does not work that way round.
//
// So the pieces go first and the feeds go after, and the prompt says which is
// which. When there are none — a fresh instance, OPINIONS=off, a day the model
// was unreachable — this returns nothing and the briefing is exactly what it
// was before, which is the fallback that has to stay working.
//
// Whole bodies, not summaries. The piece is already the compressed form of a
// day's reading; summarising a summary is where the specifics go.
func opinionContext() string {
	pieces := agentblog.OpinionsSince(time.Now().Add(-opinionWindow))
	if len(pieces) == 0 {
		return ""
	}
	// A rolling day is not a calendar day, so it can hold two days' worth: the
	// pieces published late yesterday and the ones published early today. Each
	// is capped at 2500 characters by its own prompt, and the newest are the
	// ones the briefing is about.
	if len(pieces) > maxPieces {
		pieces = pieces[:maxPieces]
	}
	var sb strings.Builder
	sb.WriteString("## What Mu published since the last briefing\n\n")
	for _, p := range pieces {
		fmt.Fprintf(&sb, "### %s\n\n%s\n\n", p.Title, strings.TrimSpace(p.Content))
	}
	return sb.String()
}

// digestSystem is the briefing's instructions, and what it is reading.
//
// Split out from the ai.Prompt so a test can read it: the whole change here is
// that the model is told the pieces come first and to build on them, and a
// prompt that is only ever assembled inside a call to a model is a prompt
// nothing can check.
// System is digestSystem, for a test in another package. See
// test/sources_test.go, which holds the one rule both writers share.
func System(fromPieces bool) string { return digestSystem(fromPieces) }

func digestSystem(fromPieces bool) string {
	// What the briefing is built on, said in the prompt rather than left for
	// the model to infer from a heading. Two different jobs: reporting the
	// day, and reporting what this instance made of the day.
	source := `You will be given news headlines, market data, and video content from today.`
	if fromPieces {
		source = `You will be given, in this order: the pieces Mu itself published since the last briefing, and then the raw news headlines, market data and video from today.

The pieces come first because they are the work. Each one is Mu reading a subject and forming a view on it, and the briefing is what those views add up to — so build on them, carry their conclusions forward, and use the raw feeds underneath for anything the pieces did not reach. Do not re-derive from the headlines what a piece has already worked out, and do not simply list the pieces: a reader who has read none of them should still get one briefing rather than eight summaries.`
	}

	return `You are a senior analyst writing a daily briefing for Mu, an independent platform built in the UK. Your audience is global and diverse, with particular relevance to Muslim readers — but the content is for everyone.

` + source + `

Write a coherent, integrated summary that connects the dots between events and market movements. The reader wants to understand what happened today and WHY markets moved — not just see raw prices.

Perspective:
- Write from a globally neutral standpoint — no US-centric framing or bias
- Never use relative phrases like "back home", "here", or "domestically" to refer to any single country
- Name countries explicitly: "in the US", "in the UK", "in Saudi Arabia"
- Give appropriate weight to events in the Muslim world, the Middle East, Africa, and Asia — not just Western markets
- Where relevant, note impacts on halal markets, Islamic finance, or Muslim-majority economies
- Treat all regions with equal editorial weight

Structure your briefing as 3-5 short paragraphs of flowing prose:
- Open with the dominant theme or story of the day
- Weave in market movements where relevant to the narrative (e.g. "Oil surged 8% as tensions in the Gulf escalated" not "Oil: $94.63")
- Cover geopolitics, finance, tech, and other notable stories
- Close with anything else worth knowing

Rules:
- Write in plain, direct prose — no bullet points, no lists, no headings
- Do NOT start with a title or heading
- Do NOT include preamble like "Here is today's briefing"
- Do NOT include a references section at the end
- Link to sources INLINE as you mention them using markdown links, e.g. "[Google released Gemma 4](https://...)" — use the URLs provided in the input
- Every major claim or event should link to its source
- Write dollar amounts as plain numbers like $94 or $1.2 trillion — NEVER use LaTeX formatting, backslashes, or math notation
- Keep it human and readable — like a morning briefing email
- CRITICAL: Keep under 2000 characters total.`
}

func generateDigestContent(context string, fromPieces bool) (string, error) {
	if context == "" {
		return "", nil
	}

	prompt := &ai.Prompt{
		System:   digestSystem(fromPieces),
		Question: context,
		Priority: ai.PriorityLow,
		Model:    ai.BackgroundModel(),
		Caller:   "daily-digest",
	}

	app.Log("digest", "Calling AI with %d chars of context", len(context))
	draft, err := ai.Ask(prompt)
	if err != nil {
		app.Log("digest", "AI call failed: %v", err)
		return "", err
	}
	app.Log("digest", "AI returned %d chars", len(draft))
	if err != nil {
		return "", err
	}

	return cleanResponse(draft), nil
}

type ref struct {
	title string
	url   string
}

// digestSources is the fixed list, and the only place to change what the daily
// briefing knows about.
//
// Every one is free, unscoped and not destructive, which is what makes it safe
// to call unattended on behalf of Micro's own account: the briefing is written
// by the instance about the world, and nothing here can read one person's data
// or spend money. Adding a line is how the digest learns about a new service —
// check those three properties on its Spec first.
//
// It was news, markets and video. Hazards and social are what "what happened
// today" was missing: an earthquake or a severe-weather alert is the most
// digest-worthy thing there is, and social is the half of the news the feeds
// have not caught up with yet.
var digestSources = []struct {
	heading string
	tool    string
	args    map[string]any
}{
	{"News Headlines", "news_list", nil},
	{"Breaking", "social_list", nil},
	{"Market Data", "markets_list", nil},
	{"Alerts", "hazards_alerts", nil},
	{"Earthquakes", "hazards_quakes", nil},
	{"Videos", "video_list", nil},
	{"Reflection", "prayer_verse", nil},
}

// Sources returns the tools the briefing reads, so a test can check the three
// properties that make the list safe to run unattended against the specs
// themselves rather than against a copy of this list.
func Sources() []string {
	out := make([]string, 0, len(digestSources))
	for _, s := range digestSources {
		out = append(out, s.tool)
	}
	return out
}

// linkPatterns pull a title and a URL back out of whatever a tool returned.
//
// This is the one thing lost by going through tools rather than reading the
// packages: news.Post had a Title and a URL as fields, and a tool answers with
// text. The references block under each digest is worth keeping, so both shapes
// a tool answers in are matched — a markdown link, and a JSON object with title
// and url next to each other. A source that answers in neither contributes no
// references and still contributes its prose, which is the right way round.
var linkPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\[([^\]]{3,200})\]\((https?://[^)\s]+)\)`),
	regexp.MustCompile(`"title"\s*:\s*"([^"]{3,200})"[^{}]*?"url"\s*:\s*"(https?://[^"]+)"`),
}

func gatherContext() (string, []ref) {
	var sb strings.Builder
	var refs []ref
	seen := map[string]bool{}

	for _, src := range digestSources {
		// Through the checked door, though nothing here was chosen by a model.
		// It costs nothing — every source passes, and the test above says so
		// against the specs — and it means that widening digestSources with
		// something destructive is refused at runtime as well as at build time.
		// A reader of this line cannot tell a fixed table from a parsed plan
		// either, which is the other half of why it goes through the door.
		text, failed, err := api.RunPlannedAs(auth.MicroID, false, src.tool, src.args)
		if err != nil || failed || strings.TrimSpace(text) == "" {
			// One quiet source must not empty the briefing.
			app.Log("digest", "%s contributed nothing: %v", src.tool, err)
			continue
		}
		fmt.Fprintf(&sb, "## %s\n\n%s\n\n", src.heading, strings.TrimSpace(text))

		for _, re := range linkPatterns {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				if seen[m[2]] {
					continue
				}
				seen[m[2]] = true
				refs = append(refs, ref{m[1], m[2]})
			}
		}
	}

	return sb.String(), refs
}

func buildReferences(refs []ref) string {
	if len(refs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n<details>\n<summary>References</summary>\n\n")
	for i, r := range refs {
		sb.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, r.title, r.url))
	}
	sb.WriteString("\n</details>")
	return sb.String()
}

func cleanResponse(s string) string {
	s = stripPreamble(s)
	s = normalizeHeadings(s)
	s = app.StripLatexDollars(s)
	return s
}

func setError(msg string) {
	mu.Lock()
	lastStatus = "error"
	lastError = msg
	mu.Unlock()
}

func setSuccess() {
	mu.Lock()
	lastDigest = time.Now()
	lastStatus = "ok"
	lastError = ""
	mu.Unlock()
	data.SaveFile("digest_last.txt", lastDigest.Format(time.RFC3339))
}

func normalizeHeadings(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for i, line := range lines {
		out = append(out, line)
		if strings.HasPrefix(strings.TrimSpace(line), "#") && i+1 < len(lines) {
			next := strings.TrimSpace(lines[i+1])
			if next != "" && !strings.HasPrefix(next, "#") {
				out = append(out, "")
			}
		}
	}
	return strings.Join(out, "\n")
}

func stripPreamble(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.SplitN(s, "\n", -1)
	for len(lines) > 0 {
		line := strings.TrimSpace(lines[0])
		lower := strings.ToLower(line)
		if line == "" ||
			strings.HasPrefix(lower, "here is") ||
			strings.HasPrefix(lower, "here's") ||
			strings.HasPrefix(lower, "below is") ||
			strings.HasPrefix(lower, "i've") ||
			strings.HasPrefix(lower, "i have") ||
			strings.HasSuffix(lower, ":") && !strings.HasPrefix(line, "**") && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "-") {
			lines = lines[1:]
			continue
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
