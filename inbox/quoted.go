package inbox

// The part of a reply that is last week's mail.
//
// A mail client quotes what it is answering. Three exchanges in, every message
// carries the two before it, so a thread of six two-line replies renders as six
// screens — and this page is already showing those messages above, in full,
// in order. The quote is not extra context here. It is the same conversation
// printed again inside itself.
//
// So it is cut and folded away rather than deleted. Deleted would be the
// cleaner-looking answer and it is the wrong one: a quote is sometimes edited,
// and somebody replying inline underneath one is saying something that only
// reads with it. What is removed is what a reader can already see; what is kept
// is one disclosure they can open.
//
// # Where the cut is
//
// Three marks, and the first one wins:
//
//   - an attribution line — "On Friday, Asim <asim@micro.mu> wrote:" — which
//     Gmail, Apple Mail and most of the rest write, sometimes wrapped over two
//     lines with "wrote:" alone on the second
//   - a separator — Outlook's "-----Original Message-----" and the row of
//     underscores it draws above a forwarded header block
//   - the start of a run of ">" lines that reaches the end of the message,
//     which is what a client that writes no attribution leaves behind
//
// Conservative on purpose, in both directions. A ">" run that stops and starts
// again is somebody answering inline and is not a boundary. And nothing is cut
// when the cut would leave nothing: a message that is quotation from its first
// line is a forward, and a forward with its body hidden is a blank message.

import (
	"strings"
)

// unquoted splits a message into what was written now and what is being quoted
// back. The second half is empty for anything with no quote in it, which is
// most messages.
func unquoted(text string) (body, quoted string) {
	lines := strings.Split(text, "\n")
	cut := quoteStart(lines)
	if cut <= 0 {
		return text, ""
	}
	body = strings.TrimRight(strings.Join(lines[:cut], "\n"), "\n \t")
	quoted = strings.Trim(strings.Join(lines[cut:], "\n"), "\n")
	// A message that is quotation all the way up is a forward, and hiding a
	// forward's body leaves an empty message with a "..." under it.
	if strings.TrimSpace(body) == "" {
		return text, ""
	}
	return body, quoted
}

// quoteStart is the line the quote begins on, or -1 for a message with none.
func quoteStart(lines []string) int {
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		switch {
		case separator(s):
			return i
		case attribution(s):
			return i
		// The attribution Gmail wrapped. "On <date>, <name> <addr>" on one line
		// and "wrote:" on the next is one sentence the renderer broke, and
		// looking at either line alone finds nothing.
		case strings.HasPrefix(s, "On ") && i+1 < len(lines) &&
			attribution(s+" "+strings.TrimSpace(lines[i+1])):
			return i
		case strings.HasPrefix(s, ">") && quotedToEnd(lines[i:]):
			return i
		}
	}
	return -1
}

// attribution is the line a client writes above what it is quoting.
//
// It ends in "wrote:" — that is the shape, in every client that writes one and
// in most translations of it that keep the colon. The second half of the test
// is what stops it matching a sentence: an attribution names a time or an
// address, and "I told them what he wrote:" names neither.
func attribution(s string) bool {
	if !strings.HasSuffix(s, "wrote:") {
		return false
	}
	return strings.HasPrefix(s, "On ") || strings.Contains(s, "@")
}

// separator is a client drawing a line above the message it is including.
//
// The dashed one is Outlook's, and it is matched on its two words rather than
// on an exact count of dashes because the count differs by version. The
// underscore rule is the horizontal rule Outlook and Exchange draw above a
// forwarded header block; a row of them is never prose.
func separator(s string) bool {
	if strings.HasPrefix(s, "---") && strings.Contains(strings.ToLower(s), "original message") {
		return true
	}
	if strings.HasPrefix(s, "---") && strings.Contains(strings.ToLower(s), "forwarded message") {
		return true
	}
	return strings.Count(s, "_") >= 8 && strings.Trim(s, "_") == ""
}

// quotedToEnd says whether everything from here on is quotation.
//
// The whole point of the ">" rule: a run that stops and starts again is
// somebody answering between the paragraphs they are answering, and folding
// that away would hide half of what they said.
func quotedToEnd(rest []string) bool {
	for _, ln := range rest {
		s := strings.TrimSpace(ln)
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ">") {
			return false
		}
	}
	return true
}
