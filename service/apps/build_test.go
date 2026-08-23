package apps

// Reading back what the model sent, and knowing what is wrong with it.
//
// The model call is not exercised — that needs a model. What is worth holding
// is the two halves either side of it: the answer is taken apart correctly even
// when it arrives untidily, and a document with something wrong with it is
// described in words that can be handed back.

import (
	"strings"
	"testing"
)

func TestTheMetadataLineComesOffTheFront(t *testing.T) {
	raw := "<!-- mu {\"title\":\"Unit Converter\",\"emoji\":\"📐\",\"tags\":\"tools,convert\"} -->\n" +
		"<!doctype html><html><head><title>Converter</title></head><body></body></html>"

	got := splitHeader(raw, "a unit converter")
	if got.Title != "Unit Converter" || got.Emoji != "📐" || got.Tags != "tools,convert" {
		t.Errorf("the header was not read: %+v", got)
	}
	if strings.Contains(got.HTML, "<!-- mu") {
		t.Errorf("the header stayed in the document: %s", got.HTML)
	}
	if !strings.HasPrefix(got.HTML, "<!doctype html>") {
		t.Errorf("the document does not start where it should: %s", got.HTML)
	}
}

// A model that wraps the whole answer in a fence, or forgets the header, still
// gives an app — losing the title is a cosmetic fault and losing the document
// is not.
func TestAnUntidyAnswerStillYieldsAnApp(t *testing.T) {
	fenced := "```html\n<!doctype html><html><head><title>Stopwatch</title></head>" +
		"<body><script>let t=0</script></body></html>\n```"
	got := splitHeader(fenced, "a stopwatch")
	if strings.Contains(got.HTML, "```") {
		t.Errorf("the fence stayed: %s", got.HTML)
	}
	if got.Title != "Stopwatch" {
		t.Errorf("the title was not taken from <title>: %q", got.Title)
	}
	if got.Emoji == "" {
		t.Error("an app with no emoji got no fallback")
	}

	// Neither header nor title: named from what was asked for, rather than
	// saved as "App".
	bare := splitHeader("<!doctype html><html><body>hi</body></html>", "a tip calculator. Split by people.")
	if bare.Title != "A tip calculator" {
		t.Errorf("the description did not become a name: %q", bare.Title)
	}
}

// The problems are the whole point of the loop, so they have to be found and
// have to read as instructions.
func TestAFaultyDocumentIsDescribed(t *testing.T) {
	// Short, no script, and reaching for a cookie: one structural fault and one
	// the scanner exists for.
	problems := buildProblems(`<html><script>document.cookie</script></html>`, "nobody")
	if len(problems) == 0 {
		t.Fatal("a document with document.cookie in it had nothing said about it")
	}
	joined := strings.ToLower(strings.Join(problems, " "))
	if !strings.Contains(joined, "cookie") {
		t.Errorf("the scanner did not run: %v", problems)
	}

	// ScanApp had no caller anywhere in the tree before this. A scanner nothing
	// runs reads as protection and is not any.
	if len(ScanApp(`<script>document.cookie</script>`)) == 0 {
		t.Error("ScanApp no longer objects to document.cookie")
	}
}

// And an ordinary app passes, or the loop never terminates and every build
// falls back to a checklist.
func TestAnOrdinaryDocumentHasNoProblems(t *testing.T) {
	const good = `<!doctype html><html><head><meta charset="utf-8"><title>Stopwatch</title>
<style>body{font-family:system-ui;padding:16px}</style></head>
<body><p id="t">0.0</p><button id="go">Start</button>
<script>let n=0,h=null;document.getElementById('go').onclick=function(){
if(h){clearInterval(h);h=null;return}h=setInterval(function(){n+=0.1;
document.getElementById('t').textContent=n.toFixed(1)},100)}</script></body></html>`

	if problems := buildProblems(good, "nobody"); len(problems) != 0 {
		t.Errorf("a working app was rejected: %v", problems)
	}
}
