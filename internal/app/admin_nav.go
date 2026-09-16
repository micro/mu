package app

// Admin pages share one way back rather than repeating the entire menu.
func adminNavigation(path string) string {
	if path == "/admin" {
		return ""
	}
	return `<nav class="admin-navigation" aria-label="Admin"><a href="/admin">Back to Admin</a></nav>`
}
