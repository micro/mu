package test

// Everything this instance publishes links what it names.
//
// The two writers disagreed. The digest asks for inline markdown links and
// gets them; the opinion said only "Do NOT include a references section" and
// asked for nothing in their place. So a piece built from three researched
// projects opened:
//
//	A bird identification system running silently on a home server. A
//	walkable cyberpunk city crafted entirely from letters and symbols in a
//	single HTML file. A weathered phone booth on a desert playa…
//
// Three specific things named, none of them reachable — while the research
// pass that found them had the URLs in hand.
//
// This is the same failure as the archive advertising kinds nothing writes and
// the agent having two constants for its own name: one decision, made twice,
// in two places, drifting. The prompts cannot share a string — they are
// different pieces of writing — so what is shared is the rule, and this is it.

import (
	"strings"
	"testing"

	agentblog "mu/agent/blog"
	"mu/agent/digest"
)

func TestEveryWriterIsToldToLinkWhatItNames(t *testing.T) {
	for what, prompt := range map[string]string{
		"the daily briefing": digest.System(true),
		"an opinion piece":   agentblog.System("Dev"),
	} {
		if !strings.Contains(prompt, "INLINE") {
			t.Errorf("%s is not asked for inline links, so it names things a reader "+
				"cannot reach", what)
		}
		if !strings.Contains(prompt, "markdown") {
			t.Errorf("%s is not told what form a link takes", what)
		}
		// And neither appends a bibliography instead. The digest builds its
		// references block itself, from what the tools returned; a model
		// writing a second one duplicates it, and in the opinion there is
		// nothing to build one from.
		if !strings.Contains(prompt, "references section") {
			t.Errorf("%s is not told to leave the references block alone", what)
		}
	}
}
