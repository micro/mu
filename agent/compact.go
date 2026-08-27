package agent

// What a conversation was about, when it no longer fits.
//
// The window is a budget (memory.go) and a conversation can exceed it. Dropping
// the oldest turns is the cheap answer and it loses the wrong thing: the
// beginning is where somebody said what they were trying to do, so a long
// working conversation forgets its own purpose while remembering the last
// twenty exchanges of detail.
//
// So the dropped turns are summarised instead, and the summary takes the place
// of the note that used to say "23 earlier messages are not shown" — which was
// honest and useless, since a model told something is missing can only say so.
//
// # What it costs, and why that is acceptable
//
// One model call, on the background model rather than the agent's own, and only
// for a conversation long enough to exceed the budget. Cached on what was
// summarised, so a conversation that keeps growing pays once per compaction
// rather than once per question — the same turns produce the same key.
//
// It never fails a question. A summary that cannot be produced falls back to
// the note, because an answer without the opening is worse than an answer with
// it and much better than no answer.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"mu/internal/ai"
	"mu/internal/app"
)

// compactionCache holds summaries by what they summarise.
//
// Keyed by the content of the dropped turns rather than by thread, because that
// is what makes it correct rather than merely fast: the same turns are the same
// summary whoever is asking, and a conversation that grows past the budget
// again produces a different key and a new summary.
var compactionCache sync.Map // string -> string

// compactionLimit bounds how much conversation is sent to be summarised.
//
// A summary of a summary is fine; a request that itself blows the context is
// not. Anything past this is dropped before summarising, oldest first, and the
// summary says how many turns it covers rather than pretending to cover all.
const compactionLimit = 120_000

// summarise turns the messages that did not fit into one paragraph the model
// can use, or "" when it could not be produced.
func summarise(dropped []QueryMessage) string {
	if len(dropped) == 0 {
		return ""
	}
	if !ai.Configured() {
		return ""
	}

	transcript, covered := transcriptOf(dropped)
	if transcript == "" {
		return ""
	}
	key := compactionKey(transcript)
	if cached, ok := compactionCache.Load(key); ok {
		return cached.(string)
	}

	answer, err := ai.Ask(&ai.Prompt{
		System: "You compress the earlier part of a conversation so it can be carried " +
			"forward. Write one paragraph, at most 120 words, in the third person. " +
			"Keep: what the user is trying to do, decisions and preferences they " +
			"stated, facts established, and anything left unresolved. Drop: " +
			"pleasantries, and detail that has since been superseded. Do not add " +
			"anything that was not said. Write the paragraph and nothing else.",
		Question: transcript,
		// The cheap model. This is not the answer, it is context for the
		// answer, and the same reasoning that puts summaries on the background
		// model everywhere else applies here.
		Model:     ai.BackgroundModel(),
		Priority:  2,
		Caller:    "agent.compact",
		MaxTokens: 400,
	})
	if err != nil {
		// Not fatal, and said once per failure rather than swallowed: a
		// conversation quietly losing its beginning is exactly the kind of
		// degradation nobody reports because nothing looks broken.
		app.Log("agent", "could not summarise %d earlier messages, carrying a note "+
			"instead: %v", len(dropped), err)
		return ""
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return ""
	}

	summary := fmt.Sprintf("[Earlier in this conversation (%d messages, summarised): %s]",
		covered, answer)
	compactionCache.Store(key, summary)
	return summary
}

// transcriptOf renders the dropped turns for summarising, newest last, and says
// how many it managed to include.
//
// Trimmed from the front when it is too long, because if anything has to go it
// should be the oldest — the same rule the window itself follows.
func transcriptOf(dropped []QueryMessage) (text string, covered int) {
	kept := make([]string, 0, len(dropped))
	spent := 0
	for i := len(dropped) - 1; i >= 0; i-- {
		t := dropped[i]
		if strings.TrimSpace(t.Text) == "" {
			continue
		}
		who := "User"
		if t.Role == "assistant" {
			who = "Assistant"
		}
		line := who + ": " + t.Text
		if spent+len(line) > compactionLimit && len(kept) > 0 {
			break
		}
		spent += len(line)
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return "", 0
	}
	var b strings.Builder
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
		b.WriteString("\n")
	}
	return b.String(), len(kept)
}

func compactionKey(transcript string) string {
	sum := sha256.Sum256([]byte(transcript))
	return hex.EncodeToString(sum[:16])
}
