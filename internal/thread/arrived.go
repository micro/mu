package thread

// Where a conversation happened, and why that decides which page it is on.
//
// /inbox and /agent were two lists of the same conversations. Both read this
// store, neither filtered it, and the only difference was the furniture around
// them — which is why nobody could say what either page was for, including the
// person who asked for both.
//
// The line that holds is the one every mail product has drawn: an inbox is what
// *arrived*, and a chat is what you *started*. A conversation on this instance's
// own screens — the web page, the CLI — is one you opened, were present for, and
// watched the answer to. Everything else happened without you: an email came in
// overnight, somebody messaged the agent on WhatsApp, the briefing turned up.
//
// So: /agent is where you talk to an agent, and its rail is the conversations
// you have had with it here. /inbox is everything that came in, whichever
// channel carried it. Same record, one filter, and two pages that can be told
// apart in a sentence.
//
// This lives with the record rather than on either page because both need it
// and a copy in each is how they drifted the first time. It is the same reason
// WebClient is here.

// CLIClient names the terminal in the record. The other local surface: `mu
// agent "..."` is somebody sitting in front of it, not something arriving.
const CLIClient = "cli"

// Arrived reports whether a conversation came in from outside this instance's
// own screens.
//
// Anything that is not the web page or the CLI. Stated as an exclusion on
// purpose: a new channel — a new bot, a new protocol — is something that
// arrives, and it should turn up in the inbox on the day it is added rather
// than on the day somebody remembers to add it to a list here.
func Arrived(t Thread) bool {
	switch t.Client {
	case WebClient, CLIClient, "":
		return false
	}
	return true
}
