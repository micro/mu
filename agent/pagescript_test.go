package agent

import (
	"regexp"
	"strings"
	"testing"
)

// Every function the conversation page calls, it also defines.
//
// renderAgentsPanel carried muAgentCsrf and window.muSeedAgent and was emitted
// only when the page was not about a named agent — while the page called both
// regardless. On /agent/<name> that threw, which silently sent every question
// to the default agent and made the delete cross do nothing.
func TestTheChatPageDefinesWhatItCalls(t *testing.T) {
	page := chatPageJS + paneJS + sessionDeleteJS("/agent")
	calls := regexp.MustCompile(`\bmu[A-Z][A-Za-z]*\(`)
	for _, m := range calls.FindAllString(page, -1) {
		fn := strings.TrimSuffix(m, "(")
		if !strings.Contains(page, "function "+fn) &&
			!strings.Contains(page, "window."+fn+"=") &&
			!strings.Contains(page, "typeof window."+fn) {
			t.Errorf("%s is called but never defined in the page's own scripts", fn)
		}
	}
}
