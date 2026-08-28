// Package eval measures whether the Code agent can actually build something.
//
// # Why this is not an ordinary test
//
// Everything else in this tree asserts on code that behaves the same way twice.
// This one asks a model to write a program and then judges the program, so it
// is a measurement rather than a check: it costs money and minutes, it will not
// give the same answer twice, and a bad score is a fact about a model on a day
// rather than a broken build. It runs only when MU_EVAL is set, and it fails
// only when nothing worked at all — a run that produces three good pages out of
// four is information, not a regression.
//
// # What it marks, and why it is the filesystem
//
// Never the model's prose. What comes back in the reply is the least reliable
// and least important part of a turn: the deliverable is a file on a machine,
// and the question is whether that file is any good.
//
// The marking scheme is deliberately the checks this codebase already trusts —
// ScanApp for what an app may not do, and TestHTML, which runs the page's own
// mu. calls server-side. Those were built as a repair loop for a model that
// wrote blind; here they are the oracle, which is the same judgement asked in
// the other direction.
//
// Two things are added because passing a scanner is not the same as working.
// Self-containment, because a page that fetches a framework off a CDN is not
// the single self-contained page it was asked for and will be broken the day
// that host is unreachable. And functional use — the page is opened in a real
// browser, its inputs are filled the way somebody would fill them, and what it
// then says is what gets marked. A tip calculator that renders beautifully and
// computes nothing passes every static check there is.
//
// # What it found, the first time it ran
//
// Zero of eight, and the cause is not the model. go-micro's atlascloud
// provider runs one round of tool calls and then asks the model to finish
// without them: the follow-up request carries the tool results and no "tools"
// key, and there is no loop, so a second call is impossible whatever the model
// intends. ai/anthropic does it correctly — tools on the follow-up, repeated
// while calls keep coming — which is why the same agent behaves differently
// depending on which key is set.
//
// It is worth knowing what that failure looks like from here, because it looks
// like something else entirely. Asked to carry on with tools it can no longer
// call, a model writes the call it wanted as ordinary text, and the reply
// arrives full of tool-call delimiters. That reads as a model too weak to
// format a tool call. It is a model being offered no way to make one, and a
// day was spent on the wrong explanation before this test was written.
//
// # What a failing score means
//
// The first job of this is comparative rather than absolute. Run it against one
// provider, then another, and the difference is a number instead of an
// anecdote — which is the thing that settles whether a bad turn is the model,
// the serving stack, or the way we are asking. Every produced file is written
// to MU_EVAL_ARTIFACTS (or a temp dir, named in the output) so a person can
// read the code the checks passed or failed on: a score nobody can look behind
// is worth very little.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mu/agent"
	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/auth"
	"mu/internal/container"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/shell"
)

// account is who the eval runs as. Its own, so a real one is never the thing
// with eval directories left in it.
const account = "evalcode"

// runs is how many times each task is asked.
//
// More than one because a single sample of a stochastic system says almost
// nothing — and not many more, because each one is a model call and a
// container. Two is enough to tell "it does this" from "it did this once",
// which is the distinction that matters when comparing providers.
const runs = 2

// A task is something to ask for and how to tell whether what came back is any
// good. Each one builds on the directory the previous left behind, because that
// is what using this actually looks like: build a thing, then change it.
type task struct {
	name   string
	prompt string
	// fill is what to type into the page's inputs, in order.
	fill []float64
	// wants are substrings the page must show once filled — the arithmetic,
	// not the markup.
	wants []string
	// keeps, for a task that changes an existing page: the fraction of the
	// previous version's lines that must still be there. A change that rewrites
	// the page from scratch has not made the change, it has made a new page.
	keeps float64
	// shell, for a task that is not a page at all: a command whose output says
	// whether it worked.
	shell string
	want  string
}

var tasks = []task{
	{
		name:   "build",
		prompt: "Build a tip calculator: a field for the bill amount, a field for the tip percentage, and it shows the tip and the total.",
		fill:   []float64{100, 20},
		wants:  []string{"20", "120"},
	},
	{
		name:   "restyle",
		prompt: "Change it to a white background with dark text.",
		fill:   []float64{100, 20},
		wants:  []string{"20", "120"},
		keeps:  0.6,
	},
	{
		name:   "extend",
		prompt: "Add a field for the number of people sharing the bill, and show what each person pays.",
		fill:   []float64{100, 20, 4},
		wants:  []string{"120", "30"},
		keeps:  0.5,
	},
	{
		name:   "script",
		prompt: "In the directory eval-files, create three files a.txt, b.txt and c.txt, then write a shell script that renames every .txt in that directory to .md, and run it.",
		shell:  "ls eval-files",
		want:   "a.md b.md c.md",
	},
}

