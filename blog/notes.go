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

// Notes is Mu's own voice on its own blog: the platform telling its story on the
// platform — actual content, actual usage.
//
// It is produced the same way as the daily brief (news/digest) and the opinion
// pieces (blog/opinion.go): the voice lives in a System prompt here in code, the
// SET of things to produce lives in an embedded JSON (notes.json, a
// name -> instruction map like chat/prompts.json), and each piece is generated
// with ai.Ask (PriorityLow, BackgroundModel) and posted as the system account on
// a cadence. Disable with the NOTES setting (off/false/0/no).

// notesJSON is the set of angles: a map of title -> writing instruction, the
// same shape as chat/prompts.json. Edit this file (data) to manage what gets
// produced — no code change needed.
//
//go:embed notes.json
var notesJSON []byte

const notesTag = "mu"

// notesCadence is the minimum spacing between notes. Low on purpose — this is
// the platform's own voice, not a feed.
const notesCadence = 72 * time.Hour

// notesVoice is the editorial voice + grounding, kept in code like the digest's
// System prompt and opinion's agentPurpose. The model may claim only what the
// angle instruction and these core facts support — never invents features,
// numbers, or users.
const notesVoice = `You are Micro, the voice of Mu, writing a short piece for Mu's own blog about Mu itself.

Core facts you may rely on (claim nothing beyond these and the specific angle you are given):
- Mu is a personal home server for the everyday internet: you ask it in plain language and it calls real services — news, mail, search, weather, markets, video, blog, places, reminders and apps — and gives a single answer. It's yours — use it hosted or self-host it — and it remembers your preferences over time.
- Mu is open source (AGPL-3.0) and self-hostable as a single Go binary; running the whole stack yourself is a real, optional path.
- Mu is built on Go Micro: every capability is a go-micro service, the assistant is a go-micro agent, and the MCP and A2A endpoints are its gateways.
- Values: no ads, no tracking, no algorithmic feed, no infinite scroll. You pay for the tools, not with your attention. AI assists, it does not replace, and is honest when it does not know.
- Available on the web at micro.mu, plus Discord, Telegram, and WhatsApp when configured; developers reach every service over REST, MCP, A2A and the CLI.
- For date-sensitive news, Mu should disclose freshness plainly: when only older news_search results are available, the answer leads with that caveat before listing stories.

Voice: plain, concrete, and honest. Lead with what a person actually gets. No hype, no superlatives, no exclamation marks, no growth-hacky tone — the same restraint Mu applies to its product (no dark patterns in the copy either). Name the trade-offs honestly. If unsure whether something is true, leave it out.

Output format:
Line 1: the title only (no quotes, no "Blog:" prefix).
Line 2: empty.
Line 3+: the body — 3 to 5 short paragraphs of flowing prose. No bullet lists, no headings, no references section. Plain dollar signs if any. Under 2000 characters total.`

// noteAngles loads the angle set from the embedded JSON. Returns nil on a
// malformed file (logged) so a bad edit never panics the binary.
func noteAngles() map[string]string {
	var angles map[string]string
	if err := json.Unmarshal(notesJSON, &angles); err != nil {
		app.Log("notes", "notes.json parse error: %v", err)
		return nil
	}
	return angles
}

// notesEnabled reports whether the loop should run. Default on; set NOTES to a
// falsey value to disable.
func notesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("NOTES"))) {
	case "off", "false", "0", "no":
		return false
	}
	return true
}

// StartNotes begins the background notes posting loop. Called from main.go after
// the building blocks are loaded (next to StartOpinion).
func StartNotes() {
	go notesLoop()
}

func notesLoop() {
	// Let the other services settle, and stagger after the opinion loop.
	time.Sleep(90 * time.Second)
	for {
		publishNextNote()
		time.Sleep(6 * time.Hour) // actual pacing is time-based, see cadence
	}
}

// publishNextNote posts one note if enabled and enough time has passed since the
// last one.
func publishNextNote() {
	if !notesEnabled() {
		return
	}
	if last := latestNoteTime(); !last.IsZero() && time.Since(last) < notesCadence {
		return // too soon
	}

	name, instruction := nextNote()
	if name == "" {
		return // no angles configured
	}
	title, body, err := generateNote(name, instruction)
	if err != nil {
		app.Log("notes", "generation failed [%s]: %v", name, err)
		return
	}
	if err := CreatePost(title, body, app.SystemUserName, app.SystemUserID, notesTag+",notes", false); err != nil {
		app.Log("notes", "failed to create post [%s]: %v", name, err)
		return
	}
	app.Log("notes", "published [%s]: %s", name, title)
}

// nextNote rotates deterministically through the angles (sorted by name) so
// consecutive posts differ, advancing one per cadence window.
func nextNote() (string, string) {
	angles := noteAngles()
	if len(angles) == 0 {
		return "", ""
	}
	names := make([]string, 0, len(angles))
	for n := range angles {
		names = append(names, n)
	}
	sort.Strings(names)
	window := time.Now().UTC().Unix() / int64(notesCadence/time.Second)
	name := names[int(window)%len(names)]
	return name, angles[name]
}

// latestNoteTime returns the creation time of the most recent note (tagged
// notesTag, authored by the system account), or zero.
func latestNoteTime() time.Time {
	var latest time.Time
	for _, post := range GetPostsByAuthor(app.SystemUserName) {
		if !strings.EqualFold(post.AuthorID, app.SystemUserID) || !hasTag(post.Tags, notesTag) {
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

// generateNote writes one piece for the given angle. Same shape as the digest
// and opinion generators: voice in System, the specific ask in Question,
// PriorityLow + BackgroundModel.
func generateNote(name, instruction string) (string, string, error) {
	prompt := &ai.Prompt{
		System:   notesVoice,
		Question: "Write today's piece on this angle — \"" + name + "\":\n" + instruction,
		Priority: ai.PriorityLow,
		Model:    ai.BackgroundModel(),
		Caller:   "notes-generate",
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
