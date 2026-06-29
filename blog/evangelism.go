package blog

import (
	"fmt"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/settings"
)

// Evangelism is Mu's own marketing/evangelism voice on its own blog: the
// platform telling its story on the platform — actual content, actual usage.
// It mirrors the opinion loop (in-process, posts as the system account) but its
// subject is Mu itself, grounded strictly in the canon below so it states only
// what is true and never invents features.
//
// Cadence is deliberately low (this is brand content, not a feed). Disable with
// the EVANGELISM setting (off/false/0/no).

const evangelismTag = "mu"

// evangelismCadence is the minimum spacing between evangelism posts.
const evangelismCadence = 72 * time.Hour

// muFacts is the ground truth the evangelism voice may draw on. Keep this in
// lockstep with the canon (README.md, docs/VISION.md, docs/PRINCIPLES.md,
// internal/docs/THESIS.md). The model is told to claim nothing beyond it.
const muFacts = `Mu — the ground truth (do not claim anything beyond this):

- What it is: an agent for everyday. The everyday internet — news, mail, search, weather, video, markets — runs on services, and a handful of platforms own all of them. Mu is the alternative: one agent across all the everyday things, each a real service.
- The agent: you ask Mu in plain language and it calls the relevant services (weather, news, market prices, mail, web search, video, blog) and gives a single answer. It remembers your preferences over time.
- Ownership: Mu is open source (AGPL-3.0) and self-hostable as a single Go binary, so a person can run the whole stack themselves instead of renting each piece from a different platform.
- Built on Go Micro: every capability is a go-micro service; the assistant is a go-micro agent; the MCP and A2A endpoints are its gateways. Mu is also the reference application that proves the framework.
- Values: no ads, no tracking, no algorithmic feed, no infinite scroll. You pay for the tools, not with your attention. AI assists, it does not replace — and it is honest when it does not know.
- Where to use it: the web at micro.mu, plus Discord and Telegram. Developers can reach every service over REST, MCP (/mcp), A2A, and the CLI.`

// evangelismAngles are the rotating subjects. Each is a true theme from the
// canon; the model writes a fresh piece on one per cycle.
var evangelismAngles = []string{
	"What Mu is, in plain terms: one agent for the everyday internet, instead of an app and a tab for everything.",
	"Owning your everyday internet: the platforms own a service for everything; Mu is the same stack, but self-hostable and yours.",
	"Pay for the tools, not with your attention: what a service with no ads, no tracking, and no algorithm actually feels like to use.",
	"Built on Go Micro: why every capability in Mu is a real service and the assistant is an agent over them — and why that makes Mu ownable.",
	"One question, many services: how the agent answers by calling weather, news, markets, mail, search and video on your behalf.",
	"Run your own Mu: what self-hosting a single Go binary gives you that a hosted platform never can.",
}

// evangelismEnabled reports whether the evangelism loop should run. Default on;
// set EVANGELISM to a falsey value to disable.
func evangelismEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("EVANGELISM"))) {
	case "off", "false", "0", "no":
		return false
	}
	return true
}

// StartEvangelism begins the background evangelism posting loop. Called from
// main.go after the building blocks are loaded (next to StartOpinion).
func StartEvangelism() {
	go evangelismLoop()
}

func evangelismLoop() {
	// Let the other services settle, and stagger after the opinion loop.
	time.Sleep(90 * time.Second)
	for {
		publishNextEvangelism()
		time.Sleep(6 * time.Hour) // actual pacing is time-based, see cadence
	}
}

// publishNextEvangelism posts one evangelism piece if enough time has passed
// since the last one and the loop is enabled.
func publishNextEvangelism() {
	if !evangelismEnabled() {
		return
	}
	// Need a configured AI provider to write anything.
	if last := latestEvangelismTime(); !last.IsZero() && time.Since(last) < evangelismCadence {
		return // too soon
	}

	angle := nextEvangelismAngle()
	title, body, err := generateEvangelism(angle)
	if err != nil {
		app.Log("evangelism", "generation failed: %v", err)
		return
	}
	if err := CreatePost(title, body, app.SystemUserName, app.SystemUserID, evangelismTag+",about", false); err != nil {
		app.Log("evangelism", "failed to create post: %v", err)
		return
	}
	app.Log("evangelism", "published: %s", title)
}

// nextEvangelismAngle rotates through the angles over time so consecutive posts
// differ. Deterministic (no RNG): advances one angle per cadence window.
func nextEvangelismAngle() string {
	window := time.Now().UTC().Unix() / int64(evangelismCadence/time.Second)
	return evangelismAngles[int(window)%len(evangelismAngles)]
}

// latestEvangelismTime returns the creation time of the most recent evangelism
// post (tagged evangelismTag, authored by the system account), or zero.
func latestEvangelismTime() time.Time {
	var latest time.Time
	for _, post := range GetPostsByAuthor(app.SystemUserName) {
		if !strings.EqualFold(post.AuthorID, app.SystemUserID) {
			continue
		}
		if !hasTag(post.Tags, evangelismTag) {
			continue
		}
		if post.CreatedAt.After(latest) {
			latest = post.CreatedAt
		}
	}
	return latest
}

func hasTag(tags, want string) bool {
	for _, t := range strings.Split(tags, ",") {
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}
	return false
}

// generateEvangelism writes one evangelism piece on the given angle, grounded
// strictly in muFacts. Returns title and body.
func generateEvangelism(angle string) (string, string, error) {
	system := `You are Micro, the voice of Mu, writing a short piece for Mu's own blog about Mu itself.

Voice: plain, concrete, and honest. Lead with what a person actually gets. No growth-hacky tone, no hype, no superlatives, no exclamation marks. You assist, you don't oversell — be honest about what Mu is and isn't. This is the same restraint Mu applies to its product: no dark patterns in the copy either.

Ground rules:
- Claim ONLY what the ground-truth facts below support. Do NOT invent features, numbers, users, or capabilities. If you're unsure, leave it out.
- Write for a curious person, not an investor. Show the thing; don't pitch it.
- It's fine — encouraged — to name the trade-off honestly (open/self-hostable, pay for tools not attention).

Output format:
Line 1: the title only (no quotes, no "Blog:" prefix).
Line 2: empty.
Line 3+: the body — 3 to 5 short paragraphs of flowing prose. No bullet lists, no headings, no references section. Plain dollar signs if any. Under 2000 characters total.`

	prompt := &ai.Prompt{
		System:   system,
		Question: muFacts + "\n\nWrite today's piece on this angle:\n" + angle,
		Priority: ai.PriorityLow,
		Caller:   "evangelism-generate",
	}

	resp, err := ai.Ask(prompt)
	if err != nil {
		return "", "", err
	}
	resp = strings.TrimSpace(app.StripLatexDollars(resp))

	parts := strings.SplitN(resp, "\n", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected response format")
	}
	title := strings.Trim(strings.TrimSpace(parts[0]), `"'`)
	body := strings.TrimSpace(parts[1])
	if title == "" || body == "" {
		return "", "", fmt.Errorf("empty title or body")
	}
	return title, body, nil
}
