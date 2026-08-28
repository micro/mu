package code

// A turn: the model works on a machine, and what it leaves there is published.
//
// This is the difference between /code and the two things it sits between.
//
// The apps page asks a model for a whole document in one reply and hosts what
// comes back. It cannot look at what it made, so when the page is wrong the
// only move is to ask again from the start — and everything the model got right
// is thrown away with everything it got wrong.
//
// The shell can look at what it made, and hosts nothing. Somebody working there
// has files, commands and no way to see the page running or to put it anywhere.
//
// A turn here is both. The model gets the box and the four tools it already
// understands — write a file, read it back, list a directory, run a command —
// works on index.html the way anybody would, and when it stops, this publishes
// the directory. The page then loads in the frame above the box, which is what
// makes the next thing you say a correction rather than a fresh order.
//
// # The model is not asked to publish
//
// It could be: apps_publish is a tool and the model can see it. It is not asked
// to, because a run that writes a good page and then fails to say one last
// sentence correctly has produced nothing, and that failure is real — a model
// that has just emitted several kilobytes of HTML is measurably worse at
// emitting a tool call immediately afterwards.
//
// So the glue is here, in Go, where it cannot be got wrong. The model's job is
// the file; hosting the file is this package's job. That is also why the tools
// are scoped to the shell alone: nothing this turn does needs anything else,
// and a smaller menu is a better one.

import (
	"context"
	"fmt"
	"strings"

	"mu/agent"
	"mu/service/apps"
)

// where an app is built. Under a directory of their own rather than at the top
// of /work, because /work is the caller's own machine and somebody using it for
// their own things should not find our directories mixed in with theirs.
func workdir(slug string) string { return "apps/" + slug }

// instructions is what the model is told it is doing.
//
// Short on purpose. It names the one file, says the page must stand alone, and
// stops — the tools describe themselves, and a long prompt explaining tools the
// model already understands is how the important sentence gets lost.
func instructions(dir string, existing bool, current string) string {
	var b strings.Builder
	b.WriteString("You are building a single web page that runs on its own.\n\n")
	fmt.Fprintf(&b, "The page is %s/index.html on your machine. ", dir)
	if existing {
		b.WriteString("It exists and you do not need to read it — it is at the " +
			"bottom of these instructions. " +
			"Change what was asked for and leave the rest alone — somebody is " +
			"looking at this page and expects to recognise it afterwards.\n\n" +
			// Because it did exactly this: read the file, then replied with the
			// whole document as prose, having changed nothing. The reply is not
			// where the page lives and saying so costs one sentence.
			"Save the changed page with the write tool. Printing it in your " +
			"reply does nothing — the file is the only thing anybody sees.\n\n" +
			// The shell's own answer to a small change, and it dodges the
			// failure that costs us most: a model that has just read four
			// kilobytes is markedly worse at emitting four kilobytes back as a
			// tool argument, and the run ends having changed nothing. A sed is
			// twenty characters and cannot fail that way.
			"For a small change — a colour, a word, a number — use a command " +
			"like sed rather than rewriting the file. Rewrite the whole page " +
			"only when the change is structural.\n\n")
	} else {
		// Said plainly because it was not: given "the page is here" for a file
		// that does not exist, a run listed the empty directory, reported that
		// it was empty, and stopped. Nothing was wrong with the model's
		// reasoning — it had not been told the file was its to create.
		b.WriteString("It does not exist yet and nothing else is going to make it. " +
			"Write it now, before looking around.\n\n")
	}
	b.WriteString("One file. Everything it needs — style, script, data — is inside it, " +
		"because it is served as a single page and nothing else is loaded. " +
		"No frameworks fetched from anywhere, no external stylesheets, no build step. " +
		"It must work when opened with nothing else present.\n\n")
	b.WriteString("Write the file with the write tool rather than a shell redirection: " +
		"HTML is full of quotes and backticks and a heredoc will mangle it.\n\n")
	b.WriteString("When the file is right, stop and say in one sentence what you did. " +
		"Do not publish it and do not create an app — that is done for you " +
		"the moment you finish.")

	// The page goes in the instructions rather than being fetched with a tool,
	// and that is the whole difference between this working and not.
	//
	// Reading it with shell_read puts four kilobytes into the conversation as a
	// tool result, and a model that has just received that is reliably unable
	// to emit the next tool call — it read the file, replied with the file, and
	// changed nothing, every time. The same bytes as context rather than as a
	// tool result leave the model with one small call to make.
	if current != "" {
		b.WriteString("\n\nThis is what the page contains now:\n\n")
		b.WriteString(current)
	}
	return b.String()
}

// result is what a turn produced, for the page to render.
type result struct {
	App   *apps.App
	Said  string   // the model's own account of the turn
	Steps []string // the tools it used, in order, for when nothing appears to happen
}

