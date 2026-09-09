package home

// What closing the sidebar does to a page.
//
// Three properties, all of them CSS, all of them invisible to every other test
// here because the markup does not change: what the browser draws does.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func styles(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Closing the sidebar gives a page more room, never less.
//
// It gave less. Collapsed capped #content at 1080px while open it is 1400, or
// 1700 on Home — so on any screen past about 1300 the page narrowed as the rail
// went away. Measured at 1728: the news card was 692px wide with the rail open
// and 272 with it closed, because the 320px right-hand track kept its width and
// the whole loss landed on the column beside it.
//
// The rule that did it is the thing to keep out. A max-width on the collapsed
// content box can only ever be a number smaller than the open one, which is the
// bug however it is spelled.
func TestClosingTheSidebarDoesNotNarrowThePage(t *testing.T) {
	css := styles(t)
	rule := regexp.MustCompile(`body\.nav-collapsed[^{}]*#content\s*\{[^}]*\}`)
	for _, block := range rule.FindAllString(css, -1) {
		if strings.Contains(block, "max-width") {
			t.Errorf("the collapsed content box is capped again, which makes a "+
				"page narrower with the rail closed than open:\n%s", block)
		}
	}
}

// A page column inherits the shell width whether the sidebar is open or closed.
func TestPageColumnsUseTheSharedFrame(t *testing.T) {
	css := styles(t)
	if !regexp.MustCompile(`\.page-col\s*\{[^}]*max-width:\s*var\(--page-width\)`).MatchString(css) {
		t.Error("page columns do not use the shared page width")
	}
	if strings.Contains(css, "#content:has(.page-col) #page-title") {
		t.Error("a page type independently moves its title")
	}
}

// Headlines form one readable column with their descriptions.
func TestTheNewsCardIsAGlance(t *testing.T) {
	css := styles(t)
	descriptions := regexp.MustCompile(`[^{}]*\.headline[^{}]*\.description[^{}]*\{[^}]*\}`).FindAllString(css, -1)
	hidden := regexp.MustCompile(`(?i)\bdisplay\s*:\s*none\s*(!\s*important\s*)?(;|\})`)
	if len(descriptions) == 0 {
		t.Fatal("no headline description styles were checked")
	}
	visible := regexp.MustCompile(`#home #news \.headline \.description\s*\{\s*display:\s*block;\s*\}`)
	if !visible.MatchString(css) {
		t.Error("Home must explicitly display headline descriptions")
	}
	for _, rule := range descriptions {
		if hidden.MatchString(rule) {
			t.Error("the Home news card hides descriptions")
		}
	}
	section := regexp.MustCompile(`#home #news \.section\s*\{[^}]*\}`).FindString(css)
	if !strings.Contains(section, "display: block") {
		t.Error("Home headlines must use one column")
	}
}
