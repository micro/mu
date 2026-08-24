package server

// What an instance says when it has finished starting.
//
// Startup printed 313 lines and none of them was an answer to the two
// questions somebody standing at a terminal actually has: where do I open it,
// and is anything missing. The log has moved to a file (internal/app/logfile.go)
// and this is what takes its place on the screen.
//
// Deliberately short. The value of this surface is that it is small enough to
// read, and every line added to it is paid for by every line already there
// becoming less likely to be read.

import (
	"fmt"
	"strings"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/settings"
)

// ready prints the address, anything an operator still has to do, and where
// the log went.
func ready(addr string, activated bool) {
	where := addr
	if strings.HasPrefix(where, ":") {
		where = "localhost" + where
	}
	app.Announce("")
	if activated {
		app.Announce("  Mu is running on a systemd socket — restarts queue rather than refusing")
	} else {
		app.Announce("  Mu is running at http://%s", where)
	}

	if missing := missingPieces(); len(missing) > 0 {
		app.Announce("")
		// Named rather than counted. "3 things are not configured" sends
		// somebody looking; naming them lets them decide whether they care —
		// an instance with no Twilio is fine if nobody wants to send a text.
		app.Announce("  Not configured yet:")
		for _, m := range missing {
			app.Announce("    %s", m)
		}
		app.Announce("")
		app.Announce("  Set these at http://%s/admin/config, or as environment variables.", where)
	}

	if path := app.LogPath(); path != "" {
		app.Announce("")
		app.Announce("  Log: %s", path)
	}
	app.Announce("")
}

// missingPieces is what this instance cannot do until somebody supplies
// something, most important first.
//
// Only things with a consequence a person would notice. A missing YouTube key
// means the video tools answer "not configured"; a missing model means the
// agent — the product — does not work at all, which is why it is first and
// why it says so in those words.
func missingPieces() []string {
	var out []string

	if !ai.Configured() {
		out = append(out, "A model. The agent cannot answer anything without one — "+
			"ANTHROPIC_API_KEY, ATLAS_API_KEY or OPENROUTER_API_KEY, or run `mu setup`.")
	}
	if strings.TrimSpace(settings.Get("MAIL_DOMAIN")) == "" {
		out = append(out, "MAIL_DOMAIN — no address here can receive or send mail without it.")
	}
	for _, k := range []struct{ env, what string }{
		{"BRAVE_API_KEY", "web search"},
		{"YOUTUBE_API_KEY", "video"},
		{"GOOGLE_API_KEY", "places and routes (open data is used without it)"},
	} {
		if strings.TrimSpace(settings.Get(k.env)) == "" {
			out = append(out, fmt.Sprintf("%s — %s", k.env, k.what))
		}
	}
	return out
}
