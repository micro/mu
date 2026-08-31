package apps

import (
	"os"
	"strings"
	"testing"
)

// Describing an app happens in one place.
//
// There were two boxes that each claimed to build an app from a sentence, and
// only one of them could iterate. The other is gone; this fails if it comes
// back, because two front doors to the same act is how one of them quietly
// stops being the one that works.
func TestThereIsOneBoxThatBuildsAnAppFromASentence(t *testing.T) {
	b, err := os.ReadFile("apps.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if strings.Contains(src, `action="/apps/generate"`) {
		t.Error("the apps page still posts a description straight to /apps/generate. " +
			"Describing an app is /code, which checks the result and lets you say " +
			"what to change; a second box that asks once is the one that will rot.")
	}
	if !strings.Contains(src, `"/code"`) {
		t.Error("nothing on the apps page leads to /code, so the page that writes " +
			"apps is unreachable from the page that lists them")
	}
}

// The web button and the agent tool build with the same builder.
//
// They did not. BuildApp — the one that writes a document and runs the scanner
// and the tests over it until they pass — was wired to the agent tool alone,
// and the button a person clicked still went to the three-shape picker that
// build.go's own package comment describes as the thing it replaced. So "build
// me a unit converter" gave an agent a unit converter and gave a person a
// checklist called Unit Converter.
func TestEveryDoorBuildsWithTheSameBuilder(t *testing.T) {
	// code.go is not in this list any more, and its absence is the point: /code
	// moved out of this package and stopped calling BuildApp at all. It runs an
	// agent on a machine and publishes the file that agent leaves, which is a
	// better answer than a single reply checked afterwards — see code/turn.go.
	// What remains here are the doors that do ask for a document in one go.
	for _, f := range []string{"micro_build.go", "service.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "BuildApp(") {
			t.Errorf("%s builds an app without going through BuildApp", f)
		}
		// BuildMicroApp is the floor BuildApp falls back to, and calling it
		// directly is choosing the floor.
		if strings.Contains(src, "= BuildMicroApp(") {
			t.Errorf("%s calls BuildMicroApp directly, which picks one of three "+
				"shapes instead of writing the app", f)
		}
	}
}
