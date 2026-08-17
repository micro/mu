// Package safety is what this instance will not generate, whoever asks.
//
// Generation is the one thing here that produces something new rather than
// reporting something that already exists, and an instance that will make
// anything is an instance that will be used to make anything. The models are
// somebody else's and so is their acceptable-use policy; the account paying is
// this instance's, and so is the domain the result is served from.
//
// # Not a service
//
// Nobody chooses to be checked. This sits with the others that are not a
// choice — internal/quota for what things cost, internal/auth for who you are —
// because a rule a caller can decline is not a rule. It is called by the thing
// doing the generating, before the model is asked and before the credit is
// spent.
//
// # Two categories, and only one of them is policy
//
// Sexual content involving children is refused always and is not configurable.
// There is no instance, no operator setting and no self-hosted deployment where
// that is somebody's own business, and a switch implies otherwise.
//
// Explicit sexual content between adults is refused by default and is an
// operator's decision, because a self-hosted instance answering to nobody but
// its owner is a different situation from this one. GENERATE_ADULT set true
// allows it. The default is no.
//
// # What this is and is not
//
// It is a door, not a classifier. Matching words catches what is written
// plainly, which is most of what arrives, and it will not catch what is written
// carefully. The honest description is that it stops casual misuse and raises
// the cost of the rest; a model-based check belongs behind Classify below, and
// is where the ambiguous middle should go when there is one to run.
//
// It errs towards refusing on the first category and towards allowing on the
// second, because the costs are not symmetric: wrongly refusing an adult a
// picture is an annoyance, and the other way round is not.
package safety

import (
	"strings"

	"mu/internal/settings"
)

// Category is why something was refused.
type Category string

const (
	// Minors is sexual content involving children. Never allowed.
	Minors Category = "minors"
	// Adult is explicit sexual content. Refused unless the operator allows it.
	Adult Category = "adult"
)

// Classify, when set, is a second opinion from a model for the cases words
// cannot settle. Nil means the word check is the whole check.
//
// A variable rather than an import because internal/ai is a heavier dependency
// than this needs, and because an instance with no model configured still has
// to refuse the plain cases.
var Classify func(prompt string) (Category, bool)

// NeverAllowed is the first category alone, for the places where the whole
// policy would be wrong.
//
// The distinction is between refusing to *make* something and refusing to
// *read* something. An agent is handed text it did not choose — an arriving
// email, a page it fetched, a message somebody else sent — and refusing to
// answer because that text mentions something is how an inbox stops working.
// So the full policy belongs where a caller asks for something to be created,
// and this narrower one belongs everywhere else: sexual content involving
// children is worth refusing even at the cost of a false positive, and adult
// content is not.
func NeverAllowed(prompt string) (reason string, refused bool) {
	if involvesMinors(normalise(prompt)) {
		return refusalMinors, true
	}
	if Classify != nil {
		if cat, yes := Classify(prompt); yes && cat == Minors {
			return refusalMinors, true
		}
	}
	return "", false
}

const (
	refusalMinors = "This instance does not generate sexual content involving children, " +
		"and that is not a setting."
	refusalAdult = "This instance does not generate explicit sexual content."
)

// Refused reports whether this instance will generate from a prompt, and says
// why not in words the person asking can act on.
//
// The reason is deliberately plain and does not quote the prompt back: an
// error that repeats what was asked for ends up in a log, a support mail and
// an admin page.
func Refused(prompt string) (reason string, refused bool) {
	p := normalise(prompt)

	if involvesMinors(p) {
		return refusalMinors, true
	}
	if explicit(p) && !adultAllowed() {
		return refusalAdult, true
	}
	if Classify != nil {
		if cat, yes := Classify(prompt); yes {
			switch cat {
			case Minors:
				return refusalMinors, true
			case Adult:
				if !adultAllowed() {
					return refusalAdult, true
				}
			}
		}
	}
	return "", false
}

// adultAllowed is the operator's decision, and only for the second category.
func adultAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("GENERATE_ADULT"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// normalise flattens the cheap evasions — spacing, punctuation between letters,
// and the usual digit substitutions — so a word check is not defeated by a
// keyboard. It is not a defence against anyone trying properly.
func normalise(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer(
		"0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t", "@", "a", "$", "s",
	)
	s = r.Replace(s)
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		default:
			b.WriteRune(' ')
		}
	}
	// Collapse runs of spaces so "l o l i" reads as one token too.
	out := b.String()
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return " " + out + " " + strings.ReplaceAll(out, " ", "") + " "
}

// young are the words that make a subject a child. Alone they are innocent —
// most of these appear in perfectly ordinary requests — so they only refuse in
// combination with something sexual.
var young = []string{
	"child", "children", "kid", "kids", "toddler", "infant", "baby", "babies",
	"minor", "minors", "underage", "preteen", "prepubescent", "schoolgirl",
	"schoolboy", "teen", "teens", "teenage", "teenager", "adolescent",
	"boy", "girl", "yearold", "yrold",
}

// sexual are the words that make a request sexual.
var sexual = []string{
	"nude", "nudes", "naked", "nsfw", "porn", "porno", "pornographic", "explicit",
	"erotic", "erotica", "sex", "sexual", "sexy", "topless", "undressed",
	"lingerie", "fetish", "hentai", "lewd", "genitals", "breasts", "orgasm",
	"masturbat", "intercourse",
}

// never are terms whose only use is the thing that is never allowed.
var never = []string{"cp", "childporn", "childp0rn", "loli", "lolicon", "shota", "jailbait"}

func involvesMinors(p string) bool {
	for _, w := range never {
		if strings.Contains(p, " "+w+" ") {
			return true
		}
	}
	return anyOf(p, young) && anyOf(p, sexual)
}

func explicit(p string) bool {
	// A narrower list than `sexual`: "sexy" or "lingerie" is not what this
	// refuses, and refusing it would make the instance useless for ordinary
	// requests while stopping nothing.
	for _, w := range []string{
		"nude", "nudes", "naked", "nsfw", "porn", "porno", "pornographic",
		"hentai", "topless", "genitals", "masturbat", "explicit sexual",
	} {
		if strings.Contains(p, " "+w+" ") || strings.Contains(p, w) && len(w) > 5 {
			return true
		}
	}
	return false
}

func anyOf(p string, words []string) bool {
	for _, w := range words {
		if strings.Contains(p, " "+w+" ") || (len(w) > 5 && strings.Contains(p, w)) {
			return true
		}
	}
	return false
}
