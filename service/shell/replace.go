package shell

// Changing part of a file, without sending the whole file.
//
// # Why this exists rather than "just write it again"
//
// Write takes the complete file, so changing one colour in a page means the
// model emitting every byte of that page as a tool argument. That is the worst
// case for the thing tool calls are least reliable at.
//
// It is not a size worry in the abstract. On this instance, asked to change the
// background of a four-kilobyte page, the model wrote its own tool-call
// delimiters into the reply as plain text instead of making a call: the whole
// document arrived as prose, nothing ran, and the page was unchanged. The same
// model handles a short call without trouble. Open-weight models emit tool
// calls as tokens the serving stack has to parse back out, and a long, exactly
// escaped argument is where that comes apart — one bad escape, one truncation,
// one delimiter split across a stream chunk, and what should have been a call
// is text nobody can act on.
//
// A replacement is a few dozen characters whichever page it is applied to. It
// does not remove the failure, but it takes the common case — change this bit,
// leave the rest — out of its range. Every serious coding agent works this way
// and this is the reason.
//
// # Why it refuses when the text appears twice
//
// Because the caller cannot see which one it would get. Silently taking the
// first is how an edit lands in the wrong place and looks like it worked, and
// that is worse than being told to be more specific: the fix is to include a
// line either side, which the caller can do, and there is no fix for a change
// applied to the wrong copy.
//
// All is the exception and it is asked for by name. "Every #1e3a5f in this
// file" is a real intention, and without it a model that finds six of them is
// stuck with sed and a quoting problem.

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/container"
)

type ReplaceRequest struct {
	Path string `json:"path" required:"true" description:"The file to change, under /work"`
	Old  string `json:"old" required:"true" description:"The exact text to replace, including its indentation. Include a line either side if the text alone appears more than once"`
	New  string `json:"new" required:"true" description:"What to put there instead. Empty deletes the old text"`
	All  bool   `json:"all" description:"Replace every occurrence. Without it, text that appears more than once is refused rather than guessed at"`
}

type ReplaceResponse struct {
	Path    string `json:"path" description:"The file that changed"`
	Changed int    `json:"changed" description:"How many occurrences were replaced"`
	Bytes   int    `json:"bytes" description:"The file's size afterwards"`
}

// Replace swaps one piece of text in a file for another.
//
// @example {"path": "apps/tally/index.html", "old": "background: #0f172a", "new": "background: #ffffff"}
func (Server) Replace(ctx context.Context, req *ReplaceRequest, rsp *ReplaceResponse) error {
	who, err := caller(ctx)
	if err != nil {
		return err
	}
	path, err := under(who, req.Path)
	if err != nil {
		return err
	}
	if req.Old == "" {
		return fmt.Errorf("say which text to replace — old is empty, and a replacement " +
			"for nothing is a write, not an edit")
	}
	if err := ready(ctx, who); err != nil {
		return err
	}

	b, err := container.ReadFile(ctx, fileRun(who), path)
	if err != nil {
		return err
	}
	body := string(b)

	n := strings.Count(body, req.Old)
	switch {
	case n == 0:
		// What was actually looked for, because the usual cause is whitespace
		// or a character the caller did not expect to matter.
		return fmt.Errorf("that text is not in %s. Looked for exactly: %q", req.Path, trimTo(req.Old, 200))
	case n > 1 && !req.All:
		return fmt.Errorf("that text appears %d times in %s, so replacing it would be a "+
			"guess. Include a line either side to say which one, or ask for all of them", n, req.Path)
	}

	changed := n
	if !req.All {
		changed = 1
	}
	body = strings.Replace(body, req.Old, req.New, changed)

	if len(body) > maxFile {
		return fmt.Errorf("that change would make %s larger than this instance will take (%d bytes)",
			req.Path, maxFile)
	}
	if err := container.WriteFile(ctx, fileRun(who), path, []byte(body)); err != nil {
		return err
	}
	rsp.Path, rsp.Changed, rsp.Bytes = path, changed, len(body)
	return nil
}
