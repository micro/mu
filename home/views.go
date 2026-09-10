package home

import (
	"fmt"
	"html"
	"strings"

	"mu/internal/auth"
)

func panelHidden(hidden bool) string {
	if hidden {
		return ` hidden`
	}
	return ""
}

func homeViews(feed bool) string {
	homeTab, feedTab := 0, -1
	if feed {
		homeTab, feedTab = -1, 0
	}
	return fmt.Sprintf(`<nav class="view-switch" role="tablist" aria-label="Home views">
<a id="home-view-personal" href="/home" role="tab" aria-controls="home-personal" aria-selected="%t" tabindex="%d">Home</a>
<a id="home-view-feed" href="/home?view=feed" role="tab" aria-controls="home-feed" aria-selected="%t" tabindex="%d">Feed</a>
</nav>`, !feed, homeTab, feed, feedTab)
}

// greeting avoids using the server timezone for a person's local time of day.
func greeting(acc *auth.Account) string {
	if acc != nil {
		if name := strings.TrimSpace(acc.Name); name != "" {
			return "Welcome back, " + html.EscapeString(name)
		}
	}
	return "Welcome back"
}
