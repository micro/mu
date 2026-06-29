package blog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/settings"
)

// Evangelism is Mu's own marketing/evangelism voice on its own blog: the
// platform telling its story on the platform — actual content, actual usage.
//
// It is produced the same way as the daily brief (news/digest) and the opinion
// pieces (blog/opinion.go): the voice lives in a System prompt here in code, the
// SET of things to produce lives in an embedded JSON (evangelism.json, a
// name -> instruction map like chat/prompts.json), and each piece is generated
// with ai.Ask (PriorityLow, BackgroundModel) and posted as the system account on
// a cadence. Disable with the EVANGELISM setting (off/false/0/no).

// evangelismJSON is the set of angles: a map of title -> writing instruction,
// the same shape as chat/prompts.json. Edit this file (data) to manage what gets
// produced — no code change needed.
//
//go:embed evangelism.json
var evangelismJSON []byte

const evangelismTag = "mu"

// evangelismCadence is the minimum spacing between evangelism posts. Low on
// purpose — this is brand content, not a feed.
const evangelismCadence = 72 * time.Hour

// evangelismVoice is the editorial voice + grounding, kept in code like the
// digest's System prompt and opinion's agentPurpose. The model may claim only
// what the angle instruction and these core facts support — never invents
// features, numbers, or users.
const evangelismVoice = `You are Micro, the voice of Mu, writing a short piece for Mu's own blog about Mu itself.

Core facts you may rely on (claim nothing beyond these and the specific angle you are given):
- Mu is an agent for everyday: you ask it in plain language and it calls real services — news, mail, search, weather, markets, video, blog — and gives a single answer. It remembers your preferences over time.
- Mu is open source (AGPL-3.0) and self-hostable as a single Go binary; running the whole stack yourself is a real, optional path.
- Mu is built on Go Micro: every capability is a go-micro service, the assistant is a go-micro agent, and the MCP and A2A endpoints are its gateways.
- Values: no ads, no tracking, no algorithmic feed, no infinite scroll. You pay for the tools, not with your attention. AI assists, it does not replace, and is honest when it does not know.
- Available on the web at micro.mu, plus Discord and Telegram; developers reach every service over REST, MCP, A2A and the CLI.

Voice: plain, concrete, and honest. Lead with what a person actually gets. No hype, no superlatives, no exclamation marks, no growth-hacky tone — the same restraint Mu applies to its product (no dark patterns in the copy either). Name the trade-offs honestly. If unsure whether something is true, leave it out.

Output format:
Line 1: the title only (no quotes, no "Blog:" prefix).
Line 2: empty.
Line 3+: the body — 3 to 5 short paragraphs of flowing prose. No bullet lists, no headings, no references section. Plain dollar signs if any. Under 2000 characters total.`

// evangelismAngles loads the angle set from the embedded JSON. Returns nil on a
// malformed file (logged) so a bad edit never panics the binary.
func evangelismAngles() map[string]string {
	var angles map[string]string
	if err := json.Unmarshal(evangelismJSON, &angles); err != nil {
		app.Log("evangelism", "evangelism.json parse error: %v", err)
		return nil
	}
	return angles
}

// evangelismEnabled reports whether the loop should run. Default on; set
// EVANGELISM to a falsey value to disable.
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

// publishNextEvangelism posts one evangelism piece if enabled and enough time
// has passed since the last one.
func publishNextEvangelism() {
	if !evangelismEnabled() {
		return
	}
	if last := latestEvangelismTime(); !last.IsZero() && time.Since(last) < evangelismCadence {
		return // too soon
	}

	name, instruction := nextEvangelismAngle()
	if name == "" {
		return // no angles configured
	}
	title, body, err := generateEvangelism(name, instruction)
	if err != nil {
		app.Log("evangelism", "generation failed [%s]: %v", name, err)
		return
	}
	if err := CreatePost(title, body, app.SystemUserName, app.SystemUserID, evangelismTag+",about", false); err != nil {
		app.Log("evangelism", "failed to create post [%s]: %v", name, err)
		return
	}
	app.Log("evangelism", "published [%s]: %s", name, title)
}

// nextEvangelismAngle rotates deterministically through the angles (sorted by
// name) so consecutive posts differ, advancing one per cadence window.
func nextEvangelismAngle() (string, string) {
	angles := evangelismAngles()
	if len(angles) == 0 {
		return "", ""
	}
	names := make([]string, 0, len(angles))
	for n := range angles {
		names = append(names, n)
	}
	sort.Strings(names)
	window := time.Now().UTC().Unix() / int64(evangelismCadence/time.Second)
	name := names[int(window)%len(names)]
	return name, angles[name]
}

// latestEvangelismTime returns the creation time of the most recent evangelism
// post (tagged evangelismTag, authored by the system account), or zero.
func latestEvangelismTime() time.Time {
	var latest time.Time
	for _, post := range GetPostsByAuthor(app.SystemUserName) {
		if !strings.EqualFold(post.AuthorID, app.SystemUserID) || !hasTag(post.Tags, evangelismTag) {
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

// generateEvangelism writes one piece for the given angle. Same shape as the
// digest and opinion generators: voice in System, the specific ask in Question,
// PriorityLow + BackgroundModel.
func generateEvangelism(name, instruction string) (string, string, error) {
	prompt := &ai.Prompt{
		System:   evangelismVoice,
		Question: "Write today's piece on this angle — \"" + name + "\":\n" + instruction,
		Priority: ai.PriorityLow,
		Model:    ai.BackgroundModel(),
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
