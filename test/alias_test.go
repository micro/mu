package test

// The names clients are already calling.
//
// A tool's aliases are its retired names. They exist because something out
// there — a saved MCP config, a skill, somebody's script — still says `news`
// or `wallet` or `search_web`, and renaming a tool must not break it.
//
// They lived on the hand-written registrations, and when thirty of those became
// derived from their Specs the aliases went with them: a Spec had no way to say
// "this method also answers to `islam_prayer`", so the field was quietly empty
// and thirteen names stopped resolving. Nothing failed. A call to a retired name
// is a client's problem, in a client's logs, weeks later.
//
// So the list is written down here rather than left implicit in whichever file
// happens to register the tool this month.

import (
	"testing"

	"mu/tool"

	"mu/internal/api"
)

// legacy is every retired name that must keep resolving, and what it resolves
// to. Add to it when a tool is renamed; never remove from it, because removing
// an entry is exactly the breakage this is here to catch.
var legacy = map[string]string{
	"search_web":     "web_search",
	"search_fetch":   "web_fetch",
	"news":           "news_list",
	"news_headlines": "news_list",
	"index":          "index_search",
	"recall":         "index_search",
	"search":         "index_search",
	"markets":        "markets_list",
	"image_generate": "images_generate",
	"image_search":   "images_search",
	"islam_prayer":   "prayer_times",
	"islam_qibla":    "prayer_qibla",
	"islam_today":    "prayer_reflection",
	"prayer_today":   "prayer_reflection",
	"social":         "social_list",
	"video":          "video_list",
	"agent":          "agent_ask",
	"wallet":         "wallet_balance",
	"stream":         "stream_list",
	"mail_read":      "mail_inbox",
}

func TestEveryRetiredNameStillResolves(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	for old, current := range legacy {
		// Only check aliases whose tool is linked into this binary: the
		// hand-written half lives in internal/server, which this package does
		// not import.
		if !api.HasTool(current) {
			continue
		}
		if !api.HasTool(old) {
			t.Errorf("%q no longer resolves — %s dropped it, and every client "+
				"still calling by the old name now gets 'unknown tool'", old, current)
		}
	}
}

// And the list is not a fossil: the names in it have to be names of tools that
// exist, or it is asserting over things that were deleted years ago.
func TestTheRetiredNamesPointAtRealTools(t *testing.T) {
	registerAll(t)
	tool.DeriveTools()

	live := 0
	for _, current := range legacy {
		if api.HasTool(current) {
			live++
		}
	}
	if live < 5 {
		t.Errorf("only %d of the %d retired names point at a tool in this binary — "+
			"either the list has gone stale or nothing is registered", live, len(legacy))
	}
}
