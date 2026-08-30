// Package brief writes the line at the top of Home that says what happened.
//
// The services fetch all day and index what they fetch, and until this existed
// nothing read any of it back. The archive is searchable if an agent chooses to
// call archive_search; the cards show the newest few of each kind. Neither
// answers the question somebody arrives with, which is whether anything
// happened worth their attention.
//
// # Why this is an agent and not a query
//
// The first attempt at this counted. It put "78 stories, 1 video and 3 posts
// today, the newest “Banque Misr, Egypt's second-largest, hit by US sanctions”"
// on the front page, and that is useless in a way worth recording: 78 is a fact
// about the fetchers rather than the world and will be near enough 78 tomorrow,
// and "the newest" is the one ranking guaranteed to be arbitrary — the most
// recently published story is a random story.
//
// Counting is the only thing that can be done to a day without judgement, and
// judgement is the whole of what a person wants here. So it is a model, once an
// hour, over the headlines — the same shape as agent/digest, which has been
// doing exactly this for the blog all along.
//
// # Not the digest
//
// agent/digest writes three to five paragraphs, publishes them as a post, and
// you go and read it. This is two sentences on a page you were already on. The
// digest is a thing to read; this is a reason to stop reading.
//
// # Why Home never calls a model
//
// home/brief.go refused one for a good reason — it is the screen somebody sees
// most often and a call per page view costs a credit and two seconds every
// visit. That objection is about *when*, not whether. Written here on a timer
// and stored, the same sentence costs one cheap call an hour for the whole
// instance, and Home reads a string.
//
// # One line for everybody
//
// The rows behind it are public: news, posts, prices. The answer is therefore
// the same for every account, and writing it per account would be the identical
// call repeated once per person. What is personal on that line is the other
// three clauses, which Home already builds from your own mail and your own
// tasks. When this learns to weigh the day against what somebody actually cares
// about, that is the point it becomes per account and not before.
package brief

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// sources are what the line is written from.
//
// Three, and they are the same door agent/digest uses: free, unscoped, not
// destructive, so they are safe to call unattended on behalf of the instance's
// own account and cannot read one person's data or spend money. Check those
// three properties on a Spec before adding a line here.
//
// Fewer than the digest reads, on purpose. The digest has five paragraphs to
// spend and can afford earthquakes, video and a reflection; this has two
// sentences, and every extra source is another thing competing to be in them.
// News is what happened, social is the half of it the feeds have not caught up
// with, and markets are whether anybody with money agreed.
var sources = []struct {
	heading string
	tool    string
}{
	{"News headlines", "news_list"},
	{"Posts", "social_list"},
	{"Markets", "markets_list"},
}

// Sources returns the tools the line is written from, so a test can check the
// three properties that make the list safe to run unattended against the specs
// themselves rather than against a copy of this list.
func Sources() []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.tool)
	}
	return out
}

// gap is the least time between two calls.
//
// An hour, because that is roughly how often the fetchers find anything, and
// because the failure this guards against is not cost — it is a front page
// whose one sentence changes every time you look at it, which reads as noise
// however good each sentence is.
const gap = time.Hour

// keep is how many past lines are held.
//
// Two weeks. They are not shown anywhere yet, and they are the reason to store
// this at all rather than hold it in memory: a fortnight of "what happened"
// dated day by day is the beginning of a record of what somebody has been
// following, which is the thing none of the rest of this has.
const keep = 14

// limit is the longest line that will be kept, in characters.
//
// 256, and the prompt asks for the same number rather than a smaller one it
// then has to be cut back from. Two sentences of real news with places and
// figures in them run to about that; asking for 200 and truncating at 280 meant
// the cap was either never reached or reached mid-word.
const limit = 256

// Entry is one line, and the day it was written about.
type Entry struct {
	Text    string    `json:"text"`
	Written time.Time `json:"written"`
	Day     string    `json:"day"` // the local date, "2006-01-02"
}

var (
	mu      sync.Mutex
	entries []Entry
	running bool
	failure string
)

// Load restores the last line and starts writing new ones.
func Load() {
	mu.Lock()
	data.LoadJSON("brief.json", &entries) //nolint:errcheck
	mu.Unlock()

	go scheduler()
}

