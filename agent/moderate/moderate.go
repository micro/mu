// Package moderate is the judgement about whether something published here
// should stay up.
//
// # Why it is an agent and not part of the record
//
// internal/flag holds the record: which items are flagged, by whom, how many
// times, and whether that adds up to hidden. Every question it answers is a
// question about state. Deciding that a particular paragraph is spam is not
// one of those — it is a model reading prose and forming an opinion — and this
// repository has a rule about exactly that:
//
//	A service answers a question about state, an agent decides which question
//	to ask, and a service calling an agent is asking the model what its own
//	answer should be.
//
// Three services were doing precisely that. service/social, service/blog and
// service/apps each called flag.CheckContent, which ran an LLM. The rule was
// not broken by an import — it was broken by a function variable, `analyzer`,
// which internal/flag declared and which was filled in at boot by, of all
// things, service/chat. CLAUDE.md names that shape: "a function variable is an
// import the compiler cannot see", and every rule in the layering tests is
// checked by reading import statements.
//
// It had the failure mode you would expect. CheckContent opened with
// `if analyzer == nil { return }`, so moderation for the entire instance was
// one unrelated service away from being silently off — no log line, nothing on
// /admin/moderate to tell "nothing was bad" from "nothing was checked". It
// worked only because chat.Load() happens to be unconditional in boot.go.
//
// So the services announce and this subscribes, which is the same shape as
// service/mail and agent/mail, and as event.RequestWork and agent/work. The
// direction is the one the layering asks for.
//
// # What it does not decide
//
// Whether hiding at three flags is right, what a flag means, or what happens
// to the content afterwards. It calls flag.AdminFlag and stops. The record is
// still the record; this only ever adds one more voice to it — the same voice
// a person adds when they press the flag button, and by the same door.
package moderate

import (
	"strings"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/flag"
)

// Load subscribes to what gets published. Called at boot.
func Load() {
	go func() {
		sub := event.Subscribe(event.EventContentPublished)
		for e := range sub.Chan {
			kind, _ := e.Data["kind"].(string)
			id, _ := e.Data["id"].(string)
			title, _ := e.Data["title"].(string)
			text, _ := e.Data["text"].(string)
			if kind == "" || id == "" {
				continue
			}
			// One goroutine per item: this is a model call, and the
			// subscription channel is small and drops when it is full. Same
			// reason agent/mail does it.
			go judge(kind, id, title, text)
		}
	}()
}

// judge classifies one item and hides it if it should not be up.
//
// Quiet when there is no model configured, and that is a real state rather
// than a fault: an instance with no AI provider has no moderator, and the
// three-user-flags rule in internal/flag still works without one. What it must
// not do is pretend — see Configured below, which is what /admin/moderate
// reads to say so on the page.
func judge(kind, id, title, text string) {
	if !Configured() {
		return
	}

	verdict, err := ai.Ask(&ai.Prompt{
		System:   prompt,
		Question: "Title: " + title + "\n\nContent: " + text,
		Model:    ai.BackgroundModel(),
	})
	if err != nil {
		// Worth a line, because a moderator that has stopped working looks
		// exactly like a well-behaved community.
		app.Log("moderate", "could not classify %s %s: %v", kind, id, err)
		return
	}

	verdict = strings.ToUpper(strings.TrimSpace(verdict))
	switch verdict {
	case "SPAM", "LOW_QUALITY", "HARMFUL":
	default:
		return
	}

	// Hidden now, not after three people have found it.
	//
	// The count is what internal/flag reads, and a system verdict goes
	// straight to it: the alternative is spam that stays up until enough
	// readers have seen it to report it, which is the wrong way round.
	if err := flag.AdminFlag(kind, id, "system:"+strings.ToLower(verdict)); err != nil {
		app.Log("moderate", "could not hide %s %s: %v", kind, id, err)
		return
	}
	app.Log("moderate", "hid %s %s: %s", kind, id, verdict)
}

// Configured reports whether this instance can moderate at all.
//
// An adjective rather than a verb, and a question rather than an instruction —
// see the naming rules. It exists so /admin/moderate can say "no model
// configured, nothing is being classified" instead of showing an empty list
// that reads as a clean bill of health.
func Configured() bool { return ai.Configured() }

// prompt is what the model is asked. Moved here from internal/flag with the
// rest of the judgement: a store does not hold an opinion about prose.
//
// Kept deliberately reluctant. The failure that matters is not missing a rude
// word, it is hiding somebody's ordinary status update — the first is visible
// and gets reported, the second is invisible and the author just concludes the
// product ate their post.
const prompt = `You are a strict content moderator for a family-friendly community. Every post should be meaningful and respectful. This is not a place to waste time, troll, or post crude content.

Classify the content with ONLY ONE WORD:
- SPAM (promotional spam, advertising, repetitive junk, SEO content)
- LOW_QUALITY (gibberish, random characters, meaningless typing like "asdf", single letters)
- HARMFUL (vulgar, crude, sexual, obscene, gossip, slander, personal attacks, mocking, trolling, shock content, swear words)
- OK (everything else — status updates, opinions, questions, short messages, work updates, casual conversation)

IMPORTANT: Short personal status updates like "Working on X", "Good morning", "Just shipped Y", "Having lunch" are ALWAYS OK. They are normal status messages, not spam or low quality. Only flag content that is clearly abusive, vulgar, or spam. When in doubt, say OK.

Respond with just the single word.`
