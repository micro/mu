// Username validation — format, reserved names, and the words a username may
// not be made of.
package auth

import "strings"

// The bounds. A username is an address local part, a page at /@name and a
// mention, so it has to be long enough to be a name and short enough to be
// typed. These are the numbers the format check has always used; they are
// named here because three places wanted to say them.
const (
	MinUsername = 4
	MaxUsername = 24
)

// bannedAnywhere is refused as a substring, wherever it appears.
//
// The list is short on purpose. A substring match cannot tell "dickson" from
// "dick", so the only words that belong here are the ones whose innocent
// occurrence in a name is rare enough to be worth the cost of refusing it.
// Everything more collision-prone goes in bannedAsWord below.
//
// "porn" is the one entry here with a real cost: Porn is an ordinary element
// of Thai given names — Pornchai, Supaporn — and this refuses all of them. It
// stays because pornbot and pornhub are the commoner case by a wide margin on
// an open signup, and because a person turned away can pick another name while
// a squatted one has to be noticed. That is a judgement, not a fact, and it is
// the entry to revisit first if somebody is wrongly refused.
var bannedAnywhere = []string{
	"nigger", "nigga", "faggot", "tranny", "wetback",
	"paedo", "pedophile", "porn",
}

// bannedAsWord is refused as a word rather than as a substring.
//
// Every one of these appears inside a name somebody is really called, or
// inside an ordinary word: Dickson and Dickens and Hancock and Woodcock,
// "spice" and "spicy" for spic, "chink" for a gap in a wall, "raccoon" for
// coon. Matching them as substrings — which this file did until now — rejects
// real people to catch a case that a determined person routes around anyway.
//
// The trade this makes is explicit: "dick_head" and "dick123" are refused,
// "dickson" is allowed, and "bigdick" gets through. Catching that last one
// costs Dickson his username, and he is the likelier caller.
var bannedAsWord = []string{
	"penis", "cock", "dick", "boob", "tits", "pussy", "vagina",
	"fuck", "shit", "cunt", "bitch", "whore", "slut",
	"hentai", "femboy", "retard", "spic", "chink", "kike", "gook",
	"coon", "nazi", "hitler", "jihad", "rape", "rapist",
}

// reserved are names this instance answers to itself.
//
// Two kinds. The first is authority — a username called admin or moderator is
// a claim about who you are, made to every other person here, and it is free.
//
// The second is delivery, and it is the sharper one: a username becomes a
// mailbox. agent@ is how an agent answers mail and no-reply@ is where event
// invites come from, so registering either takes delivery of replies meant for
// the instance — including everything anybody sends back to their own agent.
// The rest are the well-known addresses from RFC 2142, which an operator has
// to be able to receive at to run a domain at all.
var reserved = []string{
	"admin", "administrator", "system", "root", "moderator", "support",
	"agent", "noreply", "no_reply", "mailer", "postmaster", "abuse",
	"webmaster", "hostmaster", "security", "info", "help", "contact",
	"www", "api", "mail", "mu",
}

// leet maps the digits people substitute for letters, so a slur spelled with
// them is still the slur. Applied before matching, never to the stored name.
var leet = strings.NewReplacer(
	"0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t",
)

// ValidateUsername returns why a username is not allowed, or "" if it is fine.
//
// Called from Create and Claim, which are the only two functions that put an
// id into the accounts map. It used to be called from the signup handlers
// instead, which meant every other door — claiming an account, the operator
// bootstrap in internal/setup — wrote whatever it was given. That is how
// micro.mu came to have an account called 3834, which this function has always
// rejected and nothing ever asked it about.
func ValidateUsername(username string) string {
	if !validUsernameFormat(username) {
		return "Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores"
	}

	lower := strings.ToLower(username)
	for _, r := range reserved {
		if lower == r {
			return "That username is reserved."
		}
	}
	if !allowedWords(lower) {
		return "That username is not allowed."
	}
	return ""
}

// allowedWords is the blocklist, run over the spellings that mean the same
// thing as what was typed.
//
// A digit in a username is ambiguous: in d1ck it stands in for a letter, in
// fuck1234 it is padding, and no single reading catches both — substituting
// turns fuck1234 into fuckie2a, stripping turns d1ck into dck. So both
// readings are tried, and their combination, which is what catches d1ck123.
//
// Padding is trimmed from the edges of each word rather than removed
// throughout, because that is where padding goes; a digit between two letters
// is the substitution case. Underscores are word boundaries in all of it, so
// n_i_g_g_e_r is the word it is trying not to be.
func allowedWords(lower string) bool {
	trimmed := trimPadding(lower)
	forms := []string{
		lower,
		leet.Replace(lower),
		trimmed,
		leet.Replace(trimmed),
	}

	for _, form := range forms {
		// Anywhere, including across the underscores somebody put in to break
		// the word up.
		joined := strings.ReplaceAll(form, "_", "")
		for _, w := range bannedAnywhere {
			if strings.Contains(joined, w) {
				return false
			}
		}
		// As a whole word only. See the comment on bannedAsWord for why this is
		// deliberately weaker.
		for _, part := range strings.Split(form, "_") {
			for _, w := range bannedAsWord {
				if part == w {
					return false
				}
			}
		}
	}
	return true
}

// trimPadding drops digits from the edges of each underscore-separated word,
// so fuck1234 is still the word and dickson is still a surname.
func trimPadding(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = strings.Trim(p, "0123456789")
	}
	return strings.Join(parts, "_")
}

func validUsernameFormat(username string) bool {
	if len(username) < MinUsername || len(username) > MaxUsername {
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
