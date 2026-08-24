package app

import (
	"regexp"
	"strings"
	"testing"
)

// A JavaScript string inside an inline handler must not end the attribute.
//
// JSString quotes with double quotes, which is right inside a <script> and
// wrong inside onclick="…": the attribute ends at the first one, so the handler
// becomes a fragment. The browser says "Unexpected end of input" and names
// neither the element nor the value, and the control simply does nothing.
//
// That was the delete cross on a conversation and the + New button, both for as
// long as they had existed.
func TestAJSAttrDoesNotEndTheAttribute(t *testing.T) {
	for _, id := range []string{"fe3918b6a3903b29f4d09ba1", "/agent/micro", `a"b`, "it's"} {
		attr := `onclick="f(` + JSAttr(id) + `,event)"`
		// The value between the outer quotes is everything up to the next one.
		// If the helper leaked a quote, that value is a fragment.
		inner := regexp.MustCompile(`onclick="([^"]*)"`).FindStringSubmatch(attr)
		if inner == nil {
			t.Fatalf("no attribute at all for %q: %s", id, attr)
		}
		if !strings.HasSuffix(inner[1], ",event)") {
			t.Errorf("the handler for %q was cut short: %q", id, inner[1])
		}
		if strings.Count(attr, `"`) != 2 {
			t.Errorf("%q leaked a quote into the attribute: %s", id, attr)
		}
	}
}

// And the plain one still belongs in a script, quotes and all.
func TestJSStringIsStillAScriptLiteral(t *testing.T) {
	if got := JSString("abc"); got != `"abc"` {
		t.Errorf("JSString(abc) = %s, want \"abc\"", got)
	}
}