func TestTheCodeAgentBuildsSomethingThatWorks(t *testing.T) {
	if os.Getenv("MU_EVAL") == "" {
		t.Skip("an eval costs a model call and a container per run: set MU_EVAL=1")
	}
	if !ai.Configured() {
		t.Skip("no model configured")
	}
	if !shell.Configured() {
		t.Skip("no container runtime: " + container.Reason())
	}
	if micro.Get("code") == nil {
		t.Fatal("there is no Code agent registered, so there is nothing to measure")
	}

	// The services the agent is scoped to, registered.
	//
	// Importing a service package does not register it — Load does, and only
	// the server calls Load. So this measured an agent holding no tools at all,
	// which is why every failure read as the model calling something that does
	// not exist: nothing existed. Two runs and a prompt rewrite were spent on
	// that before the mapping test skipped for want of any service and said so.
	shell.Load()
	apps.Load()
	if len(service.Specs()) == 0 {
		t.Fatal("no services registered, so the agent under test has no tools " +
			"and the score would measure the harness")
	}

	if err := auth.Create(&auth.Account{ID: account, Name: account, Secret: "eval-only"}); err != nil &&
		!strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	t.Cleanup(func() { shell.DeleteMachine(account) })

	artifacts := os.Getenv("MU_EVAL_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Logf("artifacts: %s", artifacts)

	var passed, total int
	for run := 1; run <= runs; run++ {
		dir := fmt.Sprintf("eval/run%d", run)
		var previous string

		for _, tk := range tasks {
			total++
			res := attempt(t, tk, dir, previous, artifacts, run)
			if res.html != "" {
				previous = res.html
			}
			if res.ok {
				passed++
			}
			t.Logf("run %d · %-8s %s", run, tk.name, res.line())
		}
	}

	t.Logf("─── %d of %d tasks passed every check ───", passed, total)
	if passed == 0 {
		t.Errorf("nothing passed: the agent produced nothing usable in %d attempts, "+
			"which is a broken pipeline rather than a weak model", total)
	}
}

// outcome is one task attempt: what each check said, and the file it judged.
type outcome struct {
	ok     bool
	html   string
	said   string
	checks []string
}

func (o outcome) line() string { return strings.Join(o.checks, "  ") }

func (o *outcome) mark(name string, ok bool, why string) {
	switch {
	case ok:
		o.checks = append(o.checks, "✓"+name)
	default:
		o.ok = false
		o.checks = append(o.checks, "✗"+name+"("+why+")")
	}
}

func (o *outcome) skip(name, why string) {
	o.checks = append(o.checks, "–"+name+"("+why+")")
}

func attempt(t *testing.T, tk task, dir, previous, artifacts string, run int) outcome {
	t.Helper()
	out := outcome{ok: true}

	prompt := tk.prompt
	if tk.shell == "" {
		prompt = fmt.Sprintf("%s Work in the directory %s — the page is %s/index.html.", tk.prompt, dir, dir)
	}

	// The real agent, with its registered prompt and scope, so this measures
	// what an account actually talks to rather than a copy of it.
	opts := agent.PlatformOpts(micro.Get("code"))
	said, err := agent.QueryWithOpts(account, prompt, opts)
	if err != nil {
		out.mark("ran", false, err.Error())
		return out
	}
	out.mark("ran", true, "")
	// Kept for the failure line. A run that produces no file and no explanation
	// is the least useful result there is: the reply is the only evidence of
	// what the model thought it was doing, and discarding it means every
	// failure looks identical from out here.
	out.said = said

	// A task about the shell is judged by what is on the machine.
	if tk.shell != "" {
		got := box(t, tk.shell)
		out.mark("did", strings.Contains(squash(got), tk.want),
			squash(got)+"; it said: "+squash(out.said))
		return out
	}

	html := box(t, "cat "+dir+"/index.html")
	if strings.TrimSpace(html) == "" || strings.Contains(html, "No such file") {
		out.mark("wrote", false, "no file; it said: "+squash(out.said))
		return out
	}
	out.mark("wrote", true, "")
	out.html = html

	name := fmt.Sprintf("run%d-%s.html", run, tk.name)
	if err := os.WriteFile(filepath.Join(artifacts, name), []byte(html), 0o644); err != nil {
		t.Logf("could not save %s: %v", name, err)
	}

	if issues := apps.ScanApp(html); len(issues) > 0 {
		out.mark("safe", false, strings.Join(issues, "; "))
	} else {
		out.mark("safe", true, "")
	}

	if res := apps.TestHTML(html, account); res != nil && !res.OK {
		out.mark("sound", false, strings.Join(res.Issues, "; "))
	} else {
		out.mark("sound", true, "")
	}

	if bad := external(html); bad != "" {
		out.mark("alone", false, bad)
	} else {
		out.mark("alone", true, "")
	}

	// A change has to leave the rest of the page alone. Rewriting it from
	// scratch is not the change that was asked for, even when the result looks
	// right — everything unmentioned has been silently redecided.
	if tk.keeps > 0 && previous != "" {
		kept := overlap(previous, html)
		out.mark("kept", kept >= tk.keeps, fmt.Sprintf("%.0f%% of the old page", kept*100))
	}

	// And does it work.
	text, errs, ok := usePage(t, filepath.Join(artifacts, name), tk.fill)
	switch {
	case !ok:
		out.skip("works", "no browser: set MU_EVAL_BROWSER")
	case len(errs) > 0:
		out.mark("works", false, "js error: "+squash(errs[0]))
	default:
		var missing []string
		for _, w := range tk.wants {
			if !strings.Contains(text, w) {
				missing = append(missing, w)
			}
		}
		out.mark("works", len(missing) == 0, "no "+strings.Join(missing, ",")+" in: "+squash(text))
	}
	return out
}

// box runs a command on the eval account's machine and returns what it wrote.
func box(t *testing.T, command string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(service.WithAccount(context.Background(), account), 2*time.Minute)
	defer cancel()
	var r shell.RunResponse
	if err := (shell.Server{}).Run(ctx, &shell.RunRequest{Command: command}, &r); err != nil {
		return "error: " + err.Error()
	}
	return r.Output
}

// external finds the thing a self-contained page must not do.
func external(html string) string {
	lower := strings.ToLower(html)
	for _, attr := range []string{`src="http`, `src='http`, `href="http`, `href='http`} {
		if i := strings.Index(lower, attr); i >= 0 {
			// A link to a page is fine; fetching code or styling is not.
			end := i + 120
			if end > len(html) {
				end = len(html)
			}
			frag := html[i:end]
			if strings.HasPrefix(attr, "src") || strings.Contains(strings.ToLower(frag), "stylesheet") {
				return squash(frag)
			}
		}
	}
	return ""
}

// overlap is how much of the old page is still in the new one, by line.
func overlap(old, new string) float64 {
	have := map[string]bool{}
	for _, l := range strings.Split(new, "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var kept, count int
	for _, l := range strings.Split(old, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		count++
		if have[l] {
			kept++
		}
	}
	if count == 0 {
		return 1
	}
	return float64(kept) / float64(count)
}

// usePage opens the page in a browser, fills it in and reports what it says.
//
// ok is false when there is no browser to do it with, which is a skipped check
// rather than a failed one — and it is reported as skipped, because a score
// that quietly stops measuring the most important thing is worse than one that
// admits it did not.
func usePage(t *testing.T, path string, fill []float64) (text string, errs []string, ok bool) {
	t.Helper()
	root := os.Getenv("MU_EVAL_BROWSER")
	if root == "" {
		return "", nil, false
	}
	values, _ := json.Marshal(fill)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", "browser.js", path, string(values))
	cmd.Env = append(os.Environ(), "MU_EVAL_BROWSER="+root)
	out, err := cmd.Output()
	if err != nil {
		return "", []string{"harness: " + err.Error()}, true
	}
	var got struct {
		Text   string   `json:"text"`
		Errors []string `json:"errors"`
	}
	if jerr := json.Unmarshal(out, &got); jerr != nil {
		return "", []string{"harness: " + jerr.Error()}, true
	}
	return got.Text, got.Errors, true
}

func squash(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 140 {
		s = s[:140] + "…"
	}
	return s
}
