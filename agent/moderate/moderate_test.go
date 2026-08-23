package moderate

// What is asserted here is the wiring, not the model.
//
// The classifier's verdict is a model's opinion and cannot be tested; what
// went wrong before was never the opinion. It was that three services called
// a classifier through a function variable a fourth service filled in, so the
// whole thing was one unrelated package away from silently doing nothing.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No service asks a model what its own answer should be.
//
// This is TestNoNewServiceCallsAnAgent's rule, checked for the one edge it
// could not see: the call was not an import, it was flag.CheckContent, and
// what stood behind it was a function variable. Every layering test in this
// repository reads import statements, and this one was invisible to all of
// them for exactly that reason.
func TestNoServiceClassifiesItsOwnContent(t *testing.T) {
	var found []string

	err := filepath.Walk("../../service", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			// The two halves of the old arrangement: asking for a verdict, and
			// supplying the thing that gives one.
			for _, gone := range []string{"CheckContent(", "SetAnalyzer("} {
				if strings.Contains(line, gone) {
					found = append(found, path+":"+itoa(i+1)+" "+strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(found) > 0 {
		t.Errorf("a service is deciding, or supplying the means to decide, "+
			"whether content is acceptable — that is a model forming an opinion "+
			"and it belongs in agent/moderate. Publish "+
			"event.EventContentPublished instead:\n\t%s", strings.Join(found, "\n\t"))
	}
}

// And moderation does not depend on any particular service having loaded.
//
// It did: internal/flag's analyzer was filled in by service/chat, so if chat
// ever failed to load, every check returned silently and /admin/moderate
// showed an empty list — which reads as a clean community rather than as a
// moderator that was never asked. The subscription is this package's own now,
// and Configured is the honest answer to "is anything being classified".
func TestModerationDoesNotDependOnAnotherService(t *testing.T) {
	b, err := os.ReadFile("moderate.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	if !strings.Contains(src, "event.Subscribe(event.EventContentPublished)") {
		t.Error("nothing subscribes to published content, so nothing is classified")
	}
	// Its own model call, rather than one handed in from outside.
	if !strings.Contains(src, "ai.Ask(") {
		t.Error("the classifier does not call the model itself, which is how it " +
			"came to depend on whoever supplied one")
	}
	// And it says when it cannot.
	if !strings.Contains(src, "func Configured() bool") {
		t.Error("nothing reports whether moderation is possible on this " +
			"instance, so an unmoderated instance looks like a clean one")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
