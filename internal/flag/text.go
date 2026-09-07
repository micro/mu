package flag

import "regexp"

// Explicit profanity is rejected consistently without depending on a model.
// Word boundaries avoid rejecting names and ordinary words containing a fragment.
var profanity = regexp.MustCompile(`(?i)\b(?:fuck(?:s|er|ers|ing|ed|head|heads|wit|wits)?|motherfuck(?:er|ers|ing)?|shit(?:s|ty|ting|ted|head|heads)?|bullshit|cunt(?:s)?|asshole(?:s)?)\b`)

func Profane(text string) bool { return profanity.MatchString(text) }
