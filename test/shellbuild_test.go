package test

// Can an agent build an app in the shell and have it hosted, with what is
// already here?
//
// The question is worth asking before anything is built for it, because the
// answer decides how much there is to build. /code today asks a model to emit a
// whole HTML document, runs a scanner and a checker over what comes back, and
// feeds the complaints into the next prompt. That is a hand-rolled agent loop
// whose two tools the model cannot see or call. Models are good at files and
// commands; they are worse at re-emitting a document because something
// complained about it.
//
// Everything for the other shape exists. service/shell hands a caller a
// container with a persistent /work and four methods a model understands —
// Run, Write, Read, List. service/apps hosts HTML. This test is the flow an
// agent would drive between them, with no new code: write a file in the box,
// change it with a command, read it back, host it, fetch it.
//
// It is deliberately not a test of a model's judgement. What it pins down is
// whether the mechanism underneath is there, so that "the model decides to do
// this" is the only remaining unknown.
//
// # What it says about the seam
//
// The step that reads the file out and passes it to Create is the one to look
// at. It works here, and it means the document travels back through the model's
// context to get from the box to the store — the model writes the file, then
// says the whole file again as an argument. That is the token cost of writing
// it twice, and it is the thing a Publish that takes a path would remove.
//
// # What a real model did with it
//
// Run against a live instance (Atlas Cloud, DeepSeek) the halves came apart in
// a way worth writing down, because it changes what Publish is for.
//
// Given the shell, the model wrote a good page: asked for a tip calculator it
// produced five kilobytes of complete, styled HTML into /work/tip/index.html on
// the first call. That is the argument for the box, confirmed — nobody had to
// tell it what a tip calculator looks like or repair what came back.
//
// What failed was passing that document *through* the model a second time.
// Reading the file back and handing it to apps_create made the model emit its
// own tool-call markup — the ｜DSML｜ delimiters — as plain text, so no call was
// parsed and no app was created. The same request with a 47-byte page worked
// perfectly, and two small calls in one turn work perfectly, so it is the size
// of what is being carried rather than the number of steps.
//
// apps_publish is the answer to the crossing that was redundant: it takes the
// directory, reads the file inside the process, and hosts it. Both halves are
// verified against a live instance — the page serves, publishing the same
// directory again replaces the app rather than refusing, and a page reaching
// for document.cookie is refused with the reason.
//
// It is worth being exact about what that does not fix. The document still has
// to get into the box, and the only way in is a tool call carrying all of it.
// One crossing is irreducible; the second one was ours, and it is gone.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/container"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/shell"
)

// The starting page, as a model would first write it: wrong on purpose in a way
// a command can fix, so the run in the middle has something to do.
const draft = `<!doctype html>
<title>Tally</title>
<h1>Tally</h1>
<p id="n">0</p>
<button onclick="document.getElementById('n').textContent=++c">PLACEHOLDER</button>
<script>let c=0</script>
`

func TestAnAgentCanBuildAnAppInTheShellAndHostIt(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !shell.Configured() {
		t.Skip("no container runtime on this machine: " + container.Reason())
	}

	// Apps persist under $HOME/.mu, so this gets a home of its own rather than
	// leaving a Tally in the one somebody is using.
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	// An app has an author, so there has to be one.
	const who = "shellbuilder"
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "not-a-real-secret"}); err != nil &&
		!strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}

	ctx := service.WithAccount(context.Background(), who)
	t.Cleanup(func() { shell.DeleteMachine(who) })
	box := shell.Server{}

	// 1. The model writes the page. Write rather than a heredoc, for the reason
	// the method exists: the content is a JSON string and never meets a shell.
	var w shell.WriteResponse
	if err := box.Write(ctx, &shell.WriteRequest{Path: "tally/index.html", Content: draft}, &w); err != nil {
		t.Fatalf("write: %v", err)
	}

	// 2. It changes its mind about one word, and does it the way anybody with a
	// shell would: a command, not a rewrite of the document. This is the whole
	// argument for the box — the edit costs a line instead of a page.
	var r shell.RunResponse
	err := box.Run(ctx, &shell.RunRequest{
		Command: `sed -i 's/PLACEHOLDER/Add one/' index.html && grep -c 'Add one' index.html`,
		Dir:     "tally",
	}, &r)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Code != 0 {
		t.Fatalf("the edit failed in the box (exit %d): %s", r.Code, r.Output)
	}

	// 3. It reads back what it now has. Through Read rather than Run, so that a
	// bug in either door shows up instead of cancelling out.
	var rd shell.ReadResponse
	if err := box.Read(ctx, &shell.ReadRequest{Path: "tally/index.html"}, &rd); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(rd.Content, "Add one") || strings.Contains(rd.Content, "PLACEHOLDER") {
		t.Fatalf("the box did not keep the edit:\n%s", rd.Content)
	}

	// 4. And hosts it. Today this is the model passing the document it just read
	// as an argument — see the note at the top about what that costs.
	app, err := apps.CreateApp(who, "Tally", "shellbuild-tally", "a tally counter", "", rd.Content, "", 0, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if app.Slug == "" {
		t.Fatal("the app was created without a slug, so there is nothing to open")
	}

	// 5. The page is served, and it is the page that was built in the box.
	req := httptest.NewRequest("GET", "/apps/"+app.Slug+"?raw=1", nil)
	rec := httptest.NewRecorder()
	apps.Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the hosted app returned %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Add one") {
		t.Errorf("what is served is not what was built in the box:\n%s", body)
	}
}