// run does a turn and hosts what came of it.
func run(accountID, slug, prompt string, existing bool) (*result, error) {
	dir := workdir(slug)

	// What the page is now, so that a turn which changes nothing can say so.
	// Publishing an unchanged file succeeds — correctly, there is a page and it
	// is hosted — and reporting that as a turn would be a lie somebody only
	// catches by noticing their app looks identical.
	var before string
	if existing {
		if a := apps.GetApp(slug); a != nil {
			before = a.HTML
		}
	}

	var steps []string
	said, err := agent.QueryWithOpts(accountID, prompt, agent.QueryOpts{
		System: instructions(dir, existing, before),
		// The machine and nothing else. Everything this turn needs is a file
		// operation, and a model offered a hundred tools spends attention
		// deciding it does not want them.
		Tools: []string{"shell"},
		OnStep: func(s agent.Step) {
			steps = append(steps, s.Tool)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("the build did not finish: %w", err)
	}

	// What it left behind is the app. If it wrote nothing this fails here — and
	// the failure carries what the model did and said, because "no file" on its
	// own tells somebody nothing about why, and the two likely reasons want
	// opposite responses: a run that used no tools at all is a model that did
	// not understand the job, and a run that used them and still left nothing
	// is a model whose tool call did not survive being parsed.
	// The empty name is deliberate: the page names itself from its own <title>,
	// which beats anything derivable from the sentence somebody typed.
	// The prompt goes with it: the version list is the transcript on this page,
	// so a turn that does not say what it was for leaves a blank line in it.
	out, err := apps.PublishDir(context.Background(), accountID, dir, "", slug, prompt)
	if err != nil {
		return nil, fmt.Errorf("nothing to publish from %s (%s): %w",
			dir, did(steps, said), err)
	}

	got := apps.GetApp(out.Slug)
	if existing && got != nil && got.HTML == before {
		return nil, fmt.Errorf("the page is exactly as it was — nothing was changed (%s)",
			did(steps, said))
	}
	return &result{App: got, Said: account(said, steps), Steps: steps}, nil
}

// nameFor is what somebody asked for, with the asking-words taken off:
// "build me a tip calculator" is about a tip calculator, not about Build Me A
// Tip. Only slugFor uses it — an app's display name comes from the page's own
// title, because the first five words of a sentence make a poor name.
func nameFor(prompt, slug string) string {
	words := strings.Fields(strings.ToLower(prompt))
	for len(words) > 0 && filler[words[0]] {
		words = words[1:]
	}
	if len(words) == 0 {
		return strings.ReplaceAll(slug, "-", " ")
	}
	if len(words) > 5 {
		words = words[:5]
	}
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// filler is the words people put in front of what they actually want. Only the
// leading ones are dropped, so "a calculator for a builder" keeps its middle.
var filler = map[string]bool{
	"a": true, "an": true, "the": true, "me": true, "us": true,
	"build": true, "make": true, "create": true, "write": true, "please": true,
	"can": true, "you": true, "i": true, "want": true, "need": true, "would": true,
	"like": true, "new": true, "small": true, "simple": true, "app": true,
}

// slugFor is the address a new app gets, from what was asked for.
//
// Derived rather than asked of the model, because asking costs a whole extra
// round trip to name a thing, and the round trip that names it is the one that
// can fail on its own.
func slugFor(prompt string) string {
	name := nameFor(prompt, "")
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	// The slug rule is 3 characters and up, and a one-word ask can be shorter.
	if len(s) < 3 {
		s = "app-" + s
	}
	return s
}

// account is the model's own description of the turn, or ours when what came
// back is not a description at all.
//
// Some models end a run by emitting another tool call as plain text — the
// literal delimiters, in the reply. It happens most after a large tool result,
// which is exactly when this runs. Whatever the cause, protocol markup is never
// something to show a person: it reads as the page having broken, when in fact
// the change was made and only the sentence about it was lost.
func account(said string, steps []string) string {
	said = strings.TrimSpace(said)
	for _, marker := range []string{"｜DSML｜", "<|tool", "tool_calls", "<invoke name="} {
		if strings.Contains(said, marker) {
			return "Changed, though it did not say how — " + strings.Join(steps, ", ") + "."
		}
	}
	return said
}

// did says what a run got up to, for a failure message.
func did(steps []string, said string) string {
	if len(steps) == 0 {
		if said = strings.TrimSpace(said); said != "" {
			if len(said) > 200 {
				said = said[:200] + "…"
			}
			return "it used no tools and said: " + said
		}
		return "it used no tools and said nothing"
	}
	return "it used " + strings.Join(steps, ", ")
}
