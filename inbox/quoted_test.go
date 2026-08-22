package inbox

// What gets folded away, and what must never be.
//
// The failure that matters is not a quote that stays visible — that is the
// page as it was. It is a message with half of what somebody said hidden
// behind three dots nobody clicks, which is why every rule here is
// conservative and why the inline-reply and forward cases are tested.

import (
	"strings"
	"testing"
)

func TestTheQuotedTailIsCutAtTheAttribution(t *testing.T) {
	const msg = `Yes, Tuesday works.

On Fri, 22 Aug 2026 at 10:12, Asim <asim@micro.mu> wrote:
> Are you free Tuesday?
> I can do the morning.`

	body, quoted := unquoted(msg)
	if strings.TrimSpace(body) != "Yes, Tuesday works." {
		t.Errorf("the reply came back as %q", body)
	}
	if !strings.Contains(quoted, "Are you free Tuesday?") {
		t.Errorf("the quote was not kept: %q", quoted)
	}
	if strings.Contains(body, "wrote:") {
		t.Error("the attribution line stayed in the body")
	}
}

// Gmail wraps that line. "wrote:" alone on the second line is one sentence the
// renderer broke, and looking at either half finds nothing.
func TestAWrappedAttributionIsStillTheBoundary(t *testing.T) {
	const msg = `Sounds good.

On Fri, 22 Aug 2026 at 10:12, Asim Aslam <asim@micro.mu>
wrote:
> Shall we?`

	body, quoted := unquoted(msg)
	if strings.TrimSpace(body) != "Sounds good." {
		t.Errorf("the reply came back as %q", body)
	}
	if !strings.Contains(quoted, "Shall we?") {
		t.Errorf("the quote was not kept: %q", quoted)
	}
}

func TestOutlookSeparatorsAreBoundaries(t *testing.T) {
	for _, msg := range []string{
		"Noted, thanks.\n\n-----Original Message-----\nFrom: Asim\nSent: Friday\n\nAre you free?",
		"Noted, thanks.\n\n________________________________\nFrom: Asim\nSent: Friday\n\nAre you free?",
		"Noted, thanks.\n\n---------- Forwarded message ---------\nFrom: Asim\n\nAre you free?",
	} {
		body, quoted := unquoted(msg)
		if strings.TrimSpace(body) != "Noted, thanks." {
			t.Errorf("%q left the body as %q", msg, body)
		}
		if !strings.Contains(quoted, "Are you free?") {
			t.Errorf("%q lost the quote", msg)
		}
	}
}

// A client that writes no attribution leaves only the markers, and a run of
// them that reaches the end is the quote.
func TestABareQuoteRunIsTheBoundary(t *testing.T) {
	body, quoted := unquoted("Done.\n\n> the old message\n>\n> and the rest of it")
	if strings.TrimSpace(body) != "Done." {
		t.Errorf("the body came back as %q", body)
	}
	if !strings.Contains(quoted, "the old message") {
		t.Errorf("the quote was not kept: %q", quoted)
	}
}

// Somebody answering between the paragraphs they are answering. Folding at the
// first ">" would hide everything they said after it.
func TestAnInlineReplyIsNotFolded(t *testing.T) {
	const msg = `> Are you free Tuesday?

Yes.

> And can you bring the figures?

They are attached.`

	body, quoted := unquoted(msg)
	if quoted != "" {
		t.Errorf("an inline reply was folded: %q", quoted)
	}
	if body != msg {
		t.Error("an inline reply was altered")
	}
}

// A forward is quotation from its first line. Hiding it leaves a blank message
// with three dots under it.
func TestAForwardKeepsItsBody(t *testing.T) {
	const msg = "> everything here\n> is the thing being forwarded"
	body, quoted := unquoted(msg)
	if quoted != "" || body != msg {
		t.Errorf("a forward was emptied: body=%q quoted=%q", body, quoted)
	}
}

// The ordinary message, which is most of them: nothing to cut, nothing cut.
func TestAPlainMessageIsUntouched(t *testing.T) {
	const msg = "Hello.\n\nI told them what he wrote: nothing.\n\nAsim"
	body, quoted := unquoted(msg)
	if body != msg || quoted != "" {
		t.Errorf("a plain message was cut: body=%q quoted=%q", body, quoted)
	}
}

// And the rendered half: the fold is a control, and what is behind it is
// escaped like anything else somebody typed.
func TestTheFoldIsDrawnOnlyWhenThereIsAQuote(t *testing.T) {
	if got := quotedBlock("   \n  "); got != "" {
		t.Errorf("an empty quote drew %q", got)
	}
	got := quotedBlock("> <script>alert(1)</script>")
	if !strings.Contains(got, "<details class=\"ib-quoted\"") {
		t.Errorf("no fold: %q", got)
	}
	if strings.Contains(got, "<script>") {
		t.Errorf("the quote was not escaped: %q", got)
	}
}
