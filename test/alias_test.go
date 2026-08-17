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
	"markets":        "markets_list",
	"image_generate": "images_generate",
	"image_search":   "images_search",
	"islam_prayer":   "prayer_times",
	"islam_qibla":    "prayer_qibla",
	"islam_today":    "prayer_reflection",
	"prayer_today":   "prayer_reflection",
	"social":         "social_list",
	"video":          "video_list",
	"stream":         "stream_list",
	"mail_read":      "mail_inbox",
	// memory became cache, which described the machinery rather than the thing,
	// and both became notes. Every name a client ever called still resolves.
	"memory_set":    "notes_add",
	"memory_list":   "notes_list",
	"memory_delete": "notes_delete",
	"cache_set":     "notes_add",
	"cache_get":     "notes_get",
	"cache_list":    "notes_list",
	"cache_delete":  "notes_delete",
	// places answered "what is there" and "how do I get between them", and the
	// second is the Routes API's domain rather than the Places API's. The ETA
	// moved out to routes; the name it was called by still works.
	"places_eta": "routes_eta",
	// db_* is deliberately absent, and is the one retirement in this file that
	// was not a rename.
	//
	// db became docs, and docs was still a database: docs_create took a
	// collection name and a bag of JSON. Documents are a title and a body now,
	// so docs_write is not docs_create under a new name — it takes different
	// arguments and means a different thing, and an alias would point an old
	// caller at a tool that cannot do what they are asking. Nothing was stored
	// through the old shape on any instance we know of, which is why this could
	// be a break rather than a bridge.
	"reminder": "prayer_reflection",
	// sms_inbox returned both directions, which is not what an inbox is.
	"sms_inbox": "sms_history",
	// The content verbs — save, hide, flag, block — were tools pointed at page
	// handlers, then briefly a service, and are now neither: what an account
	// keeps and mutes is furniture rather than a capability, so it lives at
	// /user with no tools over it. See internal/user. There is nothing to
	// redirect them to, which is why they are not in this map: an alias to a
	// tool that no longer exists is worse than a name that plainly does not.
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
