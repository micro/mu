package agent

// What Home says you have working.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

// The agent every account already has is on the list.
//
// Preview read Agents(owner), which is an account's *own* agents, so the one
// that answers agent@ and that the chat talks to was missing from the page
// whose job is to say what you have working. /agents had the same bug and its
// fix is recorded there: leaving Micro off "meant a new account opened /agents
// and was told it had none, which is false".
func TestThePreviewIncludesTheDefaultAgent(t *testing.T) {
	got := Preview("preview-default")
	if got == "" {
		t.Fatal("an account with the default agent got an empty list")
	}

	want := Platform(DefaultPlatformAgent)
	if want == nil {
		t.Skip("no platform registry in this build")
	}
	if !strings.Contains(got, want.Name) {
		t.Errorf("the default agent is not on Home:\n%s", got)
	}
	// And it goes where /agents sends it, or the two pages name one agent and
	// point at two.
	if !strings.Contains(got, `href="/agent/`+DefaultPlatformAgent+`"`) {
		t.Errorf("the default agent does not link to its own page:\n%s", got)
	}
}

// A conversation nothing answered is not an agent's activity.
//
// Most conversations have no agent recorded on them, and this read all of
// them — so Micro's row on the front page reported, as the last thing it dealt
// with, a DMARC aggregate report from Google that nothing had read. Mail the
// agent deliberately stayed quiet on did the same: it is on a thread between
// other people and says so, and the front page turned that silence into
// activity.
func TestAConversationNothingAnsweredIsNotActivity(t *testing.T) {
	const who = "preview-silent"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	// A report that arrived and was filed. Nobody answered it.
	quiet := thread.Open(who, "mail", "<dmarc@google.com>")
	if quiet == nil {
		t.Fatal("no thread")
	}
	thread.Name(who, quiet.ID, "Report domain: micro.mu Submitter: google.com")
	thread.Add(thread.Message{Thread: quiet.ID, Account: who, Role: thread.RolePerson,
		Text: "An aggregate report is attached.", From: "noreply-dmarc@google.com"})

	got := Preview(who)
	if strings.Contains(got, "Report domain") {
		t.Error("a conversation nothing answered was reported as an agent's last activity")
	}
	if !strings.Contains(got, "Nothing yet") {
		t.Errorf("an agent that has done nothing does not say so:\n%s", got)
	}

	// And once it has answered on one, that is what it last dealt with.
	live := thread.Open(who, "mail", "<henrik@example.com>")
	thread.Name(who, live.ID, "Tuesday")
	thread.Add(thread.Message{Thread: live.ID, Account: who, Role: thread.RolePerson,
		Text: "Are you free Tuesday?", From: "henrik@example.com"})
	thread.Add(thread.Message{Thread: live.ID, Account: who, Role: thread.RoleAgent,
		Text: "I have replied."})

	if got := Preview(who); !strings.Contains(got, "Tuesday") {
		t.Errorf("a conversation the agent answered is not reported:\n%s", got)
	}
}

// Each agent on Home links to itself.
//
// Path takes the owner and was given "", so SlugFor looked every id up in
// nobody's roster, found nothing, decided it must be one of the instance's own,
// found nothing again, and fell back to the default slug. Every agent on the
// page linked to /agent/micro. The links worked; they went somewhere else.
func TestEachAgentOnHomeLinksToItself(t *testing.T) {
	const who = "preview-links"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	made, _, err := CreateAgent(who, "Research", "", "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	got := Preview(who)
	want := `href="` + Path(who, made.ID) + `"`
	if !strings.Contains(got, want) {
		t.Errorf("the agent's row does not link to it (want %s):\n%s", want, got)
	}
	if n := strings.Count(got, `href="/agent/micro"`); n != 1 {
		t.Errorf("%d rows link to the default agent, want only its own", n)
	}

	// And the link resolves back to the agent it was drawn for. Building the
	// link and reading it back are two halves that were each right alone: Path
	// was called with an empty owner, BySlug read the "micro" that came out of
	// it as the default, and the page opened somebody else's chat with somebody
	// else's conversations beside it. Only the round trip says so.
	slug := strings.TrimPrefix(Path(who, made.ID), "/agent/")
	id, ok := BySlug(who, slug)
	if !ok || id != made.ID {
		t.Errorf("/agent/%s resolves to %q (ok=%v), want %q", slug, id, ok, made.ID)
	}
	if title := agentTitle(who, id); title != "Research" {
		t.Errorf("the page at /agent/%s is titled %q", slug, title)
	}
}