// Line is what to show, or nothing.
//
// Nothing when the stored line is about a day that has ended: at nine in the
// morning "oil fell on Gulf tensions" is a sentence about yesterday with no
// date on it, which is worse than a blank space. Home is silent until the first
// run of the new day, which is the honest state.
func Line() string {
	mu.Lock()
	defer mu.Unlock()

	if len(entries) == 0 {
		return ""
	}
	last := entries[len(entries)-1]
	if last.Day != today() {
		return ""
	}
	return last.Text
}

// Status is for the diagnostics page: whether the last run worked, and when.
func Status() (ok bool, details string) {
	mu.Lock()
	defer mu.Unlock()

	switch {
	case running:
		return true, "Writing..."
	case failure != "" && len(entries) == 0:
		return false, "Failed: " + failure
	case failure != "":
		return false, fmt.Sprintf("Failed: %s (last written %s ago)",
			failure, time.Since(entries[len(entries)-1].Written).Round(time.Minute))
	case len(entries) == 0:
		return false, "Never run"
	}
	last := entries[len(entries)-1]
	return true, fmt.Sprintf("Last: %s (%s ago)",
		last.Written.Format("2 Jan 15:04"), time.Since(last.Written).Round(time.Minute))
}

// scheduler writes a line when the last one is an hour old or about yesterday.
//
// It ticks more often than it writes so that the first line of a new day
// arrives near the start of it rather than an hour in, and so a restart does
// not wait a full gap before saying anything.
func scheduler() {
	// The services have to have fetched something before there is a day to
	// describe, and boot is when they start.
	time.Sleep(2 * time.Minute)

	for {
		if due() {
			write()
		}
		time.Sleep(10 * time.Minute)
	}
}

// due is whether it is time to write another one.
func due() bool {
	mu.Lock()
	defer mu.Unlock()

	if running || len(entries) == 0 {
		return !running
	}
	last := entries[len(entries)-1]
	return last.Day != today() || time.Since(last.Written) >= gap
}

// write reads the day and asks for a sentence about it.
func write() {
	mu.Lock()
	if running {
		mu.Unlock()
		return
	}
	running = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	day := gather()
	if day == "" {
		app.Log("brief", "nothing to write about")
		return
	}

	text, err := ask(day)
	if err != nil {
		mu.Lock()
		failure = err.Error()
		mu.Unlock()
		app.Log("brief", "could not write a line: %v", err)
		return
	}

	mu.Lock()
	failure = ""
	if text != "" {
		entries = append(entries, Entry{Text: text, Written: time.Now(), Day: today()})
		if len(entries) > keep {
			entries = entries[len(entries)-keep:]
		}
		data.SaveJSON("brief.json", entries) //nolint:errcheck
	}
	mu.Unlock()

	if text == "" {
		app.Log("brief", "nothing worth saying about today")
		return
	}
	app.Log("brief", "wrote: %s", text)
}

// gather reads the sources through the checked door.
func gather() string {
	var sb strings.Builder
	for _, src := range sources {
		text, failed, err := api.RunPlannedAs(auth.MicroID, false, src.tool, nil)
		if err != nil || failed || strings.TrimSpace(text) == "" {
			// One quiet source must not silence the line.
			app.Log("brief", "%s contributed nothing: %v", src.tool, err)
			continue
		}
		fmt.Fprintf(&sb, "## %s\n\n%s\n\n", src.heading, strings.TrimSpace(text))
	}
	return sb.String()
}

// system is the whole instruction, and it leads with the budget.
//
// Every rule here exists because the first version of this line broke it. The
// length cap is first because a model given a page of headlines writes a page
// back. "Never say how many" is second because summarising a list is the thing
// it will reach for, and "12 stories about the Middle East" is the failure this
// was built to replace. And the empty answer has to be sayable, or every quiet
// Sunday gets a sentence manufactured about nothing.
const system = `You write the one line at the top of somebody's home page, about what happened in the world today. You will be given today's headlines, posts and market data.

Hard limits, most important first:
- Between 150 and 256 characters. Use the room. One clause is not a brief.
- Cover the TWO OR THREE most consequential things, most consequential first, separated by semicolons or full stops. Not one story at length.
- Plain text only. No markdown, no links, no headings, no bullets, no preamble, no quotation marks around the whole thing.
- Say what happened. NEVER say how many stories or posts there were — the reader does not care that there were 78, and a count is what this replaced.
- Name things: countries, companies, people, numbers. "Egypt's second-largest bank hit by US sanctions" is a clause. "Several developments in banking" is not.
- Weigh by what changes something. A decision, a policy, a market move or a conflict outranks an accident of the same size — an accident is news, a decision is a reason to act. If a theme runs through several headlines, say the theme rather than one instance of it.
- Mention markets only if they moved and only if you can say why.
- Write globally. Name countries explicitly — "in the US", "in Egypt" — never "here" or "at home".
- If nothing in the list would matter to a person, reply with exactly: NOTHING

Write the line and nothing else.`

