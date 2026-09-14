package app

import (
	"strings"
	"testing"
)

func TestPageTitlesNameTheDestination(t *testing.T) {
	for _, title := range []string{"Home", "Account", "Agents"} {
		page := renderShell("en", title, "", "", "", nil, "/", "/")
		if !strings.Contains(page, "<title>"+title+" | Micro</title>") {
			t.Errorf("missing destination title for %s", title)
		}
	}
	page := renderShell("en", "Micro", "", "", "", nil, "/", "/")
	if strings.Contains(page, "Micro | Micro") {
		t.Error("duplicate branding")
	}
}
