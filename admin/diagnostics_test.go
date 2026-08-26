package admin

import (
	"strings"
	"testing"
)

// An error naming an XML element survives being rendered.
//
// The federation check reported `starttls refused: <required>` and the page
// showed "starttls refused:" with nothing after it — the element name, which
// was the entire diagnosis, parsed as a tag and vanished. A page that deletes
// the informative half of an error is worse than one that shows nothing,
// because it reads as the error being empty.
func TestADetailWithAngleBracketsIsNotEatenByTheBrowser(t *testing.T) {
	const detail = "starttls refused: <required>"
	out := renderChecks([]healthCheck{{Name: "Federation", Status: "error", Detail: detail}})
	if !strings.Contains(out, "&lt;required&gt;") {
		t.Errorf("the element name is not escaped, so a browser will swallow it:\n%s", out)
	}
	if strings.Contains(out, "<required>") {
		t.Error("the detail went out as raw markup")
	}
}
