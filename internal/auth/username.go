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
// signup, so it enforces the shared username format as well as the
// blocklist and reserved names.
func ValidateUsername(username string) string {
	if !validUsernameFormat(username) {
		return "Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores"
	}

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

func validUsernameFormat(username string) bool {
	if len(username) < 4 || len(username) > 24 {
		return false
	}
	for i, r := range username {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return false
			}
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}
