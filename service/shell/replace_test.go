package shell

import (
	"context"
	"os"
	"strings"
	"testing"

	"mu/internal/container"
	"mu/internal/service"
)

// Changing a file costs a short call, whatever the file's size.
//
// This is the whole reason the method exists. Write takes the complete file, so
// changing one colour in a four-kilobyte page meant emitting four kilobytes as
// a tool argument — and on this instance that is where the model stopped
// producing tool calls and started producing their delimiters as prose. The
// page came back as text, nothing ran, and the file was untouched.
//
// The test is on the shape of the request rather than on a model's behaviour,
// because that is the part this package controls: if Replace ever grows a field
// that takes the whole file, the reason for it existing is gone.
func TestReplaceDoesNotTakeAWholeFile(t *testing.T) {
	b, err := os.ReadFile("replace.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "type ReplaceRequest struct")
	end := strings.Index(src[start:], "}")
	if start < 0 || end < 0 {
		t.Fatal("ReplaceRequest is gone")
	}
	req := src[start : start+end]
	for _, field := range []string{"Content", "Body", "Full", "File "} {
		if strings.Contains(req, field+" string") {
			t.Errorf("ReplaceRequest carries %s, which is the whole file again:\n%s", field, req)
		}
	}
}

// The whole thing, against a real container.
func TestReplaceChangesPartOfAFile(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a container")
	}
	if !Configured() {
		t.Skip("no container runtime on this machine: " + container.Reason())
	}

	const who = "replace-test-account"
	ctx := service.WithAccount(context.Background(), who)
	t.Cleanup(func() { DeleteMachine(who) })
	s := Server{}

	const page = "<style>body{background:#0f172a;color:#eee}</style>\n<h1>Tally</h1>\n<p>#0f172a</p>\n"
	if err := s.Write(ctx, &WriteRequest{Path: "p/index.html", Content: page}, &WriteResponse{}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Text that appears twice is refused rather than guessed at, because the
	// caller cannot see which one it would have got.
	var rr ReplaceResponse
	err := s.Replace(ctx, &ReplaceRequest{Path: "p/index.html", Old: "#0f172a", New: "#ffffff"}, &rr)
	if err == nil {
		t.Fatal("an ambiguous replacement was applied to whichever came first")
	}
	if !strings.Contains(err.Error(), "2 times") {
		t.Errorf("the refusal does not say how many there were, so the caller "+
			"cannot tell what to do about it: %v", err)
	}

	// Named unambiguously, it changes exactly that one.
	if err := s.Replace(ctx, &ReplaceRequest{
		Path: "p/index.html", Old: "background:#0f172a", New: "background:#ffffff",
	}, &rr); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if rr.Changed != 1 {
		t.Errorf("changed %d occurrences, want 1", rr.Changed)
	}

	var got ReadResponse
	if err := s.Read(ctx, &ReadRequest{Path: "p/index.html"}, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(got.Content, "background:#ffffff") {
		t.Errorf("the change did not land:\n%s", got.Content)
	}
	// And the other one is still there, which is the half that says it replaced
	// rather than rewrote.
	if !strings.Contains(got.Content, "<p>#0f172a</p>") {
		t.Errorf("the untouched copy was changed too:\n%s", got.Content)
	}
	if !strings.Contains(got.Content, "<h1>Tally</h1>") {
		t.Errorf("the rest of the file did not survive:\n%s", got.Content)
	}

	// All is asked for by name, and then does every one.
	if err := s.Replace(ctx, &ReplaceRequest{
		Path: "p/index.html", Old: "#0f172a", New: "#123456", All: true,
	}, &rr); err != nil {
		t.Fatalf("replace all: %v", err)
	}
	if err := s.Read(ctx, &ReadRequest{Path: "p/index.html"}, &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Content, "#0f172a") {
		t.Errorf("all did not replace every occurrence:\n%s", got.Content)
	}

	// Text that is not there is a refusal naming what was looked for, rather
	// than a silent no-op that reads as success.
	if err := s.Replace(ctx, &ReplaceRequest{
		Path: "p/index.html", Old: "not in the file", New: "x",
	}, &rr); err == nil {
		t.Error("replacing text that is absent succeeded, so a failed edit looks like a change")
	}
}
