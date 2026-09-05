package app

import "strings"

// The browser shell is Mu because Mu is the runtime, but the installable app is
// Micro: the personal assistant a person adds to their home screen. The web app
// manifest already says Micro; keep the HTML fallbacks in the same identity so
// Safari/iOS and browsers that consult application-name do not install it as Mu.
func init() {
	Template = strings.Replace(Template,
		`<meta name="apple-mobile-web-app-title" content="Mu">`,
		`<meta name="apple-mobile-web-app-title" content="Micro">`, 1)
	Template = strings.Replace(Template,
		`<meta name="application-name" content="Mu">`,
		`<meta name="application-name" content="Micro">`, 1)
}
