// Username validation — blocks obscene, offensive, and impersonation names.
package auth

import "strings"

// bannedWords are substrings that are never allowed in usernames.
// Checked case-insensitively. Keep this list tight — it's not a general
// profanity filter, just the words that have no legitimate use in a
// username.
var bannedWords = []string{
	"penis", "cock", "dick", "boob", "tits", "pussy", "vagina",
	"fuck", "shit", "cunt", "bitch", "whore", "slut", "nigger",
	"nigga", "faggot", "retard", "porn", "hentai", "femboy",
	"nazi", "hitler", "jihad",
}

// ValidateUsername returns an error string if the username is not
// allowed, or "" if it's fine. Called from both web signup and MCP
// signup. This is in addition to the regex format check — a username
// can be well-formed but still banned.
func ValidateUsername(username string) string {
	lower := strings.ToLower(username)
	for _, w := range bannedWords {
		if strings.Contains(lower, w) {
			return "That username is not allowed."
		}
	}
	// Block impersonation of system accounts.
	if lower == "admin" || lower == "system" || lower == "root" ||
		lower == "moderator" || lower == "support" {
		return "That username is reserved."
	}
	return ""
}
