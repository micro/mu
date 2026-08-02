package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The landing has to lead somewhere. Someone who reads "tools for agents" and
// is convinced needs a next move, and every one of these is a step on that
// path: see it work, see what's there, connect.
func TestLandingLeadsToTheTools(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	for _, want := range []string{`href="/tools"`, `href="/mcp"`, `href="/agent"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing has no path to %s", want)
		}
	}

	// "See it working" means watching an agent call tools, not a page of app
	// cards — the cards do not show the thing the landing just promised.
	if strings.Contains(body, `href="/home">See it working`) {
		t.Error(`"See it working" points at the home screen, which shows cards rather than tool-calling`)
	}
}