// ask calls the model and cleans up after it.
func ask(day string) (string, error) {
	out, err := ai.Ask(&ai.Prompt{
		System:   system,
		Question: day,
		Priority: ai.PriorityLow,
		Model:    ai.BackgroundModel(),
		Caller:   "brief",
		// Not two sentences' worth, which is what this was and why a third of
		// the runs came back blank.
		//
		// The cheap models are thinking models, and the budget covers thinking
		// as well as answer: at 200 the reasoning spent it and the response was
		// empty, with no error, at a rate that depended on how hard the day was
		// to summarise. The length rule lives in the prompt and in clean(),
		// which are the two places it can be enforced. This only has to be
		// large enough that the model reaches the answer.
		MaxTokens: 2048,
	})
	if err != nil {
		return "", err
	}
	// A blank response is a failure, not a judgement. The model has a way to
	// say a day was quiet and it is the word NOTHING; silence is a provider
	// that returned nothing, and treating it as "quiet" would take the line off
	// Home on a busy day and log that there was nothing to say.
	if strings.TrimSpace(out) == "" {
		return "", errEmpty
	}
	return clean(out), nil
}

// errEmpty is a provider answering with nothing at all.
var errEmpty = errors.New("the model returned nothing")

// markdown is the formatting a model reaches for however plainly it is asked
// not to: a link, a bold run, a leading bullet, a heading. The last is matched
// separately from the rest because a heading is dropped rather than unwrapped —
// "## Today" with its marker taken off is the word "Today" on the front page.
var (
	mdLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdMark = regexp.MustCompile(`\*{1,2}|_{2}|` + "`")
	mdLead = regexp.MustCompile(`^\s*(?:[-*•]|\d+\.)\s+`)
	mdHead = regexp.MustCompile(`^\s*#{1,6}\s`)
)

// first picks the line that is the answer.
//
// A model that ignored "2 sentences" ignored it by adding paragraphs, and the
// answer is somewhere in the first few. Not simply the first line, because the
// two things it puts above the answer are a heading and a preamble — "## Today"
// and "Here is the line:" both survive taking the first line, and both are
// worse than nothing on the front page. So headings are dropped outright, a
// line ending in a colon is dropped when something follows it, and what is left
// is the first line with a sentence in it.
func first(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if mdHead.MatchString(line) {
			continue
		}
		line = strings.TrimSpace(mdLead.ReplaceAllString(line, ""))
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}
		return line
	}
	return ""
}

// clean turns whatever came back into a clause, or nothing.
func clean(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(strings.TrimRight(s, ".!"), "nothing") {
		return ""
	}

	s = first(s)
	if s == "" {
		return ""
	}

	s = mdLink.ReplaceAllString(s, "$1")
	s = mdMark.ReplaceAllString(s, "")
	s = app.StripLatexDollars(s)
	s = strings.TrimSpace(s)

	// Quotes around the whole line, which is how a model says "here is the line
	// you asked for" without a preamble.
	if len(s) > 1 {
		for _, pair := range []string{`""`, `“”`, `''`} {
			q := []rune(pair)
			if r := []rune(s); r[0] == q[0] && r[len(r)-1] == q[1] {
				s = strings.TrimSpace(string(r[1 : len(r)-1]))
			}
		}
	}

	if r := []rune(s); len(r) > limit {
		s = strings.TrimRight(string(r[:limit]), " ,;:—-") + "…"
	}
	return s
}

// today is the local date, which is the unit the line is about.
func today() string { return time.Now().Format("2006-01-02") }
